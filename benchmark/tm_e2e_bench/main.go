package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tmconfig "github.com/sei-protocol/sei-chain/sei-tendermint/config"
	"github.com/sei-protocol/sei-chain/sei-tendermint/crypto/ed25519"
	tmbytes "github.com/sei-protocol/sei-chain/sei-tendermint/libs/bytes"
	"github.com/sei-protocol/sei-chain/sei-tendermint/node"
	"github.com/sei-protocol/sei-chain/sei-tendermint/privval"
	tmlocal "github.com/sei-protocol/sei-chain/sei-tendermint/rpc/client/local"
	e2eapp "github.com/sei-protocol/sei-chain/sei-tendermint/test/e2e/app"
	tmtypes "github.com/sei-protocol/sei-chain/sei-tendermint/types"
)

type options struct {
	outDir           string
	backends         string
	duration         time.Duration
	maxTxs           int64
	concurrency      int
	payloadBytes     int
	keyspace         int
	indexer          string
	blockMaxBytes    int64
	mempoolTxs       int
	mempoolBytes     int64
	mempoolNotifyTxs uint64
	mempoolTTL       time.Duration
	readRequests     int
	readConcurrency  int
	readScenario     string
	readBatchSize    int
	readSampleRate   int
	readValidate     bool
	readIndexWait    time.Duration
	readIndexPoll    time.Duration
	readStartDelay   time.Duration
	waitTimeout      time.Duration
	waitTargetTxs    int64
	settle           time.Duration
	disableEmpty     bool
	bypassCommitWait bool
	proposeTimeout   time.Duration
	proposeDelta     time.Duration
	voteTimeout      time.Duration
	voteDelta        time.Duration
	commitTimeout    time.Duration
	failOnTimeout    bool
	cpuProfile       string
	memProfile       string
}

type benchResult struct {
	Backend                string        `json:"backend"`
	Home                   string        `json:"home"`
	DBBackend              string        `json:"db_backend"`
	Indexer                string        `json:"indexer"`
	DurationSeconds        float64       `json:"duration_seconds"`
	LoadSeconds            float64       `json:"load_seconds"`
	WaitSeconds            float64       `json:"wait_seconds"`
	DrainSeconds           float64       `json:"drain_seconds"`
	ProducerAttempts       int64         `json:"producer_attempts"`
	ProducerAccepted       int64         `json:"producer_accepted"`
	ProducerErrors         int64         `json:"producer_errors"`
	TimedOut               bool          `json:"timed_out"`
	WaitError              string        `json:"wait_error,omitempty"`
	StartHeight            int64         `json:"start_height"`
	EndHeight              int64         `json:"end_height"`
	CommittedTxs           int64         `json:"committed_txs"`
	NonEmptyBlocks         int64         `json:"non_empty_blocks"`
	BlockCount             int64         `json:"block_count"`
	AvgCommitTPS           float64       `json:"avg_commit_tps"`
	DrainTPS               float64       `json:"drain_tps"`
	AvgBlockTxs            float64       `json:"avg_block_txs"`
	MaxBlockTxs            int           `json:"max_block_txs"`
	P50BlockTxs            int           `json:"p50_block_txs"`
	P95BlockTxs            int           `json:"p95_block_txs"`
	TotalBlockBytes        int64         `json:"total_block_bytes"`
	AvgBlockBytes          float64       `json:"avg_block_bytes"`
	MaxBlockBytes          int           `json:"max_block_bytes"`
	P50BlockBytes          int           `json:"p50_block_bytes"`
	P95BlockBytes          int           `json:"p95_block_bytes"`
	FinalHeight            int64         `json:"final_height"`
	DataBytes              int64         `json:"data_bytes"`
	ReadScenario           string        `json:"read_scenario,omitempty"`
	ReadRequests           int           `json:"read_requests,omitempty"`
	ReadConcurrency        int           `json:"read_concurrency,omitempty"`
	ReadBatchSize          int           `json:"read_batch_size,omitempty"`
	ReadLatencySampleRate  int           `json:"read_latency_sample_rate,omitempty"`
	ReadLatencySamples     int           `json:"read_latency_samples,omitempty"`
	ReadHashCandidates     int           `json:"read_hash_candidates,omitempty"`
	ReadValidationSeconds  float64       `json:"read_validation_seconds,omitempty"`
	ReadValidationComplete bool          `json:"read_validation_complete"`
	ReadValidationError    string        `json:"read_validation_error,omitempty"`
	ReadStartDelaySeconds  float64       `json:"read_start_delay_seconds,omitempty"`
	ReadSeconds            float64       `json:"read_seconds,omitempty"`
	ReadRPS                float64       `json:"read_rps,omitempty"`
	ReadErrors             int64         `json:"read_errors,omitempty"`
	ReadFirstError         string        `json:"read_first_error,omitempty"`
	ReadP50Millis          float64       `json:"read_p50_ms,omitempty"`
	ReadP90Millis          float64       `json:"read_p90_ms,omitempty"`
	ReadP95Millis          float64       `json:"read_p95_ms,omitempty"`
	ReadP99Millis          float64       `json:"read_p99_ms,omitempty"`
	ReadP999Millis         float64       `json:"read_p999_ms,omitempty"`
	ReadMaxMillis          float64       `json:"read_max_ms,omitempty"`
	ReadHashCount          int           `json:"read_hash_count,omitempty"`
	TxPayloadBytes         int           `json:"tx_payload_bytes"`
	Keyspace               int           `json:"keyspace"`
	Concurrency            int           `json:"concurrency"`
	BlockMaxBytes          int64         `json:"block_max_bytes"`
	MempoolNotifyTxs       uint64        `json:"mempool_notify_txs"`
	MempoolTTL             time.Duration `json:"mempool_ttl"`
	ProposeTimeout         time.Duration `json:"propose_timeout"`
	ProposeDelta           time.Duration `json:"propose_delta"`
	VoteTimeout            time.Duration `json:"vote_timeout"`
	VoteDelta              time.Duration `json:"vote_delta"`
	CommitTimeout          time.Duration `json:"commit_timeout"`
	BypassCommitWait       bool          `json:"bypass_commit_wait"`
	TargetDuration         time.Duration `json:"target_duration"`
	TargetMaxTxs           int64         `json:"target_max_txs"`
	WaitTargetTxs          int64         `json:"wait_target_txs"`
	NodeStartSeconds       float64       `json:"node_start_seconds"`
	FirstHeightWaitSeconds float64       `json:"first_height_wait_seconds"`
}

