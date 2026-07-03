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
  -profile-dir /tmp/sei-tm-e2e-profiles \
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
  -profile-dir /tmp/sei-tm-e2e-megablock-profiles \
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
- `<profile-dir>/<backend>.cpu.pprof`: per-backend CPU profile when
  `-profile-dir` is set.
- `<profile-dir>/<backend>.heap.pprof`: per-backend heap profile when
  `-profile-dir` is set.

Each JSON result includes a `phase_timings` object with backend startup, first
height wait, load, settle, commit wait, block-stat collection, tx-hash
collection, tx-index readiness, read delay, read phase, and data-size walk
timings. The same phase values are repeated in `summary.txt` with
`phase_*_s` keys for simple diffing.

When `-profile-dir` is set, CPU profiling is active for the full backend run.
That makes the profile representative of the measured backend envelope, but it
also means profile overhead is included in the timing fields. Compare profile
runs against profile runs, and use `pprof -diff_base` for before/after claims:

```bash
go tool pprof -top \
  -diff_base /tmp/sei-tm-e2e-megablock-profiles/goleveldb.cpu.pprof \
  /tmp/sei-tm-e2e-megablock-profiles/treedb.cpu.pprof
```

Use the report at `benchmark/reports/treedb-vs-leveldb-sei-e2e-imrad-report.md`
for the July 2026 TreeDB versus GoLevelDB evidence snapshot.
