package store

import (
"database/sql"
"time"
)

type Artifact struct {
Digest       string
WorkflowID   string
ArtifactJSON string
ContentRef   string
CreatedAt    time.Time
}

func (d *DB) SaveArtifact(tx *sql.Tx, a *Artifact) error {
_, err := tx.Exec(`
INSERT OR IGNORE INTO artifacts (digest, workflow_id, artifact_json, content_ref, created_at)
VALUES (?, ?, ?, ?, ?)`,
a.Digest, a.WorkflowID, a.ArtifactJSON, nullString(a.ContentRef), now(),
)
return err
}

func (d *DB) GetArtifact(digest string) (*Artifact, error) {
row := d.sql.QueryRow(`
SELECT digest, workflow_id, artifact_json, content_ref, created_at
FROM artifacts WHERE digest = ?`, digest)
var a Artifact
var contentRef sql.NullString
var createdAt string
err := row.Scan(&a.Digest, &a.WorkflowID, &a.ArtifactJSON, &contentRef, &createdAt)
if err == sql.ErrNoRows {
return nil, nil
}
if err != nil {
return nil, err
}
a.ContentRef = contentRef.String
a.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
return &a, nil
}

func (d *DB) ListArtifacts(workflowID string) ([]*Artifact, error) {
rows, err := d.sql.Query(`
SELECT digest, workflow_id, artifact_json, content_ref, created_at
FROM artifacts WHERE workflow_id = ? ORDER BY created_at ASC`, workflowID)
if err != nil {
return nil, err
}
defer rows.Close()
var out []*Artifact
for rows.Next() {
var a Artifact
var contentRef sql.NullString
var createdAt string
if err := rows.Scan(&a.Digest, &a.WorkflowID, &a.ArtifactJSON, &contentRef, &createdAt); err != nil {
return nil, err
}
a.ContentRef = contentRef.String
a.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
out = append(out, &a)
}
return out, rows.Err()
}