type readResult struct {
	requests       int
	errors         int64
	firstError     string
	seconds        float64
	latencySamples int
	latency        []time.Duration
}

func main() {
	var opts options
	flag.StringVar(&opts.outDir, "out", "tm-e2e-ab", "output directory")
	flag.StringVar(&opts.backends, "backends", "goleveldb", "comma-separated DB backends")
	flag.DurationVar(&opts.duration, "duration", 30*time.Second, "tx production duration")
	flag.Int64Var(&opts.maxTxs, "max-txs", 0, "stop after this many accepted txs; 0 disables")
	flag.IntVar(&opts.concurrency, "concurrency", 64, "producer concurrency")
	flag.IntVar(&opts.payloadBytes, "payload-bytes", 1024, "target tx bytes")
	flag.IntVar(&opts.keyspace, "keyspace", 128, "number of application keys to update")
	flag.StringVar(&opts.indexer, "indexer", "kv", "tx indexer backend: kv or null")
	flag.Int64Var(&opts.blockMaxBytes, "block-max-bytes", 2*1024*1024, "consensus max block bytes")
	flag.IntVar(&opts.mempoolTxs, "mempool-txs", 250000, "mempool tx capacity")
	flag.Int64Var(&opts.mempoolBytes, "mempool-bytes", 4*1024*1024*1024, "mempool byte capacity")
	flag.Uint64Var(&opts.mempoolNotifyTxs, "mempool-notify-txs", 0, "minimum ready tx count for mempool reaping; useful for forced mega-block proposals")
	flag.DurationVar(&opts.mempoolTTL, "mempool-ttl", 0, "mempool TTL; 0 disables expiry for benchmark runs")
	flag.IntVar(&opts.readRequests, "read-requests", 0, "post-load read RPC request count; 0 disables")
	flag.IntVar(&opts.readConcurrency, "read-concurrency", 16, "post-load read RPC concurrency")
	flag.StringVar(&opts.readScenario, "read-scenario", "blockchain", "post-load read scenario: blockchain, block_results, commit, tx, tx_search_all")
	flag.IntVar(&opts.readBatchSize, "read-batch-size", 1, "post-load read scheduler batch size")
	flag.IntVar(&opts.readSampleRate, "read-latency-sample-rate", 1, "record one read latency per N requests; 0 disables latency recording")
	flag.BoolVar(&opts.readValidate, "read-validate-hashes", true, "pre-filter collected tx hashes to successful Tx(hash) lookups before timing")
	flag.DurationVar(&opts.readIndexWait, "read-index-wait-timeout", 30*time.Second, "maximum time to wait for collected tx hashes to become readable before timing")
	flag.DurationVar(&opts.readIndexPoll, "read-index-poll-interval", 250*time.Millisecond, "poll interval while waiting for collected tx hashes to become readable")
	flag.DurationVar(&opts.readStartDelay, "read-start-delay", 0, "extra idle delay after read-index readiness and before timed reads")
	flag.DurationVar(&opts.waitTimeout, "wait-timeout", 90*time.Second, "timeout waiting for accepted txs to commit")
	flag.Int64Var(&opts.waitTargetTxs, "wait-target-txs", 0, "if non-zero, wait for this many committed txs instead of all accepted txs")
	flag.DurationVar(&opts.settle, "settle", 2*time.Second, "extra wait after producer stops before counting")
	flag.BoolVar(&opts.disableEmpty, "disable-empty-blocks", true, "disable empty blocks")
	flag.BoolVar(&opts.bypassCommitWait, "bypass-commit-wait", true, "set genesis timeout.bypass_commit_timeout=true")
	flag.DurationVar(&opts.proposeTimeout, "propose-timeout", 20*time.Millisecond, "genesis timeout.propose and consensus unsafe propose-timeout override")
	flag.DurationVar(&opts.proposeDelta, "propose-delta", 10*time.Millisecond, "genesis timeout.propose_delta and consensus unsafe propose-timeout-delta override")
	flag.DurationVar(&opts.voteTimeout, "vote-timeout", 5*time.Millisecond, "genesis timeout.vote and consensus unsafe vote-timeout override")
	flag.DurationVar(&opts.voteDelta, "vote-delta", 5*time.Millisecond, "genesis timeout.vote_delta and consensus unsafe vote-timeout-delta override")
	flag.DurationVar(&opts.commitTimeout, "commit-timeout", 5*time.Millisecond, "genesis timeout.commit and consensus unsafe commit-timeout override")
	flag.BoolVar(&opts.failOnTimeout, "fail-on-timeout", false, "return an error if accepted txs do not fully commit by wait-timeout")
	flag.StringVar(&opts.cpuProfile, "cpuprofile", "", "write CPU profile to file")
	flag.StringVar(&opts.memProfile, "memprofile", "", "write heap profile to file after the run")
	flag.Parse()

	if opts.cpuProfile != "" {
		if err := os.MkdirAll(filepath.Dir(opts.cpuProfile), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "tm_e2e_bench: %v\n", err)
			os.Exit(1)
		}
		f, err := os.Create(opts.cpuProfile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tm_e2e_bench: %v\n", err)
			os.Exit(1)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			_ = f.Close()
			fmt.Fprintf(os.Stderr, "tm_e2e_bench: %v\n", err)
			os.Exit(1)
		}
		defer func() {
			pprof.StopCPUProfile()
			_ = f.Close()
		}()
	}

	err := run(context.Background(), opts)
	if opts.memProfile != "" {
		if profileErr := writeHeapProfile(opts.memProfile); profileErr != nil && err == nil {
			err = profileErr
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "tm_e2e_bench: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, opts options) error {
	if opts.concurrency < 1 {
		return fmt.Errorf("concurrency must be >= 1")
	}
	if opts.keyspace < 1 {
		return fmt.Errorf("keyspace must be >= 1")
	}
	if opts.payloadBytes < 16 {
		return fmt.Errorf("payload-bytes must be >= 16")
	}
	if opts.readConcurrency < 1 {
		return fmt.Errorf("read-concurrency must be >= 1")
	}
	if opts.readBatchSize < 1 {
		return fmt.Errorf("read-batch-size must be >= 1")
	}
	if opts.readSampleRate < 0 {
		return fmt.Errorf("read-latency-sample-rate must be >= 0")
	}
	if opts.readIndexWait < 0 {
		return fmt.Errorf("read-index-wait-timeout must be >= 0")
	}
	if opts.readIndexPoll <= 0 {
		return fmt.Errorf("read-index-poll-interval must be > 0")
	}
	if opts.readStartDelay < 0 {
		return fmt.Errorf("read-start-delay must be >= 0")
	}
	if opts.indexer != "kv" && opts.indexer != "null" {
		return fmt.Errorf("indexer must be kv or null")
	}
	if opts.waitTargetTxs < 0 {
		return fmt.Errorf("wait-target-txs must be >= 0")
	}
	outDir, err := safeOutputDir(opts.outDir)
	if err != nil {
		return err
	}
	opts.outDir = outDir
	if err := os.RemoveAll(opts.outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(opts.outDir, 0755); err != nil {
		return err
	}

	backends := strings.Split(opts.backends, ",")
	var results []benchResult
	for _, backend := range backends {
		backend = strings.TrimSpace(backend)
		if backend == "" {
			continue
		}
		result, err := runBackend(ctx, opts, backend)
		if err != nil {
			return fmt.Errorf("%s: %w", backend, err)
		}
		results = append(results, result)
		if err := writeJSON(filepath.Join(opts.outDir, backend, "result.json"), result); err != nil {
			return err
		}
	}

	if err := writeJSON(filepath.Join(opts.outDir, "results.json"), results); err != nil {
		return err
	}
	if err := writeSummary(filepath.Join(opts.outDir, "summary.txt"), results); err != nil {
		return err
	}
	return nil
}

func safeOutputDir(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("output directory must not be empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(abs)

	if clean == string(filepath.Separator) {
		return "", fmt.Errorf("refusing unsafe output directory %q", path)
	}
	if cwd, err := os.Getwd(); err == nil && sameOrParent(clean, filepath.Clean(cwd)) {
		return "", fmt.Errorf("refusing unsafe output directory %q", path)
	}
	if home, err := os.UserHomeDir(); err == nil {
		if clean == filepath.Clean(home) {
			return "", fmt.Errorf("refusing unsafe output directory %q", path)
		}
	}
	return clean, nil
}

func sameOrParent(parent string, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func runBackend(ctx context.Context, opts options, backend string) (benchResult, error) {
	home := filepath.Join(opts.outDir, backend, "home")
	if err := os.MkdirAll(filepath.Join(home, "config"), 0755); err != nil {
		return benchResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(home, "data", "app"), 0755); err != nil {
		return benchResult{}, err
	}

	valKey := ed25519.GenerateSecretKey()
	pubKey := valKey.Public()
	if err := tmtypes.GenNodeKey().SaveAs(filepath.Join(home, "config", "node_key.json")); err != nil {
		return benchResult{}, err
	}
	filePV := privval.NewFilePV(
		valKey,
		filepath.Join(home, "config", "priv_validator_key.json"),
		filepath.Join(home, "data", "priv_validator_state.json"),
	)
	if err := filePV.Save(); err != nil {
		return benchResult{}, err
	}

	genesis := tmtypes.GenesisDoc{
		GenesisTime:   time.Now(),
		ChainID:       fmt.Sprintf("tm-e2e-%s", backend),
		InitialHeight: 1,
		ConsensusParams: &tmtypes.ConsensusParams{
			Block: tmtypes.BlockParams{
				MaxBytes:      opts.blockMaxBytes,
				MaxGas:        -1,
				MaxGasWanted:  -1,
				MinTxsInBlock: 1,
			},
			Evidence:  tmtypes.DefaultEvidenceParams(),
			Validator: tmtypes.DefaultValidatorParams(),
			Version:   tmtypes.DefaultVersionParams(),
			Synchrony: tmtypes.DefaultSynchronyParams(),
			Timeout: tmtypes.TimeoutParams{
				Propose:             opts.proposeTimeout,
				ProposeDelta:        opts.proposeDelta,
				Vote:                opts.voteTimeout,
				VoteDelta:           opts.voteDelta,
				Commit:              opts.commitTimeout,
				BypassCommitTimeout: opts.bypassCommitWait,
			},
			ABCI: tmtypes.DefaultABCIParams(),
		},
		Validators: []tmtypes.GenesisValidator{
			{
				Name:    "validator01",
				Address: pubKey.Address(),
				PubKey:  pubKey,
				Power:   10,
			},
		},
	}
	if err := genesis.ValidateAndComplete(); err != nil {
		return benchResult{}, err
	}
	if err := genesis.SaveAs(filepath.Join(home, "config", "genesis.json")); err != nil {
		return benchResult{}, err
	}

	appCfg := e2eapp.DefaultConfig(filepath.Join(home, "data", "app"))
	appCfg.PersistInterval = 0
	appCfg.SnapshotInterval = 0
	appCfg.ValidatorUpdates = map[string]map[string]uint8{
		"0": {
			base64.StdEncoding.EncodeToString(pubKey.Bytes()): 10,
		},
	}
	app, err := e2eapp.NewApplication(appCfg)
	if err != nil {
		return benchResult{}, err
	}

	cfg := tmconfig.DefaultConfig()
	rpcPort, err := freeTCPPort()
	if err != nil {
		return benchResult{}, err
	}
	p2pPort, err := freeTCPPort()
	if err != nil {
		return benchResult{}, err
	}
	cfg.SetRoot(home)
	cfg.Moniker = fmt.Sprintf("tm-e2e-%s", backend)
	cfg.Mode = tmconfig.ModeValidator
	cfg.DBBackend = backend
	cfg.DBPath = "data"
	cfg.LogLevel = "error"
	cfg.RPC.ListenAddress = fmt.Sprintf("tcp://127.0.0.1:%d", rpcPort)
	cfg.RPC.PprofListenAddress = ""
	cfg.P2P.ListenAddress = fmt.Sprintf("tcp://127.0.0.1:%d", p2pPort)
	cfg.P2P.ExternalAddress = ""
	cfg.P2P.BootstrapPeers = ""
	cfg.P2P.PersistentPeers = ""
	cfg.P2P.PexReactor = false
	cfg.Mempool.Broadcast = false
	cfg.Mempool.Size = opts.mempoolTxs
	cfg.Mempool.PendingSize = opts.mempoolTxs
	cfg.Mempool.MaxTxsBytes = opts.mempoolBytes
	cfg.Mempool.MaxPendingTxsBytes = opts.mempoolBytes
	cfg.Mempool.MaxTxBytes = max(opts.payloadBytes*2, 1024*1024)
	cfg.Mempool.CacheSize = opts.mempoolTxs * 2
	cfg.Mempool.DuplicateTxsCacheSize = opts.mempoolTxs * 2
	cfg.Mempool.TxNotifyThreshold = opts.mempoolNotifyTxs
	cfg.Mempool.TTLDuration = opts.mempoolTTL
	cfg.Mempool.TTLNumBlocks = 0
	cfg.Mempool.RemoveExpiredTxsFromQueue = opts.mempoolTTL > 0
	cfg.Consensus.CreateEmptyBlocks = !opts.disableEmpty
	cfg.Consensus.CreateEmptyBlocksInterval = 0
	cfg.Consensus.PeerGossipSleepDuration = 5 * time.Millisecond
	cfg.Consensus.PeerQueryMaj23SleepDuration = 250 * time.Millisecond
	cfg.Consensus.UnsafeOverridesEnabled = true
	cfg.Consensus.UnsafeProposeTimeoutOverride = opts.proposeTimeout
	cfg.Consensus.UnsafeProposeTimeoutDeltaOverride = opts.proposeDelta
	cfg.Consensus.UnsafeVoteTimeoutOverride = opts.voteTimeout
	cfg.Consensus.UnsafeVoteTimeoutDeltaOverride = opts.voteDelta
	cfg.Consensus.UnsafeCommitTimeoutOverride = opts.commitTimeout
	cfg.Consensus.UnsafeBypassCommitTimeoutOverride = &opts.bypassCommitWait
	cfg.TxIndex.Indexer = []string{opts.indexer}
	cfg.Instrumentation.Prometheus = false
	cfg.PrivValidator.Key = "config/priv_validator_key.json"
	cfg.PrivValidator.State = "data/priv_validator_state.json"
	cfg.PrivValidator.ListenAddr = ""

	if err := cfg.ValidateBasic(); err != nil {
		return benchResult{}, err
	}
	if err := tmconfig.WriteConfigFile(home, cfg); err != nil {
		return benchResult{}, err
	}

	nodeStart := time.Now()
	svc, err := node.New(ctx, cfg, func() {}, app, &genesis, nil, node.NoOpMetricsProvider(), tmtypes.DefaultConsensusPolicy())
	if err != nil {
		return benchResult{}, err
	}
	if err := svc.Start(ctx); err != nil {
		return benchResult{}, err
	}
	defer svc.Stop()

	client, err := tmlocal.New(svc)
	if err != nil {
		return benchResult{}, err
	}
	firstHeightStart := time.Now()
	if err := waitForHeight(ctx, client, 1, 30*time.Second); err != nil {
		return benchResult{}, err
	}
	startStatus, err := client.Status(ctx)
	if err != nil {
		return benchResult{}, err
	}
	startHeight := startStatus.SyncInfo.LatestBlockHeight

	loadStart := time.Now()
	attempts, accepted, errorsCount := produceTxs(ctx, client, opts)
	loadElapsed := time.Since(loadStart)
	if opts.settle > 0 {
		time.Sleep(opts.settle)
	}

	waitTarget := accepted
	if opts.waitTargetTxs > 0 {
		waitTarget = opts.waitTargetTxs
	}
	waitStart := time.Now()
	waitErr := waitForCommitted(ctx, client, startHeight+1, waitTarget, opts.waitTimeout)
	if waitErr != nil && opts.failOnTimeout {
		return benchResult{}, waitErr
	}
	waitElapsed := time.Since(waitStart)

	endStatus, err := client.Status(ctx)
	if err != nil {
		return benchResult{}, err
	}
	endHeight := endStatus.SyncInfo.LatestBlockHeight
	blockStats, err := collectBlockStats(ctx, client, startHeight+1, endHeight)
	if err != nil {
		return benchResult{}, err
	}

	var rr readResult
	var readHashes []tmbytes.HexBytes
	var readHashCandidates int
	var readValidation readableTxHashesResult
	if opts.readRequests > 0 {
		if opts.readScenario == "tx" {
			readHashes, err = collectTxHashes(ctx, client, startHeight+1, endHeight, opts.readRequests)
			if err != nil {
				return benchResult{}, err
			}
			readHashCandidates = len(readHashes)
			if opts.readValidate {
				readValidation, err = waitForReadableTxHashes(ctx, client, readHashes, opts.readIndexWait, opts.readIndexPoll)
				if err != nil {
					return benchResult{}, err
				}
				readHashes = readValidation.hashes
			}
			if len(readHashes) == 0 {
				return benchResult{}, fmt.Errorf("no committed tx hashes available for tx read phase after %.3fs: %s",
					readValidation.seconds, readValidation.firstError)
			}
		}
		if opts.readStartDelay > 0 {
			time.Sleep(opts.readStartDelay)
		}
		rr = runReadPhase(ctx, client, opts, startHeight+1, endHeight, readHashes)
	}

	dataBytes, err := dirSize(filepath.Join(home, "data"))
	if err != nil {
		return benchResult{}, err
	}
	result := benchResult{
		Backend:                backend,
		Home:                   home,
		DBBackend:              backend,
		Indexer:                opts.indexer,
		DurationSeconds:        time.Since(loadStart).Seconds(),
		LoadSeconds:            loadElapsed.Seconds(),
		WaitSeconds:            waitElapsed.Seconds(),
		DrainSeconds:           (loadElapsed + waitElapsed).Seconds(),
		ProducerAttempts:       attempts,
		ProducerAccepted:       accepted,
		ProducerErrors:         errorsCount,
		TimedOut:               waitErr != nil,
		StartHeight:            startHeight,
		EndHeight:              endHeight,
		CommittedTxs:           blockStats.committedTxs,
		NonEmptyBlocks:         blockStats.nonEmptyBlocks,
		BlockCount:             blockStats.blockCount,
		AvgCommitTPS:           float64(blockStats.committedTxs) / math.Max(loadElapsed.Seconds(), 0.001),
		DrainTPS:               float64(blockStats.committedTxs) / math.Max((loadElapsed+waitElapsed).Seconds(), 0.001),
		AvgBlockTxs:            blockStats.avgBlockTxs,
		MaxBlockTxs:            blockStats.maxBlockTxs,
		P50BlockTxs:            blockStats.p50BlockTxs,
		P95BlockTxs:            blockStats.p95BlockTxs,
		TotalBlockBytes:        blockStats.totalBlockBytes,
		AvgBlockBytes:          blockStats.avgBlockBytes,
		MaxBlockBytes:          blockStats.maxBlockBytes,
		P50BlockBytes:          blockStats.p50BlockBytes,
		P95BlockBytes:          blockStats.p95BlockBytes,
		FinalHeight:            endStatus.SyncInfo.LatestBlockHeight,
		DataBytes:              dataBytes,
		ReadScenario:           opts.readScenario,
		ReadRequests:           rr.requests,
		ReadConcurrency:        opts.readConcurrency,
		ReadBatchSize:          opts.readBatchSize,
		ReadLatencySampleRate:  opts.readSampleRate,
		ReadLatencySamples:     rr.latencySamples,
		ReadHashCandidates:     readHashCandidates,
		ReadValidationSeconds:  readValidation.seconds,
		ReadValidationComplete: readValidation.complete,
		ReadValidationError:    readValidation.firstError,
		ReadStartDelaySeconds:  opts.readStartDelay.Seconds(),
		ReadSeconds:            rr.seconds,
		ReadRPS:                float64(rr.requests) / math.Max(rr.seconds, 0.001),
		ReadErrors:             rr.errors,
		ReadFirstError:         rr.firstError,
		ReadHashCount:          len(readHashes),
		TxPayloadBytes:         opts.payloadBytes,
		Keyspace:               opts.keyspace,
		Concurrency:            opts.concurrency,
		BlockMaxBytes:          opts.blockMaxBytes,
		MempoolNotifyTxs:       opts.mempoolNotifyTxs,
		MempoolTTL:             opts.mempoolTTL,
		ProposeTimeout:         opts.proposeTimeout,
		ProposeDelta:           opts.proposeDelta,
		VoteTimeout:            opts.voteTimeout,
		VoteDelta:              opts.voteDelta,
		CommitTimeout:          opts.commitTimeout,
		BypassCommitWait:       opts.bypassCommitWait,
		TargetDuration:         opts.duration,
		TargetMaxTxs:           opts.maxTxs,
		WaitTargetTxs:          waitTarget,
		NodeStartSeconds:       time.Since(nodeStart).Seconds(),
		FirstHeightWaitSeconds: time.Since(firstHeightStart).Seconds(),
	}
	if waitErr != nil {
		result.WaitError = waitErr.Error()
	}
	if len(rr.latency) > 0 {
		sort.Slice(rr.latency, func(i, j int) bool { return rr.latency[i] < rr.latency[j] })
		result.ReadP50Millis = percentileMillis(rr.latency, 50)
		result.ReadP90Millis = percentileMillis(rr.latency, 90)
		result.ReadP95Millis = percentileMillis(rr.latency, 95)
		result.ReadP99Millis = percentileMillis(rr.latency, 99)
		result.ReadP999Millis = percentileMillisFloat(rr.latency, 99.9)
		result.ReadMaxMillis = durationMillis(rr.latency[len(rr.latency)-1])
	}
	return result, nil
}

type blockStats struct {
	committedTxs    int64
	nonEmptyBlocks  int64
	blockCount      int64
	avgBlockTxs     float64
	maxBlockTxs     int
	p50BlockTxs     int
	p95BlockTxs     int
	totalBlockBytes int64
	avgBlockBytes   float64
	maxBlockBytes   int
	p50BlockBytes   int
	p95BlockBytes   int
}

func produceTxs(ctx context.Context, client *tmlocal.Local, opts options) (int64, int64, int64) {
	ctx, cancel := context.WithTimeout(ctx, opts.duration)
	defer cancel()

	var nextID atomic.Int64
	var reserved atomic.Int64
	var attempts atomic.Int64
	var accepted atomic.Int64
	var errorsCount atomic.Int64
	var wg sync.WaitGroup
	for worker := range opts.concurrency {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if opts.maxTxs > 0 {
					if reserved.Add(1) > opts.maxTxs {
						reserved.Add(-1)
						return
					}
				}
				id := nextID.Add(1)
				tx := makeTx(id, worker, opts)
				attempts.Add(1)
				res, err := client.BroadcastTxSync(ctx, tx)
				if err != nil {
					errorsCount.Add(1)
					if opts.maxTxs > 0 {
						reserved.Add(-1)
					}
					continue
				}
				if res.Code != 0 {
					errorsCount.Add(1)
					if opts.maxTxs > 0 {
						reserved.Add(-1)
					}
					continue
				}
				accepted.Add(1)
			}
		}(worker)
	}
	wg.Wait()
	return attempts.Load(), accepted.Load(), errorsCount.Load()
}

func makeTx(id int64, worker int, opts options) tmtypes.Tx {
	key := fmt.Sprintf("k%06d", id%int64(opts.keyspace))
	prefix := fmt.Sprintf("%s=%012d-%03d-", key, id, worker)
	if len(prefix) >= opts.payloadBytes {
		return tmtypes.Tx(prefix)
	}
	var b strings.Builder
	b.Grow(opts.payloadBytes)
	b.WriteString(prefix)
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	for b.Len() < opts.payloadBytes {
		b.WriteByte(alphabet[int(id+int64(b.Len()))%len(alphabet)])
	}
	return tmtypes.Tx(b.String())
}

func waitForHeight(ctx context.Context, client *tmlocal.Local, height int64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := client.Status(ctx)
		if err == nil && status.SyncInfo.LatestBlockHeight >= height {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for height %d", height)
}

func waitForCommitted(ctx context.Context, client *tmlocal.Local, startHeight, targetTxs int64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	nextHeight := startHeight
	var committed int64
	for time.Now().Before(deadline) {
		status, err := client.Status(ctx)
		if err == nil && status.SyncInfo.LatestBlockHeight >= nextHeight {
			n, err := collectCommittedTxs(ctx, client, nextHeight, status.SyncInfo.LatestBlockHeight)
			if err == nil {
				committed += n
				nextHeight = status.SyncInfo.LatestBlockHeight + 1
			}
			if committed >= targetTxs {
				return nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	status, err := client.Status(ctx)
	if err != nil {
		return fmt.Errorf("timed out waiting for %d txs and status failed: %w", targetTxs, err)
	}
	stats, err := collectBlockStats(ctx, client, startHeight, status.SyncInfo.LatestBlockHeight)
	if err != nil {
		return fmt.Errorf("timed out waiting for %d txs and stats failed: %w", targetTxs, err)
	}
	return fmt.Errorf("timed out waiting for %d txs, saw %d at height %d", targetTxs, stats.committedTxs, status.SyncInfo.LatestBlockHeight)
}

func collectCommittedTxs(ctx context.Context, client *tmlocal.Local, minHeight, maxHeight int64) (int64, error) {
	if maxHeight < minHeight {
		return 0, nil
	}
	var total int64
	for lo := minHeight; lo <= maxHeight; lo += 20 {
		hi := min(lo+19, maxHeight)
		info, err := client.BlockchainInfo(ctx, lo, hi)
		if err != nil {
			return 0, err
		}
		for _, meta := range info.BlockMetas {
			total += int64(meta.NumTxs)
		}
	}
	return total, nil
}

func collectBlockStats(ctx context.Context, client *tmlocal.Local, minHeight, maxHeight int64) (blockStats, error) {
	if maxHeight < minHeight {
		return blockStats{}, nil
	}
	var txsPerBlock []int
	var bytesPerBlock []int
	var total int64
	var totalBytes int64
	var nonEmpty int64
	for lo := minHeight; lo <= maxHeight; lo += 20 {
		hi := min(lo+19, maxHeight)
		info, err := client.BlockchainInfo(ctx, lo, hi)
		if err != nil {
			return blockStats{}, err
		}
		for _, meta := range info.BlockMetas {
			n := meta.NumTxs
			txsPerBlock = append(txsPerBlock, n)
			bytesPerBlock = append(bytesPerBlock, meta.BlockSize)
			total += int64(n)
			totalBytes += int64(meta.BlockSize)
			if n > 0 {
				nonEmpty++
			}
		}
	}
	sort.Ints(txsPerBlock)
	sort.Ints(bytesPerBlock)
	stats := blockStats{
		committedTxs:    total,
		nonEmptyBlocks:  nonEmpty,
		blockCount:      int64(len(txsPerBlock)),
		totalBlockBytes: totalBytes,
	}
	if len(txsPerBlock) > 0 {
		stats.avgBlockTxs = float64(total) / float64(len(txsPerBlock))
		stats.maxBlockTxs = txsPerBlock[len(txsPerBlock)-1]
		stats.p50BlockTxs = percentileInt(txsPerBlock, 50)
		stats.p95BlockTxs = percentileInt(txsPerBlock, 95)
		stats.avgBlockBytes = float64(totalBytes) / float64(len(bytesPerBlock))
		stats.maxBlockBytes = bytesPerBlock[len(bytesPerBlock)-1]
		stats.p50BlockBytes = percentileInt(bytesPerBlock, 50)
		stats.p95BlockBytes = percentileInt(bytesPerBlock, 95)
	}
	return stats, nil
}

func collectTxHashes(ctx context.Context, client *tmlocal.Local, minHeight, maxHeight int64, limit int) ([]tmbytes.HexBytes, error) {
	if maxHeight < minHeight || limit <= 0 {
		return nil, nil
	}
	hashes := make([]tmbytes.HexBytes, 0, limit)
	for height := minHeight; height <= maxHeight && len(hashes) < limit; height++ {
		h := height
		block, err := client.Block(ctx, &h)
		if err != nil {
			return nil, err
		}
		if block.Block == nil {
			continue
		}
		for _, tx := range block.Block.Txs {
			hashes = append(hashes, tx.Hash().Bytes())
			if len(hashes) >= limit {
				break
			}
		}
	}
	return hashes, nil
}

func filterReadableTxHashes(ctx context.Context, client *tmlocal.Local, hashes []tmbytes.HexBytes) ([]tmbytes.HexBytes, error) {
	readable, _, err := filterReadableTxHashesWithError(ctx, client, hashes)
	return readable, err
}

type readableTxHashesResult struct {
	hashes     []tmbytes.HexBytes
	seconds    float64
	complete   bool
	firstError string
}

func waitForReadableTxHashes(
	ctx context.Context,
	client *tmlocal.Local,
	hashes []tmbytes.HexBytes,
	timeout time.Duration,
	pollInterval time.Duration,
) (readableTxHashesResult, error) {
	start := time.Now()
	deadline := start.Add(timeout)
	var result readableTxHashesResult

	for {
		readable, firstError, err := filterReadableTxHashesWithError(ctx, client, hashes)
		result = readableTxHashesResult{
			hashes:     readable,
			seconds:    time.Since(start).Seconds(),
			complete:   len(readable) == len(hashes),
			firstError: firstError,
		}
		if err != nil {
			return result, err
		}
		if result.complete || timeout == 0 || time.Now().After(deadline) {
			return result, nil
		}

		wait := min(pollInterval, time.Until(deadline))
		if wait <= 0 {
			return result, nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return result, ctx.Err()
		case <-timer.C:
		}
	}
}

func filterReadableTxHashesWithError(
	ctx context.Context,
	client *tmlocal.Local,
	hashes []tmbytes.HexBytes,
) ([]tmbytes.HexBytes, string, error) {
	if len(hashes) == 0 {
		return hashes, "", nil
	}
	readable := make([]tmbytes.HexBytes, 0, len(hashes))
	var firstError string
	for _, hash := range hashes {
		if _, err := client.Tx(ctx, hash, false); err == nil {
			readable = append(readable, hash)
		} else if firstError == "" {
			firstError = err.Error()
		}
		if err := ctx.Err(); err != nil {
			return readable, firstError, err
		}
	}
	return readable, firstError, nil
}

func runReadPhase(ctx context.Context, client *tmlocal.Local, opts options, startHeight, endHeight int64, hashes []tmbytes.HexBytes) readResult {
	if endHeight < startHeight || opts.readRequests <= 0 {
		return readResult{}
	}
	var next atomic.Int64
	var errorsCount atomic.Int64
	var firstErr atomic.Value
	firstErr.Store("")
	var firstErrOnce sync.Once
	var latencies []time.Duration
	if opts.readSampleRate > 0 {
		latencies = make([]time.Duration, (opts.readRequests+opts.readSampleRate-1)/opts.readSampleRate)
	}
	readBatchSize := opts.readBatchSize
	start := time.Now()
	var wg sync.WaitGroup
	for range opts.readConcurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				batchStart := int(next.Add(int64(readBatchSize))) - readBatchSize
				if batchStart >= opts.readRequests {
					return
				}
				batchEnd := min(batchStart+readBatchSize, opts.readRequests)
				for i := batchStart; i < batchEnd; i++ {
					height := startHeight + int64(i)%max(endHeight-startHeight+1, 1)
					if opts.readSampleRate > 0 && i%opts.readSampleRate == 0 {
						t0 := time.Now()
						if err := readOnce(ctx, client, opts.readScenario, height, hashes, i); err != nil {
							errorsCount.Add(1)
							firstErrOnce.Do(func() { firstErr.Store(err.Error()) })
						}
						latencies[i/opts.readSampleRate] = time.Since(t0)
						continue
					}
					if err := readOnce(ctx, client, opts.readScenario, height, hashes, i); err != nil {
						errorsCount.Add(1)
						firstErrOnce.Do(func() { firstErr.Store(err.Error()) })
					}
				}
			}
		}()
	}
	wg.Wait()
	firstError, _ := firstErr.Load().(string)
	return readResult{
		requests:       opts.readRequests,
		errors:         errorsCount.Load(),
		firstError:     firstError,
		seconds:        time.Since(start).Seconds(),
		latencySamples: len(latencies),
		latency:        latencies,
	}
}

func readOnce(ctx context.Context, client *tmlocal.Local, scenario string, height int64, hashes []tmbytes.HexBytes, requestIndex int) error {
	switch scenario {
	case "blockchain":
		_, err := client.BlockchainInfo(ctx, max(height-19, 1), height)
		return err
	case "block_results":
		_, err := client.BlockResults(ctx, &height)
		return err
	case "commit":
		_, err := client.Commit(ctx, &height)
		return err
	case "tx":
		if len(hashes) == 0 {
			return fmt.Errorf("tx read scenario has no hashes")
		}
		idx := int((uint64(requestIndex) * 11400714819323198485) % uint64(len(hashes)))
		_, err := client.Tx(ctx, hashes[idx], false)
		return err
	case "tx_search_all":
		page := 1
		perPage := 100
		_, err := client.TxSearch(ctx, "tx.height > 0", false, &page, &perPage, "desc")
		return err
	default:
		return fmt.Errorf("unknown read scenario %q", scenario)
	}
}

func dirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func freeTCPPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address %T", l.Addr())
	}
	return addr.Port, nil
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

func writeHeapProfile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	runtime.GC()
	return pprof.WriteHeapProfile(f)
}

func writeSummary(path string, results []benchResult) error {
	var b strings.Builder
	b.WriteString("tm_e2e_bench summary\n")
	for _, r := range results {
		fmt.Fprintf(&b, "%s committed_txs=%d wait_target_txs=%d accepted=%d errors=%d timed_out=%t load_s=%.3f wait_s=%.3f drain_s=%.3f drain_tps=%.2f avg_accept_tps=%.2f non_empty_blocks=%d max_block_txs=%d max_block_bytes=%d data_bytes=%d read_rps=%.2f read_errors=%d read_first_error=%q read_hash_candidates=%d read_hash_count=%d read_validation_s=%.3f read_validation_complete=%t read_start_delay_s=%.3f read_p50_ms=%.3f read_p90_ms=%.3f read_p95_ms=%.3f read_p99_ms=%.3f read_p999_ms=%.3f read_max_ms=%.3f read_batch_size=%d read_latency_sample_rate=%d read_latency_samples=%d\n",
			r.Backend, r.CommittedTxs, r.WaitTargetTxs, r.ProducerAccepted, r.ProducerErrors, r.TimedOut, r.LoadSeconds, r.WaitSeconds, r.DrainSeconds, r.DrainTPS, r.AvgCommitTPS, r.NonEmptyBlocks, r.MaxBlockTxs, r.MaxBlockBytes, r.DataBytes, r.ReadRPS, r.ReadErrors, r.ReadFirstError, r.ReadHashCandidates, r.ReadHashCount, r.ReadValidationSeconds, r.ReadValidationComplete, r.ReadStartDelaySeconds, r.ReadP50Millis, r.ReadP90Millis, r.ReadP95Millis, r.ReadP99Millis, r.ReadP999Millis, r.ReadMaxMillis, r.ReadBatchSize, r.ReadLatencySampleRate, r.ReadLatencySamples)
	}
	if len(results) >= 2 {
		base := results[0]
		for _, r := range results[1:] {
			fmt.Fprintf(&b, "%s_vs_%s committed_ratio=%.4f accepted_ratio=%.4f drain_tps_ratio=%.4f accept_tps_ratio=%.4f max_block_bytes_ratio=%.4f data_bytes_ratio=%.4f read_rps_ratio=%.4f read_p95_ratio=%.4f read_p999_ratio=%.4f read_max_ratio=%.4f\n",
				r.Backend, base.Backend,
				ratio(float64(r.CommittedTxs), float64(base.CommittedTxs)),
				ratio(float64(r.ProducerAccepted), float64(base.ProducerAccepted)),
				ratio(r.DrainTPS, base.DrainTPS),
				ratio(r.AvgCommitTPS, base.AvgCommitTPS),
				ratio(float64(r.MaxBlockBytes), float64(base.MaxBlockBytes)),
				ratio(float64(r.DataBytes), float64(base.DataBytes)),
				ratio(r.ReadRPS, base.ReadRPS),
				ratio(r.ReadP95Millis, base.ReadP95Millis),
				ratio(r.ReadP999Millis, base.ReadP999Millis),
				ratio(r.ReadMaxMillis, base.ReadMaxMillis))
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func percentileInt(v []int, p int) int {
	if len(v) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(p)/100*float64(len(v)))) - 1
	idx = max(0, min(idx, len(v)-1))
	return v[idx]
}

func percentileMillis(v []time.Duration, p int) float64 {
	return percentileMillisFloat(v, float64(p))
}

func percentileMillisFloat(v []time.Duration, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100*float64(len(v)))) - 1
	idx = max(0, min(idx, len(v)-1))
	return durationMillis(v[idx])
}

func durationMillis(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}

func ratio(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
