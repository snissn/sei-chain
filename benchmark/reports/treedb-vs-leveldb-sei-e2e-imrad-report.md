# TreeDB vs GoLevelDB in Sei Tendermint E2E Load and Tx-Index Reads

Date: 2026-07-02

Workspace root: `/home/mikers/dev/snissn/sei-chain`

Artifact root: `/mnt/fast4tb/sei-chain-e2e-goal-20260702`

TreeDB Sei worktree: `/mnt/fast4tb/sei-chain-treedb-bench-20260702T203653Z/sei-chain-treedb`

TreeDB tm-db shim: `/mnt/fast4tb/sei-chain-treedb-bench-20260702T203653Z/cometbft-db-v20260420`

## Abstract

This study compares Sei Tendermint's default GoLevelDB storage backend with TreeDB under transaction-indexed load. The benchmark separates four phases that can otherwise be conflated: transaction submission, commit visibility, tx-index readability, and already-readable `Tx(hash)` lookup throughput. Timed tx-read measurements only begin after all collected committed hashes are queryable, so read throughput and latency are measured against successful reads.

The strongest TreeDB result appears under a near-100 MiB mega-block workload with the KV tx index enabled. In that shape, both backends commit and index the same 95,511 transactions, but TreeDB reaches commit visibility at `1.2734x` GoLevelDB drain throughput and reaches "committed and queryable by hash" at `1.6079x` GoLevelDB throughput. Once the tx index is readable, GoLevelDB has higher raw read RPS in the 2,000,000-read mega-block run (`801,755` vs `723,262`), while TreeDB has much lower p95 read latency (`0.017 ms` vs `0.987 ms`).

The p95 result is real for high-concurrency reads, but it is not a backend-wide constant. With full latency sampling, GoLevelDB has lower p95 at read concurrency 1, while TreeDB has much lower p95 at concurrency 16 and 128. Longer c=128 runs confirm that the high-concurrency p95 gap persists with immediate reads, with a 10-second post-readiness delay, and with reversed backend order. The far tail remains a separate issue: in the 1,000,000-read c=128 runs, TreeDB has lower p95 and p99 but worse p99.9 and max outliers.

## Introduction

Sei is performance-oriented, so a useful storage comparison must stress the paths where storage can dominate end-to-end behavior. Small blocks can leave consensus scheduling, app execution, mempool admission, and benchmark overhead as the limiting factors. This study therefore uses two complementary shapes:

- A successful-read workload with 20,063 committed transactions and 2,000,000 tx-index reads.
- A forced mega-block workload with one near-100 MiB block, 95,511 committed transactions, and the KV tx index enabled.

The central question is not whether TreeDB is uniformly faster on every metric. The question is where TreeDB changes the performance envelope for Cosmos/Tendermint-style operation. The measurements below keep phase boundaries explicit: commit visibility, tx-index readiness, raw read RPS, and read latency are reported separately.

## Methods

### Code Under Test

The comparison used the TreeDB-enabled Sei worktree and tm-db wrapper prepared for this benchmark:

- Sei worktree: `/mnt/fast4tb/sei-chain-treedb-bench-20260702T203653Z/sei-chain-treedb`
- tm-db shim: `/mnt/fast4tb/sei-chain-treedb-bench-20260702T203653Z/cometbft-db-v20260420`
- E2E harness source: `/mnt/fast4tb/sei-chain-e2e-goal-20260702/tm_e2e_bench/main.go`
- E2E harness binaries:
  - `/mnt/fast4tb/sei-chain-e2e-goal-20260702/bin/tm_e2e_bench_readwait`
  - `/mnt/fast4tb/sei-chain-e2e-goal-20260702/bin/tm_e2e_bench_tail`

The tx-index lookup path uses generated `TxResult.Unmarshal` and, when the backing store supports it, reads with `GetAppend` into a pooled buffer. The external `TxIndexer` API and result ownership semantics are unchanged.

### Phase Definitions

The E2E harness reports these phases independently:

| Phase | Definition |
| --- | --- |
| Load | Time spent submitting transactions through `BroadcastTxSync`. |
| Drain | Load plus the wait for the target committed transactions to become visible in block metadata. |
| Tx-index readiness | Time after drain until all collected committed tx hashes return successful `Tx(hash)` lookups. |
| Timed reads | Successful read phase after tx-index readiness; read errors are counted and reported. |

