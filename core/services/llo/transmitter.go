package llo

// TODO: llo transmitter

import (
	"context"
	"crypto/ed25519"
	"fmt"

	relayllo "github.com/smartcontractkit/chainlink-relay/pkg/reportingplugins/llo"
	"github.com/smartcontractkit/chainlink-relay/pkg/services"
	ocrtypes "github.com/smartcontractkit/libocr/offchainreporting2plus/types"

	"github.com/smartcontractkit/chainlink/v2/core/logger"
	"github.com/smartcontractkit/chainlink/v2/core/services/relay/evm/mercury/wsrpc"
)

type Transmitter interface {
	relayllo.Transmitter
	services.Service
}

type transmitter struct {
	services.StateMachine
	lggr        logger.Logger
	rpcClient   wsrpc.Client
	fromAccount string
}

func NewTransmitter(lggr logger.Logger, rpcClient wsrpc.Client, fromAccount ed25519.PublicKey) Transmitter {
	return &transmitter{
		services.StateMachine{},
		lggr,
		rpcClient,
		fmt.Sprintf("%x", fromAccount),
	}
}

func (t *transmitter) Start(ctx context.Context) error {
	// TODO
	return nil
}

func (t *transmitter) Close() error {
	// TODO
	return nil
}

func (t *transmitter) HealthReport() map[string]error {
	report := map[string]error{t.Name(): t.Healthy()}
	services.CopyHealth(report, t.rpcClient.HealthReport())
	// FIXME
	// services.CopyHealth(report, t.queue.HealthReport())
	return report
}

func (t *transmitter) Name() string { return t.lggr.Name() }

func (t *transmitter) Transmit(ctx context.Context, reportCtx ocrtypes.ReportContext, report ocrtypes.Report, signatures []ocrtypes.AttributedOnchainSignature) error {
	// TODO
	return nil
}

// FromAccount returns the stringified (hex) CSA public key
func (t *transmitter) FromAccount() (ocrtypes.Account, error) {
	return ocrtypes.Account(t.fromAccount), nil
}

// LatestConfigDigestAndEpoch retrieves the latest config digest and epoch from the OCR2 contract.
func (t *transmitter) LatestConfigDigestAndEpoch(ctx context.Context) (cd ocrtypes.ConfigDigest, epoch uint32, err error) {
	panic("not needed for OCR3")
}
