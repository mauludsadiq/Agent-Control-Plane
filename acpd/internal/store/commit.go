package store

import (
"crypto/sha256"
"database/sql"
"encoding/json"
"fmt"
)

// TransitionResult is the output of a FARD bridge transition call.
// Go receives this from bridge.Run and passes it to CommitTransition.
type TransitionResult struct {
OK            bool            `json:"ok"`
StateJSON     string          `json:"state_json"`
StateHash     string          `json:"state_hash"`
ReceiptJSON   string          `json:"receipt_json"`
ReceiptDigest string          `json:"receipt_digest"`
PolicyOK      bool            `json:"policy_ok"`
Violations    json.RawMessage `json:"violations"`
Seq           int             `json:"seq"`
Stage         string          `json:"stage"`
}

// CommitParams carries everything needed to commit one transition atomically.
type CommitParams struct {
WorkflowID    string
Result        *TransitionResult
Patches       []map[string]any // for delta construction
Kind          string           // transition kind
ActorID       string
SnapshotEvery int // take full snapshot every N transitions
}

// CommitTransition atomically writes:
//   - updated workflow row (stage, state_hash, current_seq, snapshot_seq)
//   - snapshot (if interval hit or seq == 1)
//   - delta record
//   - receipt record
//
// If any write fails the entire transaction rolls back.
// The store never holds a partially committed transition.
func (d *DB) CommitTransition(p *CommitParams) error {
return d.Tx(func(tx *sql.Tx) error {
// 1. Get current workflow state for prev hashes
var currentSeq, snapshotSeq int
var prevReceiptDigest, prevDeltaHash sql.NullString
err := txQueryRow(d, tx, `
SELECT current_seq, snapshot_seq FROM workflows WHERE workflow_id = ?`,
p.WorkflowID).Scan(&currentSeq, &snapshotSeq)
if err != nil {
return fmt.Errorf("get workflow for commit: %w", err)
}

// Get prev receipt digest for chain linking
err = txQueryRow(d, tx, `
SELECT receipt_digest FROM receipts WHERE workflow_id = ?
ORDER BY seq DESC LIMIT 1`, p.WorkflowID).Scan(&prevReceiptDigest)
if err != nil && err != sql.ErrNoRows {
return fmt.Errorf("get prev receipt: %w", err)
}

// Get prev delta hash for delta chain linking
err = txQueryRow(d, tx, `
SELECT delta_hash FROM deltas WHERE workflow_id = ?
ORDER BY seq DESC LIMIT 1`, p.WorkflowID).Scan(&prevDeltaHash)
if err != nil && err != sql.ErrNoRows {
return fmt.Errorf("get prev delta: %w", err)
}

seq := p.Result.Seq

// 2. Append receipt
_, err = txExec(d, tx, `
INSERT INTO receipts (workflow_id, seq, receipt_digest, receipt_json, prev_receipt_digest, created_at)
VALUES (?, ?, ?, ?, ?, ?)`,
p.WorkflowID, seq, p.Result.ReceiptDigest, p.Result.ReceiptJSON,
prevReceiptDigest, now(),
)
if err != nil {
return fmt.Errorf("insert receipt: %w", err)
}

// 3. Build and append delta
patchesJSON, _ := json.Marshal(p.Patches)
deltaBody := map[string]any{
"delta_type":         "acp.delta.v1",
"seq":                seq,
"kind":               p.Kind,
"actor":              p.ActorID,
"patches":            p.Patches,
"artifact_digests":   []string{},
"state_before_hash":  "", // populated from receipt
"state_after_hash":   p.Result.StateHash,
"receipt_digest":     p.Result.ReceiptDigest,
}
// Extract state_before_hash from receipt JSON
var receiptRecord map[string]any
if err := json.Unmarshal([]byte(p.Result.ReceiptJSON), &receiptRecord); err == nil {
if sbh, ok := receiptRecord["state_before_hash"].(string); ok {
deltaBody["state_before_hash"] = sbh
}
}
deltaBodyJSON, _ := json.Marshal(deltaBody)
deltaHash := hashBytes(deltaBodyJSON)
deltaJSON, _ := json.Marshal(map[string]any{
"delta":      deltaBody,
"delta_hash": deltaHash,
"patches":    string(patchesJSON),
})

_, err = txExec(d, tx, `
INSERT INTO deltas (workflow_id, seq, delta_json, delta_hash, prev_delta_hash, created_at)
VALUES (?, ?, ?, ?, ?, ?)`,
p.WorkflowID, seq, string(deltaJSON), deltaHash,
prevDeltaHash, now(),
)
if err != nil {
return fmt.Errorf("insert delta: %w", err)
}

// 4. Always save latest snapshot so GetLatestSnapshot returns current state.
// Additionally save a permanent snapshot at interval boundaries for
// efficient delta reconstruction at arbitrary seq.
_, err = txExec(d, tx, d.insertOrReplaceSnapshot(),
p.WorkflowID, seq, p.Result.StateJSON, p.Result.StateHash, now(),
)
if err != nil {
return fmt.Errorf("insert snapshot: %w", err)
}
newSnapshotSeq := seq

// 5. Update workflow summary row
stage := p.Result.Stage
if stage == "" {
// Parse stage from state JSON if not provided directly
var stateRecord map[string]any
if err := json.Unmarshal([]byte(p.Result.StateJSON), &stateRecord); err == nil {
if s, ok := stateRecord["stage"].(string); ok {
stage = s
}
}
}
_, err = txExec(d, tx, `
UPDATE workflows
SET stage=?, state_hash=?, current_seq=?, snapshot_seq=?, updated_at=?
WHERE workflow_id=?`,
stage, p.Result.StateHash, seq, newSnapshotSeq, now(), p.WorkflowID,
)
if err != nil {
return fmt.Errorf("update workflow: %w", err)
}

return nil
})
}