The mega-block "committed and queryable" metric is computed as:

```text
committed_txs / (drain_seconds + read_validation_seconds)
```

This metric is the fairest end-to-end denominator for users who need submitted transactions to be both committed and queryable by hash.

### Workloads

The final benchmark set contains four workloads:

| Workload | Purpose | Shape |
| --- | --- | --- |
| Tx-index microbenchmark | Isolate `TxIndex.Get(hash)` cost. | 100,000 indexed synthetic tx results, 1024-byte tx payloads. |
| Successful-read E2E | Compare raw successful read RPS and latency under a smaller committed set. | 20,063 committed txs, 512-byte tx target payloads, 2,000,000 timed `Tx(hash)` reads. |
| Mega-block E2E | Stress block persistence and KV tx-index write/readiness behavior. | 95,511 committed txs in one near-100 MiB block, 1024-byte tx target payloads, 2,000,000 timed `Tx(hash)` reads. |
| Execution TPS profile | Identify CPU and allocation bottlenecks in the load/drain path. | Two near-100 MiB tx blocks per backend, reads disabled, CPU and heap profiles captured per backend. |
| P95 audit | Test whether the high-concurrency p95 gap is caused by sampling, readiness, or backend ordering. | Full latency sampling at read concurrency 1, 16, and 128; longer c=128 runs with immediate start, 10-second delay, and reversed backend order. |

The tx-read workloads collect committed tx hashes from the committed blocks, wait until the hashes are readable through the real `Tx(hash)` path, and then start the timed read phase.

## Results

### Tx-Index Lookup Microbenchmark

Final tx-index lookup benchmark results:

| Benchmark | Backend | ns/op | MB/s | B/op | allocs/op |
| --- | --- | ---: | ---: | ---: | ---: |
| `BenchmarkTxIndexGet` | GoLevelDB | 2267 | 451.78 | 3269 | 20 |
| `BenchmarkTxIndexGet` | TreeDB | 5509 | 185.88 | 3399 | 9 |
| `BenchmarkTxIndexGetParallel` | GoLevelDB | 776.3 | 1319.05 | 3197 | 18 |
| `BenchmarkTxIndexGetParallel` | TreeDB | 1186 | 863.50 | 2687 | 9 |

GoLevelDB is faster in this isolated lookup benchmark. TreeDB performs fewer allocations, and in the parallel case it also allocates fewer bytes per operation.

### Successful-Read E2E Workload

Run directory: `/mnt/fast4tb/sei-chain-e2e-goal-20260702/tm-e2e-txread-20k-512-r2m-readopt`

| Backend | Committed txs | Drain TPS | Timed read hashes | Read RPS | Read errors | p50 ms | p95 ms | p99 ms | Data bytes |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| GoLevelDB | 20,063 | 16,236.96 | 20,063 | 1,042,603 | 0 | 0.005 | 0.517 | 3.139 | 33,696,345 |
| TreeDB | 20,063 | 16,412.80 | 20,063 | 874,234 | 0 | 0.007 | 0.017 | 0.080 | 28,878,588 |

| Metric | TreeDB / GoLevelDB |
| --- | ---: |
| Drain TPS | 1.0108x |
| Read RPS | 0.8385x |
| Read p95 | 0.0329x |
| Data bytes | 0.8570x |

This workload does not show a TreeDB raw read-RPS win. It does show much lower TreeDB tail latency and a smaller on-disk footprint.

### Mega-Block E2E Workload

Run directory: `/mnt/fast4tb/sei-chain-e2e-goal-20260702/tm-e2e-megablock-100m-kv-90k-p1024-readwait-ab`

| Backend | Committed txs | Max block bytes | Drain TPS | Tx-index readiness s | Read RPS | Read errors | p50 ms | p95 ms | p99 ms | Data bytes |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| GoLevelDB | 95,511 | 98,090,400 | 34,624.21 | 1.471 | 801,755 | 0 | 0.009 | 0.987 | 2.664 | 126,459,147 |
| TreeDB | 95,511 | 98,090,397 | 44,091.33 | 0.464 | 723,262 | 0 | 0.011 | 0.017 | 0.823 | 128,710,397 |

| Metric | TreeDB / GoLevelDB |
| --- | ---: |
| Drain TPS | 1.2734x |
| Committed-and-queryable TPS | 1.6079x |
| Read RPS | 0.9021x |
| Read p95 | 0.0172x |
| Tx-index readiness time | 0.3156x |
| Data bytes | 1.0178x |

