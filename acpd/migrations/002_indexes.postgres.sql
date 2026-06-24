-- v0.3.0 performance indexes (Postgres)

CREATE INDEX IF NOT EXISTS idx_workflows_stage_updated
    ON workflows(stage, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_workflows_owner
    ON workflows(owner, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_receipts_chain
    ON receipts(workflow_id, seq ASC, receipt_digest);

CREATE INDEX IF NOT EXISTS idx_deltas_since_seq
    ON deltas(workflow_id, seq ASC, delta_hash);

CREATE INDEX IF NOT EXISTS idx_snapshots_nearest
    ON workflow_snapshots(workflow_id, seq DESC, state_hash);

CREATE INDEX IF NOT EXISTS idx_tasks_queue
    ON tasks(agent, status, priority DESC, created_at ASC);

CREATE INDEX IF NOT EXISTS idx_tasks_expiry
    ON tasks(status, claimed_at, timeout_sec)
    WHERE status = 'claimed';

CREATE INDEX IF NOT EXISTS idx_gates_inbox
    ON gates(status, created_at ASC)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_audit_time_range
    ON audit_events(created_at ASC, workflow_id);

INSERT INTO schema_migrations (version, applied_at)
VALUES ('002_indexes', NOW()::TEXT)
ON CONFLICT (version) DO NOTHING;
