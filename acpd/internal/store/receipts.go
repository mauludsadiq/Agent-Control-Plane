package store

import (
"database/sql"
"time"
)

type Receipt struct {
WorkflowID         string
Seq                int
ReceiptDigest      string
ReceiptJSON        string
PrevReceiptDigest  string
CreatedAt          time.Time
}

func (d *DB) AppendReceipt(tx *sql.Tx, r *Receipt) error {
_, err := txExec(d, tx, `
INSERT INTO receipts (workflow_id, seq, receipt_digest, receipt_json, prev_receipt_digest, created_at)
VALUES (?, ?, ?, ?, ?, ?)`,
r.WorkflowID, r.Seq, r.ReceiptDigest, r.ReceiptJSON,
nullString(r.PrevReceiptDigest), now(),
)
return err
}

func (d *DB) GetReceipts(workflowID string) ([]*Receipt, error) {
rows, err := d.query(`
SELECT workflow_id, seq, receipt_digest, receipt_json, prev_receipt_digest, created_at
FROM receipts WHERE workflow_id = ? ORDER BY seq ASC`, workflowID)
if err != nil {
return nil, err
}
defer rows.Close()
return scanReceipts(rows)
}

func (d *DB) GetReceiptsSince(workflowID string, fromSeq int) ([]*Receipt, error) {
rows, err := d.query(`
SELECT workflow_id, seq, receipt_digest, receipt_json, prev_receipt_digest, created_at
FROM receipts WHERE workflow_id = ? AND seq >= ? ORDER BY seq ASC`,
workflowID, fromSeq)
if err != nil {
return nil, err
}
defer rows.Close()
return scanReceipts(rows)
}

func (d *DB) GetLatestReceiptDigest(workflowID string) (string, error) {
var digest string
err := d.queryRow(`
SELECT receipt_digest FROM receipts WHERE workflow_id = ?
ORDER BY seq DESC LIMIT 1`, workflowID).Scan(&digest)
if err == sql.ErrNoRows {
return "", nil
}
return digest, err
}

func scanReceipts(rows *sql.Rows) ([]*Receipt, error) {
var out []*Receipt
for rows.Next() {
var r Receipt
var prevDigest sql.NullString
var createdAt string
if err := rows.Scan(&r.WorkflowID, &r.Seq, &r.ReceiptDigest, &r.ReceiptJSON, &prevDigest, &createdAt); err != nil {
return nil, err
}
r.PrevReceiptDigest = prevDigest.String
r.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
out = append(out, &r)
}
return out, rows.Err()
}
