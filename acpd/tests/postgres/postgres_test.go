package postgres_test

import (
"context"
"database/sql"
"fmt"
"os"
"sync"
"sync/atomic"
"testing"
"time"

tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/bridge"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
)

// startPostgres starts a real Postgres container and returns the DSN.
// Skips the test if Docker is not available.
func startPostgres(t *testing.T) string {
t.Helper()
ctx := context.Background()

pg, err := tcpostgres.Run(ctx,
"postgres:16-alpine",
tcpostgres.WithDatabase("acp"),
tcpostgres.WithUsername("acp"),
tcpostgres.WithPassword("acp"),
tcpostgres.BasicWaitStrategies(),
)
if err != nil {
t.Skipf("Docker not available, skipping Postgres tests: %v", err)
}
t.Cleanup(func() { _ = pg.Terminate(ctx) })

host, err := pg.Host(ctx)
if err != nil {
t.Fatalf("get postgres host: %v", err)
}
port, err := pg.MappedPort(ctx, "5432")
if err != nil {
t.Fatalf("get postgres port: %v", err)
}

dsn := fmt.Sprintf("postgres://acp:acp@%s:%s/acp?sslmode=disable", host, port.Port())
t.Logf("postgres DSN: %s", dsn)
return dsn
}

// findDir walks up from cwd to find a directory by name.
func findDir(t *testing.T, name string) string {
t.Helper()
candidates := []string{
name,
"../" + name,
"../../" + name,
"../../../" + name,
}
for _, c := range candidates {
if _, err := os.Stat(c); err == nil {
return c
}
}
t.Fatalf("could not find %s dir", name)
return ""
}

// ─── Schema migration test ────────────────────────────────────────────────────

// TestPostgresMigrations verifies that both migration files apply cleanly to
// a real Postgres instance and all 11 tables exist afterward.
func TestPostgresMigrations(t *testing.T) {
dsn := startPostgres(t)
store.MigrationDir = findDir(t, "migrations")

db, err := store.Open(dsn)
if err != nil {
t.Fatalf("open postgres db: %v", err)
}
defer db.Close()

// Verify all tables exist
tables := []string{
"actors", "workflows", "workflow_snapshots", "deltas",
"receipts", "artifacts", "gates", "tasks",
"audit_events", "policy_versions", "schema_migrations",
}
rawDB := db.RawDB()
for _, table := range tables {
var count int
err := rawDB.QueryRow(
`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = $1 AND table_schema = 'public'`,
table,
).Scan(&count)
if err != nil || count == 0 {
t.Errorf("table %s not found after migration", table)
}
}

// Verify indexes exist
var idxCount int
_ = rawDB.QueryRow(`SELECT COUNT(*) FROM pg_indexes WHERE schemaname = 'public'`).Scan(&idxCount)
if idxCount < 15 {
t.Errorf("expected at least 15 indexes, got %d", idxCount)
}

// Verify schema_migrations rows
var migrations []string
rows, _ := rawDB.Query(`SELECT version FROM schema_migrations ORDER BY version`)
defer rows.Close()
for rows.Next() {
var v string
_ = rows.Scan(&v)
migrations = append(migrations, v)
}
t.Logf("applied migrations: %v", migrations)
if len(migrations) < 2 {
t.Errorf("expected at least 2 migrations applied, got %d", len(migrations))
}
}

// ─── Idempotency test ─────────────────────────────────────────────────────────

// TestPostgresMigrationsIdempotent verifies that running migrations twice
// does not error (INSERT ON CONFLICT DO NOTHING, CREATE IF NOT EXISTS).
func TestPostgresMigrationsIdempotent(t *testing.T) {
dsn := startPostgres(t)
store.MigrationDir = findDir(t, "migrations")

db, err := store.Open(dsn)
if err != nil {
t.Fatalf("first open: %v", err)
}
db.Close()

// Open again — migrations run again
db2, err := store.Open(dsn)
if err != nil {
t.Fatalf("second open (idempotency): %v", err)
}
defer db2.Close()
t.Log("migrations are idempotent on Postgres")
}

// ─── CRUD parity test ─────────────────────────────────────────────────────────

