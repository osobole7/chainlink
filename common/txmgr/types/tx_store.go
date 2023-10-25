package types

import (
	"context"
	"math/big"
	"time"

	"github.com/google/uuid"

	feetypes "github.com/smartcontractkit/chainlink/v2/common/fee/types"
	"github.com/smartcontractkit/chainlink/v2/common/types"
	"github.com/smartcontractkit/chainlink/v2/core/services/pg"
)

// TxStore is a superset of all the needed persistence layer methods
//
//go:generate mockery --quiet --name TxStore --output ./mocks/ --case=underscore
type TxStore[
	// Represents an account address, in native chain format.
	ADDR types.Hashable,
	// Represents a chain id to be used for the chain.
	CHAIN_ID types.ID,
	// Represents a unique Tx Hash for a chain
	TX_HASH types.Hashable,
	// Represents a unique Block Hash for a chain
	BLOCK_HASH types.Hashable,
	// Represents a onchain receipt object that a chain's RPC returns
	R ChainReceipt[TX_HASH, BLOCK_HASH],
	// Represents the sequence type for a chain. For example, nonce for EVM.
	SEQ types.Sequence,
	// Represents the chain specific fee type
	FEE feetypes.Fee,
] interface {
	UnstartedTxQueuePruner
	TxHistoryReaper[CHAIN_ID]
	TransactionStore[ADDR, CHAIN_ID, TX_HASH, BLOCK_HASH, SEQ, FEE]
	SequenceStore[ADDR, CHAIN_ID, SEQ]

	// methods for saving & retreiving receipts
	FindReceiptsPendingConfirmation(ctx context.Context, blockNum int64, chainID CHAIN_ID) (receiptsPlus []ReceiptPlus[R], err error)
	SaveFetchedReceipts(ctx context.Context, receipts []R, chainID CHAIN_ID) (err error)

	// additional methods for tx store management
	CheckTxQueueCapacity(ctx context.Context, fromAddress ADDR, maxQueuedTransactions uint64, chainID CHAIN_ID) (err error)
	Close()
	Abandon(ctx context.Context, id CHAIN_ID, addr ADDR) error
}

type TransactionManagerStore[
	ADDR types.Hashable,
	CHAIN_ID types.ID,
	TX_HASH types.Hashable,
	BLOCK_HASH types.Hashable,
	SEQ types.Sequence,
	FEE feetypes.Fee,
] interface {
	FindTxWithIdempotencyKey(ctx context.Context, idempotencyKey string, chainID CHAIN_ID) (tx *Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error)
	CheckTxQueueCapacity(ctx context.Context, fromAddress ADDR, maxQueuedTransactions uint64, chainID CHAIN_ID) (err error)
	CreateTransaction(ctx context.Context, txRequest TxRequest[ADDR, TX_HASH], chainID CHAIN_ID) (tx Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error)
}

