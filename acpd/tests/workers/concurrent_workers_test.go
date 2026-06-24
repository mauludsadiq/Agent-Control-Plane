package workers_test

import (
"database/sql"
"fmt"
"os"
"sync"
"sync/atomic"
"testing"
"time"

"github.com/google/uuid"
"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
)

func findMigrationsDir(t *testing.T) string {
t.Helper()
for _, c := range []string{"migrations", "../migrations", "../../migrations", "../../../migrations"} {
if _, err := os.Stat(c); err == nil {
return c
}
}
t.Fatal("migrations dir not found")
return ""
}

// TestNoConcurrentDoubleClaimSQLite verifies that 100 concurrent workers
// polling simultaneously never double-claim a task on SQLite.
func TestNoConcurrentDoubleClaimSQLite(t *testing.T) {
testNoConcurrentDoubleClaim(t, ":memory:")
}

func testNoConcurrentDoubleClaim(t *testing.T, dsn string) {
t.Helper()
store.MigrationDir = findMigrationsDir(t)
db, err := store.Open(dsn)
if err != nil {
t.Fatalf("open db: %v", err)
}
defer db.Close()

const numTasks = 50
const numWorkers = 100

// Enqueue 50 tasks for agent:test
for i := 0; i < numTasks; i++ {
taskID := fmt.Sprintf("task_%04d", i)
wfID := fmt.Sprintf("wf_%04d", i)
_ = db.CreateWorkflow(&store.Workflow{
WorkflowID: wfID, Goal: "test", Owner: "test",
Stage: "created", StateHash: "sha256:test",
})
err := db.Tx(func(tx *sql.Tx) error {
return db.EnqueueTask(tx, &store.Task{
TaskID: taskID, WorkflowID: wfID,
NodeID: "work", Agent: "agent:test",
InputJSON: `{}`, TimeoutSec: 300, MaxAttempts: 3,
})
})
if err != nil {
t.Fatalf("enqueue task %d: %v", i, err)
}
}

// 100 workers race to claim tasks simultaneously
var (
claimed    atomic.Int64
doubleClaim atomic.Int64
wg         sync.WaitGroup
)
claimedTasks := sync.Map{}

start := make(chan struct{})
for w := 0; w < numWorkers; w++ {
wg.Add(1)
go func() {
defer wg.Done()
<-start // wait for all goroutines to be ready
for {
task, err := db.ClaimNextTask("agent:test")
if err != nil || task == nil {
return
}
// Check for double-claim
if _, loaded := claimedTasks.LoadOrStore(task.TaskID, true); loaded {
doubleClaim.Add(1)
t.Errorf("DOUBLE CLAIM: task %s claimed by two workers", task.TaskID)
}
claimed.Add(1)
}
}()
}

close(start) // release all workers simultaneously
wg.Wait()

t.Logf("tasks enqueued: %d, claimed: %d, double-claims: %d",
numTasks, claimed.Load(), doubleClaim.Load())

if doubleClaim.Load() > 0 {
t.Errorf("FAIL: %d double-claims detected", doubleClaim.Load())
}
if claimed.Load() != int64(numTasks) {
t.Errorf("expected %d claims, got %d", numTasks, claimed.Load())
}
}

// TestHeartbeatPreventsRequeue verifies that a task with a recent heartbeat
// is NOT requeued by the expiry loop.
func TestHeartbeatPreventsRequeue(t *testing.T) {
store.MigrationDir = findMigrationsDir(t)
db, err := store.Open(":memory:")
if err != nil {
t.Fatalf("open db: %v", err)
}
defer db.Close()

wfID := "wf_heartbeat_" + uuid.New().String()[:8]
_ = db.CreateWorkflow(&store.Workflow{
WorkflowID: wfID, Goal: "test", Owner: "test",
Stage: "created", StateHash: "sha256:test",
})

taskID := "task_hb_" + uuid.New().String()[:8]
_ = db.Tx(func(tx *sql.Tx) error {
return db.EnqueueTask(tx, &store.Task{
TaskID: taskID, WorkflowID: wfID,
NodeID: "work", Agent: "agent:hb",
InputJSON: `{}`, TimeoutSec: 1, // 1 second timeout
MaxAttempts: 3,
})
})

// Claim the task
task, err := db.ClaimNextTask("agent:hb")
if err != nil || task == nil {
t.Fatalf("claim task: %v", err)
}

// Send heartbeat immediately
if err := db.HeartbeatTask(taskID, "agent:hb"); err != nil {
t.Fatalf("heartbeat: %v", err)
}

// Wait for timeout to pass
time.Sleep(1500 * time.Millisecond)

// Send another heartbeat — resets the clock
if err := db.HeartbeatTask(taskID, "agent:hb"); err != nil {
t.Fatalf("second heartbeat: %v", err)
}

// Run requeue — task should NOT be requeued because heartbeat is fresh
requeued, deadLettered, err := db.RequeueExpiredTasksWithDeadLetter()
if err != nil {
t.Fatalf("requeue: %v", err)
}
if requeued > 0 || deadLettered > 0 {
t.Errorf("heartbeating task was requeued: requeued=%d deadLettered=%d",
requeued, deadLettered)
}

// Verify still claimed
got, _ := db.GetTask(taskID)
if got.Status != "claimed" {
t.Errorf("expected status claimed, got %s", got.Status)
}
t.Log("heartbeat correctly prevented requeue")
}

