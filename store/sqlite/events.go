package sqlite

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

func (t *tx) DueWork(now time.Time, limit int) ([]port.WorkItem, error) {
	if !t.scope.Installation {
		return nil, port.Errorf(port.CodePermissionDenied, "due work is read in the installation scope")
	}
	return scan(t, `SELECT tenant_id, kind, id, due FROM due_work WHERE due <= ?3 ORDER BY due, tenant_id, kind, id LIMIT ?4`,
		func(r *sql.Rows) (w port.WorkItem, err error) {
			var due string
			if err = r.Scan(&w.Tenant, &w.Kind, &w.ID, &due); err == nil {
				w.Due, err = time.Parse(time.RFC3339Nano, due)
			}
			return w, err
		}, stamp(now), limitOf(limit))
}

func (t *tx) Audit(r port.AuditRow) error {
	if !t.ownsTenant(r.Tenant) {
		return port.Errorf(port.CodePermissionDenied, "audit rows of tenant %q are outside this unit of work", r.Tenant)
	}
	doc, err := encode(r)
	if err != nil {
		return err
	}
	_, err = t.exec(`INSERT INTO audit (tenant_id, subject, doc) VALUES (?3, ?4, ?5)`, string(r.Tenant), r.Subject, doc)
	return err
}

func (t *tx) ListAudit(subject string) ([]port.AuditRow, error) {
	return docs[port.AuditRow](t, `SELECT doc FROM audit WHERE `+owns+` AND (?3 = '' OR subject = ?3) ORDER BY id`, subject)
}

func (t *tx) Emit(e port.Event) (port.Event, error) {
	if e.Type == "" {
		return e, port.Errorf(port.CodeInvalidArgument, "an event has a type")
	}
	if !t.ownsTenant(e.Tenant) {
		return e, port.Errorf(port.CodePermissionDenied, "events of tenant %q are outside this unit of work", e.Tenant)
	}
	e.Source = port.SourceOf(e.Tenant)
	e.Stream = e.Source
	seq, err := one[int64](t, `INSERT INTO streams (stream, tenant_id, last) VALUES (?3, ?4, 1)
		ON CONFLICT (stream) DO UPDATE SET last = last + 1 WHERE streams.tenant_id = excluded.tenant_id RETURNING last`, e.Stream, string(e.Tenant))
	if port.IsCode(err, port.CodeNotFound) {
		return e, port.Errorf(port.CodePermissionDenied, "stream %s belongs to another tenant", e.Stream)
	}
	if err != nil {
		return e, err
	}
	e.Sequence, e.ID = seq, uuid()
	doc, err := encode(e)
	if err != nil {
		return e, err
	}
	_, err = t.exec(`INSERT INTO outbox (id, tenant_id, stream, sequence, doc) VALUES (?3, ?4, ?5, ?6, ?7)`, e.ID, string(e.Tenant), e.Stream, e.Sequence, doc)
	return e, err
}

func (t *tx) EventsAfter(stream string, sequence int64, limit int) ([]port.Event, error) {
	return docs[port.Event](t, `SELECT doc FROM outbox WHERE stream = ?3 AND sequence > ?4 AND `+owns+` ORDER BY sequence LIMIT ?5`, stream, sequence, limitOf(limit))
}

func (t *tx) SeenInbox(id string) (bool, error) {
	return one[bool](t, `SELECT EXISTS (SELECT 1 FROM inbox WHERE tenant_id = ?1 AND id = ?3)`, id)
}

func (t *tx) MarkInbox(id string, at time.Time) (bool, error) {
	n, err := t.exec(`INSERT INTO inbox (tenant_id, id, at) VALUES (?1, ?3, ?4) ON CONFLICT DO NOTHING`, id, stamp(at))
	return n == 1, err
}

func (t *tx) SetInboxPayload(id string, payload []byte) error {
	_, err := t.exec(`UPDATE inbox SET payload = ?3 WHERE tenant_id = ?1 AND id = ?4`, string(payload), id)
	return err
}

func (t *tx) InboxPayload(id string) ([]byte, error) {
	v, err := many[string](t, `SELECT coalesce(payload, '') FROM inbox WHERE tenant_id = ?1 AND id = ?3`, id)
	if err != nil || len(v) == 0 {
		return nil, err
	}
	return []byte(v[0]), nil
}

func uuid() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
