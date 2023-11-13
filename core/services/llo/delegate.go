package llo

import (
	"context"
	"fmt"
	"log"

	relayllo "github.com/smartcontractkit/chainlink-relay/pkg/reportingplugins/llo"
	"github.com/smartcontractkit/chainlink/v2/core/services/job"
	"github.com/smartcontractkit/chainlink/v2/core/services/pg"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
	ocr2plus "github.com/smartcontractkit/libocr/offchainreporting2plus"
	"github.com/smartcontractkit/libocr/offchainreporting2plus/ocr3types"

	"github.com/smartcontractkit/libocr/commontypes"
)

var _ job.Delegate = &Delegate{}

// Delegate is a container struct for an Oracle plugin. This struct provides
// the ability to start and stop underlying services associated with the
// plugin instance.
type Delegate struct {
	llo    ocr2plus.Oracle
	logger *log.Logger
}

type DelegateConfig struct {
	BinaryNetworkEndpointFactory types.BinaryNetworkEndpointFactory
	V2Bootstrappers              []commontypes.BootstrapperLocator
	ContractConfigTracker        types.ContractConfigTracker
	ContractTransmitter          ocr3types.ContractTransmitter[relayllo.ReportInfo]
	KeepersDatabase              ocr3types.Database
	Logger                       commontypes.Logger
	MonitoringEndpoint           commontypes.MonitoringEndpoint
	OffchainConfigDigester       types.OffchainConfigDigester
	OffchainKeyring              types.OffchainKeyring
	OnchainKeyring               ocr3types.OnchainKeyring[relayllo.ReportInfo]
	LocalConfig                  types.LocalConfig
}

func NewDelegate(c DelegateConfig) (*Delegate, error) {
	return &Delegate{}
}

func (d *Delegate) Start(_ context.Context) error {
	return d.llo.Start()
}

func (d *Delegate) JobType() job.Type {
	// FIXME: Is this correct?
	return job.OffchainReporting2
}

// BeforeJobCreated is only called once on first time job create.
func (d *Delegate) BeforeJobCreated(jb job.Job) {}

// ServicesForSpec returns services to be started and stopped for this
// job. In case a given job type relies upon well-defined startup/shutdown
// ordering for services, they are started in the order they are given
// and stopped in reverse order.
func (d *Delegate) ServicesForSpec(jb job.Job) ([]job.ServiceCtx, error) {
	// create the oracle from config values
	llo, err := ocr2plus.NewOracle(ocr2plus.OCR3OracleArgs[relayllo.ReportInfo]{
		BinaryNetworkEndpointFactory: c.BinaryNetworkEndpointFactory,
		V2Bootstrappers:              c.V2Bootstrappers,
		ContractConfigTracker:        c.ContractConfigTracker,
		ContractTransmitter:          c.ContractTransmitter,
		Database:                     c.KeepersDatabase,
		LocalConfig:                  c.LocalConfig,
		Logger:                       c.Logger,
		MonitoringEndpoint:           c.MonitoringEndpoint,
		OffchainConfigDigester:       c.OffchainConfigDigester,
		OffchainKeyring:              c.OffchainKeyring,
		OnchainKeyring:               c.OnchainKeyring,
		ReportingPluginFactory:       relayllo.NewLLOPluginFactory(),
	})

	if err != nil {
		return nil, fmt.Errorf("%w: failed to create new OCR oracle", err)
	}

	return []job.ServiceCtx{llo}
}
func (d *Delegate) AfterJobCreated(jb job.Job)  {}
func (d *Delegate) BeforeJobDeleted(jb job.Job) {}

// OnDeleteJob will be called from within DELETE db transaction.  Any db
// commands issued within OnDeleteJob() should be performed first, before any
// non-db side effects.  This is required in order to guarantee mutual atomicity between
// all tasks intended to happen during job deletion.  For the same reason, the job will
// not show up in the db within OnDeleteJob(), even though it is still actively running.
func (d *Delegate) OnDeleteJob(jb job.Job, q pg.Queryer) error {
	return nil
}