// TestExpiredTaskRequeues verifies that a task with no heartbeat within
// timeout_sec is requeued.
func TestExpiredTaskRequeues(t *testing.T) {
store.MigrationDir = findMigrationsDir(t)
db, err := store.Open(":memory:")
if err != nil {
t.Fatalf("open db: %v", err)
}
defer db.Close()

wfID := "wf_expire_" + uuid.New().String()[:8]
_ = db.CreateWorkflow(&store.Workflow{
WorkflowID: wfID, Goal: "test", Owner: "test",
Stage: "created", StateHash: "sha256:test",
})

taskID := "task_expire_" + uuid.New().String()[:8]
_ = db.Tx(func(tx *sql.Tx) error {
return db.EnqueueTask(tx, &store.Task{
TaskID: taskID, WorkflowID: wfID,
NodeID: "work", Agent: "agent:expire",
InputJSON: `{}`, TimeoutSec: 1,
MaxAttempts: 3,
})
})

// Claim but never heartbeat
task, err := db.ClaimNextTask("agent:expire")
if err != nil || task == nil {
t.Fatalf("claim: %v", err)
}
if task.AttemptCount != 1 {
t.Errorf("expected attempt_count 1, got %d", task.AttemptCount)
}

// Wait for timeout
time.Sleep(1500 * time.Millisecond)

// Requeue
requeued, _, err := db.RequeueExpiredTasksWithDeadLetter()
if err != nil {
t.Fatalf("requeue: %v", err)
}
if requeued != 1 {
t.Errorf("expected 1 requeued, got %d", requeued)
}

// Verify pending and re-claimable
got, _ := db.GetTask(taskID)
if got.Status != "pending" {
t.Errorf("expected status pending, got %s", got.Status)
}
t.Logf("task requeued after timeout, attempt_count=%d", got.AttemptCount)
}

// TestDeadLetterAfterMaxAttempts verifies that a task dead-lettered after
// exhausting max_attempts and is not requeued again.
func TestDeadLetterAfterMaxAttempts(t *testing.T) {
store.MigrationDir = findMigrationsDir(t)
db, err := store.Open(":memory:")
if err != nil {
t.Fatalf("open db: %v", err)
}
defer db.Close()

wfID := "wf_dl_" + uuid.New().String()[:8]
_ = db.CreateWorkflow(&store.Workflow{
WorkflowID: wfID, Goal: "test", Owner: "test",
Stage: "created", StateHash: "sha256:test",
})

taskID := "task_dl_" + uuid.New().String()[:8]
_ = db.Tx(func(tx *sql.Tx) error {
return db.EnqueueTask(tx, &store.Task{
TaskID: taskID, WorkflowID: wfID,
NodeID: "work", Agent: "agent:dl",
InputJSON: `{}`, TimeoutSec: 1,
MaxAttempts: 2, // low for test
})
})

// Attempt 1: claim and let expire
task, _ := db.ClaimNextTask("agent:dl")
if task == nil {
t.Fatal("claim attempt 1 failed")
}
time.Sleep(1500 * time.Millisecond)
requeued, _, _ := db.RequeueExpiredTasksWithDeadLetter()
if requeued != 1 {
t.Fatalf("expected requeue after attempt 1, got %d", requeued)
}

// Attempt 2: claim and let expire — this exhausts max_attempts
task, _ = db.ClaimNextTask("agent:dl")
if task == nil {
t.Fatal("claim attempt 2 failed")
}
if task.AttemptCount != 2 {
t.Errorf("expected attempt_count 2, got %d", task.AttemptCount)
}
time.Sleep(1500 * time.Millisecond)
_, deadLettered, _ := db.RequeueExpiredTasksWithDeadLetter()
if deadLettered != 1 {
t.Errorf("expected 1 dead-lettered, got %d", deadLettered)
}

// Verify dead-lettered
got, _ := db.GetTask(taskID)
if !got.DeadLettered {
t.Error("expected task to be dead-lettered")
}
if got.Status != "failed" {
t.Errorf("expected status failed, got %s", got.Status)
}

// Verify not re-claimable
task, _ = db.ClaimNextTask("agent:dl")
if task != nil {
t.Error("dead-lettered task should not be claimable")
}

// Verify in dead letter table
dlTasks, _ := db.ListDeadLetterTasks(wfID)
if len(dlTasks) != 1 {
t.Errorf("expected 1 dead letter task, got %d", len(dlTasks))
}
t.Logf("task correctly dead-lettered after %d attempts", got.AttemptCount)
}