The mega-block shape exposes the storage difference more clearly than the smaller successful-read shape. TreeDB commits the same near-100 MiB block faster, reaches tx-index readability faster, and has much lower p95/p99 read latency after readiness. GoLevelDB still has higher already-readable raw read RPS in this run.

### Execution TPS Profile

Run directories:

- `/mnt/fast4tb/sei-chain-e2e-goal-20260702/tm-e2e-exec-profile-2blocks-goleveldb`
- `/mnt/fast4tb/sei-chain-e2e-goal-20260702/tm-e2e-exec-profile-2blocks-treedb`

The execution profile uses the mega-block shape with reads disabled. Each backend commits two non-empty near-100 MiB tx blocks.

| Backend | Committed txs | Non-empty blocks | Max block txs | Total block bytes | Load s | Wait s | Drain s | Drain TPS | Data bytes |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| GoLevelDB | 200,459 | 2 | 102,100 | 205,873,197 | 1.065 | 4.321 | 5.387 | 37,214 | 245,792,906 |
| TreeDB | 200,511 | 2 | 101,966 | 205,925,997 | 0.995 | 3.254 | 4.249 | 47,190 | 248,581,387 |

TreeDB's execution-profile drain throughput is `1.268x` GoLevelDB, consistent with the one-block mega result.

CPU profile summary:

| Area | GoLevelDB CPU | TreeDB CPU | Interpretation |
| --- | ---: | ---: | --- |
| SHA-256 tx hashing | `15.24%` cumulative in `sha256.Sum256`; `12.40%` cumulative in `Tx.Hash` | `17.93%` cumulative in `sha256.Sum256`; `14.95%` cumulative in `Tx.Hash` | Repeated tx hashing is a first-order shared cost. |
| GC scan/mark | `28.25%` cumulative in `gcDrain`; `27.34%` cumulative in `scanobject` | `18.84%` cumulative in `gcDrain`; `17.57%` cumulative in `scanobject` | Allocation pressure is a major execution-path cost, especially for GoLevelDB. |
| Memory copy | `13.17%` flat in `runtime.memmove` | `11.14%` flat in `runtime.memmove` | Large tx/block/index payload movement is material for both backends. |
| Mempool admission | `10.64%` cumulative in `TxMempool.CheckTx` | `13.95%` cumulative in `TxMempool.CheckTx` | CheckTx, duplicate-cache, and tx-store work are visible outside the DB backend. |
| Harness tx construction | `7.73%` cumulative in `main.makeTx` | `7.16%` cumulative in `main.makeTx` | The load generator is not free; storage-only claims should account for it. |
| KV tx-index write | `10.72%` cumulative in `TxIndex.Index` | `5.34%` cumulative in `TxIndex.Index` | TreeDB spends materially less CPU in tx-index write for this shape. |
| Backend batch/write | `7.73%` cumulative in `goLevelDBBatch.Set`; `7.27%` in `leveldb.Batch.grow`; `2.76%` in compaction | `1.90%` cumulative in TreeDB `Batch.writeRegular`/`CommitSync`; `3.99%` compression trainer; `3.44%` zstd encode | GoLevelDB pays large-batch growth/copy cost; TreeDB pays compression/dictionary costs. |

Allocation profile summary:

| Area | GoLevelDB alloc_space | TreeDB alloc_space | Interpretation |
| --- | ---: | ---: | --- |
| Total sampled allocation | `10.28 GiB` | `8.15 GiB` | TreeDB allocates less overall in this execution profile. |
| Backend batch/index allocation | `4.87 GiB` flat in `leveldb.Batch.grow`; `4.65 GiB` cumulative in `TxIndex.Index` | `944 MiB` flat in TreeDB memtable entry pooling; `764 MiB` cumulative in `TxIndex.Index` | GoLevelDB's large tx-index batch dominates allocation. |
| Common block/app allocation | `1.17 GiB` in `io.ReadAll`; `774 MiB` in `proto.Marshal`; `653 MiB` in app tx parsing | `1.17 GiB` in `io.ReadAll`; `800 MiB` in `proto.Marshal`; `609 MiB` in app tx parsing | Block reconstruction, protobuf materialization, and app parsing are shared costs. |
| TreeDB compression allocation | not applicable | `500 MiB` zstd history, `410 MiB` dictionary build, `249 MiB` compression batch totals | Compression setup is a visible TreeDB-specific optimization target. |

