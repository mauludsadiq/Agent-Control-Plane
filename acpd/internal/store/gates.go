package store

import (
"database/sql"
"time"
)

type Gate struct {
Token      string
WorkflowID string
Seq        int
Status     string
GateJSON   string
ResolvedAt *time.Time
ResolvedBy string
CreatedAt  time.Time
}

func (d *DB) CreateGate(tx *sql.Tx, g *Gate) error {
_, err := tx.Exec(`
INSERT INTO gates (token, workflow_id, seq, status, gate_json, created_at)
VALUES (?, ?, ?, ?, ?, ?)`,
g.Token, g.WorkflowID, g.Seq, g.Status, g.GateJSON, now(),
)
return err
}

func (d *DB) GetGate(token string) (*Gate, error) {
row := d.sql.QueryRow(`
SELECT token, workflow_id, seq, status, gate_json, resolved_at, resolved_by, created_at
FROM gates WHERE token = ?`, token)
return scanGate(row)
}

func (d *DB) GetPendingGate(workflowID string) (*Gate, error) {
row := d.sql.QueryRow(`
SELECT token, workflow_id, seq, status, gate_json, resolved_at, resolved_by, created_at
FROM gates WHERE workflow_id = ? AND status = 'pending'
ORDER BY created_at DESC LIMIT 1`, workflowID)
return scanGate(row)
}

func (d *DB) ListPendingGates() ([]*Gate, error) {
rows, err := d.sql.Query(`
SELECT token, workflow_id, seq, status, gate_json, resolved_at, resolved_by, created_at
FROM gates WHERE status = 'pending' ORDER BY created_at ASC`)
if err != nil {
return nil, err
}
defer rows.Close()
var out []*Gate
for rows.Next() {
g, err := scanGate(rows)
if err != nil {
return nil, err
}
out = append(out, g)
}
return out, rows.Err()
}

func (d *DB) ResolveGate(tx *sql.Tx, token, status, resolvedBy string) error {
_, err := tx.Exec(`
UPDATE gates SET status=?, resolved_at=?, resolved_by=? WHERE token=?`,
status, now(), resolvedBy, token,
)
return err
}

func scanGate(row scanner) (*Gate, error) {
var g Gate
var resolvedAt sql.NullString
var resolvedBy sql.NullString
var createdAt string
err := row.Scan(&g.Token, &g.WorkflowID, &g.Seq, &g.Status, &g.GateJSON,
&resolvedAt, &resolvedBy, &createdAt)
if err == sql.ErrNoRows {
return nil, nil
}
if err != nil {
return nil, err
}
if resolvedAt.Valid {
t, _ := time.Parse(time.RFC3339Nano, resolvedAt.String)
g.ResolvedAt = &t
}
g.ResolvedBy = resolvedBy.String
g.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
return &g, nil
}
