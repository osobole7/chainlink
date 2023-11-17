package evm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/smartcontractkit/libocr/gethwrappers2/ocr2aggregator"
	"github.com/smartcontractkit/libocr/offchainreporting2plus/chains/evmutil"
	"github.com/smartcontractkit/libocr/offchainreporting2plus/ocr3types"
	"github.com/smartcontractkit/ocr2keepers/pkg/v3/plugin"

	relaytypes "github.com/smartcontractkit/chainlink-relay/pkg/types"

	ocrtypes "github.com/smartcontractkit/libocr/offchainreporting2plus/types"

	txmgrcommon "github.com/smartcontractkit/chainlink/v2/common/txmgr"
	"github.com/smartcontractkit/chainlink/v2/core/chains/evm"
	txm "github.com/smartcontractkit/chainlink/v2/core/chains/evm/txmgr"
	"github.com/smartcontractkit/chainlink/v2/core/logger"
	"github.com/smartcontractkit/chainlink/v2/core/services/job"
	"github.com/smartcontractkit/chainlink/v2/core/services/keystore"
	"github.com/smartcontractkit/chainlink/v2/core/services/ocrcommon"
	"github.com/smartcontractkit/chainlink/v2/core/services/pipeline"
	functionsRelay "github.com/smartcontractkit/chainlink/v2/core/services/relay/evm/functions"
	"github.com/smartcontractkit/chainlink/v2/core/services/relay/evm/types"
)

var (
	_ OCR2KeeperRelayer  = (*ocr2keeperRelayer)(nil)
	_ OCR2KeeperProvider = (*ocr2keeperProvider)(nil)
)

// OCR2KeeperProviderOpts is the custom options to create a keeper provider
type OCR2KeeperProviderOpts struct {
	RArgs      relaytypes.RelayArgs
	PArgs      relaytypes.PluginArgs
	InstanceID int
}

// OCR2KeeperProvider provides all components needed for a OCR2Keeper plugin.
type OCR2KeeperProvider interface {
	relaytypes.Plugin
}

// OCR2KeeperRelayer contains the relayer and instantiating functions for OCR2Keeper providers.
type OCR2KeeperRelayer interface {
	NewOCR2KeeperProvider(rargs relaytypes.RelayArgs, pargs relaytypes.PluginArgs, eth keystore.Eth) (OCR2KeeperProvider, error)
}

// ocr2keeperRelayer is the relayer with added DKG and OCR2Keeper provider functions.
type ocr2keeperRelayer struct {
	db    *sqlx.DB
	chain evm.Chain
	pr    pipeline.Runner
	spec  job.Job
	lggr  logger.Logger
}

// NewOCR2KeeperRelayer is the constructor of ocr2keeperRelayer
func NewOCR2KeeperRelayer(db *sqlx.DB, chain evm.Chain, pr pipeline.Runner, spec job.Job, lggr logger.Logger) OCR2KeeperRelayer {
	return &ocr2keeperRelayer{
		db:    db,
		chain: chain,
		pr:    pr,
		spec:  spec,
		lggr:  lggr,
	}
}

func (r *ocr2keeperRelayer) NewOCR2KeeperProvider(rargs relaytypes.RelayArgs, pargs relaytypes.PluginArgs, ethKeystore keystore.Eth) (OCR2KeeperProvider, error) {
	configWatcher, err := newOCR2KeeperConfigProvider(r.lggr, r.chain, rargs)
	if err != nil {
		return nil, err
	}

	//gasLimit := configWatcher.chain.Config().EVM().OCR2().Automation().GasLimit()
	//contractTransmitter, err := newPipelineContractTransmitter(r.lggr, rargs, pargs.TransmitterID, &gasLimit, cfgWatcher, r.spec, r.pr)
	//if err != nil {
	//	return nil, err
	//}

	//var pluginConfig ocr2keeper.PluginConfig
	//if err2 := json.Unmarshal(pargs.PluginConfig, &pluginConfig); err2 != nil {
	//	return nil, err2
	//}

	contractTransmitter, err := newKeepersContractTransmitter(1, rargs, pargs.TransmitterID, configWatcher, ethKeystore, r.lggr)
	if err != nil {
		return nil, err
	}

	return &ocr2keeperProvider{
		configWatcher:       configWatcher,
		contractTransmitter: contractTransmitter,
	}, nil
}

type ocr3keeperProviderContractTransmitter struct {
	contractTransmitter ocrtypes.ContractTransmitter
}

var _ ocr3types.ContractTransmitter[plugin.AutomationReportInfo] = &ocr3keeperProviderContractTransmitter{}

func NewKeepersOCR3ContractTransmitter(ocr2ContractTransmitter ocrtypes.ContractTransmitter) *ocr3keeperProviderContractTransmitter {
	return &ocr3keeperProviderContractTransmitter{ocr2ContractTransmitter}
}

