-- v0.3.0 performance indexes
-- Tuned for 10,000 concurrent workflow query patterns.
-- Safe to run against both SQLite and Postgres.

-- Workflow portfolio queries: list by stage + updated_at
CREATE INDEX IF NOT EXISTS idx_workflows_stage_updated
    ON workflows(stage, updated_at DESC);

-- Workflow portfolio queries: list by owner
CREATE INDEX IF NOT EXISTS idx_workflows_owner
    ON workflows(owner, updated_at DESC);

-- Receipt chain reconstruction: get all receipts for a workflow in order
-- Already covered by idx_receipts_workflow_seq but add covering index
-- for the chain verification query pattern (workflow_id, seq, receipt_digest)
CREATE INDEX IF NOT EXISTS idx_receipts_chain
    ON receipts(workflow_id, seq ASC, receipt_digest);

-- Delta reconstruction: get deltas after a snapshot seq
CREATE INDEX IF NOT EXISTS idx_deltas_since_seq
    ON deltas(workflow_id, seq ASC, delta_hash);

-- Snapshot lookup: find nearest snapshot at or before a target seq
CREATE INDEX IF NOT EXISTS idx_snapshots_nearest
    ON workflow_snapshots(workflow_id, seq DESC, state_hash);

-- Task queue: hot path for worker polling
-- Covers: agent + status + priority + created_at
CREATE INDEX IF NOT EXISTS idx_tasks_queue
    ON tasks(agent, status, priority DESC, created_at ASC);

-- Task queue: find expired claimed tasks for requeue
CREATE INDEX IF NOT EXISTS idx_tasks_expiry
    ON tasks(status, claimed_at, timeout_sec)
    WHERE status = 'claimed';

-- Gate inbox: operator inbox query (status=pending ordered by created_at)
CREATE INDEX IF NOT EXISTS idx_gates_inbox
    ON gates(status, created_at ASC)
    WHERE status = 'pending';

-- Audit log: time-range queries for compliance reporting
CREATE INDEX IF NOT EXISTS idx_audit_time_range
    ON audit_events(created_at ASC, workflow_id);

INSERT OR IGNORE INTO schema_migrations (version, applied_at)
VALUES ('002_indexes', datetime('now'));