// TransactionStore contains the persistence layer methods needed to manage Txs and TxAttempts
type TransactionStore[
	ADDR types.Hashable,
	CHAIN_ID types.ID,
	TX_HASH types.Hashable,
	BLOCK_HASH types.Hashable,
	SEQ types.Sequence,
	FEE feetypes.Fee,
] interface {
	// METRICS / STATS FUNCTIONS
	////////
	// NOTE(jtw): used in broadcaster
	CountUnconfirmedTransactions(ctx context.Context, fromAddress ADDR, chainID CHAIN_ID) (count uint32, err error)
	// NOTE(jtw): used in broadcaster
	CountUnstartedTransactions(ctx context.Context, fromAddress ADDR, chainID CHAIN_ID) (count uint32, err error)
	// SEARCH FUNCTIONS
	////////
	// NOTE(jtw): used in TXM Manager
	// Search for Tx using the idempotencyKey and chainID
	FindTxWithIdempotencyKey(ctx context.Context, idempotencyKey string, chainID CHAIN_ID) (tx *Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error)
	// NOTE(jtw): used in broadcaster
	// TODO(jtw): unclear of this responsibility
	FindNextUnstartedTransactionFromAddress(ctx context.Context, etx *Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], fromAddress ADDR, chainID CHAIN_ID) error
	// NOTE(jtw): used in broadcaster
	GetTxInProgress(ctx context.Context, fromAddress ADDR) (etx *Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error)
	// MODIFICATION FUNCTIONS
	////////
	// NOTE(jtw): used in txn manager, blockhashstore, flux monitor v2?, transmitter, pipeline service, vrf v2
	CreateTransaction(ctx context.Context, txRequest TxRequest[ADDR, TX_HASH], chainID CHAIN_ID) (tx Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error)
	// NOTE(jtw): used in broadcaster
	UpdateTxAttemptInProgressToBroadcast(etx *Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], attempt TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], NewAttemptState TxAttemptState, incrNextSequenceCallback QueryerFunc, qopts ...pg.QOpt) error
	// NOTE(jtw): used in broadcaster
	UpdateTxUnstartedToInProgress(ctx context.Context, etx *Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], attempt *TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) error
	// NOTE(jtw): used in broadcaster
	UpdateTxFatalError(ctx context.Context, etx *Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) error
	// NOTE(jtw): used in confirmer, broadcaster
	SaveReplacementInProgressAttempt(ctx context.Context, oldAttempt TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], replacementAttempt *TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) error

	// SEARCH FUNCTIONS
	////////
	// NOTE(jtw): used in confirmer
	FindTxsRequiringGasBump(ctx context.Context, address ADDR, blockNum, gasBumpThreshold, depth int64, chainID CHAIN_ID) (etxs []*Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error)
	// NOTE(jtw): used in confirmer
	FindTxsRequiringResubmissionDueToInsufficientFunds(ctx context.Context, address ADDR, chainID CHAIN_ID) (etxs []*Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error)
	// NOTE(jtw): used in confirmer
	FindTxAttemptsConfirmedMissingReceipt(ctx context.Context, chainID CHAIN_ID) (attempts []TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error)
	// NOTE(jtw): used in confirmer
	FindTxAttemptsRequiringReceiptFetch(ctx context.Context, chainID CHAIN_ID) (attempts []TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error)
	// NOTE(jtw): used in resender
	FindTxAttemptsRequiringResend(ctx context.Context, olderThan time.Time, maxInFlightTransactions uint32, chainID CHAIN_ID, address ADDR) (attempts []TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error)
	// NOTE(jtw): used in confirmer
	// Search for Tx using the fromAddress and sequence
	FindTxWithSequence(ctx context.Context, fromAddress ADDR, seq SEQ) (etx *Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error)
	// NOTE(jtw): used in confirmer
	FindTransactionsConfirmedInBlockRange(ctx context.Context, highBlockNumber, lowBlockNumber int64, chainID CHAIN_ID) (etxs []*Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error)
	// NOTE(jtw): used in confirmer
	GetInProgressTxAttempts(ctx context.Context, address ADDR, chainID CHAIN_ID) (attempts []TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error)
	// NOTE(jtw): used in nonce syncer
	HasInProgressTransaction(ctx context.Context, account ADDR, chainID CHAIN_ID) (exists bool, err error)
	// MODIFICATION FUNCTIONS
	////////
	// NOTE(jtw): used in confirmer
	DeleteInProgressAttempt(ctx context.Context, attempt TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) error
	// NOTE(jtw): used in confirmer
	// TODO(jtw): unclear of this responsibility
	LoadTxAttempts(ctx context.Context, etx *Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) error
	// NOTE(jtw): used in confirmer
	MarkAllConfirmedMissingReceipt(ctx context.Context, chainID CHAIN_ID) (err error)
	// NOTE(jtw): used in confirmer
	MarkOldTxesMissingReceiptAsErrored(ctx context.Context, blockNum int64, finalityDepth uint32, chainID CHAIN_ID) error
	// NOTE(jtw): used in confirmer
	PreloadTxes(ctx context.Context, attempts []TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) error
	// NOTE(jtw): used in confirmer
	SaveConfirmedMissingReceiptAttempt(ctx context.Context, timeout time.Duration, attempt *TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], broadcastAt time.Time) error
	// NOTE(jtw): used in confirmer
	SaveInProgressAttempt(ctx context.Context, attempt *TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) error
	// NOTE(jtw): used in confirmer
	SaveInsufficientFundsAttempt(ctx context.Context, timeout time.Duration, attempt *TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], broadcastAt time.Time) error
	// NOTE(jtw): used in confirmer
	SaveSentAttempt(ctx context.Context, timeout time.Duration, attempt *TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], broadcastAt time.Time) error
	// NOTE(jtw): used in confirmer
	SetBroadcastBeforeBlockNum(ctx context.Context, blockNum int64, chainID CHAIN_ID) error
	// NOTE(jtw): used in confirmer, resender
	UpdateBroadcastAts(ctx context.Context, now time.Time, etxIDs []int64) error
	// NOTE(jtw): used in confirmer
	UpdateTxsUnconfirmed(ctx context.Context, ids []int64) error
	// NOTE(jtw): used in confirmer
	UpdateTxForRebroadcast(ctx context.Context, etx Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], etxAttempt TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) error
}

type TxHistoryReaper[CHAIN_ID types.ID] interface {
	ReapTxHistory(ctx context.Context, minBlockNumberToKeep int64, timeThreshold time.Time, chainID CHAIN_ID) error
}

type UnstartedTxQueuePruner interface {
	PruneUnstartedTxQueue(ctx context.Context, queueSize uint32, subject uuid.UUID) (n int64, err error)
}

type SequenceStore[
	ADDR types.Hashable,
	CHAIN_ID types.ID,
	SEQ types.Sequence,
] interface {
	UpdateKeyNextSequence(newNextSequence, currentNextSequence SEQ, address ADDR, chainID CHAIN_ID, qopts ...pg.QOpt) error
}

// R is the raw unparsed transaction receipt
type ReceiptPlus[R any] struct {
	ID           uuid.UUID `db:"pipeline_run_id"`
	Receipt      R         `db:"receipt"`
	FailOnRevert bool      `db:"fail_on_revert"`
}

type QueryerFunc = func(tx pg.Queryer) error

type ChainReceipt[TX_HASH, BLOCK_HASH types.Hashable] interface {
	GetStatus() uint64
	GetTxHash() TX_HASH
	GetBlockNumber() *big.Int
	IsZero() bool
	IsUnmined() bool
	GetFeeUsed() uint64
	GetTransactionIndex() uint
	GetBlockHash() BLOCK_HASH
}
