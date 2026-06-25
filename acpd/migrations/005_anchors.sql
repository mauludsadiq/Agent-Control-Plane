-- v0.9.5 external chain anchoring
CREATE TABLE IF NOT EXISTS anchors (
    proof_id        TEXT    NOT NULL PRIMARY KEY,
    workflow_id     TEXT    NOT NULL,
    seq_from        INTEGER NOT NULL,
    seq_to          INTEGER NOT NULL,
    chain_root      TEXT    NOT NULL,
    payload_digest  TEXT    NOT NULL,
    proof_digest    TEXT    NOT NULL,
    external_kind   TEXT    NOT NULL, -- ethereum | rfc3161 | ct_log | local
    external_ref    TEXT    NOT NULL, -- JSON
    anchored_by     TEXT    NOT NULL,
    payload_json    TEXT    NOT NULL,
    proof_json      TEXT    NOT NULL,
    created_at      TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_anchors_workflow
    ON anchors(workflow_id, seq_from, seq_to);

CREATE INDEX IF NOT EXISTS idx_anchors_kind
    ON anchors(external_kind, created_at DESC);

INSERT OR IGNORE INTO schema_migrations (version, applied_at)
VALUES ('005_anchors', datetime('now'));
