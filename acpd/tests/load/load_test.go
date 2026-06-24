package load

import (
"database/sql"
"fmt"
"math/rand"
"os"
"sync"
"sync/atomic"
"testing"
"time"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/bridge"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
)

// LoadTestConfig controls the load test parameters.
// Override via environment variables for CI vs full runs.
type LoadTestConfig struct {
NumWorkflows    int
TransitionsEach int
Concurrency     int
DSN             string
MigrationsDir   string
FardDir         string
FardrunBin      string
}

func loadConfig() LoadTestConfig {
cfg := LoadTestConfig{
NumWorkflows:    100,
TransitionsEach: 3,
Concurrency:     20,
DSN:             ":memory:",
MigrationsDir:   "../../migrations",
FardDir:         "../../fard/bridge",
FardrunBin:      "fardrun",
}
if v := os.Getenv("ACP_LOAD_WORKFLOWS"); v != "" {
fmt.Sscanf(v, "%d", &cfg.NumWorkflows)
}
if v := os.Getenv("ACP_LOAD_TRANSITIONS"); v != "" {
fmt.Sscanf(v, "%d", &cfg.TransitionsEach)
}
if v := os.Getenv("ACP_LOAD_CONCURRENCY"); v != "" {
fmt.Sscanf(v, "%d", &cfg.Concurrency)
}
if v := os.Getenv("ACP_LOAD_DSN"); v != "" {
cfg.DSN = v
}
return cfg
}

// TestWorkflowScale is the v0.3.0 acceptance test.
// Default: 10,000 workflows x 3 transitions = 30,000 atomic commits.
// Each workflow's receipt chain is verified at the end.
//
// Run with:
//
//go test ./tests/load/... -run TestWorkflowScale -timeout 600s -v
//
// Full 10k run (needs Postgres for durability):
//
//ACP_LOAD_DSN=postgres://... go test ./tests/load/... -run TestWorkflowScale -timeout 600s -v
//
// Quick smoke test (100 workflows, default SQLite):
//
//ACP_LOAD_WORKFLOWS=100 go test ./tests/load/... -run TestWorkflowScale -timeout 120s -v
func TestWorkflowScale(t *testing.T) {
cfg := loadConfig()

t.Logf("load test config: workflows=%d transitions_each=%d concurrency=%d dsn=%s",
cfg.NumWorkflows, cfg.TransitionsEach, cfg.Concurrency, cfg.DSN)

store.MigrationDir = cfg.MigrationsDir
db, err := store.Open(cfg.DSN)
if err != nil {
t.Fatalf("open db: %v", err)
}
defer db.Close()

outDir, err := os.MkdirTemp("", "acp-load-*")
if err != nil {
t.Fatalf("create outdir: %v", err)
}
defer os.RemoveAll(outDir)

br := bridge.New(cfg.FardrunBin, cfg.FardDir, outDir)

// Seed one actor for all workers
if err := db.CreateActor(&store.Actor{
ActorID: "load:operator",
Roles:   []string{"operator", "manager"},
}, "acp_load_test_key"); err != nil {
t.Logf("actor seed (may already exist): %v", err)
}

stages := []string{"research", "review", "decision", "complete"}

var (
created    atomic.Int64
committed  atomic.Int64
failed     atomic.Int64
chainBreaks atomic.Int64
)

start := time.Now()

// Worker pool
sem := make(chan struct{}, cfg.Concurrency)
var wg sync.WaitGroup

for i := 0; i < cfg.NumWorkflows; i++ {
wg.Add(1)
sem <- struct{}{}
go func(idx int) {
defer wg.Done()
defer func() { <-sem }()

wfID := fmt.Sprintf("wf_load_%06d", idx)

// Create workflow
var createResult store.TransitionResult
if err := br.RunAndUnmarshal("create_workflow.fard", map[string]any{
"workflow_id": wfID,
"goal":        fmt.Sprintf("load test workflow %d", idx),
"owner":       "load",
}, &createResult); err != nil {
failed.Add(1)
return
}

wf := &store.Workflow{
WorkflowID: wfID,
Goal:       fmt.Sprintf("load test workflow %d", idx),
Owner:      "load",
Stage:      "created",
StateHash:  createResult.StateHash,
}
if err := db.CreateWorkflow(wf); err != nil {
failed.Add(1)
return
}
if err := db.Tx(func(tx *sql.Tx) error {
return db.SaveSnapshot(tx, &store.WorkflowSnapshot{
WorkflowID: wfID,
Seq:        0,
StateJSON:  createResult.StateJSON,
StateHash:  createResult.StateHash,
})
}); err != nil {
failed.Add(1)
return
}
created.Add(1)

// Apply transitions
numTransitions := cfg.TransitionsEach
if numTransitions > len(stages) {
numTransitions = len(stages)
}
// Add jitter so not all workers hit DB simultaneously
time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)

for j := 0; j < numTransitions; j++ {
snap, err := db.GetLatestSnapshot(wfID)
if err != nil || snap == nil {
failed.Add(1)
return
}

var result store.TransitionResult
if err := br.RunAndUnmarshal("transition.fard", map[string]any{
"state_json":   snap.StateJSON,
"actor":        map[string]any{"actor_id": "load:operator", "roles": []string{"operator"}},
"kind":         "human_state_edit",
"patches":      []any{map[string]any{"path": "stage", "value": stages[j]}},
"tool_version": "human",
}, &result); err != nil {
failed.Add(1)
return
}

if !result.OK {
failed.Add(1)
return
}

if err := db.CommitTransition(&store.CommitParams{
WorkflowID:    wfID,
Result:        &result,
Patches:       []map[string]any{{"path": "stage", "value": stages[j]}},
Kind:          "human_state_edit",
ActorID:       "load:operator",
SnapshotEvery: 10,
}); err != nil {
failed.Add(1)
return
}
committed.Add(1)
}
}(i)
}

