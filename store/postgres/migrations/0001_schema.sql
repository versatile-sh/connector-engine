CREATE TABLE records (
    kind text NOT NULL,
    k text NOT NULL,
    tenant_id text NOT NULL,
    uniq text,
    rev bigint NOT NULL,
    doc jsonb NOT NULL,
    PRIMARY KEY (kind, k)
);
CREATE UNIQUE INDEX records_uniq ON records (kind, uniq) WHERE uniq IS NOT NULL;
CREATE INDEX records_tenant ON records (tenant_id, kind);
CREATE INDEX records_doc ON records USING gin (doc jsonb_path_ops);

CREATE TABLE audit (
    id bigserial PRIMARY KEY,
    tenant_id text NOT NULL,
    subject text NOT NULL,
    doc jsonb NOT NULL
);
CREATE INDEX audit_subject ON audit (tenant_id, subject);

CREATE TABLE streams (
    stream text PRIMARY KEY,
    tenant_id text NOT NULL,
    last bigint NOT NULL
);

CREATE TABLE outbox (
    pos bigserial PRIMARY KEY,
    id text NOT NULL UNIQUE,
    tenant_id text NOT NULL,
    stream text NOT NULL,
    sequence bigint NOT NULL,
    doc jsonb NOT NULL,
    delivered_at timestamptz,
    UNIQUE (stream, sequence)
);
CREATE INDEX outbox_pending ON outbox (pos) WHERE delivered_at IS NULL;

CREATE TABLE inbox (
    tenant_id text NOT NULL,
    id text NOT NULL,
    at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, id)
);

CREATE TABLE due_work (
    rec_kind text NOT NULL,
    k text NOT NULL,
    tenant_id text NOT NULL,
    kind text NOT NULL,
    id text NOT NULL,
    due timestamptz NOT NULL,
    PRIMARY KEY (rec_kind, k)
);
CREATE INDEX due_work_due ON due_work (due);

CREATE FUNCTION cn_tenant() RETURNS text LANGUAGE sql STABLE
    AS $$ SELECT nullif(current_setting('cn.tenant', true), '') $$;
CREATE FUNCTION cn_installation() RETURNS boolean LANGUAGE sql STABLE
    AS $$ SELECT coalesce(current_setting('cn.installation', true), '') = 'on' $$;
CREATE FUNCTION cn_relay() RETURNS boolean LANGUAGE sql STABLE
    AS $$ SELECT coalesce(current_setting('cn.relay', true), '') = 'on' $$;
CREATE FUNCTION cn_owns(owner text) RETURNS boolean LANGUAGE sql STABLE
    AS $$ SELECT owner = cn_tenant() OR (owner = '' AND cn_installation()) $$;

DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['records', 'audit', 'streams', 'outbox', 'inbox', 'due_work'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    END LOOP;
END $$;

CREATE POLICY shared_read ON records FOR SELECT
    USING (cn_owns(tenant_id) OR (tenant_id = '' AND kind IN ('listing', 'version')) OR (cn_installation() AND kind IN ('tenant', 'offer')));
CREATE POLICY owner_write ON records FOR ALL
    USING (CASE WHEN kind IN ('tenant', 'offer') THEN cn_installation() ELSE cn_owns(tenant_id) END)
    WITH CHECK (CASE WHEN kind IN ('tenant', 'offer') THEN cn_installation() ELSE cn_owns(tenant_id) END);
CREATE POLICY tenant_isolation ON audit USING (cn_owns(tenant_id));
CREATE POLICY tenant_isolation ON streams USING (cn_owns(tenant_id));
CREATE POLICY tenant_isolation ON inbox USING (cn_owns(tenant_id));
CREATE POLICY tenant_isolation ON outbox USING (cn_owns(tenant_id) OR cn_relay()) WITH CHECK (cn_owns(tenant_id) OR cn_relay());
CREATE POLICY tenant_isolation ON due_work USING (cn_owns(tenant_id) OR cn_installation()) WITH CHECK (cn_owns(tenant_id));

CREATE FUNCTION track_due() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    rec records;
    work text;
    at timestamptz;
BEGIN
    rec := CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
    IF rec.kind NOT IN ('approval', 'call', 'grant', 'pause') THEN
        RETURN NULL;
    END IF;
    DELETE FROM due_work WHERE rec_kind = rec.kind AND k = rec.k;
    IF TG_OP = 'DELETE' THEN
        RETURN NULL;
    END IF;
    work := CASE rec.kind
        WHEN 'approval' THEN CASE rec.doc->>'state'
            WHEN 'pending' THEN 'approval_expiry'
            WHEN 'approved' THEN 'approved_record'
            WHEN 'executing' THEN 'execute_deadline'
            ELSE CASE WHEN rec.doc ? 'result_ref' OR rec.doc ? 'arguments' THEN 'result_retention' END
        END
        WHEN 'call' THEN CASE WHEN rec.doc->>'state' = 'started' THEN 'call_deadline' END
        WHEN 'grant' THEN 'grant_expiry'
        WHEN 'pause' THEN CASE WHEN (rec.doc->>'paused')::boolean THEN 'pause_expiry' END
    END;
    at := CASE work
        WHEN 'approval_expiry' THEN (rec.doc->>'expires_at')::timestamptz
        WHEN 'approved_record' THEN (rec.doc->>'decided_at')::timestamptz
        WHEN 'execute_deadline' THEN (rec.doc->>'execution_started_at')::timestamptz
            + make_interval(secs => coalesce((rec.doc->>'call_deadline')::bigint, 0) / 1e9)
        WHEN 'result_retention' THEN (rec.doc->>'result_expires_at')::timestamptz
        WHEN 'call_deadline' THEN (rec.doc->>'deadline')::timestamptz
        WHEN 'grant_expiry' THEN (rec.doc->>'expires_at')::timestamptz
        WHEN 'pause_expiry' THEN (rec.doc->>'expires_at')::timestamptz
    END;
    IF at > '0001-01-02 00:00:00+00' THEN
        INSERT INTO due_work VALUES (rec.kind, rec.k, rec.tenant_id, work, CASE rec.kind WHEN 'pause' THEN rec.doc->>'id' ELSE rec.k END, at);
    END IF;
    RETURN NULL;
END $$;
CREATE TRIGGER records_due AFTER INSERT OR UPDATE OR DELETE ON records FOR EACH ROW EXECUTE FUNCTION track_due();