### P95 Audit

The p95 audit uses full latency sampling, so every timed read contributes to the reported percentiles.

| Read concurrency | GoLevelDB RPS | TreeDB RPS | RPS ratio | GoLevelDB p95 ms | TreeDB p95 ms | p95 ratio |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 163,310 | 123,905 | 0.7587x | 0.008 | 0.012 | 1.5000x |
| 16 | 732,928 | 1,062,729 | 1.4500x | 0.045 | 0.010 | 0.2222x |
| 128 | 683,389 | 877,523 | 1.2841x | 1.036 | 0.010 | 0.0097x |

The p95 ratio is therefore concurrency-sensitive. GoLevelDB has lower p95 in a single-reader regime; TreeDB has much lower p95 once the read phase becomes moderately or highly concurrent.

Longer c=128 checks confirm that the high-concurrency p95 gap is not explained by sparse sampling, tx-index readiness, or backend order:

| Run | Backend | Read RPS | p50 ms | p90 ms | p95 ms | p99 ms | p99.9 ms | Max ms |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Immediate, GoLevelDB first | GoLevelDB | 757,938 | 0.009 | 0.307 | 1.010 | 2.960 | 10.941 | 24.630 |
| Immediate, GoLevelDB first | TreeDB | 945,329 | 0.008 | 0.010 | 0.011 | 0.035 | 37.424 | 358.972 |
| 10s delay, GoLevelDB first | GoLevelDB | 779,810 | 0.009 | 0.299 | 0.995 | 2.853 | 9.852 | 36.116 |
| 10s delay, GoLevelDB first | TreeDB | 672,811 | 0.011 | 0.014 | 0.017 | 0.251 | 48.959 | 182.913 |
| Immediate, TreeDB first | TreeDB | 762,388 | 0.009 | 0.013 | 0.016 | 0.134 | 44.439 | 272.332 |
| Immediate, TreeDB first | GoLevelDB | 752,115 | 0.009 | 0.341 | 1.027 | 2.831 | 8.318 | 43.642 |

All three long c=128 runs validate all 95,511 tx hashes before timing and record zero read errors. GoLevelDB p95 remains near `1 ms` with immediate reads, after a 10-second post-readiness delay, and when GoLevelDB runs second. TreeDB p95 remains in the `0.011-0.017 ms` range.

The same table also shows why p95 should not be treated as the whole tail distribution. TreeDB p95 and p99 are lower, but TreeDB has worse p99.9 and max latency in these long c=128 runs.

## Discussion

### Phase Boundaries Matter

The main storage result is phase-dependent. For the mega-block workload, TreeDB is faster to commit the block and faster to make the committed transactions queryable by hash. Those are the metrics most relevant to "submitted transactions become committed and queryable." Once all hashes are already readable, GoLevelDB can still serve more raw tx-read requests per second in the 2,000,000-read mega-block run.

The same distinction explains the tail-latency results. The high-concurrency p95 gap is measured after tx-index readiness, so it is not a proxy for the index drain. It is an already-readable tx lookup distribution. It survives full sampling, a post-readiness delay, and reversed backend order.

### Where TreeDB Wins

TreeDB's strongest evidence in this benchmark set is:

- `1.2734x` GoLevelDB drain throughput on the near-100 MiB mega-block workload.
- `1.6079x` GoLevelDB committed-and-queryable throughput on the same workload.
- `1.268x` GoLevelDB drain throughput in the two-block execution profile.
- Much lower p95/p99 read latency in the successful-read and mega-block E2E workloads.
- Much lower high-concurrency p95 in the full-sampling audit.

These are substantial results for a storage-backed Tendermint workload, but they are not the same as a blanket raw read-RPS win.

### Where GoLevelDB Remains Stronger

GoLevelDB remains stronger on:

- The isolated `TxIndex.Get` microbenchmark.
- Raw read RPS in the smaller successful-read E2E workload.
- Raw read RPS in the 2,000,000-read mega-block workload.
- Single-reader p95 in the full-sampling audit.
- Far-tail p99.9/max latency in the long c=128 audit runs.

### Remaining Read-Path Costs

