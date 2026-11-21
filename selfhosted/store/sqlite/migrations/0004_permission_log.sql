CREATE TABLE IF NOT EXISTS permission_head (
    tenant_id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS permission_changes (
    tenant_id TEXT NOT NULL,
    revision INTEGER NOT NULL,
    seq INTEGER NOT NULL,
    at TEXT NOT NULL,
    actor TEXT NOT NULL,
    op TEXT NOT NULL,
    kind TEXT NOT NULL,
    key TEXT NOT NULL,
    before TEXT,
    after TEXT,
    PRIMARY KEY (tenant_id, revision, seq)
);
