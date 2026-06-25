package store

import (
"database/sql"
"encoding/json"
"fmt"
"time"
)

// Anchor records an external chain anchor proof for a workflow.
type Anchor struct {
ProofID       string
WorkflowID    string
SeqFrom       int
SeqTo         int
ChainRoot     string
PayloadDigest string
ProofDigest   string
ExternalKind  string
ExternalRef   string // JSON
AnchoredBy    string
PayloadJSON   string
ProofJSON     string
CreatedAt     time.Time
}

func (d *DB) CreateAnchor(a *Anchor) error {
_, err := d.exec(`
INSERT INTO anchors
  (proof_id, workflow_id, seq_from, seq_to, chain_root,
   payload_digest, proof_digest, external_kind, external_ref,
   anchored_by, payload_json, proof_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
a.ProofID, a.WorkflowID, a.SeqFrom, a.SeqTo, a.ChainRoot,
a.PayloadDigest, a.ProofDigest, a.ExternalKind, a.ExternalRef,
a.AnchoredBy, a.PayloadJSON, a.ProofJSON, now(),
)
return err
}

func (d *DB) ListAnchors(workflowID string) ([]*Anchor, error) {
rows, err := d.query(`
SELECT proof_id, workflow_id, seq_from, seq_to, chain_root,
       payload_digest, proof_digest, external_kind, external_ref,
       anchored_by, payload_json, proof_json, created_at
FROM anchors WHERE workflow_id = ?
ORDER BY seq_from ASC`, workflowID)
if err != nil {
return nil, err
}
defer rows.Close()
var out []*Anchor
for rows.Next() {
a, err := scanAnchor(rows)
if err != nil {
return nil, err
}
out = append(out, a)
}
return out, rows.Err()
}

func (d *DB) GetAnchorGaps(workflowID string, maxSeq int) ([]map[string]int, error) {
anchors, err := d.ListAnchors(workflowID)
if err != nil {
return nil, err
}

// Build coverage bitmap
covered := make(map[int]bool)
for _, a := range anchors {
for seq := a.SeqFrom; seq <= a.SeqTo; seq++ {
covered[seq] = true
}
}

// Find gaps
var gaps []map[string]int
gapStart := -1
for seq := 1; seq <= maxSeq; seq++ {
if !covered[seq] {
if gapStart == -1 {
gapStart = seq
}
} else {
if gapStart != -1 {
gaps = append(gaps, map[string]int{"from": gapStart, "to": seq - 1})
gapStart = -1
}
}
}
if gapStart != -1 {
gaps = append(gaps, map[string]int{"from": gapStart, "to": maxSeq})
}
return gaps, nil
}

// AnchorSummary returns coverage stats for a workflow's anchor log.
func (d *DB) AnchorSummary(workflowID string) (map[string]any, error) {
anchors, err := d.ListAnchors(workflowID)
if err != nil {
return nil, err
}

snap, _ := d.GetLatestSnapshot(workflowID)
maxSeq := 0
if snap != nil {
var state struct{ Seq int `json:"seq"` }
if err := json.Unmarshal([]byte(snap.StateJSON), &state); err == nil {
maxSeq = state.Seq
}
}

gaps, _ := d.GetAnchorGaps(workflowID, maxSeq)
covered := 0
for _, a := range anchors {
covered += a.SeqTo - a.SeqFrom + 1
}

fullyAnchored := len(gaps) == 0 && maxSeq > 0
return map[string]any{
"workflow_id":    workflowID,
"proof_count":    len(anchors),
"max_seq":        maxSeq,
"covered_seqs":   covered,
"gap_count":      len(gaps),
"gaps":           gaps,
"fully_anchored": fullyAnchored,
}, nil
}

func scanAnchor(row scanner) (*Anchor, error) {
var a Anchor
var createdAt string
err := row.Scan(
&a.ProofID, &a.WorkflowID, &a.SeqFrom, &a.SeqTo, &a.ChainRoot,
&a.PayloadDigest, &a.ProofDigest, &a.ExternalKind, &a.ExternalRef,
&a.AnchoredBy, &a.PayloadJSON, &a.ProofJSON, &createdAt,
)
if err == sql.ErrNoRows {
return nil, nil
}
if err != nil {
return nil, fmt.Errorf("scan anchor: %w", err)
}
a.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
return &a, nil
}
