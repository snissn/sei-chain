package indexer

import (
	"context"
	"errors"
	"slices"

	abci "github.com/sei-protocol/sei-chain/sei-tendermint/abci/types"
	"github.com/sei-protocol/sei-chain/sei-tendermint/internal/pubsub/query"
	"github.com/sei-protocol/sei-chain/sei-tendermint/libs/utils"
	"github.com/sei-protocol/sei-chain/sei-tendermint/types"
)

// TxIndexer interface defines methods to index and search transactions.
type TxIndexer interface {
	// Index analyzes, indexes and stores transactions. For indexing multiple
	// Transacions must guarantee the Index of the TxResult is in order.
	// See Batch struct.
	Index(results []*abci.TxResultV2) error

	// Get returns the transaction specified by hash or nil if the transaction is not indexed
	// or stored.
	Get(hash []byte) (*abci.TxResultV2, error)

	// Search allows you to query for transactions.
	Search(ctx context.Context, q *query.Query) ([]*abci.TxResultV2, error)
}

// BlockIndexer defines an interface contract for indexing block events.
type BlockIndexer interface {
	// Has returns true if the given height has been indexed. An error is returned
	// upon database query failure.
	Has(height int64) (bool, error)

	// Index indexes FinalizeBlock events for a given block by its height.
	Index(types.EventDataNewBlockHeader) error

	// Search performs a query for block heights that match a given FinalizeBlock
	// event search criteria.
	Search(ctx context.Context, q *query.Query) ([]int64, error)
}

// Batch groups together multiple Index operations to be performed at the same time.
// NOTE: Batch is NOT thread-safe and must not be modified after starting its execution.
type Batch struct {
	Ops              []*abci.TxResultV2
	txHashes         []types.TxHash
	txHashesPresent  []bool
	txHashesComplete int
	Pending          int64
}

// NewBatch creates a new Batch.
func NewBatch(n int64) *Batch {
	return &Batch{
		Ops:             make([]*abci.TxResultV2, n),
		txHashes:        make([]types.TxHash, n),
		txHashesPresent: make([]bool, n),
		Pending:         n,
	}
}

// Add or update an entry for the given result.Index.
func (b *Batch) Add(result *abci.TxResultV2) error {
	return b.add(result, utils.None[types.TxHash]())
}

// AddTxEvent adds an EventDataTx and preserves its optional tx-hash metadata.
func (b *Batch) AddTxEvent(event types.EventDataTx) error {
	txResult := event.TxResultV2
	return b.add(&txResult, event.TxHashMetadata())
}

func (b *Batch) add(result *abci.TxResultV2, txHash utils.Option[types.TxHash]) error {
	if b.Ops[result.Index] == nil {
		b.Pending--
		b.Ops[result.Index] = result
	}
	if hash, ok := txHash.Get(); ok && !b.txHashesPresent[result.Index] {
		b.txHashes[result.Index] = hash
		b.txHashesPresent[result.Index] = true
		b.txHashesComplete++
	}
	return nil
}

// Size returns the total number of operations inside the batch.
func (b *Batch) Size() int { return len(b.Ops) }

// TxHashes returns the complete ordered tx-hash metadata when every tx event
// carried a hash. The returned slice is caller-owned.
func (b *Batch) TxHashes() ([]types.TxHash, bool) {
	if b.txHashesComplete != len(b.Ops) {
		return nil, false
	}
	return slices.Clone(b.txHashes), true
}

// ErrorEmptyHash indicates empty hash
var ErrorEmptyHash = errors.New("transaction hash cannot be empty")
