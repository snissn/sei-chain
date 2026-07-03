package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBenchResultPhaseTimingsJSON(t *testing.T) {
	result := benchResult{
		Backend: "goleveldb",
		PhaseTimings: phaseTimings{
			BackendSeconds:           12.5,
			NodeStartSeconds:         0.75,
			FirstHeightWaitSeconds:   0.25,
			LoadSeconds:              3.0,
			SettleSeconds:            2.0,
			WaitForCommitSeconds:     4.0,
			CollectBlockStatsSeconds: 0.1,
			TxIndexReadinessSeconds:  1.25,
			ReadSeconds:              2.5,
			DirSizeSeconds:           0.2,
		},
		ProfileArtifacts: &profileArtifacts{
			Scope:       "backend",
			CPUProfile:  "/tmp/goleveldb.cpu.pprof",
			HeapProfile: "/tmp/goleveldb.heap.pprof",
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["phase_timings"].(map[string]any); !ok {
		t.Fatalf("phase_timings missing or not an object: %#v", decoded["phase_timings"])
	}
	if _, ok := decoded["profile_artifacts"].(map[string]any); !ok {
		t.Fatalf("profile_artifacts missing or not an object: %#v", decoded["profile_artifacts"])
	}
}

func TestWriteSummaryIncludesPhaseAndProfileEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summary.txt")
	result := benchResult{
		Backend:      "treedb",
		CommittedTxs: 100,
		DrainTPS:     50,
		PhaseTimings: phaseTimings{
			BackendSeconds:           9,
			CollectBlockStatsSeconds: 0.5,
			TxIndexReadinessSeconds:  1.5,
			ReadSeconds:              2.5,
		},
		ProfileArtifacts: &profileArtifacts{
			Scope:       "backend",
			CPUProfile:  "/profiles/treedb.cpu.pprof",
			HeapProfile: "/profiles/treedb.heap.pprof",
		},
	}

	if err := writeSummary(path, []benchResult{result}); err != nil {
		t.Fatal(err)
	}
	summary := readFileString(t, path)
	for _, want := range []string{
		"phase_backend_s=9.000",
		"phase_collect_block_stats_s=0.500",
		"phase_tx_index_readiness_s=1.500",
		"profile_scope=\"backend\"",
		"profile_cpu=\"/profiles/treedb.cpu.pprof\"",
		"profile_heap=\"/profiles/treedb.heap.pprof\"",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestBackendProfileArtifacts(t *testing.T) {
	dir := t.TempDir()
	artifacts := backendProfileArtifacts(dir, "goleveldb")
	if artifacts.Scope != "backend" {
		t.Fatalf("unexpected scope %q", artifacts.Scope)
	}
	if artifacts.CPUProfile != filepath.Join(dir, "goleveldb.cpu.pprof") {
		t.Fatalf("unexpected CPU profile path %q", artifacts.CPUProfile)
	}
	if artifacts.HeapProfile != filepath.Join(dir, "goleveldb.heap.pprof") {
		t.Fatalf("unexpected heap profile path %q", artifacts.HeapProfile)
	}
	if !strings.Contains(artifacts.DiffBaseHint, "diff_base") {
		t.Fatalf("diff base hint missing: %q", artifacts.DiffBaseHint)
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