// TestPostgresStoreParity verifies that all store operations work identically
// on Postgres as they do on SQLite — same API, same semantics.
func TestPostgresStoreParity(t *testing.T) {
dsn := startPostgres(t)
store.MigrationDir = findDir(t, "migrations")

db, err := store.Open(dsn)
if err != nil {
t.Fatalf("open postgres db: %v", err)
}
defer db.Close()

// Create actor
if err := db.CreateActor(&store.Actor{
ActorID: "operator:pg-test",
Roles:   []string{"operator", "manager"},
}, "acp_pg_test_key"); err != nil {
t.Fatalf("create actor: %v", err)
}

// Resolve API key
actor, err := db.ResolveAPIKey("acp_pg_test_key")
if err != nil || actor == nil {
t.Fatalf("resolve api key: %v", err)
}
if actor.ActorID != "operator:pg-test" {
t.Errorf("wrong actor id: %s", actor.ActorID)
}

// Create workflow
if err := db.CreateWorkflow(&store.Workflow{
WorkflowID: "wf_pg_001",
Goal:       "postgres parity test",
Owner:      "test",
Stage:      "created",
StateHash:  "sha256:test",
}); err != nil {
t.Fatalf("create workflow: %v", err)
}

// Get workflow
wf, err := db.GetWorkflow("wf_pg_001")
if err != nil || wf == nil {
t.Fatalf("get workflow: %v", err)
}
if wf.Goal != "postgres parity test" {
t.Errorf("wrong goal: %s", wf.Goal)
}

// Save snapshot
if err := db.Tx(func(tx *sql.Tx) error {
return db.SaveSnapshot(tx, &store.WorkflowSnapshot{
WorkflowID: "wf_pg_001",
Seq:        0,
StateJSON:  `{"schema":"acp.workflow_state.v1","workflow_id":"wf_pg_001"}`,
StateHash:  "sha256:genesis",
})
}); err != nil {
t.Fatalf("save snapshot: %v", err)
}

// Get latest snapshot
snap, err := db.GetLatestSnapshot("wf_pg_001")
if err != nil || snap == nil {
t.Fatalf("get latest snapshot: %v", err)
}
if snap.StateHash != "sha256:genesis" {
t.Errorf("wrong state hash: %s", snap.StateHash)
}

// Append receipt
if err := db.Tx(func(tx *sql.Tx) error {
return db.AppendReceipt(tx, &store.Receipt{
WorkflowID:    "wf_pg_001",
Seq:           1,
ReceiptDigest: "sha256:receipt1",
ReceiptJSON:   `{"receipt_type":"acp.state_transition.v1","seq":1}`,
})
}); err != nil {
t.Fatalf("append receipt: %v", err)
}

// Get receipts
receipts, err := db.GetReceipts("wf_pg_001")
if err != nil || len(receipts) != 1 {
t.Fatalf("get receipts: got %d, want 1, err=%v", len(receipts), err)
}

// Append delta
if err := db.Tx(func(tx *sql.Tx) error {
return db.AppendDelta(tx, &store.Delta{
WorkflowID: "wf_pg_001",
Seq:        1,
DeltaJSON:  `{"delta_type":"acp.delta.v1","seq":1}`,
DeltaHash:  "sha256:delta1",
})
}); err != nil {
t.Fatalf("append delta: %v", err)
}

// Save artifact
if err := db.Tx(func(tx *sql.Tx) error {
return db.SaveArtifact(tx, &store.Artifact{
Digest:       "sha256:artifact1",
WorkflowID:   "wf_pg_001",
ArtifactJSON: `{"artifact_type":"acp.artifact.v1"}`,
})
}); err != nil {
t.Fatalf("save artifact: %v", err)
}

// List artifacts
arts, err := db.ListArtifacts("wf_pg_001")
if err != nil || len(arts) != 1 {
t.Fatalf("list artifacts: got %d, want 1, err=%v", len(arts), err)
}

// Enqueue and claim task
if err := db.Tx(func(tx *sql.Tx) error {
return db.EnqueueTask(tx, &store.Task{
TaskID:     "task_pg_001",
WorkflowID: "wf_pg_001",
NodeID:     "research",
Agent:      "agent:research",
InputJSON:  `{"task_id":"task_pg_001"}`,
TimeoutSec: 300,
})
}); err != nil {
t.Fatalf("enqueue task: %v", err)
}

task, err := db.ClaimNextTask("agent:research")
if err != nil || task == nil {
t.Fatalf("claim task: %v", err)
}
if task.TaskID != "task_pg_001" {
t.Errorf("wrong task id: %s", task.TaskID)
}

// Create and get gate
if err := db.Tx(func(tx *sql.Tx) error {
return db.CreateGate(tx, &store.Gate{
Token:      "sha256:gate_token_pg",
WorkflowID: "wf_pg_001",
Seq:        2,
Status:     "pending",
GateJSON:   `{"gate_type":"acp.gate.v1"}`,
})
}); err != nil {
t.Fatalf("create gate: %v", err)
}

gate, err := db.GetGate("sha256:gate_token_pg")
if err != nil || gate == nil {
t.Fatalf("get gate: %v", err)
}
if gate.Status != "pending" {
t.Errorf("wrong gate status: %s", gate.Status)
}

// List pending gates
pending, err := db.ListPendingGates()
if err != nil || len(pending) != 1 {
t.Fatalf("list pending gates: got %d, want 1, err=%v", len(pending), err)
}

t.Log("all store operations pass on Postgres")
}

