CREATE TABLE records (
    kind TEXT NOT NULL,
    k TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    uniq TEXT,
    rev INTEGER NOT NULL,
    doc TEXT NOT NULL,
    PRIMARY KEY (kind, k)
);
CREATE UNIQUE INDEX records_uniq ON records (kind, uniq) WHERE uniq IS NOT NULL;
CREATE INDEX records_tenant ON records (tenant_id, kind);

CREATE TABLE audit (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id TEXT NOT NULL,
    subject TEXT NOT NULL,
    doc TEXT NOT NULL
);
CREATE INDEX audit_subject ON audit (tenant_id, subject);

CREATE TABLE streams (
    stream TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    last INTEGER NOT NULL
);

CREATE TABLE outbox (
    pos INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    stream TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    doc TEXT NOT NULL,
    delivered_at TEXT,
    UNIQUE (stream, sequence)
);
CREATE INDEX outbox_pending ON outbox (pos) WHERE delivered_at IS NULL;

CREATE TABLE inbox (
    tenant_id TEXT NOT NULL,
    id TEXT NOT NULL,
    at TEXT NOT NULL,
    PRIMARY KEY (tenant_id, id)
);

CREATE TABLE due_work (
    rec_kind TEXT NOT NULL,
    k TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    id TEXT NOT NULL,
    due TEXT NOT NULL,
    PRIMARY KEY (rec_kind, k)
);
CREATE INDEX due_work_due ON due_work (due);
