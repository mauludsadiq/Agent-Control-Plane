package store

import (
"database/sql"
"fmt"
"time"
)

type Workflow struct {
WorkflowID  string
Goal        string
Owner       string
Stage       string
StateHash   string
CurrentSeq  int
SnapshotSeq int
CreatedAt   time.Time
UpdatedAt   time.Time
}

type WorkflowSnapshot struct {
WorkflowID string
Seq        int
StateJSON  string
StateHash  string
CreatedAt  time.Time
}

func (d *DB) CreateWorkflow(wf *Workflow) error {
_, err := d.exec(`
INSERT INTO workflows (workflow_id, goal, owner, stage, state_hash, current_seq, snapshot_seq, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
wf.WorkflowID, wf.Goal, wf.Owner, wf.Stage, wf.StateHash,
wf.CurrentSeq, wf.SnapshotSeq, now(), now(),
)
if err != nil {
return fmt.Errorf("create workflow: %w", err)
}
return nil
}

func (d *DB) GetWorkflow(id string) (*Workflow, error) {
row := d.queryRow(`
SELECT workflow_id, goal, owner, stage, state_hash, current_seq, snapshot_seq, created_at, updated_at
FROM workflows WHERE workflow_id = ?`, id)
return scanWorkflow(row)
}

func (d *DB) UpdateWorkflow(tx *sql.Tx, wf *Workflow) error {
_, err := txExec(d, tx, `
UPDATE workflows SET stage=?, state_hash=?, current_seq=?, snapshot_seq=?, updated_at=?
WHERE workflow_id=?`,
wf.Stage, wf.StateHash, wf.CurrentSeq, wf.SnapshotSeq, now(), wf.WorkflowID,
)
return err
}

func (d *DB) ListWorkflows(stage string, limit int) ([]*Workflow, error) {
q := `SELECT workflow_id, goal, owner, stage, state_hash, current_seq, snapshot_seq, created_at, updated_at FROM workflows`
args := []any{}
if stage != "" {
q += ` WHERE stage = ?`
args = append(args, stage)
}
q += ` ORDER BY updated_at DESC`
if limit > 0 {
q += fmt.Sprintf(" LIMIT %d", limit)
}
rows, err := d.query(d.rebind(q), args...)
if err != nil {
return nil, err
}
defer rows.Close()
var out []*Workflow
for rows.Next() {
wf, err := scanWorkflow(rows)
if err != nil {
return nil, err
}
out = append(out, wf)
}
return out, rows.Err()
}

func (d *DB) SaveSnapshot(tx *sql.Tx, snap *WorkflowSnapshot) error {
_, err := txExec(d, tx, d.insertOrReplaceSnapshot(),
snap.WorkflowID, snap.Seq, snap.StateJSON, snap.StateHash, now(),
)
return err
}

func (d *DB) GetLatestSnapshot(workflowID string) (*WorkflowSnapshot, error) {
row := d.queryRow(`
SELECT workflow_id, seq, state_json, state_hash, created_at
FROM workflow_snapshots WHERE workflow_id = ?
ORDER BY seq DESC LIMIT 1`, workflowID)
var s WorkflowSnapshot
var createdAt string
err := row.Scan(&s.WorkflowID, &s.Seq, &s.StateJSON, &s.StateHash, &createdAt)
if err == sql.ErrNoRows {
return nil, nil
}
if err != nil {
return nil, err
}
s.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
return &s, nil
}

func (d *DB) GetSnapshotAtSeq(workflowID string, seq int) (*WorkflowSnapshot, error) {
row := d.queryRow(`
SELECT workflow_id, seq, state_json, state_hash, created_at
FROM workflow_snapshots WHERE workflow_id = ? AND seq <= ?
ORDER BY seq DESC LIMIT 1`, workflowID, seq)
var s WorkflowSnapshot
var createdAt string
err := row.Scan(&s.WorkflowID, &s.Seq, &s.StateJSON, &s.StateHash, &createdAt)
if err == sql.ErrNoRows {
return nil, nil
}
if err != nil {
return nil, err
}
s.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
return &s, nil
}

type scanner interface {
Scan(dest ...any) error
}

func scanWorkflow(row scanner) (*Workflow, error) {
var wf Workflow
var createdAt, updatedAt string
err := row.Scan(
&wf.WorkflowID, &wf.Goal, &wf.Owner, &wf.Stage,
&wf.StateHash, &wf.CurrentSeq, &wf.SnapshotSeq,
&createdAt, &updatedAt,
)
if err == sql.ErrNoRows {
return nil, nil
}
if err != nil {
return nil, err
}
wf.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
wf.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
return &wf, nil
}
