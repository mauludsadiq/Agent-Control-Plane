package store

import (
"database/sql"
	"encoding/json"
	"fmt"
"strings"
"time"
)

type Branch struct {
BranchID         string
ParentID         string
BranchPointSeq   int
BranchPointHash  string
Reason           string
Kind             string // "fork" | "counterfactual"
CreatedAt        time.Time
}

func (d *DB) CreateBranch(tx *sql.Tx, b *Branch) error {
kind := b.Kind
if kind == "" {
kind = "fork"
}
_, err := txExec(d, tx, `
INSERT INTO branches
  (branch_id, parent_id, branch_point_seq, branch_point_hash, reason, kind, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
b.BranchID, b.ParentID, b.BranchPointSeq, b.BranchPointHash,
b.Reason, kind, now(),
)
return err
}

func (d *DB) GetBranch(branchID string) (*Branch, error) {
row := d.queryRow(`
SELECT branch_id, parent_id, branch_point_seq, branch_point_hash,
       reason, kind, created_at
FROM branches WHERE branch_id = ?`, branchID)
return scanBranch(row)
}

func (d *DB) ListBranches(parentID string) ([]*Branch, error) {
rows, err := d.query(`
SELECT branch_id, parent_id, branch_point_seq, branch_point_hash,
       reason, kind, created_at
FROM branches WHERE parent_id = ?
ORDER BY branch_point_seq ASC, created_at ASC`, parentID)
if err != nil {
return nil, err
}
defer rows.Close()
var out []*Branch
for rows.Next() {
b, err := scanBranch(rows)
if err != nil {
return nil, err
}
out = append(out, b)
}
return out, rows.Err()
}

func (d *DB) CountBranches(parentID string) (int, error) {
var n int
row := d.queryRow(`SELECT COUNT(*) FROM branches WHERE parent_id = ?`, parentID)
err := row.Scan(&n)
return n, err
}

func scanBranch(row scanner) (*Branch, error) {
var b Branch
var createdAt string
err := row.Scan(
&b.BranchID, &b.ParentID, &b.BranchPointSeq, &b.BranchPointHash,
&b.Reason, &b.Kind, &createdAt,
)
if err == sql.ErrNoRows {
return nil, nil
}
if err != nil {
return nil, err
}
b.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
return &b, nil
}

// ForkParams describes a fork request.
type ForkParams struct {
ParentWorkflowID string
NewWorkflowID    string
BranchPointSeq   int
Reason           string
Kind             string // "fork" | "counterfactual"
}

// ForkResult is returned by CommitFork.
type ForkResult struct {
BranchID        string
NewWorkflowID   string
BranchPointSeq  int
BranchPointHash string
}

// CommitFork creates a forked workflow from a parent at a given seq.
// forkedStateJSON and forkedStateHash come from the FARD bridge (done by caller).
func (d *DB) CommitFork(p *ForkParams, forkedStateJSON, forkedStateHash string) (*ForkResult, error) {
snap, err := d.getSnapshotAtSeq(p.ParentWorkflowID, p.BranchPointSeq)
if err != nil || snap == nil {
return nil, fmt.Errorf("get parent snapshot at seq %d: %w", p.BranchPointSeq, err)
}
parent, err := d.GetWorkflow(p.ParentWorkflowID)
if err != nil || parent == nil {
return nil, fmt.Errorf("get parent workflow: %w", err)
}

kind := p.Kind
if kind == "" {
kind = "fork"
}

forkedStateJSON = strings.Replace(forkedStateJSON, `"lineage":null`, `"lineage":{}`, 1)

err = d.Tx(func(tx *sql.Tx) error {
_, err := tx.Exec(d.rebind(`
INSERT INTO workflows
  (workflow_id, goal, owner, stage, state_hash, current_seq, snapshot_seq, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
p.NewWorkflowID,
parent.Goal + " [" + kind + ": " + p.Reason + "]",
parent.Owner,
parent.Stage,
forkedStateHash,
0,
0,
now(),
now(),
)
if err != nil {
return fmt.Errorf("create forked workflow: %w", err)
}
if err := d.SaveSnapshot(tx, &WorkflowSnapshot{
WorkflowID: p.NewWorkflowID,
Seq:        0,
StateJSON:  forkedStateJSON,
StateHash:  forkedStateHash,
}); err != nil {
return fmt.Errorf("save fork snapshot: %w", err)
}
if err := d.CreateBranch(tx, &Branch{
BranchID:        p.NewWorkflowID,
ParentID:        p.ParentWorkflowID,
BranchPointSeq:  p.BranchPointSeq,
BranchPointHash: snap.StateHash,
Reason:          p.Reason,
Kind:            kind,
}); err != nil {
return fmt.Errorf("create branch record: %w", err)
}
return nil
})
if err != nil {
return nil, err
}
return &ForkResult{
BranchID:        p.NewWorkflowID,
NewWorkflowID:   p.NewWorkflowID,
BranchPointSeq:  p.BranchPointSeq,
BranchPointHash: snap.StateHash,
}, nil
}

// getSnapshotAtSeq returns the snapshot at or before a given seq.
func (d *DB) getSnapshotAtSeq(workflowID string, seq int) (*WorkflowSnapshot, error) {
row := d.queryRow(`
SELECT workflow_id, seq, state_json, state_hash, created_at
FROM workflow_snapshots
WHERE workflow_id = ? AND seq <= ?
ORDER BY seq DESC LIMIT 1`, workflowID, seq)
var snap WorkflowSnapshot
var createdAt string
err := row.Scan(&snap.WorkflowID, &snap.Seq, &snap.StateJSON, &snap.StateHash, &createdAt)
if err == sql.ErrNoRows {
return nil, nil
}
if err != nil {
return nil, err
}
return &snap, nil
}

// stripLineageFromJSON removes the lineage field from state JSON
// to prevent FARD spread errors when forking a fork.
func stripLineageFromJSON(stateJSON string) string {
var state map[string]any
if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
return stateJSON
}
delete(state, "lineage")
clean, err := json.Marshal(state)
if err != nil {
return stateJSON
}
return string(clean)
}
