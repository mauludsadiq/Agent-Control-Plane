-- v0.4.0 worker hardening (Postgres)

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS attempt_count  INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS max_attempts   INTEGER NOT NULL DEFAULT 3;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS last_heartbeat TEXT;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS dead_lettered  INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS priority_lane  TEXT    NOT NULL DEFAULT 'normal';

CREATE TABLE IF NOT EXISTS dead_letter_tasks (
    task_id           TEXT    NOT NULL PRIMARY KEY,
    workflow_id       TEXT    NOT NULL,
    node_id           TEXT    NOT NULL,
    agent             TEXT    NOT NULL,
    input_json        TEXT    NOT NULL,
    failed_reason     TEXT,
    attempt_count     INTEGER NOT NULL DEFAULT 0,
    original_created  TEXT    NOT NULL,
    dead_lettered_at  TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_dead_letter_workflow ON dead_letter_tasks(workflow_id);
CREATE INDEX IF NOT EXISTS idx_dead_letter_agent    ON dead_letter_tasks(agent);

CREATE TABLE IF NOT EXISTS workers (
    worker_id       TEXT NOT NULL PRIMARY KEY,
    agent           TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'active',
    last_heartbeat  TEXT NOT NULL,
    current_task_id TEXT,
    registered_at   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_workers_agent  ON workers(agent);
CREATE INDEX IF NOT EXISTS idx_workers_status ON workers(status);

CREATE INDEX IF NOT EXISTS idx_tasks_heartbeat
    ON tasks(last_heartbeat, status)
    WHERE status = 'claimed';

CREATE INDEX IF NOT EXISTS idx_tasks_priority_lane
    ON tasks(agent, status, priority_lane, priority DESC, created_at ASC);

INSERT INTO schema_migrations (version, applied_at)
VALUES ('003_worker_hardening', NOW()::TEXT)
ON CONFLICT (version) DO NOTHING;