The remaining read-cost profile is not just harness overhead. The major remaining categories are backend read/decode work, protobuf `TxResult` materialization, memory clear/copy work, and file read/syscall activity. The next plausible optimization targets are below the E2E harness: TreeDB value-log read/decode policy, compression policy for this payload class, and tx-result materialization.

### Execution-Path Optimization Targets

The execution TPS profile points to five optimization lanes:

1. Cache or carry tx hashes through the mempool, proposal, commit, and tx-index lifecycle. `Tx.Hash` and SHA-256 are a double-digit CPU cost in both backends, so avoiding repeated hashing of immutable transaction bytes is a high-leverage shared optimization.
2. Reduce GoLevelDB tx-index batch growth and copying. In the GoLevelDB profile, `leveldb.Batch.grow` alone allocates `4.87 GiB`; large KV tx-index batches are a primary reason the default backend loses mega-block drain TPS.
3. Tune TreeDB compression for tx-index-shaped batches. TreeDB spends visible CPU and allocation in dictionary training, zstd history setup, and compression batch accounting. Caching dictionaries, reducing training frequency, or using a cheaper policy for this payload class may improve write TPS without changing Tendermint semantics.
4. Reduce protobuf/event materialization churn. `proto.Marshal`, `io.ReadAll`, event publication, and app tx parsing are common allocation sources. Pooled buffers or narrower event payload materialization could reduce GC pressure.
5. Separate benchmark generator overhead from node execution when making storage-only claims. `main.makeTx` is about 7 percent cumulative CPU in both profiles; pre-generated tx payloads would make storage and Tendermint execution profiles cleaner.

## Limitations

- These are single-host, single-node Tendermint runs, not a multi-machine cluster benchmark.
- The mega-block workload uses one near-100 MiB tx block. It is intentionally storage-heavy and should not be read as a typical live network block distribution.
- The p95 audit uses a small number of long replicates. It is strong enough to reject sampling, readiness, and backend-order explanations for the c=128 p95 gap, but not enough to characterize all p99.9/max behavior.
- Full latency sampling adds measurement overhead. It is appropriate for percentile validation, but its RPS should not be mixed directly with sparse-sampling RPS runs.
- The benchmark reports successful `Tx(hash)` reads after tx-index readiness. It does not model external RPC network hops or multi-node contention.

## Conclusion

TreeDB materially improves the storage-heavy mega-block path in Sei Tendermint when the KV tx index is enabled. In the final near-100 MiB workload, TreeDB commits the same 95,511 transactions faster, makes them queryable faster, and reaches `1.6079x` GoLevelDB throughput for the combined committed-and-queryable phase.

TreeDB is not a universal raw tx-read-RPS winner in this benchmark set. GoLevelDB is faster in the isolated tx-index lookup benchmark and in the main already-readable raw read-RPS measurements. TreeDB's read advantage is instead in tail latency at moderate and high concurrency, especially p95 and p99. That result is stable for c=128 under full sampling, delay, and reverse-order checks, but the p99.9/max outliers remain an open tail-risk area.

## Appendix: Reproduction and Artifacts

Representative tx-index benchmark command:

```bash
GOWORK=off \
TMPDIR=/mnt/fast4tb/sei-chain-e2e-goal-20260702/tmp \
GOCACHE=/mnt/fast4tb/sei-chain-e2e-goal-20260702/gocache \
GOMODCACHE=/mnt/fast4tb/sei-chain-e2e-goal-20260702/gomodcache \
go test ./sei-tendermint/internal/state/indexer/tx/kv \
  -run '^$' \
  -bench 'BenchmarkTxIndexGet' \
  -benchmem
```

Primary code artifacts:

| Artifact | SHA-256 |
| --- | --- |
| `tm_e2e_bench/main.go` | `eb1320f83c57fd7069b5772420af48da96c6f447b40f4c30530085e95a09052d` |
| `bin/tm_e2e_bench_readwait` | `68a9be98cf679cf4d8cf49cb153f91ba348353fafd80de5df203c8c333a53898` |
| `bin/tm_e2e_bench_tail` | `41279b88c3a937dc876570d172b679583ff8c076e660d59199a6181c46fd1cbe` |
| `kv/kv.go` | `95395218d4646015d3937f9d81843a35b4bc09d5a6d2b18b618881c0ca339f90` |
| `kv/kv_test.go` | `e352827631539d8c80a21c1ce3791af4295b9431457c8cc6474a03fa31601c40` |

