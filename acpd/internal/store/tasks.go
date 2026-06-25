package store

import (
	"context"
"database/sql"
"fmt"
"time"

	"github.com/mauludsadiq/agent-control-plane/acpd/internal/telemetry"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type Task struct {
TaskID           string
WorkflowID       string
NodeID           string
Agent            string
Status           string
Priority         int
PriorityLane     string
InputJSON        string
OutputJSON       string
PolicyResultJSON string
ClaimedBy        string
ClaimedAt        *time.Time
LastHeartbeat    *time.Time
TimeoutSec       int
AttemptCount     int
MaxAttempts      int
DeadLettered     bool
CompletedAt      *time.Time
FailedReason     string
CreatedAt        time.Time
UpdatedAt        time.Time
}

const (
PriorityLaneCritical = "critical"
PriorityLaneNormal   = "normal"
PriorityLaneBackground = "background"
)

func (d *DB) EnqueueTask(tx *sql.Tx, t *Task) error {
lane := t.PriorityLane
if lane == "" {
lane = PriorityLaneNormal
}
maxAttempts := t.MaxAttempts
if maxAttempts == 0 {
maxAttempts = 3
}
_, err := txExec(d, tx, `
INSERT INTO tasks (task_id, workflow_id, node_id, agent, status, priority,
                   priority_lane, input_json, timeout_sec, max_attempts,
                   attempt_count, created_at, updated_at)
VALUES (?, ?, ?, ?, 'pending', ?, ?, ?, ?, ?, 0, ?, ?)`,
t.TaskID, t.WorkflowID, t.NodeID, t.Agent, t.Priority,
lane, t.InputJSON, t.TimeoutSec, maxAttempts,
now(), now(),
)
return err
}

// ClaimNextTask atomically claims the next available task for the given agent.
// Uses a two-step pattern: select candidate by ID, then update WHERE status='pending'.
// Only one concurrent worker wins even if both read the same candidate simultaneously —
// the UPDATE is conditional on status still being 'pending'.
func (d *DB) ClaimNextTask(agent string) (*Task, error) {
var claimed *Task
err := d.Tx(func(tx *sql.Tx) error {
// Step 1: find the best candidate task ID
var taskID string
row := txQueryRow(d, tx, `
SELECT task_id FROM tasks
WHERE agent = ? AND status = 'pending' AND dead_lettered = 0
  AND attempt_count < max_attempts
ORDER BY
  CASE priority_lane
    WHEN 'critical'   THEN 0
    WHEN 'normal'     THEN 1
    WHEN 'background' THEN 2
    ELSE 1
  END,
  priority DESC,
  created_at ASC
LIMIT 1`, agent)
if err := row.Scan(&taskID); err == sql.ErrNoRows {
return nil
} else if err != nil {
return fmt.Errorf("scan candidate: %w", err)
}

// Step 2: atomically claim it — only succeeds if still pending
result, err := txExec(d, tx, `
UPDATE tasks
SET status='claimed', claimed_by=?, claimed_at=?, last_heartbeat=?,
    attempt_count=attempt_count+1, updated_at=?
WHERE task_id=? AND status='pending' AND dead_lettered=0`,
agent, now(), now(), now(), taskID,
)
if err != nil {
return fmt.Errorf("claim update: %w", err)
}
n, _ := result.RowsAffected()
if n == 0 {
// Another worker won the race — no task for us this call
return nil
}

// Step 3: read the claimed task back
t, err := getTaskTx(d, tx, taskID)
if err != nil {
return fmt.Errorf("read claimed task: %w", err)
}
claimed = t
return nil
})
if err == nil && claimed != nil && telemetry.TasksClaimed != nil {
telemetry.TasksClaimed.Add(context.Background(), 1,
metric.WithAttributes(attribute.String("agent", agent)))
}
return claimed, err
}

// Heartbeat updates the last_heartbeat timestamp for a claimed task.
// Workers should call this periodically to prevent requeue.
func (d *DB) HeartbeatTask(taskID, workerID string) error {
_, err := d.exec(`
UPDATE tasks SET last_heartbeat=?, updated_at=?
WHERE task_id=? AND status='claimed' AND claimed_by=?`,
now(), now(), taskID, workerID,
)
return err
}

func (d *DB) CompleteTask(tx *sql.Tx, taskID, outputJSON, policyResultJSON string) error {
_, err := txExec(d, tx, `
UPDATE tasks SET status='completed', output_json=?, policy_result_json=?,
                 completed_at=?, updated_at=?
WHERE task_id=?`,
outputJSON, policyResultJSON, now(), now(), taskID,
)
return err
}

func (d *DB) FailTask(tx *sql.Tx, taskID, reason string) error {
// Check if we've exhausted attempts — if so, dead letter
var attemptCount, maxAttempts int
row := d.queryRow(`SELECT attempt_count, max_attempts FROM tasks WHERE task_id=?`, taskID)
_ = row.Scan(&attemptCount, &maxAttempts)

if attemptCount >= maxAttempts {
return d.deadLetterTask(tx, taskID, reason)
}

_, err := txExec(d, tx, `
UPDATE tasks SET status='failed', failed_reason=?, updated_at=?
WHERE task_id=?`,
reason, now(), taskID,
)
return err
}

func (d *DB) deadLetterTask(tx *sql.Tx, taskID, reason string) error {
// Copy to dead_letter_tasks
var t Task
row := txQueryRow(d, tx, `
SELECT task_id, workflow_id, node_id, agent, input_json,
       attempt_count, created_at
FROM tasks WHERE task_id=?`, taskID)
var createdAt string
if err := row.Scan(&t.TaskID, &t.WorkflowID, &t.NodeID, &t.Agent,
&t.InputJSON, &t.AttemptCount, &createdAt); err != nil {
return fmt.Errorf("read task for dead letter: %w", err)
}

_, err := txExec(d, tx, `
INSERT INTO dead_letter_tasks
  (task_id, workflow_id, node_id, agent, input_json, failed_reason,
   attempt_count, original_created, dead_lettered_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
t.TaskID, t.WorkflowID, t.NodeID, t.Agent, t.InputJSON,
reason, t.AttemptCount, createdAt, now(),
)
if err != nil {
return fmt.Errorf("insert dead letter: %w", err)
}

_, err = txExec(d, tx, `
UPDATE tasks SET status='failed', failed_reason=?, dead_lettered=1, updated_at=?
WHERE task_id=?`,
reason, now(), taskID,
)
return err
}

// RequeueExpiredTasks requeues tasks where the worker has not sent a heartbeat
// within timeout_sec seconds. Uses last_heartbeat if set, otherwise claimed_at.
func (d *DB) RequeueExpiredTasks() (int, error) {
res, err := d.exec(`
UPDATE tasks
SET status='pending', claimed_by=NULL, claimed_at=NULL,
    last_heartbeat=NULL, updated_at=?
WHERE status='claimed' AND dead_lettered=0
  AND attempt_count < max_attempts
  AND (
    (last_heartbeat IS NOT NULL AND
     datetime(last_heartbeat, '+' || timeout_sec || ' seconds') <= datetime('now'))
    OR
    (last_heartbeat IS NULL AND
     datetime(claimed_at,    '+' || timeout_sec || ' seconds') <= datetime('now'))
  )`,
now(),
)
if err != nil {
return 0, err
}
n, _ := res.RowsAffected()
return int(n), nil
}

// RequeueExpiredTasksWithDeadLetter requeues expired tasks and dead-letters
// those that have exhausted max_attempts.
func (d *DB) RequeueExpiredTasksWithDeadLetter() (requeued, deadLettered int, err error) {
// Find expired tasks
rows, err := d.query(`
SELECT task_id, attempt_count, max_attempts, workflow_id, node_id,
       agent, input_json, created_at
FROM tasks
WHERE status='claimed' AND dead_lettered=0
  AND (
    (last_heartbeat IS NOT NULL AND
     datetime(last_heartbeat, '+' || timeout_sec || ' seconds') <= datetime('now'))
    OR
    (last_heartbeat IS NULL AND
     datetime(claimed_at, '+' || timeout_sec || ' seconds') <= datetime('now'))
  )`)
if err != nil {
return 0, 0, err
}
defer rows.Close()

type expiredTask struct {
taskID       string
attemptCount int
maxAttempts  int
workflowID   string
nodeID       string
agent        string
inputJSON    string
createdAt    string
}
var expired []expiredTask
for rows.Next() {
var t expiredTask
if err := rows.Scan(&t.taskID, &t.attemptCount, &t.maxAttempts,
&t.workflowID, &t.nodeID, &t.agent, &t.inputJSON, &t.createdAt); err != nil {
return 0, 0, err
}
expired = append(expired, t)
}
rows.Close()

for _, t := range expired {
if t.attemptCount >= t.maxAttempts {
// Dead letter
dbErr := d.Tx(func(tx *sql.Tx) error {
_, err := txExec(d, tx, `
INSERT OR IGNORE INTO dead_letter_tasks
  (task_id, workflow_id, node_id, agent, input_json,
   failed_reason, attempt_count, original_created, dead_lettered_at)
VALUES (?, ?, ?, ?, ?, 'max_attempts_exceeded', ?, ?, ?)`,
t.taskID, t.workflowID, t.nodeID, t.agent, t.inputJSON,
t.attemptCount, t.createdAt, now(),
)
if err != nil {
return err
}
_, err = txExec(d, tx, `
UPDATE tasks SET status='failed', dead_lettered=1,
                 failed_reason='max_attempts_exceeded', updated_at=?
WHERE task_id=?`, now(), t.taskID)
return err
})
if dbErr == nil {
deadLettered++
}
} else {
// Requeue
_, dbErr := d.exec(`
UPDATE tasks SET status='pending', claimed_by=NULL,
                 claimed_at=NULL, last_heartbeat=NULL, updated_at=?
WHERE task_id=? AND status='claimed'`, now(), t.taskID)
if dbErr == nil {
requeued++
}
}
}
return requeued, deadLettered, nil
}

func (d *DB) GetTask(taskID string) (*Task, error) {
row := d.queryRow(`
SELECT task_id, workflow_id, node_id, agent, status, priority,
       priority_lane, input_json, output_json, policy_result_json,
       claimed_by, claimed_at, last_heartbeat, timeout_sec,
       attempt_count, max_attempts, dead_lettered,
       completed_at, failed_reason, created_at, updated_at
FROM tasks WHERE task_id=?`, taskID)
return scanTask(d, row)
}

func getTaskTx(d *DB, tx *sql.Tx, taskID string) (*Task, error) {
row := txQueryRow(d, tx, `
SELECT task_id, workflow_id, node_id, agent, status, priority,
       priority_lane, input_json, output_json, policy_result_json,
       claimed_by, claimed_at, last_heartbeat, timeout_sec,
       attempt_count, max_attempts, dead_lettered,
       completed_at, failed_reason, created_at, updated_at
FROM tasks WHERE task_id=?`, taskID)
return scanTask(d, row)
}

func (d *DB) ListTasks(workflowID, status string) ([]*Task, error) {
q := `SELECT task_id, workflow_id, node_id, agent, status, priority,
priority_lane, input_json, output_json, policy_result_json,
claimed_by, claimed_at, last_heartbeat, timeout_sec,
attempt_count, max_attempts, dead_lettered,
completed_at, failed_reason, created_at, updated_at
FROM tasks WHERE workflow_id=?`
args := []any{workflowID}
if status != "" {
q += ` AND status=?`
args = append(args, status)
}
q += ` ORDER BY created_at ASC`
rows, err := d.query(q, args...)
if err != nil {
return nil, err
}
defer rows.Close()
var out []*Task
for rows.Next() {
t, err := scanTask(d, rows)
if err != nil {
return nil, err
}
out = append(out, t)
}
return out, rows.Err()
}

func (d *DB) ListDeadLetterTasks(workflowID string) ([]*Task, error) {
q := `SELECT task_id, workflow_id, node_id, agent, '' as status, 0 as priority,
'normal' as priority_lane, input_json, '' as output_json, '' as policy_result_json,
'' as claimed_by, NULL as claimed_at, NULL as last_heartbeat, 0 as timeout_sec,
attempt_count, 0 as max_attempts, 1 as dead_lettered,
NULL as completed_at, failed_reason, original_created as created_at, dead_lettered_at as updated_at
FROM dead_letter_tasks`
args := []any{}
if workflowID != "" {
q += ` WHERE workflow_id=?`
args = append(args, workflowID)
}
rows, err := d.query(q, args...)
if err != nil {
return nil, err
}
defer rows.Close()
var out []*Task
for rows.Next() {
t, err := scanTask(d, rows)
if err != nil {
return nil, err
}
out = append(out, t)
}
return out, rows.Err()
}

type scanner interface {
Scan(dest ...any) error
}

func scanTask(d *DB, row scanner) (*Task, error) {
var t Task
var outputJSON, policyResultJSON, claimedBy, failedReason, priorityLane sql.NullString
var claimedAt, completedAt, lastHeartbeat sql.NullString
var deadLettered int
var createdAt, updatedAt string
err := row.Scan(
&t.TaskID, &t.WorkflowID, &t.NodeID, &t.Agent, &t.Status, &t.Priority,
&priorityLane, &t.InputJSON, &outputJSON, &policyResultJSON,
&claimedBy, &claimedAt, &lastHeartbeat, &t.TimeoutSec,
&t.AttemptCount, &t.MaxAttempts, &deadLettered,
&completedAt, &failedReason, &createdAt, &updatedAt,
)
if err == sql.ErrNoRows {
return nil, nil
}
if err != nil {
return nil, err
}
t.OutputJSON = outputJSON.String
t.PolicyResultJSON = policyResultJSON.String
t.ClaimedBy = claimedBy.String
t.FailedReason = failedReason.String
t.PriorityLane = priorityLane.String
t.DeadLettered = deadLettered == 1
if claimedAt.Valid {
ts, _ := time.Parse(time.RFC3339Nano, claimedAt.String)
t.ClaimedAt = &ts
}
if completedAt.Valid {
ts, _ := time.Parse(time.RFC3339Nano, completedAt.String)
t.CompletedAt = &ts
}
if lastHeartbeat.Valid {
ts, _ := time.Parse(time.RFC3339Nano, lastHeartbeat.String)
t.LastHeartbeat = &ts
}
t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
t.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
return &t, nil
}
