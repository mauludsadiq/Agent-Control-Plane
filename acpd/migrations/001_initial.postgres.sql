-- Agent Control Plane v0.3.0 Postgres schema
-- Postgres-specific version of 001_initial.sql
-- Run this instead of 001_initial.sql when using Postgres.

CREATE TABLE IF NOT EXISTS schema_migrations (
    version     TEXT    NOT NULL PRIMARY KEY,
    applied_at  TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS actors (
    actor_id               TEXT    NOT NULL PRIMARY KEY,
    api_key_hash           TEXT    NOT NULL UNIQUE,
    roles_json             TEXT    NOT NULL DEFAULT '[]',
    allowed_agents_json    TEXT,
    allowed_workflows_json TEXT,
    created_at             TEXT    NOT NULL,
    updated_at             TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_actors_api_key_hash ON actors(api_key_hash);

CREATE TABLE IF NOT EXISTS workflows (
    workflow_id  TEXT    NOT NULL PRIMARY KEY,
    goal         TEXT    NOT NULL,
    owner        TEXT    NOT NULL,
    stage        TEXT    NOT NULL DEFAULT 'created',
    state_hash   TEXT    NOT NULL,
    current_seq  INTEGER NOT NULL DEFAULT 0,
    snapshot_seq INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT    NOT NULL,
    updated_at   TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_workflows_stage      ON workflows(stage);
CREATE INDEX IF NOT EXISTS idx_workflows_updated_at ON workflows(updated_at);

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

CREATE TABLE IF NOT EXISTS deltas (
    workflow_id     TEXT    NOT NULL,
    seq             INTEGER NOT NULL,
    delta_json      TEXT    NOT NULL,
    delta_hash      TEXT    NOT NULL,
    prev_delta_hash TEXT,
    created_at      TEXT    NOT NULL,
    PRIMARY KEY (workflow_id, seq),
    FOREIGN KEY (workflow_id) REFERENCES workflows(workflow_id)
);

CREATE INDEX IF NOT EXISTS idx_deltas_workflow_seq ON deltas(workflow_id, seq ASC);

CREATE TABLE IF NOT EXISTS receipts (
    workflow_id         TEXT    NOT NULL,
    seq                 INTEGER NOT NULL,
    receipt_digest      TEXT    NOT NULL UNIQUE,
    receipt_json        TEXT    NOT NULL,
    prev_receipt_digest TEXT,
    created_at          TEXT    NOT NULL,
    PRIMARY KEY (workflow_id, seq),
    FOREIGN KEY (workflow_id) REFERENCES workflows(workflow_id)
);

CREATE INDEX IF NOT EXISTS idx_receipts_workflow_seq ON receipts(workflow_id, seq ASC);
CREATE INDEX IF NOT EXISTS idx_receipts_digest       ON receipts(receipt_digest);

CREATE TABLE IF NOT EXISTS artifacts (
    digest        TEXT NOT NULL PRIMARY KEY,
    workflow_id   TEXT NOT NULL,
    artifact_json TEXT NOT NULL,
    content_ref   TEXT,
    created_at    TEXT NOT NULL,
    FOREIGN KEY (workflow_id) REFERENCES workflows(workflow_id)
);

CREATE INDEX IF NOT EXISTS idx_artifacts_workflow ON artifacts(workflow_id);

CREATE TABLE IF NOT EXISTS gates (
    token       TEXT    NOT NULL PRIMARY KEY,
    workflow_id TEXT    NOT NULL,
    seq         INTEGER NOT NULL,
    status      TEXT    NOT NULL DEFAULT 'pending',
    gate_json   TEXT    NOT NULL,
    resolved_at TEXT,
    resolved_by TEXT,
    created_at  TEXT    NOT NULL,
    FOREIGN KEY (workflow_id) REFERENCES workflows(workflow_id)
);

CREATE INDEX IF NOT EXISTS idx_gates_workflow ON gates(workflow_id);
CREATE INDEX IF NOT EXISTS idx_gates_status   ON gates(status);

CREATE TABLE IF NOT EXISTS tasks (
    task_id            TEXT    NOT NULL PRIMARY KEY,
    workflow_id        TEXT    NOT NULL,
    node_id            TEXT    NOT NULL,
    agent              TEXT    NOT NULL,
    status             TEXT    NOT NULL DEFAULT 'pending',
    priority           INTEGER NOT NULL DEFAULT 0,
    input_json         TEXT    NOT NULL,
    output_json        TEXT,
    policy_result_json TEXT,
    claimed_by         TEXT,
    claimed_at         TEXT,
    timeout_sec        INTEGER NOT NULL DEFAULT 300,
    completed_at       TEXT,
    failed_reason      TEXT,
    created_at         TEXT    NOT NULL,
    updated_at         TEXT    NOT NULL,
    FOREIGN KEY (workflow_id) REFERENCES workflows(workflow_id)
);

CREATE INDEX IF NOT EXISTS idx_tasks_agent_status ON tasks(agent, status);
CREATE INDEX IF NOT EXISTS idx_tasks_workflow      ON tasks(workflow_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status        ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_claimed_at    ON tasks(claimed_at) WHERE status = 'claimed';

CREATE TABLE IF NOT EXISTS audit_events (
    id          BIGSERIAL PRIMARY KEY,
    workflow_id TEXT,
    actor_id    TEXT,
    event_type  TEXT NOT NULL,
    event_json  TEXT NOT NULL,
    digest      TEXT NOT NULL,
    prev_digest TEXT,
    created_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_workflow   ON audit_events(workflow_id);
CREATE INDEX IF NOT EXISTS idx_audit_actor      ON audit_events(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_event_type ON audit_events(event_type);
CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_events(created_at);

CREATE TABLE IF NOT EXISTS policy_versions (
    policy_version TEXT NOT NULL PRIMARY KEY,
    policy_json    TEXT NOT NULL,
    policy_digest  TEXT NOT NULL,
    created_at     TEXT NOT NULL
);

INSERT INTO schema_migrations (version, applied_at)
VALUES ('001_initial', NOW()::TEXT)
ON CONFLICT (version) DO NOTHING;