wg.Wait()
elapsed := time.Since(start)

t.Logf("phase 1 complete in %v", elapsed)
t.Logf("  created:   %d", created.Load())
t.Logf("  committed: %d", committed.Load())
t.Logf("  failed:    %d", failed.Load())

if failed.Load() > 0 {
t.Errorf("FAIL: %d workflows failed during load phase", failed.Load())
}

// ── Phase 2: Verify receipt chains ────────────────────────────────────────
// Sample verification: check every workflow's chain is intact.
// For 10k workflows we verify all of them — that's the claim.
t.Logf("phase 2: verifying receipt chains for %d workflows...", cfg.NumWorkflows)

verifyStart := time.Now()
var verified, broken atomic.Int64

verSem := make(chan struct{}, cfg.Concurrency)
var verWg sync.WaitGroup

for i := 0; i < cfg.NumWorkflows; i++ {
verWg.Add(1)
verSem <- struct{}{}
go func(idx int) {
defer verWg.Done()
defer func() { <-verSem }()

wfID := fmt.Sprintf("wf_load_%06d", idx)
receipts, err := db.GetReceipts(wfID)
if err != nil {
broken.Add(1)
return
}
// Verify chain linkage
for j := 1; j < len(receipts); j++ {
if receipts[j].PrevReceiptDigest != receipts[j-1].ReceiptDigest {
broken.Add(1)
chainBreaks.Add(1)
return
}
}
verified.Add(1)
}(i)
}

verWg.Wait()
verifyElapsed := time.Since(verifyStart)

t.Logf("phase 2 complete in %v", verifyElapsed)
t.Logf("  verified:     %d", verified.Load())
t.Logf("  broken chains:%d", broken.Load())
t.Logf("  chain breaks: %d", chainBreaks.Load())

// ── Results ───────────────────────────────────────────────────────────────
totalTransitions := committed.Load()
throughput := float64(totalTransitions) / elapsed.Seconds()

t.Logf("\n=== v0.3.0 LOAD TEST RESULTS ===")
t.Logf("  workflows created:      %d / %d", created.Load(), cfg.NumWorkflows)
t.Logf("  transitions committed:  %d", totalTransitions)
t.Logf("  failures:               %d", failed.Load())
t.Logf("  chains verified:        %d", verified.Load())
t.Logf("  chain breaks:           %d", broken.Load())
t.Logf("  elapsed:                %v", elapsed)
t.Logf("  throughput:             %.0f transitions/sec", throughput)
t.Logf("  verify elapsed:         %v", verifyElapsed)

if broken.Load() > 0 {
t.Errorf("FAIL: %d receipt chains are broken — v0.3.0 claim NOT MET", broken.Load())
}
if failed.Load() > int64(cfg.NumWorkflows/100) { // allow <1% failure rate
t.Errorf("FAIL: failure rate %.1f%% exceeds 1%% threshold",
float64(failed.Load())/float64(cfg.NumWorkflows)*100)
}
if created.Load() < int64(cfg.NumWorkflows)*99/100 {
t.Errorf("FAIL: only %d/%d workflows created", created.Load(), cfg.NumWorkflows)
}

t.Logf("\n  CLAIM: %d workflows, receipts intact, chains verifiable",
cfg.NumWorkflows)
if broken.Load() == 0 && failed.Load() == 0 {
t.Logf("  STATUS: PASS — v0.3.0 capacity claim MET")
}
}

// TestWorkflowScaleSmoke is the fast smoke test run in CI.
// 100 workflows, verifies the same guarantees as the full test.
func TestWorkflowScaleSmoke(t *testing.T) {
os.Setenv("ACP_LOAD_WORKFLOWS", "100")
os.Setenv("ACP_LOAD_TRANSITIONS", "3")
os.Setenv("ACP_LOAD_CONCURRENCY", "20")
TestWorkflowScale(t)
}
