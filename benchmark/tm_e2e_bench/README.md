# Tendermint E2E DB Benchmark

`tm_e2e_bench` runs an in-process single-node Sei Tendermint load benchmark and
emits per-backend JSON plus a text summary. It is intended for storage-backend
comparisons where block persistence, the KV tx index, and post-commit read
queries should be measured as separate phases.

The default command runs the stock GoLevelDB backend:

```bash
GOWORK=off go run ./benchmark/tm_e2e_bench -out /tmp/sei-tm-e2e-smoke
```

TreeDB comparison runs require a TreeDB-enabled `github.com/tendermint/tm-db`
dependency in the checkout. Once that dependency is present, run both backends
explicitly:

```bash
GOWORK=off go run ./benchmark/tm_e2e_bench \
  -out /tmp/sei-tm-e2e-treedb \
  -backends goleveldb,treedb \
  -indexer kv \
  -read-scenario tx \
  -read-requests 2000000 \
  -read-concurrency 128
```

The near-100 MiB mega-block shape used in the checked-in report can be
reproduced with:

```bash
GOWORK=off go run ./benchmark/tm_e2e_bench \
  -out /tmp/sei-tm-e2e-megablock \
  -backends goleveldb,treedb \
  -indexer kv \
  -duration 30s \
  -max-txs 95511 \
  -wait-target-txs 95511 \
  -payload-bytes 1024 \
  -block-max-bytes 104857600 \
  -mempool-txs 250000 \
  -mempool-bytes 4294967296 \
  -mempool-notify-txs 90000 \
  -read-scenario tx \
  -read-requests 2000000 \
  -read-concurrency 128
```

Primary outputs:

- `<out>/results.json`: machine-readable result array.
- `<out>/<backend>/result.json`: per-backend result.
- `<out>/summary.txt`: compact metrics and backend ratios.

Use the report at `benchmark/reports/treedb-vs-leveldb-sei-e2e-imrad-report.md`
for the July 2026 TreeDB versus GoLevelDB evidence snapshot.