// CommitArtifact atomically writes an artifact and commits the state transition
// that recorded the artifact digest.
type ArtifactCommitParams struct {
WorkflowID    string
ArtifactJSON  string
ArtifactDigest string
Result        *TransitionResult
ActorID       string
SnapshotEvery int
}

func (d *DB) CommitArtifact(p *ArtifactCommitParams) error {
return d.Tx(func(tx *sql.Tx) error {
// Write artifact
_, err := txExec(d, tx, d.insertOrIgnoreArtifactNoRef(),
p.ArtifactDigest, p.WorkflowID, p.ArtifactJSON, now(),
)
if err != nil {
return fmt.Errorf("insert artifact: %w", err)
}

// Get prev receipt digest
var prevReceiptDigest, prevDeltaHash sql.NullString
var currentSeq, snapshotSeq int
_ = txQueryRow(d, tx, `SELECT current_seq, snapshot_seq FROM workflows WHERE workflow_id=?`,
p.WorkflowID).Scan(&currentSeq, &snapshotSeq)
_ = txQueryRow(d, tx, `SELECT receipt_digest FROM receipts WHERE workflow_id=? ORDER BY seq DESC LIMIT 1`,
p.WorkflowID).Scan(&prevReceiptDigest)
_ = txQueryRow(d, tx, `SELECT delta_hash FROM deltas WHERE workflow_id=? ORDER BY seq DESC LIMIT 1`,
p.WorkflowID).Scan(&prevDeltaHash)

seq := p.Result.Seq

// Write receipt
_, err = txExec(d, tx, `
INSERT INTO receipts (workflow_id, seq, receipt_digest, receipt_json, prev_receipt_digest, created_at)
VALUES (?, ?, ?, ?, ?, ?)`,
p.WorkflowID, seq, p.Result.ReceiptDigest, p.Result.ReceiptJSON,
prevReceiptDigest, now(),
)
if err != nil {
return fmt.Errorf("insert artifact receipt: %w", err)
}

// Write delta with artifact_digests
deltaBody := map[string]any{
"delta_type":        "acp.delta.v1",
"seq":               seq,
"kind":              "artifact_committed",
"actor":             p.ActorID,
"patches":           []any{},
"artifact_digests":  []string{p.ArtifactDigest},
"state_before_hash": "",
"state_after_hash":  p.Result.StateHash,
"receipt_digest":    p.Result.ReceiptDigest,
}
var receiptRecord map[string]any
if err := json.Unmarshal([]byte(p.Result.ReceiptJSON), &receiptRecord); err == nil {
if sbh, ok := receiptRecord["state_before_hash"].(string); ok {
deltaBody["state_before_hash"] = sbh
}
}
deltaBodyJSON, _ := json.Marshal(deltaBody)
deltaHash := hashBytes(deltaBodyJSON)
deltaJSON, _ := json.Marshal(map[string]any{"delta": deltaBody, "delta_hash": deltaHash})

_, err = txExec(d, tx, `
INSERT INTO deltas (workflow_id, seq, delta_json, delta_hash, prev_delta_hash, created_at)
VALUES (?, ?, ?, ?, ?, ?)`,
p.WorkflowID, seq, string(deltaJSON), deltaHash, prevDeltaHash, now(),
)
if err != nil {
return fmt.Errorf("insert artifact delta: %w", err)
}

// Snapshot if interval hit
_, err = txExec(d, tx, d.insertOrReplaceSnapshot(),
p.WorkflowID, seq, p.Result.StateJSON, p.Result.StateHash, now(),
)
if err != nil {
return fmt.Errorf("insert artifact snapshot: %w", err)
}
newSnapshotSeq := seq

// Parse stage from state
stage := p.Result.Stage
if stage == "" {
var stateRecord map[string]any
if err := json.Unmarshal([]byte(p.Result.StateJSON), &stateRecord); err == nil {
if s, ok := stateRecord["stage"].(string); ok {
stage = s
}
}
}

// Update workflow row
_, err = txExec(d, tx, `
UPDATE workflows SET stage=?, state_hash=?, current_seq=?, snapshot_seq=?, updated_at=?
WHERE workflow_id=?`,
stage, p.Result.StateHash, seq, newSnapshotSeq, now(), p.WorkflowID,
)
if err != nil {
return fmt.Errorf("update workflow after artifact: %w", err)
}

return nil
})
}

