package evm

import (
	"context"
	"errors"

	relayllo "github.com/smartcontractkit/chainlink-relay/pkg/reportingplugins/llo"
	"github.com/smartcontractkit/chainlink-relay/pkg/services"
	relaytypes "github.com/smartcontractkit/chainlink-relay/pkg/types"
	ocrtypes "github.com/smartcontractkit/libocr/offchainreporting2plus/types"

	"github.com/smartcontractkit/chainlink/v2/core/logger"
	"github.com/smartcontractkit/chainlink/v2/core/services/llo"
)

var _ relaytypes.LLOProvider = (*lloProvider)(nil)

type lloProvider struct {
	configWatcher          *configWatcher
	transmitter            llo.Transmitter
	logger                 logger.Logger
	channelDefinitionCache llo.ChannelDefinitionCache

	ms services.MultiStart
}

func NewLLOProvider(
	configWatcher *configWatcher,
	transmitter llo.Transmitter,
	lggr logger.Logger,
	channelDefinitionCache llo.ChannelDefinitionCache,
) relaytypes.LLOProvider {
	return &lloProvider{
		configWatcher,
		transmitter,
		lggr,
		channelDefinitionCache,
		services.MultiStart{},
	}
}

func (p *lloProvider) Start(ctx context.Context) error {
	return p.ms.Start(ctx, p.configWatcher, p.transmitter, p.channelDefinitionCache)
}

func (p *lloProvider) Close() error {
	return p.ms.Close()
}

func (p *lloProvider) Ready() error {
	return errors.Join(p.configWatcher.Ready(), p.transmitter.Ready(), p.channelDefinitionCache.Ready())
}

func (p *lloProvider) Name() string {
	return p.logger.Name()
}

func (p *lloProvider) HealthReport() map[string]error {
	report := map[string]error{}
	services.CopyHealth(report, p.configWatcher.HealthReport())
	services.CopyHealth(report, p.transmitter.HealthReport())
	services.CopyHealth(report, p.channelDefinitionCache.HealthReport())
	return report
}

func (p *lloProvider) ContractConfigTracker() ocrtypes.ContractConfigTracker {
	return p.configWatcher.ContractConfigTracker()
}

func (p *lloProvider) OffchainConfigDigester() ocrtypes.OffchainConfigDigester {
	return p.configWatcher.OffchainConfigDigester()
}

func (p *lloProvider) OnchainConfigCodec() relayllo.OnchainConfigCodec {
	return &relayllo.JSONOnchainConfigCodec{}
}

func (p *lloProvider) ContractTransmitter() ocrtypes.ContractTransmitter {
	return p.transmitter
}

func (p *lloProvider) ChannelDefinitionCache() relayllo.ChannelDefinitionCache {
	return p.channelDefinitionCache
}
