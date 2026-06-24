package store

import (
"database/sql"
"time"
)

type Task struct {
TaskID           string
WorkflowID       string
NodeID           string
Agent            string
Status           string
Priority         int
InputJSON        string
OutputJSON       string
PolicyResultJSON string
ClaimedBy        string
ClaimedAt        *time.Time
TimeoutSec       int
CompletedAt      *time.Time
FailedReason     string
CreatedAt        time.Time
UpdatedAt        time.Time
}

func (d *DB) EnqueueTask(tx *sql.Tx, t *Task) error {
_, err := tx.Exec(`
INSERT INTO tasks (task_id, workflow_id, node_id, agent, status, priority, input_json, timeout_sec, created_at, updated_at)
VALUES (?, ?, ?, ?, 'pending', ?, ?, ?, ?, ?)`,
t.TaskID, t.WorkflowID, t.NodeID, t.Agent, t.Priority,
t.InputJSON, t.TimeoutSec, now(), now(),
)
return err
}

func (d *DB) ClaimNextTask(agent string) (*Task, error) {
var t *Task
err := d.Tx(func(tx *sql.Tx) error {
row := tx.QueryRow(`
SELECT task_id, workflow_id, node_id, agent, status, priority, input_json,
       output_json, policy_result_json, claimed_by, claimed_at, timeout_sec,
       completed_at, failed_reason, created_at, updated_at
FROM tasks
WHERE agent = ? AND status = 'pending'
ORDER BY priority DESC, created_at ASC
LIMIT 1`, agent)
var err error
t, err = scanTask(row)
if err != nil || t == nil {
return err
}
_, err = tx.Exec(`
UPDATE tasks SET status='claimed', claimed_by=?, claimed_at=?, updated_at=?
WHERE task_id=? AND status='pending'`,
agent, now(), now(), t.TaskID,
)
return err
})
return t, err
}

func (d *DB) CompleteTask(tx *sql.Tx, taskID, outputJSON, policyResultJSON string) error {
_, err := tx.Exec(`
UPDATE tasks SET status='completed', output_json=?, policy_result_json=?, completed_at=?, updated_at=?
WHERE task_id=?`,
outputJSON, policyResultJSON, now(), now(), taskID,
)
return err
}

func (d *DB) FailTask(tx *sql.Tx, taskID, reason string) error {
_, err := tx.Exec(`
UPDATE tasks SET status='failed', failed_reason=?, updated_at=? WHERE task_id=?`,
reason, now(), taskID,
)
return err
}

func (d *DB) RequeueExpiredTasks() (int, error) {
res, err := d.sql.Exec(`
UPDATE tasks SET status='pending', claimed_by=NULL, claimed_at=NULL, updated_at=?
WHERE status='claimed'
AND datetime(claimed_at, '+' || timeout_sec || ' seconds') < datetime('now')`,
now(),
)
if err != nil {
return 0, err
}
n, _ := res.RowsAffected()
return int(n), nil
}

func (d *DB) GetTask(taskID string) (*Task, error) {
row := d.sql.QueryRow(`
SELECT task_id, workflow_id, node_id, agent, status, priority, input_json,
       output_json, policy_result_json, claimed_by, claimed_at, timeout_sec,
       completed_at, failed_reason, created_at, updated_at
FROM tasks WHERE task_id=?`, taskID)
return scanTask(row)
}

func (d *DB) ListTasks(workflowID, status string) ([]*Task, error) {
q := `SELECT task_id, workflow_id, node_id, agent, status, priority, input_json,
output_json, policy_result_json, claimed_by, claimed_at, timeout_sec,
completed_at, failed_reason, created_at, updated_at
FROM tasks WHERE workflow_id=?`
args := []any{workflowID}
if status != "" {
q += ` AND status=?`
args = append(args, status)
}
q += ` ORDER BY created_at ASC`
rows, err := d.sql.Query(q, args...)
if err != nil {
return nil, err
}
defer rows.Close()
var out []*Task
for rows.Next() {
t, err := scanTask(rows)
if err != nil {
return nil, err
}
out = append(out, t)
}
return out, rows.Err()
}

func scanTask(row scanner) (*Task, error) {
var t Task
var outputJSON, policyResultJSON, claimedBy, failedReason sql.NullString
var claimedAt, completedAt sql.NullString
var createdAt, updatedAt string
err := row.Scan(
&t.TaskID, &t.WorkflowID, &t.NodeID, &t.Agent, &t.Status, &t.Priority,
&t.InputJSON, &outputJSON, &policyResultJSON, &claimedBy, &claimedAt,
&t.TimeoutSec, &completedAt, &failedReason, &createdAt, &updatedAt,
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
if claimedAt.Valid {
ts, _ := time.Parse(time.RFC3339Nano, claimedAt.String)
t.ClaimedAt = &ts
}
if completedAt.Valid {
ts, _ := time.Parse(time.RFC3339Nano, completedAt.String)
t.CompletedAt = &ts
}
t.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
t.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
return &t, nil
}