// CommitGateResolution atomically resolves a gate and commits the resulting
// state transition.
func (d *DB) CommitGateResolution(workflowID, token, resolution, resolvedBy string, result *TransitionResult, snapshotEvery int) error {
return d.Tx(func(tx *sql.Tx) error {
// Resolve gate
_, err := txExec(d, tx, `
UPDATE gates SET status=?, resolved_at=?, resolved_by=? WHERE token=?`,
resolution, now(), resolvedBy, token,
)
if err != nil {
return fmt.Errorf("resolve gate: %w", err)
}

if !result.OK {
// Rejected or policy blocked — still update workflow state
_, err = txExec(d, tx, `
UPDATE workflows SET stage=?, state_hash=?, current_seq=?, updated_at=?
WHERE workflow_id=?`,
result.Stage, result.StateHash, result.Seq, now(), workflowID,
)
return err
}

// Approved — full commit
var prevReceiptDigest, prevDeltaHash sql.NullString
var snapshotSeq int
_ = txQueryRow(d, tx, `SELECT snapshot_seq FROM workflows WHERE workflow_id=?`, workflowID).Scan(&snapshotSeq)
_ = txQueryRow(d, tx, `SELECT receipt_digest FROM receipts WHERE workflow_id=? ORDER BY seq DESC LIMIT 1`, workflowID).Scan(&prevReceiptDigest)
_ = txQueryRow(d, tx, `SELECT delta_hash FROM deltas WHERE workflow_id=? ORDER BY seq DESC LIMIT 1`, workflowID).Scan(&prevDeltaHash)

seq := result.Seq

_, err = txExec(d, tx, `
INSERT INTO receipts (workflow_id, seq, receipt_digest, receipt_json, prev_receipt_digest, created_at)
VALUES (?, ?, ?, ?, ?, ?)`,
workflowID, seq, result.ReceiptDigest, result.ReceiptJSON, prevReceiptDigest, now(),
)
if err != nil {
return fmt.Errorf("gate receipt: %w", err)
}

deltaBody := map[string]any{
"delta_type": "acp.delta.v1", "seq": seq,
"kind": "gate_resume", "actor": resolvedBy,
"patches": []any{}, "artifact_digests": []string{},
"state_after_hash": result.StateHash, "receipt_digest": result.ReceiptDigest,
}
deltaBodyJSON, _ := json.Marshal(deltaBody)
deltaHash := hashBytes(deltaBodyJSON)
deltaJSON, _ := json.Marshal(map[string]any{"delta": deltaBody, "delta_hash": deltaHash})

_, err = txExec(d, tx, `
INSERT INTO deltas (workflow_id, seq, delta_json, delta_hash, prev_delta_hash, created_at)
VALUES (?, ?, ?, ?, ?, ?)`,
workflowID, seq, string(deltaJSON), deltaHash, prevDeltaHash, now(),
)
if err != nil {
return fmt.Errorf("gate delta: %w", err)
}

_, err = txExec(d, tx, d.insertOrReplaceSnapshot(),
workflowID, seq, result.StateJSON, result.StateHash, now(),
)
if err != nil {
return fmt.Errorf("gate snapshot: %w", err)
}
newSnapshotSeq := seq

stage := result.Stage
if stage == "" {
var stateRecord map[string]any
if err := json.Unmarshal([]byte(result.StateJSON), &stateRecord); err == nil {
if s, ok := stateRecord["stage"].(string); ok {
stage = s
}
}
}

_, err = txExec(d, tx, `
UPDATE workflows SET stage=?, state_hash=?, current_seq=?, snapshot_seq=?, updated_at=?
WHERE workflow_id=?`,
stage, result.StateHash, seq, newSnapshotSeq, now(), workflowID,
)
return err
})
}

func hashBytes(b []byte) string {
h := sha256.Sum256(b)
return fmt.Sprintf("sha256:%x", h)
}
