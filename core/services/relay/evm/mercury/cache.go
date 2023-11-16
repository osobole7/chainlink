package mercury

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/jpillora/backoff"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/smartcontractkit/chainlink-relay/pkg/services"
	"github.com/smartcontractkit/chainlink/v2/core/logger"
	mercuryutils "github.com/smartcontractkit/chainlink/v2/core/services/relay/evm/mercury/utils"
	"github.com/smartcontractkit/chainlink/v2/core/utils"
)

var (
	promFetchFailedCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mercury_cache_fetch_failure_count",
		Help: "Number of times we tried to call FetchLatestPrice from the mercury server, but some kind of error occurred",
	},
		[]string{"feedID"},
	)
)

// fetchDeadline controls how long to wait for a response before giving up on
// fetching LatestPrice
// TODO: Move this to the config?
const fetchDeadline = 5 * time.Second

type Cache interface {
	services.Service
	GetLatestPrice(ctx context.Context, feedID mercuryutils.FeedID) (*big.Int, error)
}

type FetchFunc func(ctx context.Context, feedID mercuryutils.FeedID) (*big.Int, error)

type CacheConfig struct {
	Logger           logger.Logger
	FetchLatestPrice FetchFunc
	// LatestPriceTTL controls how "stale" we will allow a price to be e.g. if
	// set to 1s, a new price will always be fetched if the last result was
	// from more than 1 second ago.
	//
	// Another way of looking at it is such: the cache will _never_ return a
	// price that was queried from before now-LatestPriceTTL.
	//
	// Setting to zero disables caching entirely.
	LatestPriceTTL time.Duration
	// MaxStaleAge is that maximum amount of time that a value can be stale
	// before it is deleted from the cache (a form of garbage collection).
	//
	// This should generally be set to something much larger than
	// LatestPriceTTL. Setting to zero disables garbage collection.
	MaxStaleAge time.Duration
}

func NewCache(c CacheConfig) Cache {
	if c.LatestPriceTTL == 0 {
		return &NullCache{}
	}
	return newMemCache(c)
}

type cacheVal struct {
	sync.RWMutex

	fetching bool
	fetchCh  chan (struct{})

	val *big.Int

	expiresAt time.Time
}

func (v *cacheVal) read() *big.Int {
	v.RLock()
	defer v.RUnlock()
	return v.val
}

// memCache stores values in memory
// it will never return a stale value older than latestPriceTTL, instead
// waiting for a successful fetch or caller context cancels, whichever comes
// first
type memCache struct {
	services.StateMachine
	lggr logger.Logger

	latestPriceTTL time.Duration
	maxStaleAge    time.Duration

	fetchLatestPrice FetchFunc

	cache sync.Map

	wg     sync.WaitGroup
	chStop utils.StopChan
}

func newMemCache(c CacheConfig) *memCache {
	return &memCache{
		services.StateMachine{},
		c.Logger.Named("MercuryMemCache"),
		c.LatestPriceTTL,
		c.MaxStaleAge,
		c.FetchLatestPrice,
		sync.Map{},
		sync.WaitGroup{},
		make(chan (struct{})),
	}
}

type ErrStopped struct{}

func (e ErrStopped) Error() string {
	return "memCache was stopped"
}

// GetLatestPrice
// FIXME: This will actually block on all types of errors, even non timeouts. Context should be set carefully
func (m *memCache) GetLatestPrice(ctx context.Context, feedID mercuryutils.FeedID) (*big.Int, error) {
	vi, _ := m.cache.LoadOrStore(feedID, &cacheVal{
		sync.RWMutex{},
		false,
		nil,
		nil,
		time.Now().Add(m.latestPriceTTL),
	})
	v := vi.(*cacheVal)

	fWaitForResult := func(ch <-chan struct{}) (*big.Int, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-m.chStop:
			return nil, ErrStopped{}
		case <-ch:
			return v.read(), nil
		}
	}

	// HOT PATH
	v.RLock()
	if time.Now().After(v.expiresAt) {
		// if someone else is fetching then wait for the fetch to complete
		if v.fetching {
			ch := v.fetchCh
			v.RUnlock()
			return fWaitForResult(ch)
		} else {
			v.RUnlock()
			// EXIT HOT PATH
		}
	} else {
		defer v.RUnlock()
		return v.val, nil
	}

	// COLD PATH
	v.Lock()
	// if someone else is fetching then wait for the fetch to complete
	if v.fetching {
		ch := v.fetchCh
		v.Unlock()
		return fWaitForResult(ch)
	} else {
		// initiate the fetch
		v.fetching = true
		ch := make(chan struct{})
		v.fetchCh = ch
		v.Unlock()

		ok := m.IfStarted(func() {
			m.wg.Add(1)
			go m.fetch(feedID, v)
		})
		if !ok {
			return nil, fmt.Errorf("memCache must be started, but is: %v", m.State())
		}
		return fWaitForResult(ch)
	}
}