Primary benchmark artifacts:

| Artifact | SHA-256 |
| --- | --- |
| `profiles/txindex-get-optimized.bench.txt` | `b8737c1513b1b6af6a715a6c69f21f739104c83f477aaa040ec32c9b323ae833` |
| `profiles/txindex-get-optimized.cpu.pprof` | `505b04af21eed9289df7728058e4d4af7aa83808a80085f3e6c8e4fe83af3225` |
| `profiles/txindex-get-optimized.mem.pprof` | `b8088141eacc6512e5b8f51f0218b7ab6921ba8e74258c4e81ca` |
| `tm-e2e-txread-20k-512-r2m-readopt/summary.txt` | `d49eebd58d5ca42fefd3f2fee806acde0b66f2809b18f1eeb5065d82ce4a6fb6` |
| `tm-e2e-txread-20k-512-r2m-readopt/results.json` | `7c7bc678ab9c7ab2ba74d1dda494ea82d7c549277a023ad6dd7431100a438dae` |
| `tm-e2e-megablock-100m-kv-90k-p1024-readwait-ab/summary.txt` | `369dee975700459c8ab479707b8e80ae66d9cf03306802fba5a98c3c85c771ec` |
| `tm-e2e-megablock-100m-kv-90k-p1024-readwait-ab/results.json` | `3d91b9b8739d0092c3936b0f705153188b8575d71c42ae3dd352b3ef73d4a9ec` |
| `tm-e2e-exec-profile-2blocks-goleveldb/summary.txt` | `cbd4615e4f581c528af57d25a7b01bf61cd8306850a7eb35cea775ed154c5b89` |
| `tm-e2e-exec-profile-2blocks-goleveldb/results.json` | `97add82498e17856067902802bb360cf607ae0f3e0d43196c9071eb740bca791` |
| `profiles/exec-tps-2blocks/goleveldb.cpu.pprof` | `b592853e570850b4856ecfe72b9800a7f0ec12e24d14da727edd73d33b09f1a7` |
| `profiles/exec-tps-2blocks/goleveldb.mem.pprof` | `7a7f257069c36f0332eddfbe234186f6657be2432867dedc72b688cdcc3f087b` |
| `tm-e2e-exec-profile-2blocks-treedb/summary.txt` | `ea9e1d5484db3d625321dccc71486c4ba3a46c02a018abcf1e24278ad992e668` |
| `tm-e2e-exec-profile-2blocks-treedb/results.json` | `b4ff4f13366d025489c0e0c90a1f5facc5910e9d883a040c96267f18619c50bd` |
| `profiles/exec-tps-2blocks/treedb.cpu.pprof` | `89a26877315d13451e2e110a4aaabd7c4bbbf83f0364841410eadf14eaf4c5fb` |
| `profiles/exec-tps-2blocks/treedb.mem.pprof` | `a1e4bff344c50be3efbf559c3d9ed10f7db60c7b5664b94d7f41aaddc4c0ec3b` |
| `tm-e2e-megap95-c1-sample1-r200k/results.json` | `3843e9a02f24deb0ce2e60345ef9ffd67e589aff2ace2f4247debb224be1e3fb` |
| `tm-e2e-megap95-c16-sample1-r200k/results.json` | `7202a7193f4d5e29de4b45948ca5aabc3b388dbeff274858605670bdda449312` |
| `tm-e2e-megap95-c128-sample1-r200k/results.json` | `e4fd059b98d03a2633bffdbfd0624f8d2f65149694c8ab14170857f5d7b8d622` |
| `tm-e2e-megap95-c128-sample1-r1m-delay0/results.json` | `27e157b051a01f9e11b38ea794054086fb2dc91c8263582a864cf135a316a926` |
| `tm-e2e-megap95-c128-sample1-r1m-delay10/results.json` | `f405c187d340c796a01a2e840b8f347efedc6d33c5cb6fbcd23f06222972e5c5` |
| `tm-e2e-megap95-c128-sample1-r1m-reverse-delay0/results.json` | `5bdb527177be1408a01bec5197e396d4358994da22c930cb7e53054246c538d1` |

Validation:

- `gofmt -s -l` on touched Go files: clean.
- `goimports -l` on touched Go files: clean.
- `go test ./...` for the E2E harness package: pass.
- `GOWORK=off go test ./sei-tendermint/internal/state/indexer/tx/kv`: pass.
