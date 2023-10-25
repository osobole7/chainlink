package types

import (
	"github.com/pkg/errors"
	feetypes "github.com/smartcontractkit/chainlink/v2/common/fee/types"
	"github.com/smartcontractkit/chainlink/v2/common/types"
)

var (
	ErrAddressNotFound = errors.New("tx_cache: address not found")
	ErrTxnNotFound     = errors.New("tx_cache: transaction not found")
)

// This represents the TxRequest, as requested by the caller.
// Maps to existing txmgr.types.TxRequest type: https://github.com/smartcontractkit/chainlink/blob/e11ba4b9b263d823da3b1ae4878617424ca014fb/common/txmgr/types/tx.go#L45
type InMemoryTxRequest[
	CHAIN_ID types.ID,
	ADDR, TX_HASH, BLOCK_HASH types.Hashable,
	SEQ types.Sequence,
	FEE feetypes.Fee,
] struct {
	// will have existing fields from txmgr.types.TxRequest
	TxRequest[ADDR, TX_HASH]

	// The Tx associated with this request
	Tx *InMemoryTx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
}

// This represents the Tx, as requested by the caller.
// Maps to existing txmgr.types.Tx type: https://github.com/smartcontractkit/chainlink/blob/e11ba4b9b263d823da3b1ae4878617424ca014fb/common/txmgr/types/tx.go#L144
type InMemoryTx[
	CHAIN_ID types.ID,
	ADDR, TX_HASH, BLOCK_HASH types.Hashable,
	SEQ types.Sequence, FEE feetypes.Fee,
] struct {
	// will have existing fields from txmgr.types.Tx
	Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]

	// All TxRequests associated with this tx. If batch_size == 1, then this will have only 1 item.
	txRequests []*InMemoryTxRequest[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
	// All Txs associated with this tx
	attempts []*InMemoryTxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
}

// This represents the onchain Tx attempt.
// Maps to existing txmgr.types.TxAttempt type: https://github.com/smartcontractkit/chainlink/blob/5bd5238c1719f7799d3d2a7ac5198b9a2019cd4f/common/txmgr/types/tx.go#L116
type InMemoryTxAttempt[CHAIN_ID types.ID, ADDR, TX_HASH, BLOCK_HASH types.Hashable, SEQ types.Sequence, FEE feetypes.Fee] struct {
	// will have existing fields from txmgr.types.TxAttempt
	TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]

	// The tx associated with this attempt
	tx *InMemoryTx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]

	// Receipt associated with this attempt, if present
	receipt *ChainReceipt[TX_HASH, BLOCK_HASH]
}

/* TODO(jtw): is this needed?
// Receipt for an onchain confirmed tx.
// Maps to existing txmgr.types.Receipt type: https://github.com/smartcontractkit/chainlink/blob/930491645925956b6dbeb4ffc92dcfec4154cc84/common/txmgr/types/tx_store.go#L101
type InMemoryReceipt struct {
	// existing fields from txmgr.types.Receipt
}
*/

