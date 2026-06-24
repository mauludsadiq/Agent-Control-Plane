package store

import (
"database/sql"
"time"
)

type Delta struct {
WorkflowID    string
Seq           int
DeltaJSON     string
DeltaHash     string
PrevDeltaHash string
CreatedAt     time.Time
}

func (d *DB) AppendDelta(tx *sql.Tx, delta *Delta) error {
_, err := tx.Exec(`
INSERT INTO deltas (workflow_id, seq, delta_json, delta_hash, prev_delta_hash, created_at)
VALUES (?, ?, ?, ?, ?, ?)`,
delta.WorkflowID, delta.Seq, delta.DeltaJSON, delta.DeltaHash,
nullString(delta.PrevDeltaHash), now(),
)
return err
}

func (d *DB) GetDeltasSince(workflowID string, fromSeq int) ([]*Delta, error) {
rows, err := d.sql.Query(`
SELECT workflow_id, seq, delta_json, delta_hash, prev_delta_hash, created_at
FROM deltas WHERE workflow_id = ? AND seq > ?
ORDER BY seq ASC`, workflowID, fromSeq)
if err != nil {
return nil, err
}
defer rows.Close()
var out []*Delta
for rows.Next() {
var delta Delta
var prevHash sql.NullString
var createdAt string
if err := rows.Scan(&delta.WorkflowID, &delta.Seq, &delta.DeltaJSON,
&delta.DeltaHash, &prevHash, &createdAt); err != nil {
return nil, err
}
delta.PrevDeltaHash = prevHash.String
delta.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
out = append(out, &delta)
}
return out, rows.Err()
}

func (d *DB) GetLatestDeltaHash(workflowID string) (string, error) {
var hash sql.NullString
err := d.sql.QueryRow(`
SELECT delta_hash FROM deltas WHERE workflow_id = ?
ORDER BY seq DESC LIMIT 1`, workflowID).Scan(&hash)
if err == sql.ErrNoRows {
return "", nil
}
return hash.String, err
}
