package indexer

import (
	"testing"

	"github.com/stretchr/testify/require"

	abci "github.com/sei-protocol/sei-chain/sei-tendermint/abci/types"
	"github.com/sei-protocol/sei-chain/sei-tendermint/types"
)

func TestBatchTxHashesComplete(t *testing.T) {
	batch := NewBatch(2)
	tx0 := types.Tx("zero")
	tx1 := types.Tx("one")

	require.NoError(t, batch.AddTxEvent(types.NewEventDataTxWithHash(abci.TxResultV2{
		Height: 1,
		Index:  0,
		Tx:     tx0,
	}, tx0.Hash())))
	_, complete := batch.TxHashes()
	require.False(t, complete)

	require.NoError(t, batch.AddTxEvent(types.NewEventDataTxWithHash(abci.TxResultV2{
		Height: 1,
		Index:  1,
		Tx:     tx1,
	}, tx1.Hash())))
	hashes, complete := batch.TxHashes()
	require.True(t, complete)
	require.Equal(t, []types.TxHash{tx0.Hash(), tx1.Hash()}, hashes)

	hashes[0] = types.TxHash{}
	hashes, complete = batch.TxHashes()
	require.True(t, complete)
	require.Equal(t, tx0.Hash(), hashes[0])
}

func TestBatchTxHashesIncompleteWithoutMetadata(t *testing.T) {
	batch := NewBatch(1)
	tx := types.Tx("zero")

	require.NoError(t, batch.AddTxEvent(types.EventDataTx{TxResultV2: abci.TxResultV2{
		Height: 1,
		Index:  0,
		Tx:     tx,
	}}))

	_, complete := batch.TxHashes()
	require.False(t, complete)
}