func (t *ocr3keeperProviderContractTransmitter) Transmit(
	ctx context.Context,
	digest ocrtypes.ConfigDigest,
	seqNr uint64,
	reportWithInfo ocr3types.ReportWithInfo[plugin.AutomationReportInfo],
	aoss []ocrtypes.AttributedOnchainSignature,
) error {
	return t.contractTransmitter.Transmit(
		ctx,
		ocrtypes.ReportContext{
			ReportTimestamp: ocrtypes.ReportTimestamp{
				ConfigDigest: digest,
				Epoch:        uint32(seqNr),
			},
		},
		reportWithInfo.Report,
		aoss,
	)
}

func (t *ocr3keeperProviderContractTransmitter) FromAccount() (ocrtypes.Account, error) {
	return t.contractTransmitter.FromAccount()
}

type ocr2keeperProvider struct {
	*configWatcher
	contractTransmitter ContractTransmitter
}

func (c *ocr2keeperProvider) ContractTransmitter() ocrtypes.ContractTransmitter {
	return c.contractTransmitter
}

func newOCR2KeeperConfigProvider(lggr logger.Logger, chain evm.Chain, rargs relaytypes.RelayArgs) (*configWatcher, error) {
	var relayConfig types.RelayConfig
	err := json.Unmarshal(rargs.RelayConfig, &relayConfig)
	if err != nil {
		return nil, err
	}
	if !common.IsHexAddress(rargs.ContractID) {
		return nil, fmt.Errorf("invalid contract address '%s'", rargs.ContractID)
	}

	contractAddress := common.HexToAddress(rargs.ContractID)
	contractABI, err := abi.JSON(strings.NewReader(ocr2aggregator.OCR2AggregatorMetaData.ABI))
	if err != nil {
		return nil, errors.Wrap(err, "could not get OCR2Aggregator ABI JSON")
	}

	configPoller, err := NewConfigPoller(
		lggr.With("contractID", rargs.ContractID),
		chain.Client(),
		chain.LogPoller(),
		contractAddress,
		// TODO: Does ocr2keeper need to support config contract? DF-19182
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create config poller")
	}

	offchainConfigDigester := evmutil.EVMOffchainConfigDigester{
		ChainID:         chain.Config().EVM().ChainID().Uint64(),
		ContractAddress: contractAddress,
	}

	return newConfigWatcher(
		lggr,
		contractAddress,
		contractABI,
		offchainConfigDigester,
		configPoller,
		chain,
		relayConfig.FromBlock,
		rargs.New,
	), nil
}

func newKeepersContractTransmitter(contractVersion uint32, rargs relaytypes.RelayArgs, transmitterID string, configWatcher *configWatcher, ethKeystore keystore.Eth, lggr logger.Logger) (ContractTransmitter, error) {
	var relayConfig types.RelayConfig
	if err := json.Unmarshal(rargs.RelayConfig, &relayConfig); err != nil {
		return nil, err
	}
	var fromAddresses []common.Address
	sendingKeys := relayConfig.SendingKeys
	if !relayConfig.EffectiveTransmitterID.Valid {
		return nil, errors.New("EffectiveTransmitterID must be specified")
	}
	effectiveTransmitterAddress := common.HexToAddress(relayConfig.EffectiveTransmitterID.String)

	sendingKeysLength := len(sendingKeys)
	if sendingKeysLength == 0 {
		return nil, errors.New("no sending keys provided")
	}

	// If we are using multiple sending keys, then a forwarder is needed to rotate transmissions.
	// Ensure that this forwarder is not set to a local sending key, and ensure our sending keys are enabled.
	for _, s := range sendingKeys {
		if sendingKeysLength > 1 && s == effectiveTransmitterAddress.String() {
			return nil, errors.New("the transmitter is a local sending key with transaction forwarding enabled")
		}
		if err := ethKeystore.CheckEnabled(common.HexToAddress(s), configWatcher.chain.Config().EVM().ChainID()); err != nil {
			return nil, errors.Wrap(err, "one of the sending keys given is not enabled")
		}
		fromAddresses = append(fromAddresses, common.HexToAddress(s))
	}

	scoped := configWatcher.chain.Config()
	strategy := txmgrcommon.NewQueueingTxStrategy(rargs.ExternalJobID, scoped.OCR2().DefaultTransactionQueueDepth(), scoped.Database().DefaultQueryTimeout())

	var checker txm.TransmitCheckerSpec
	if configWatcher.chain.Config().OCR2().SimulateTransactions() {
		checker.CheckerType = txm.TransmitCheckerTypeSimulate
	}

	gasLimit := configWatcher.chain.Config().EVM().GasEstimator().LimitDefault()
	ocr2Limit := configWatcher.chain.Config().EVM().GasEstimator().LimitJobType().OCR2()
	if ocr2Limit != nil {
		gasLimit = *ocr2Limit
	}

	transmitter, err := ocrcommon.NewTransmitter(
		configWatcher.chain.TxManager(),
		fromAddresses,
		gasLimit,
		effectiveTransmitterAddress,
		strategy,
		checker,
		configWatcher.chain.ID(),
		ethKeystore,
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to create transmitter")
	}

	keepersTransmitter, err := functionsRelay.NewFunctionsContractTransmitter(
		configWatcher.chain.Client(),
		configWatcher.contractABI,
		transmitter,
		configWatcher.chain.LogPoller(),
		lggr,
		nil,
		contractVersion,
	)

	return keepersTransmitter, err
}
