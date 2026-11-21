// Package postgres is the Postgres port.Store and port.Outbox; forced row-level security isolates tenants.
package postgres

import (
	"context"
	"embed"
	"errors"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/versatile-sh/connector-engine/port"
)

//go:embed migrations/*.sql
var migrations embed.FS

type mode int

const (
	modeRead mode = iota
	modeWrite
	modeRelay
)

// Store is the Postgres port.Store and port.Outbox.
type Store struct{ pool *pgxpool.Pool }

var (
	_ port.Store  = (*Store)(nil)
	_ port.Outbox = (*Store)(nil)
)

// New connects with dsn and refuses a role that would bypass row-level security.
func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fail(err)
	}
	var bypass bool
	if err := pool.QueryRow(ctx, `SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&bypass); err != nil || bypass {
		pool.Close()
		if err == nil {
			err = port.Errorf(port.CodeInvalidArgument, "database role bypasses row-level security")
		}
		return nil, fail(err)
	}
	return &Store{pool: pool}, nil
}

// Close releases the connection pool.
func (s *Store) Close() { s.pool.Close() }

// Migrate applies the embedded migrations that have not run yet, serialised by an advisory lock.
func (s *Store) Migrate(ctx context.Context) error {
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return fail(err)
	}
	q, err := s.pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = q.Rollback(ctx) }()
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(7243002);
		CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY)`); err != nil {
		return fail(err)
	}
	known := make(map[string]bool, len(files))
	for _, f := range files {
		known[f] = true
	}
	rows, err := q.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return fail(err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return fail(err)
		}
		if !known[v] {
			rows.Close()
			return port.Errorf(port.CodeConflict, "database has migration %s this binary does not know; upgrade the binary instead of downgrading", v)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	for _, f := range files {
		var done bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, f).Scan(&done); err != nil {
			return fail(err)
		}
		if done {
			continue
		}
		sql, err := migrations.ReadFile(f)
		if err != nil {
			return fail(err)
		}
		if _, err := q.Exec(ctx, string(sql)); err != nil {
			return fail(err)
		}
		if _, err := q.Exec(ctx, `INSERT INTO schema_migrations VALUES ($1)`, f); err != nil {
			return fail(err)
		}
	}
	return fail(q.Commit(ctx))
}

// Read runs fn in a read-only transaction scoped to scope.
func (s *Store) Read(ctx context.Context, scope port.Scope, fn func(port.Tx) error) error {
	return s.run(ctx, scope, modeRead, func(t *tx) error { return fn(t) })
}

// Write runs fn in one transaction scoped to scope and commits only when fn returns nil.
func (s *Store) Write(ctx context.Context, scope port.Scope, fn func(port.Tx) error) error {
	return s.run(ctx, scope, modeWrite, func(t *tx) error { return fn(t) })
}

// Pending returns up to limit undelivered events in emit order across tenants; limit <= 0 means all.
func (s *Store) Pending(ctx context.Context, limit int) ([]port.Event, error) {
	var out []port.Event
	err := s.run(ctx, port.Scope{Installation: true}, modeRelay, func(t *tx) (err error) {
		out, err = many[port.Event](t, `SELECT doc FROM outbox WHERE delivered_at IS NULL ORDER BY pos LIMIT nullif($1::int, 0)`, max(limit, 0))
		return err
	})
	return out, err
}

// MarkDelivered flags the events with the given ids; unknown ids are ignored.
func (s *Store) MarkDelivered(ctx context.Context, ids []string) error {
	return s.run(ctx, port.Scope{Installation: true}, modeRelay, func(t *tx) error {
		_, err := t.exec(`UPDATE outbox SET delivered_at = now() WHERE id = ANY($1) AND delivered_at IS NULL`, ids)
		return err
	})
}

func (s *Store) run(ctx context.Context, scope port.Scope, m mode, fn func(*tx) error) error {
	if scope.Tenant == "" && !scope.Installation {
		return port.Errorf(port.CodeInvalidArgument, "scope names a tenant or the installation")
	}
	opts := pgx.TxOptions{}
	if m == modeRead {
		opts.AccessMode = pgx.ReadOnly
	}
	q, err := s.pool.BeginTx(ctx, opts)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = q.Rollback(ctx) }()
	flag := func(on bool) string {
		if on {
			return "on"
		}
		return "off"
	}
	if _, err := q.Exec(ctx, `SELECT set_config('cn.tenant', $1, true), set_config('cn.installation', $2, true), set_config('cn.relay', $3, true)`,
		string(scope.Tenant), flag(scope.Installation), flag(m == modeRelay)); err != nil {
		return fail(err)
	}
	if err := fn(&tx{ctx: ctx, q: q, scope: scope}); err != nil {
		return err
	}
	return fail(q.Commit(ctx))
}

func fail(err error) error {
	var pe *pgconn.PgError
	switch {
	case err == nil:
		return nil
	case errors.As(err, new(*port.Error)):
		return err
	case errors.Is(err, pgx.ErrNoRows):
		return port.Errorf(port.CodeNotFound, "no such record")
	case errors.As(err, &pe):
		switch pe.Code {
		case "23505", "40001", "40P01":
			return port.Errorf(port.CodeConflict, "%s", pe.Message)
		case "42501":
			return port.Errorf(port.CodePermissionDenied, "%s", pe.Message)
		}
		return port.Errorf(port.CodeInternal, "%s", pe.Message)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return port.Errorf(port.CodeUnavailable, "%v", err)
	}
	return port.Errorf(port.CodeUnavailable, "database: %v", err)
}
