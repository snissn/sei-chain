package kv

import (
	"fmt"
	"testing"

	dbm "github.com/tendermint/tm-db"

	abci "github.com/sei-protocol/sei-chain/sei-tendermint/abci/types"
	"github.com/sei-protocol/sei-chain/sei-tendermint/types"
)

func BenchmarkTxIndexBlock(b *testing.B) {
	for _, tc := range []struct {
		name        string
		txCount     int
		txSizeBytes int
	}{
		{name: "100tx", txCount: 100, txSizeBytes: 500},
		{name: "1000tx", txCount: 1000, txSizeBytes: 1024},
		{name: "5000tx", txCount: 5000, txSizeBytes: 1024},
	} {
		results, txHashes, totalTxBytes := makeTxIndexBlockBenchData(tc.txCount, func(int) int {
			return tc.txSizeBytes
		})
		runTxIndexBlockBenchCases(b, tc.name, results, txHashes, totalTxBytes)
	}
}

func BenchmarkTxIndexBlockMixedSizes(b *testing.B) {
	results, txHashes, totalTxBytes := makeTxIndexBlockBenchData(1000, func(index int) int {
		sizes := [...]int{250, 500, 750, 1024, 1536, 2048}
		return sizes[index%len(sizes)]
	})
	runTxIndexBlockBenchCases(b, "1000tx", results, txHashes, totalTxBytes)
}

func runTxIndexBlockBenchCases(
	b *testing.B,
	name string,
	results []*abci.TxResultV2,
	txHashes []types.TxHash,
	totalTxBytes int,
) {
	b.Run(name, func(b *testing.B) {
		b.Run("default-hash", func(b *testing.B) {
			txIndexer := NewTxIndex(dbm.NewMemDB())
			runTxIndexBlockBench(b, totalTxBytes, func() error {
				return txIndexer.Index(results)
			})
		})
		b.Run("verified-hash", func(b *testing.B) {
			txIndexer := NewTxIndex(dbm.NewMemDB())
			runTxIndexBlockBench(b, totalTxBytes, func() error {
				return txIndexer.IndexWithVerifiedHashes(results, txHashes)
			})
		})
	})
}

func runTxIndexBlockBench(b *testing.B, totalTxBytes int, index func() error) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(totalTxBytes))
	b.ResetTimer()

	for range b.N {
		if err := index(); err != nil {
			b.Fatal(err)
		}
	}
}

func makeTxIndexBlockBenchData(txCount int, txSizeBytes func(int) int) ([]*abci.TxResultV2, []types.TxHash, int) {
	results := make([]*abci.TxResultV2, txCount)
	txHashes := make([]types.TxHash, txCount)
	var totalTxBytes int

	for i := range txCount {
		tx := makeTxIndexBlockBenchTx(i, txSizeBytes(i))
		totalTxBytes += len(tx)
		results[i] = &abci.TxResultV2{
			Height: 1,
			Index:  uint32(i),
			Tx:     tx,
			Result: abci.ExecTxResult{
				Code:   abci.CodeTypeOK,
				Events: makeTxIndexBlockBenchEvents(i),
			},
		}
		txHashes[i] = tx.Hash()
	}

	return results, txHashes, totalTxBytes
}

func makeTxIndexBlockBenchTx(index int, size int) types.Tx {
	tx := make(types.Tx, size)
	prefix := fmt.Appendf(nil, "tx-%08d:", index)
	copy(tx, prefix)
	for i := len(prefix); i < len(tx); i++ {
		tx[i] = byte('a' + (index+i)%26)
	}
	return tx
}

func makeTxIndexBlockBenchEvents(index int) []abci.Event {
	return []abci.Event{
		{
			Type: "transfer",
			Attributes: []abci.EventAttribute{
				{Key: []byte("sender"), Value: []byte(fmt.Sprintf("account-%04d", index%512)), Index: true},
				{Key: []byte("amount"), Value: []byte(fmt.Sprintf("%d", 1+index%1000)), Index: true},
			},
		},
		{
			Type: "message",
			Attributes: []abci.EventAttribute{
				{Key: []byte("action"), Value: []byte("send"), Index: true},
			},
		},
	}
}
