-- v0.8.0 decision variants (Postgres)

CREATE TABLE IF NOT EXISTS branches (
    branch_id           TEXT NOT NULL PRIMARY KEY,
    parent_id           TEXT NOT NULL,
    branch_point_seq    INTEGER NOT NULL,
    branch_point_hash   TEXT NOT NULL,
    reason              TEXT NOT NULL,
    kind                TEXT NOT NULL DEFAULT 'fork',
    created_at          TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_branches_parent
    ON branches(parent_id, branch_point_seq);

CREATE INDEX IF NOT EXISTS idx_branches_kind
    ON branches(kind, created_at DESC);

INSERT INTO schema_migrations (version, applied_at)
VALUES ('004_branches', NOW()::TEXT)
ON CONFLICT (version) DO NOTHING;