// TestPriorityLanes verifies that critical tasks are claimed before normal
// and background tasks regardless of creation order.
func TestPriorityLanes(t *testing.T) {
store.MigrationDir = findMigrationsDir(t)
db, err := store.Open(":memory:")
if err != nil {
t.Fatalf("open db: %v", err)
}
defer db.Close()

wfID := "wf_lanes_" + uuid.New().String()[:8]
_ = db.CreateWorkflow(&store.Workflow{
WorkflowID: wfID, Goal: "test", Owner: "test",
Stage: "created", StateHash: "sha256:test",
})

// Enqueue background first, then normal, then critical
for _, task := range []struct {
id   string
lane string
}{
{"task_bg", store.PriorityLaneBackground},
{"task_normal", store.PriorityLaneNormal},
{"task_critical", store.PriorityLaneCritical},
} {
_ = db.Tx(func(tx *sql.Tx) error {
return db.EnqueueTask(tx, &store.Task{
TaskID: task.id, WorkflowID: wfID,
NodeID: "work", Agent: "agent:lanes",
InputJSON: `{}`, TimeoutSec: 300,
MaxAttempts: 3, PriorityLane: task.lane,
})
})
time.Sleep(10 * time.Millisecond) // ensure different created_at
}

// Should claim critical first
t1, _ := db.ClaimNextTask("agent:lanes")
if t1 == nil || t1.TaskID != "task_critical" {
t.Errorf("expected task_critical first, got %v", t1)
}
// Then normal
t2, _ := db.ClaimNextTask("agent:lanes")
if t2 == nil || t2.TaskID != "task_normal" {
t.Errorf("expected task_normal second, got %v", t2)
}
// Then background
t3, _ := db.ClaimNextTask("agent:lanes")
if t3 == nil || t3.TaskID != "task_bg" {
t.Errorf("expected task_bg third, got %v", t3)
}
t.Log("priority lanes: critical → normal → background ✓")
}

// TestConcurrentWorkersScaleSmoke is the v0.4.0 smoke test.
// 100 concurrent workers, 500 tasks, 0 double-claims, 0 lost tasks.
func TestConcurrentWorkersScaleSmoke(t *testing.T) {
store.MigrationDir = findMigrationsDir(t)
db, err := store.Open(":memory:")
if err != nil {
t.Fatalf("open db: %v", err)
}
defer db.Close()

const numWorkers = 100
const numTasks = 500

// Create one workflow and enqueue all tasks
_ = db.CreateWorkflow(&store.Workflow{
WorkflowID: "wf_scale", Goal: "scale test", Owner: "test",
Stage: "created", StateHash: "sha256:test",
})
for i := 0; i < numTasks; i++ {
_ = db.Tx(func(tx *sql.Tx) error {
return db.EnqueueTask(tx, &store.Task{
TaskID:      fmt.Sprintf("task_scale_%04d", i),
WorkflowID:  "wf_scale",
NodeID:      "work",
Agent:       "agent:scale",
InputJSON:   `{}`,
TimeoutSec:  300,
MaxAttempts: 3,
})
})
}

var claimed atomic.Int64
claimedIDs := sync.Map{}
var doubleClaims atomic.Int64
var wg sync.WaitGroup
start := make(chan struct{})

for w := 0; w < numWorkers; w++ {
wg.Add(1)
go func() {
defer wg.Done()
<-start
for {
task, err := db.ClaimNextTask("agent:scale")
if err != nil || task == nil {
return
}
if _, loaded := claimedIDs.LoadOrStore(task.TaskID, true); loaded {
doubleClaims.Add(1)
}
claimed.Add(1)
}
}()
}

startTime := time.Now()
close(start)
wg.Wait()
elapsed := time.Since(startTime)

t.Logf("=== v0.4.0 CONCURRENT WORKER RESULTS ===")
t.Logf("  workers:       %d", numWorkers)
t.Logf("  tasks:         %d", numTasks)
t.Logf("  claimed:       %d", claimed.Load())
t.Logf("  double-claims: %d", doubleClaims.Load())
t.Logf("  elapsed:       %v", elapsed)

if doubleClaims.Load() > 0 {
t.Errorf("FAIL: %d double-claims — v0.4.0 claim NOT safe", doubleClaims.Load())
}
if claimed.Load() != int64(numTasks) {
t.Errorf("FAIL: claimed %d/%d tasks — tasks lost", claimed.Load(), numTasks)
}
if doubleClaims.Load() == 0 && claimed.Load() == int64(numTasks) {
t.Logf("STATUS: PASS — v0.4.0 concurrent worker claim is race-safe")
}
}
