CREATE TABLE IF NOT EXISTS permission_head (
    tenant_id text PRIMARY KEY,
    revision bigint NOT NULL
);

CREATE TABLE IF NOT EXISTS permission_changes (
    tenant_id text NOT NULL,
    revision bigint NOT NULL,
    seq integer NOT NULL,
    at timestamptz NOT NULL,
    actor text NOT NULL,
    op text NOT NULL,
    kind text NOT NULL,
    key text NOT NULL,
    before jsonb,
    after jsonb,
    PRIMARY KEY (tenant_id, revision, seq)
);

ALTER TABLE permission_head ENABLE ROW LEVEL SECURITY;
ALTER TABLE permission_head FORCE ROW LEVEL SECURITY;
ALTER TABLE permission_changes ENABLE ROW LEVEL SECURITY;
ALTER TABLE permission_changes FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON permission_head;
CREATE POLICY tenant_isolation ON permission_head USING (cn_owns(tenant_id)) WITH CHECK (cn_owns(tenant_id));
DROP POLICY IF EXISTS tenant_isolation ON permission_changes;
CREATE POLICY tenant_isolation ON permission_changes USING (cn_owns(tenant_id)) WITH CHECK (cn_owns(tenant_id));