// ─── Receipt chain integrity test ────────────────────────────────────────────

// TestPostgresReceiptChain verifies that the receipt chain links correctly
// across multiple commits on Postgres — the same invariant as SQLite.
func TestPostgresReceiptChain(t *testing.T) {
dsn := startPostgres(t)
store.MigrationDir = findDir(t, "migrations")

db, err := store.Open(dsn)
if err != nil {
t.Fatalf("open postgres db: %v", err)
}
defer db.Close()

outDir, err := os.MkdirTemp("", "acp-pg-test-*")
if err != nil {
t.Fatalf("create outdir: %v", err)
}
defer os.RemoveAll(outDir)

br := bridge.New("fardrun", findDir(t, "fard/bridge"), outDir)

// Create workflow
var createResult store.TransitionResult
if err := br.RunAndUnmarshal("create_workflow.fard", map[string]any{
"workflow_id": "wf_pg_chain_001",
"goal":        "postgres receipt chain test",
"owner":       "test",
}, &createResult); err != nil {
t.Fatalf("create workflow bridge: %v", err)
}

if err := db.CreateWorkflow(&store.Workflow{
WorkflowID: "wf_pg_chain_001",
Goal:       "postgres receipt chain test",
Owner:      "test",
Stage:      "created",
StateHash:  createResult.StateHash,
}); err != nil {
t.Fatalf("store workflow: %v", err)
}
if err := db.Tx(func(tx *sql.Tx) error {
return db.SaveSnapshot(tx, &store.WorkflowSnapshot{
WorkflowID: "wf_pg_chain_001",
Seq:        0,
StateJSON:  createResult.StateJSON,
StateHash:  createResult.StateHash,
})
}); err != nil {
t.Fatalf("save initial snapshot: %v", err)
}

// Commit 5 transitions
stages := []string{"research", "review", "decision", "approval", "complete"}
for i, stage := range stages {
snap, err := db.GetLatestSnapshot("wf_pg_chain_001")
if err != nil || snap == nil {
t.Fatalf("step %d: get snapshot: %v", i, err)
}

var result store.TransitionResult
if err := br.RunAndUnmarshal("transition.fard", map[string]any{
"state_json":   snap.StateJSON,
"actor":        map[string]any{"actor_id": "operator:pg", "roles": []string{"operator"}},
"kind":         "human_state_edit",
"patches":      []any{map[string]any{"path": "stage", "value": stage}},
"tool_version": "human",
}, &result); err != nil {
t.Fatalf("step %d: transition bridge: %v", i, err)
}

if err := db.CommitTransition(&store.CommitParams{
WorkflowID:    "wf_pg_chain_001",
Result:        &result,
Patches:       []map[string]any{{"path": "stage", "value": stage}},
Kind:          "human_state_edit",
ActorID:       "operator:pg",
SnapshotEvery: 10,
}); err != nil {
t.Fatalf("step %d: commit: %v", i, err)
}
}

// Verify chain
receipts, err := db.GetReceipts("wf_pg_chain_001")
if err != nil {
t.Fatalf("get receipts: %v", err)
}
if len(receipts) != 5 {
t.Fatalf("expected 5 receipts, got %d", len(receipts))
}
for i := 1; i < len(receipts); i++ {
if receipts[i].PrevReceiptDigest != receipts[i-1].ReceiptDigest {
t.Errorf("chain broken at seq %d: prev=%s want=%s",
receipts[i].Seq, receipts[i].PrevReceiptDigest, receipts[i-1].ReceiptDigest)
}
}
t.Logf("receipt chain verified: %d receipts, all linked", len(receipts))
}

// ─── Concurrent write test ────────────────────────────────────────────────────