// fetch continually tries to call FetchLatestPrice and write the result to v
func (m *memCache) fetch(feedID mercuryutils.FeedID, v *cacheVal) {
	defer m.wg.Done()
	t := time.Now()
	b := backoff.Backoff{
		Min:    50 * time.Millisecond, // TODO: or m.latestPriceTTL/2 if less
		Max:    m.latestPriceTTL / 2,
		Factor: 2,
		Jitter: true,
	}
	memcacheCtx, cancel := m.chStop.NewCtx()
	defer cancel()

	for {
		ctx, cancel := context.WithTimeoutCause(memcacheCtx, fetchDeadline, errors.New("fetch deadline exceeded"))
		val, err := m.fetchLatestPrice(ctx, feedID)
		cancel()
		if memcacheCtx.Err() != nil {
			// stopped
			return
		} else if err != nil {
			m.lggr.Warnw("FetchLatestPrice failed", "err", err)
			promFetchFailedCount.WithLabelValues(feedID.String()).Inc()
			select {
			case <-m.chStop:
				return
			case <-time.After(b.Duration()):
				continue
			}
		}
		v.Lock()
		v.val = val
		v.expiresAt = t.Add(m.latestPriceTTL)
		close(v.fetchCh)
		v.fetchCh = nil
		v.fetching = false
		v.Unlock()
		return
	}
}

func (m *memCache) Start(context.Context) error {
	return m.StartOnce(m.Name(), func() error {
		m.wg.Add(1)
		go m.runloop()
		return nil
	})
}

func (m *memCache) runloop() {
	defer m.wg.Done()

	if m.maxStaleAge == 0 {
		return
	}
	t := time.NewTicker(utils.WithJitter(m.maxStaleAge))

	for {
		select {
		case <-t.C:
			m.cleanup()
			t.Reset(utils.WithJitter(m.maxStaleAge))
		case <-m.chStop:
			return
		}
	}
}

// remove anything that has been stale for longer than maxStaleAge so that cache doesn't grow forever and cause memory leaks
func (m *memCache) cleanup() {
	m.cache.Range(func(k, vi any) bool {
		v := vi.(*cacheVal)
		if time.Now().After(v.expiresAt.Add(m.maxStaleAge)) {
			// garbage collection
			// FIXME: What if its fetching or locked, is this a problem?
			m.cache.Delete(k)
		}
		return true
	})
}

func (m *memCache) Close() error {
	return m.StopOnce(m.Name(), func() error {
		close(m.chStop)
		m.wg.Wait()
		return nil
	})
}
func (m *memCache) HealthReport() map[string]error {
	return map[string]error{
		m.Name(): m.Ready(),
	}
}
func (m *memCache) Name() string { return m.lggr.Name() }
func (m *memCache) Ready() error { return nil }

// NullCache will always be a cache miss
type NullCache struct{}

func (n *NullCache) GetLatestPrice(ctx context.Context, feedID mercuryutils.FeedID) (*big.Int, error) {
	return nil, nil
}
func (n *NullCache) Start(context.Context) error { return nil }
func (n *NullCache) Close() error                { return nil }
func (n *NullCache) HealthReport() map[string]error {
	return map[string]error{
		n.Name(): n.Ready(),
	}
}
func (n *NullCache) Name() string { return "NullCache" }
func (n *NullCache) Ready() error { return nil }
