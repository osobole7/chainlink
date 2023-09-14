package llo

// TODO: llo datasource
import (
	"context"
	"fmt"
	"math/big"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	relayllo "github.com/smartcontractkit/chainlink-relay/pkg/reportingplugins/llo"
	"github.com/smartcontractkit/chainlink/v2/core/logger"
)

var (
	promMissingStreamCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "llo_stream_missing_count",
		Help: "Number of times we tried to observe a stream, but it was missing",
	},
		[]string{"streamID"},
	)
	promObservationErrorCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "llo_stream_observation_error_count",
		Help: "Number of times we tried to observe a stream, but it failed with an error",
	},
		[]string{"streamID"},
	)
)

type ErrMissingStream struct {
	id string
}

func (e ErrMissingStream) Error() string {
	return fmt.Sprintf("missing stream definition for: %q", e.id)
}

var _ relayllo.DataSource = &dataSource{}

type dataSource struct {
	lggr        logger.Logger
	streamCache StreamCache
}

func NewDataSource(lggr logger.Logger, streamCache StreamCache) relayllo.DataSource {
	// TODO: lggr should include job ID
	return &dataSource{lggr, streamCache}
}

func (d *dataSource) Observe(ctx context.Context, streamIDs map[relayllo.StreamID]struct{}) (relayllo.StreamValues, error) {
	// There is no "observationSource" (AKA pipeline)
	// Need a concept of "streams"
	// Streams are referenced by ID from the on-chain config
	// Each stream contains its own pipeline
	// See: https://docs.google.com/document/d/1l1IiDOL1QSteLTnhmiGnJAi6QpcSpyOe0nkqS7D3SvU/edit for stream ID naming

	var wg sync.WaitGroup
	wg.Add(len(streamIDs))
	sv := make(relayllo.StreamValues)
	var mu sync.Mutex

	for streamID := range streamIDs {
		go func() {
			defer wg.Done()

			var res relayllo.ObsResult[*big.Int]

			stream, exists := d.streamCache.Get(streamID)
			if exists {
				res.Val, res.Err = stream.Observe(ctx)
				if res.Err != nil {
					d.lggr.Debugw("Observation failed for stream", "err", res.Err, "streamID", streamID)
					promObservationErrorCount.WithLabelValues(streamID.String()).Inc()
				}
			} else {
				d.lggr.Errorw(fmt.Sprintf("Missing stream: %q"), "streamID", streamID)
				promMissingStreamCount.WithLabelValues(streamID.String()).Inc()
				res.Err = ErrMissingStream{streamID.String()}
			}

			mu.Lock()
			defer mu.Unlock()
			sv[streamID] = res
		}()
	}

	wg.Wait()

	return sv, nil
}