// TestPostgresConcurrentWrites verifies that Postgres handles concurrent
// workflow creation and transitions without corruption or deadlock.
// This is the key test SQLite cannot pass under real concurrency.
func TestPostgresConcurrentWrites(t *testing.T) {
dsn := startPostgres(t)
store.MigrationDir = findDir(t, "migrations")

db, err := store.Open(dsn)
if err != nil {
t.Fatalf("open postgres db: %v", err)
}
defer db.Close()

outDir, err := os.MkdirTemp("", "acp-pg-concurrent-*")
if err != nil {
t.Fatalf("create outdir: %v", err)
}
defer os.RemoveAll(outDir)

br := bridge.New("fardrun", findDir(t, "fard/bridge"), outDir)

const numWorkers = 20
const workflowsPerWorker = 5

var (
created  atomic.Int64
failed   atomic.Int64
wg       sync.WaitGroup
)

start := time.Now()

for w := 0; w < numWorkers; w++ {
wg.Add(1)
go func(workerID int) {
defer wg.Done()
for i := 0; i < workflowsPerWorker; i++ {
wfID := fmt.Sprintf("wf_pg_conc_%02d_%02d", workerID, i)

var cr store.TransitionResult
if err := br.RunAndUnmarshal("create_workflow.fard", map[string]any{
"workflow_id": wfID,
"goal":        fmt.Sprintf("concurrent test w%d i%d", workerID, i),
"owner":       "test",
}, &cr); err != nil {
failed.Add(1)
continue
}

if err := db.CreateWorkflow(&store.Workflow{
WorkflowID: wfID,
Goal:       fmt.Sprintf("concurrent test w%d i%d", workerID, i),
Owner:      "test",
Stage:      "created",
StateHash:  cr.StateHash,
}); err != nil {
failed.Add(1)
continue
}

if err := db.Tx(func(tx *sql.Tx) error {
return db.SaveSnapshot(tx, &store.WorkflowSnapshot{
WorkflowID: wfID,
Seq:        0,
StateJSON:  cr.StateJSON,
StateHash:  cr.StateHash,
})
}); err != nil {
failed.Add(1)
continue
}

// Two transitions per workflow
for _, stage := range []string{"research", "review"} {
snap, err := db.GetLatestSnapshot(wfID)
if err != nil || snap == nil {
failed.Add(1)
break
}
var result store.TransitionResult
if err := br.RunAndUnmarshal("transition.fard", map[string]any{
"state_json":   snap.StateJSON,
"actor":        map[string]any{"actor_id": "operator:pg", "roles": []string{"operator"}},
"kind":         "human_state_edit",
"patches":      []any{map[string]any{"path": "stage", "value": stage}},
"tool_version": "human",
}, &result); err != nil {
failed.Add(1)
break
}
if err := db.CommitTransition(&store.CommitParams{
WorkflowID:    wfID,
Result:        &result,
Patches:       []map[string]any{{"path": "stage", "value": stage}},
Kind:          "human_state_edit",
ActorID:       "operator:pg",
SnapshotEvery: 10,
}); err != nil {
failed.Add(1)
break
}
}
created.Add(1)
}
}(w)
}

wg.Wait()
elapsed := time.Since(start)

total := numWorkers * workflowsPerWorker
t.Logf("concurrent writes: %d/%d workflows, %d failed, elapsed %v",
created.Load(), total, failed.Load(), elapsed)

if failed.Load() > int64(total/10) {
t.Errorf("too many failures: %d/%d", failed.Load(), total)
}

// Verify no chain breaks
wfs, _ := db.ListWorkflows("", 0)
broken := 0
for _, wf := range wfs {
receipts, _ := db.GetReceipts(wf.WorkflowID)
for i := 1; i < len(receipts); i++ {
if receipts[i].PrevReceiptDigest != receipts[i-1].ReceiptDigest {
broken++
}
}
}
if broken > 0 {
t.Errorf("FAIL: %d broken receipt chains under concurrent writes", broken)
} else {
t.Logf("all receipt chains intact under %d concurrent workers", numWorkers)
}
}

// ─── Load test on Postgres ────────────────────────────────────────────────────

