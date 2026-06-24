-- Agent Control Plane v0.2.0 schema
-- SQLite-first, Postgres-compatible.
-- No SQLite-specific types. INTEGER PRIMARY KEY only for rowid optimization.
-- All digests are sha256: prefixed text. All JSON is valid JSON text.
-- Timestamps are ISO 8601 UTC text for portability.

-- ─── ACTORS ───────────────────────────────────────────────────────────────────
-- API key auth. Keys are stored as SHA-256 hashes — never plaintext.
-- roles_json: ["operator","manager"] etc.
-- allowed_agents_json: null means no restriction, ["research_agent"] restricts.
-- allowed_workflows_json: null means all workflows.

CREATE TABLE IF NOT EXISTS actors (
   actor_id              TEXT    NOT NULL PRIMARY KEY,
   api_key_hash          TEXT    NOT NULL UNIQUE,
   roles_json            TEXT    NOT NULL DEFAULT '[]',
   allowed_agents_json   TEXT,
   allowed_workflows_json TEXT,
   created_at            TEXT    NOT NULL,
   updated_at            TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_actors_api_key_hash ON actors(api_key_hash);

-- ─── WORKFLOWS ────────────────────────────────────────────────────────────────
-- Summary row updated on every transition. Canonical state lives in snapshots.
-- state_hash matches receipt.state_after_hash for the latest committed seq.
-- snapshot_seq: seq of the most recent full snapshot (for delta reconstruction).

CREATE TABLE IF NOT EXISTS workflows (
   workflow_id     TEXT    NOT NULL PRIMARY KEY,
   goal            TEXT    NOT NULL,
   owner           TEXT    NOT NULL,
   stage           TEXT    NOT NULL DEFAULT 'created',
   state_hash      TEXT    NOT NULL,
   current_seq     INTEGER NOT NULL DEFAULT 0,
   snapshot_seq    INTEGER NOT NULL DEFAULT 0,
   created_at      TEXT    NOT NULL,
   updated_at      TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_workflows_stage ON workflows(stage);
CREATE INDEX IF NOT EXISTS idx_workflows_updated_at ON workflows(updated_at);

-- ─── WORKFLOW SNAPSHOTS ───────────────────────────────────────────────────────
-- Full state JSON stored every SNAPSHOT_INTERVAL transitions (default: 10).
-- state_hash must equal receipt.digest(state_json).
-- delta.reconstruct_at uses the nearest snapshot as baseline.

CREATE TABLE IF NOT EXISTS workflow_snapshots (
   workflow_id TEXT    NOT NULL,
   seq         INTEGER NOT NULL,
   state_json  TEXT    NOT NULL,
   state_hash  TEXT    NOT NULL,
   created_at  TEXT    NOT NULL,
   PRIMARY KEY (workflow_id, seq),
   FOREIGN KEY (workflow_id) REFERENCES workflows(workflow_id)
);

CREATE INDEX IF NOT EXISTS idx_snapshots_workflow_seq ON workflow_snapshots(workflow_id, seq DESC);

-- ─── DELTAS ───────────────────────────────────────────────────────────────────
-- One row per transition between snapshots.
-- delta_json matches acp.delta.v1 schema from delta.fard.
-- prev_delta_hash: delta_hash of the previous row (null for first delta after snapshot).
-- Forms a linked hash list between snapshots.

CREATE TABLE IF NOT EXISTS deltas (
   workflow_id      TEXT    NOT NULL,
   seq              INTEGER NOT NULL,
   delta_json       TEXT    NOT NULL,
   delta_hash       TEXT    NOT NULL,
   prev_delta_hash  TEXT,
   created_at       TEXT    NOT NULL,
   PRIMARY KEY (workflow_id, seq),
   FOREIGN KEY (workflow_id) REFERENCES workflows(workflow_id)
);

CREATE INDEX IF NOT EXISTS idx_deltas_workflow_seq ON deltas(workflow_id, seq ASC);

-- ─── RECEIPTS ─────────────────────────────────────────────────────────────────
-- One row per committed transition. Forms the cryptographic receipt chain.
-- receipt_json matches acp.state_transition.v1 schema from receipt.fard.
-- prev_receipt_digest: receipt_digest of the previous row (null for seq 1).
-- chain_root is recomputed on demand via receipt.chain_root(receipts).

CREATE TABLE IF NOT EXISTS receipts (
   workflow_id          TEXT    NOT NULL,
   seq                  INTEGER NOT NULL,
   receipt_digest       TEXT    NOT NULL UNIQUE,
   receipt_json         TEXT    NOT NULL,
   prev_receipt_digest  TEXT,
   created_at           TEXT    NOT NULL,
   PRIMARY KEY (workflow_id, seq),
   FOREIGN KEY (workflow_id) REFERENCES workflows(workflow_id)
);

CREATE INDEX IF NOT EXISTS idx_receipts_workflow_seq ON receipts(workflow_id, seq ASC);
CREATE INDEX IF NOT EXISTS idx_receipts_digest ON receipts(receipt_digest);

-- ─── ARTIFACTS ────────────────────────────────────────────────────────────────
-- artifact_json matches acp.artifact.v1 schema from artifact.fard.
-- Content capped at 1MB for SQLite. Production uses external object store.
-- content_ref: optional external storage reference (S3 key, etc.)

CREATE TABLE IF NOT EXISTS artifacts (
   digest        TEXT    NOT NULL PRIMARY KEY,
   workflow_id   TEXT    NOT NULL,
   artifact_json TEXT    NOT NULL,
   content_ref   TEXT,
   created_at    TEXT    NOT NULL,
   FOREIGN KEY (workflow_id) REFERENCES workflows(workflow_id)
);

CREATE INDEX IF NOT EXISTS idx_artifacts_workflow ON artifacts(workflow_id);

-- ─── GATES ────────────────────────────────────────────────────────────────────
-- One row per open gate. Status: pending | approved | rejected | expired.
-- gate_json matches acp.gate.v1 schema from gate.fard.
-- token is the sha256: digest used to resume. Stored plaintext (it's public).
-- resolved_at: set when status transitions out of pending.
-- resolved_by: actor_id who resolved the gate.

CREATE TABLE IF NOT EXISTS gates (
   token         TEXT    NOT NULL PRIMARY KEY,
   workflow_id   TEXT    NOT NULL,
   seq           INTEGER NOT NULL,
   status        TEXT    NOT NULL DEFAULT 'pending',
   gate_json     TEXT    NOT NULL,
   resolved_at   TEXT,
   resolved_by   TEXT,
   created_at    TEXT    NOT NULL,
   FOREIGN KEY (workflow_id) REFERENCES workflows(workflow_id)
);

CREATE INDEX IF NOT EXISTS idx_gates_workflow ON gates(workflow_id);
CREATE INDEX IF NOT EXISTS idx_gates_status ON gates(status);

-- ─── TASKS ────────────────────────────────────────────────────────────────────
-- Pull-based worker queue. Workers poll GET /tasks/next?agent=<agent>.
-- status: pending | claimed | completed | failed | cancelled
-- claimed_at + timeout_sec: background goroutine requeues expired claims.
-- input_json: agent_worker_input record (task_id, state_hash, allowed_tools, etc.)
-- output_json: agent_worker_output record after completion.
-- policy_result_json: policy check result at commit time.

CREATE TABLE IF NOT EXISTS tasks (
   task_id             TEXT    NOT NULL PRIMARY KEY,
   workflow_id         TEXT    NOT NULL,
   node_id             TEXT    NOT NULL,
   agent               TEXT    NOT NULL,
   status              TEXT    NOT NULL DEFAULT 'pending',
   priority            INTEGER NOT NULL DEFAULT 0,
   input_json          TEXT    NOT NULL,
   output_json         TEXT,
   policy_result_json  TEXT,
   claimed_by          TEXT,
   claimed_at          TEXT,
   timeout_sec         INTEGER NOT NULL DEFAULT 300,
   completed_at        TEXT,
   failed_reason       TEXT,
   created_at          TEXT    NOT NULL,
   updated_at          TEXT    NOT NULL,
   FOREIGN KEY (workflow_id) REFERENCES workflows(workflow_id)
);

CREATE INDEX IF NOT EXISTS idx_tasks_agent_status ON tasks(agent, status);
CREATE INDEX IF NOT EXISTS idx_tasks_workflow ON tasks(workflow_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_claimed_at ON tasks(claimed_at) WHERE status = 'claimed';

-- ─── AUDIT EVENTS ─────────────────────────────────────────────────────────────
-- Append-only log of all API-level events. Separate from timeline events
-- (which are FARD-layer). Audit events are Go-layer: auth, errors, admin ops.
-- event_json: arbitrary JSON payload per event_type.
-- digest: sha256 of (prev_digest + event_json) — linked audit log.

CREATE TABLE IF NOT EXISTS audit_events (
   id           INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
   workflow_id  TEXT,
   actor_id     TEXT,
   event_type   TEXT    NOT NULL,
   event_json   TEXT    NOT NULL,
   digest       TEXT    NOT NULL,
   prev_digest  TEXT,
   created_at   TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_workflow ON audit_events(workflow_id);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_events(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_event_type ON audit_events(event_type);
CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_events(created_at);

-- ─── POLICY CACHE ─────────────────────────────────────────────────────────────
-- Cached policy records by version. Policy is re-evaluated from FARD on version
-- mismatch. Go never reimplements policy logic — always calls FARD bridge.

CREATE TABLE IF NOT EXISTS policy_versions (
   policy_version  TEXT    NOT NULL PRIMARY KEY,
   policy_json     TEXT    NOT NULL,
   policy_digest   TEXT    NOT NULL,
   created_at      TEXT    NOT NULL
);

-- ─── SCHEMA VERSION ───────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS schema_migrations (
   version     TEXT    NOT NULL PRIMARY KEY,
   applied_at  TEXT    NOT NULL
);

INSERT OR IGNORE INTO schema_migrations (version, applied_at)
VALUES ('001_initial', datetime('now'));