/*
// State represents the state of all txs for a single fromAddress
type State[CHAIN_ID types.ID, ADDR, TX_HASH, BLOCK_HASH types.Hashable, SEQ types.Sequence, FEE feetypes.Fee] struct {
	// All in-flight TxRequests
	txRequests TxRequestList[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]

	// All in-flight Txes
	txs TxList[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]

	// All in-flight TxAttempts
	Attempts AttemptList[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
}

// Contains all txRequests for a single fromAddress, grouped by state
type TxRequestList[CHAIN_ID types.ID, ADDR, TX_HASH, BLOCK_HASH types.Hashable, SEQ types.Sequence, FEE feetypes.Fee] struct {
	unstarted  []InMemoryTxRequest[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
	inProgress []InMemoryTxRequest[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
	fatalError []InMemoryTxRequest[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
}

// Contains all txes for a single fromAddress, grouped by state
type TxList[CHAIN_ID types.ID, ADDR, TX_HASH, BLOCK_HASH types.Hashable, SEQ types.Sequence, FEE feetypes.Fee] struct {
	unstarted      []InMemoryTx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
	inProgress     []InMemoryTx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
	Unconfirmed    []InMemoryTx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
	Confirmed      []InMemoryTx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
	missingReceipt []InMemoryTx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
	fatalError     []InMemoryTx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
}

// Contains all txAttempts for a single fromAddress, grouped by state
type AttemptList[CHAIN_ID types.ID, ADDR, TX_HASH, BLOCK_HASH types.Hashable, SEQ types.Sequence, FEE feetypes.Fee] struct {
	inProgress        []InMemoryTxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
	insufficientFunds []InMemoryTxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
	Broadcast         []InMemoryTxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
}

// FullState is main container type, which contains complete state for a chainId
type FullState[CHAIN_ID types.ID, ADDR, TX_HASH, BLOCK_HASH types.Hashable, SEQ types.Sequence, FEE feetypes.Fee] struct {
	sync.RWMutex

	// This shards the state by fromAddress
	stateByFromAddress map[string]State[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]

	// Map from id to ALL Txes
	txByID map[int64]*InMemoryTx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
	// Map from TxHash to ALL TxAttempts
	attemptByTxHash map[string]*InMemoryTxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
	// Map from TxHash to ALL Receipts
	receiptByTxHash map[string]*ChainReceipt[TX_HASH, BLOCK_HASH]

	// Map from idempotencyKey to Tx
	idempotencyKeyToTx map[string]*InMemoryTx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
}

// NewFullState creates a new FullState
func NewFullState[CHAIN_ID types.ID, ADDR, TX_HASH, BLOCK_HASH types.Hashable, SEQ types.Sequence, FEE feetypes.Fee]() *FullState[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE] {
	return &FullState[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]{
		stateByFromAddress: make(map[string]State[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]),
		txByID:             make(map[int64]*InMemoryTx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]),
		attemptByTxHash:    make(map[string]*InMemoryTxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]),
		receiptByTxHash:    make(map[string]*ChainReceipt[TX_HASH, BLOCK_HASH]),
		idempotencyKeyToTx: make(map[string]*InMemoryTx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]),
	}
}

// CreateTransaction creates a new transaction for a given txRequest.
func (fs *FullState[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) CreateTransaction(txRequest TxRequest[ADDR, TX_HASH], chainID CHAIN_ID) (*InMemoryTx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], error) {
	fs.RLock()
	defer fs.RUnlock()

	// if there is a PipelineTaskRunID in the txRequest, find the tx by that id and chainID
	// is the idempotency key the same as the PipelineTaskRunID? in flux manager v2 it looks like it is set to the run id
	if txRequest.PipelineTaskRunID != nil {
		// if found return it
		for _, txn := range fs.txByID {
			// TODO(jtw): This is not ideal for searching
			if txn.PipelineTaskRunID.UUID.String() == txRequest.PipelineTaskRunID.String() && txn.ChainID.String() == chainID.String() {
				return txn, nil
			}
		}
	}

	// if a tx exists then return it without making a new one
	// if the tx does not exist then create the tx and return it
	return nil, fmt.Errorf("not implemented")
}

// FindTxWithIdempotencyKey finds a transaction with the given idempotencyKey and chainID.
func (fs *FullState[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) FindTxWithIdempotencyKey(idempotencyKey string) (*InMemoryTx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], error) {
	fs.RLock()
	defer fs.RUnlock()

	// find if txn exists with idempotencyKey and chain id
	tx, ok := fs.idempotencyKeyToTx[idempotencyKey]
	if !ok {
		return nil, ErrTxnNotFound
	}
	// TODO(jtw): is this needed? can an idempotency key be used for multiple txns and have different chain ids?
	// looks like it currently impossible due to the uniqueness constraint for the column
	//	if tx.ChainID != chainID {
		//	return nil, ErrTxnNotFound
	//	}

	return tx, nil
}

// CheckTxQueueCapacity checks the queue capacity for a given fromAddress.
// TODO(jtw): is chainID needed?
func (fs *FullState[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) CheckTxQueueCapacity(fromAddress ADDR, maxQueuedTransactions uint64) (err error) {
	if maxQueuedTransactions == 0 {
		return nil
	}

	fs.RLock()
	defer fs.RUnlock()
	// find all txs for fromAddress
	state, ok := fs.stateByFromAddress[fromAddress.String()]
	if !ok {
		return ErrAddressNotFound
	}

	count := uint64(len(state.txs.unstarted))
	// if the number of unstarted txs is greater than the maxQueuedTransactions then return error
	if count >= maxQueuedTransactions {
		return fmt.Errorf("cannot create transaction; too many unstarted transactions in the queue (%v/%v). %s", count, maxQueuedTransactions, label.MaxQueuedTransactionsWarning)
	}

	return nil
}
*/