// TestPostgresScale is the v0.3.0 capacity claim on real Postgres.
// Default: 1,000 workflows (smoke). Set ACP_PG_LOAD_WORKFLOWS=10000 for full run.
func TestPostgresScale(t *testing.T) {
numWorkflows := 1000
if v := os.Getenv("ACP_PG_LOAD_WORKFLOWS"); v != "" {
fmt.Sscanf(v, "%d", &numWorkflows)
}

dsn := startPostgres(t)
store.MigrationDir = findDir(t, "migrations")

db, err := store.Open(dsn)
if err != nil {
t.Fatalf("open postgres db: %v", err)
}
defer db.Close()

outDir, err := os.MkdirTemp("", "acp-pg-scale-*")
if err != nil {
t.Fatalf("create outdir: %v", err)
}
defer os.RemoveAll(outDir)

br := bridge.New("fardrun", findDir(t, "fard/bridge"), outDir)

if err := db.CreateActor(&store.Actor{
ActorID: "load:pg",
Roles:   []string{"operator"},
}, "acp_pg_load_key"); err != nil {
t.Logf("seed actor (may exist): %v", err)
}

var created, committed, failed atomic.Int64
sem := make(chan struct{}, 20)
var wg sync.WaitGroup
start := time.Now()

for i := 0; i < numWorkflows; i++ {
wg.Add(1)
sem <- struct{}{}
go func(idx int) {
defer wg.Done()
defer func() { <-sem }()

wfID := fmt.Sprintf("wf_pg_scale_%06d", idx)
var cr store.TransitionResult
if err := br.RunAndUnmarshal("create_workflow.fard", map[string]any{
"workflow_id": wfID,
"goal":        fmt.Sprintf("pg scale test %d", idx),
"owner":       "test",
}, &cr); err != nil {
failed.Add(1)
return
}
if err := db.CreateWorkflow(&store.Workflow{
WorkflowID: wfID, Goal: fmt.Sprintf("pg scale test %d", idx),
Owner: "test", Stage: "created", StateHash: cr.StateHash,
}); err != nil {
failed.Add(1)
return
}
if err := db.Tx(func(tx *sql.Tx) error {
return db.SaveSnapshot(tx, &store.WorkflowSnapshot{
WorkflowID: wfID, Seq: 0,
StateJSON: cr.StateJSON, StateHash: cr.StateHash,
})
}); err != nil {
failed.Add(1)
return
}
created.Add(1)

for _, stage := range []string{"research", "review", "complete"} {
snap, err := db.GetLatestSnapshot(wfID)
if err != nil || snap == nil {
failed.Add(1)
return
}
var result store.TransitionResult
if err := br.RunAndUnmarshal("transition.fard", map[string]any{
"state_json":   snap.StateJSON,
"actor":        map[string]any{"actor_id": "load:pg", "roles": []string{"operator"}},
"kind":         "human_state_edit",
"patches":      []any{map[string]any{"path": "stage", "value": stage}},
"tool_version": "human",
}, &result); err != nil {
failed.Add(1)
return
}
if err := db.CommitTransition(&store.CommitParams{
WorkflowID: wfID, Result: &result,
Patches:       []map[string]any{{"path": "stage", "value": stage}},
Kind:          "human_state_edit",
ActorID:       "load:pg",
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

// Verify chains
var broken atomic.Int64
wfs, _ := db.ListWorkflows("", 0)
verSem := make(chan struct{}, 20)
var verWg sync.WaitGroup
verStart := time.Now()
for _, wf := range wfs {
verWg.Add(1)
verSem <- struct{}{}
go func(wfID string) {
defer verWg.Done()
defer func() { <-verSem }()
receipts, _ := db.GetReceipts(wfID)
for i := 1; i < len(receipts); i++ {
if receipts[i].PrevReceiptDigest != receipts[i-1].ReceiptDigest {
broken.Add(1)
return
}
}
}(wf.WorkflowID)
}
verWg.Wait()
verifyElapsed := time.Since(verStart)

throughput := float64(committed.Load()) / elapsed.Seconds()
t.Logf("\n=== v0.3.0 POSTGRES SCALE RESULTS ===")
t.Logf("  workflows created:     %d / %d", created.Load(), numWorkflows)
t.Logf("  transitions committed: %d", committed.Load())
t.Logf("  failures:              %d", failed.Load())
t.Logf("  chain breaks:          %d", broken.Load())
t.Logf("  elapsed:               %v", elapsed)
t.Logf("  throughput:            %.0f transitions/sec", throughput)
t.Logf("  verify elapsed:        %v", verifyElapsed)

if broken.Load() > 0 {
t.Errorf("FAIL: %d receipt chains broken on Postgres", broken.Load())
}
if failed.Load() > int64(numWorkflows/20) {
t.Errorf("FAIL: failure rate %.1f%% exceeds 5%%",
float64(failed.Load())/float64(numWorkflows)*100)
}
if broken.Load() == 0 && failed.Load() == 0 {
t.Logf("STATUS: PASS — v0.3.0 Postgres capacity claim MET (%d workflows)", numWorkflows)
}
}
