// Package sqlite is the embedded SQLite port.Store and port.Outbox of a single-host install; every query is scoped to the unit of work's tenant, as row-level security does in Postgres.
package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/versatile-sh/connector-engine/port"
)

//go:embed migrations/*.sql
var migrations embed.FS

const pragmas = "_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(FULL)"

const stampLayout = "2006-01-02T15:04:05.000000000Z"

// Store is the SQLite port.Store and port.Outbox: one writer connection and a pool of read-only ones on the same WAL file.
type Store struct {
	writer *sql.DB
	reader *sql.DB
}

var (
	_ port.Store  = (*Store)(nil)
	_ port.Outbox = (*Store)(nil)

	registerOnce sync.Once
	errRegister  error
)

// Open opens or creates the database file at path, mode 0600 with its WAL files, in WAL mode with full fsync and applies the embedded migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	registerOnce.Do(func() { errRegister = sqlite.RegisterDeterministicScalarFunction("cn_time", 1, cnTime) })
	if errRegister != nil {
		return nil, fail(errRegister)
	}
	if strings.Contains(path, "?") {
		return nil, port.Errorf(port.CodeInvalidArgument, "the database path %q holds a ?, which the driver reads as its options", path)
	}
	if err := ownerOnly(path); err != nil {
		return nil, fail(err)
	}
	writer, err := sql.Open("sqlite", path+"?_txlock=immediate&"+pragmas)
	if err != nil {
		return nil, fail(err)
	}
	writer.SetMaxOpenConns(1)
	reader, err := sql.Open("sqlite", path+"?"+pragmas+"&_pragma=query_only(1)")
	if err != nil {
		writer.Close()
		return nil, fail(err)
	}
	s := &Store{writer: writer, reader: reader}
	if err := s.migrate(ctx); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func ownerOnly(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(p, 0o600); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// Close releases both connection pools.
func (s *Store) Close() {
	_ = s.reader.Close()
	_ = s.writer.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return fail(err)
	}
	q, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = q.Rollback() }()
	var version int
	if err := q.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fail(err)
	}
	if version > len(files) {
		return port.Errorf(port.CodeConflict, "database schema is version %d but this binary knows %d; upgrade the binary instead of downgrading", version, len(files))
	}
	if version == len(files) {
		return nil
	}
	for i := version; i < len(files); i++ {
		ddl, err := migrations.ReadFile(files[i])
		if err != nil {
			return fail(err)
		}
		if _, err := q.ExecContext(ctx, string(ddl)); err != nil {
			return fail(err)
		}
		if files[i] == canonicalGrantsMigration {
			if err := canonicalGrants(ctx, q); err != nil {
				return err
			}
		}
	}
	if _, err := q.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, len(files))); err != nil {
		return fail(err)
	}
	return fail(q.Commit())
}

// Read runs fn in a read-only transaction scoped to scope.
func (s *Store) Read(ctx context.Context, scope port.Scope, fn func(port.Tx) error) error {
	return s.run(ctx, scope, s.reader, func(t *tx) error { return fn(t) })
}

// Write runs fn in one immediate transaction scoped to scope and commits only when fn returns nil.
func (s *Store) Write(ctx context.Context, scope port.Scope, fn func(port.Tx) error) error {
	return s.run(ctx, scope, s.writer, func(t *tx) error { return fn(t) })
}

// Pending returns up to limit undelivered events in emit order across tenants; limit <= 0 means all.
func (s *Store) Pending(ctx context.Context, limit int) ([]port.Event, error) {
	var out []port.Event
	err := s.run(ctx, port.Scope{Installation: true}, s.reader, func(t *tx) (err error) {
		out, err = docs[port.Event](t, `SELECT doc FROM outbox WHERE delivered_at IS NULL ORDER BY pos LIMIT ?3`, limitOf(limit))
		return err
	})
	return out, err
}

// MarkDelivered flags the events with the given ids; unknown ids are ignored.
func (s *Store) MarkDelivered(ctx context.Context, ids []string) error {
	return s.run(ctx, port.Scope{Installation: true}, s.writer, func(t *tx) error {
		_, err := t.exec(`UPDATE outbox SET delivered_at = ?3 WHERE id IN (SELECT value FROM json_each(?4)) AND delivered_at IS NULL`, stamp(time.Now()), jsonList(ids))
		return err
	})
}

func (s *Store) run(ctx context.Context, scope port.Scope, db *sql.DB, fn func(*tx) error) error {
	if scope.Tenant == "" && !scope.Installation {
		return port.Errorf(port.CodeInvalidArgument, "scope names a tenant or the installation")
	}
	q, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = q.Rollback() }()
	if err := fn(&tx{ctx: ctx, q: q, scope: scope}); err != nil {
		return err
	}
	return fail(q.Commit())
}

func limitOf(n int) int {
	if n <= 0 {
		return -1
	}
	return n
}

func jsonList(v []string) string {
	if v == nil {
		v = []string{}
	}
	raw, _ := json.Marshal(v)
	return string(raw)
}

func stamp(t time.Time) string { return t.UTC().Format(stampLayout) }

func cnTime(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	s, ok := args[0].(string)
	if !ok {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil, nil
	}
	return stamp(t), nil
}

func fail(err error) error {
	var se *sqlite.Error
	switch {
	case err == nil:
		return nil
	case errors.As(err, new(*port.Error)):
		return err
	case errors.Is(err, sql.ErrNoRows):
		return port.Errorf(port.CodeNotFound, "no such record")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return port.Errorf(port.CodeUnavailable, "%v", err)
	case errors.As(err, &se):
		switch se.Code() {
		case sqlite3.SQLITE_CONSTRAINT_UNIQUE, sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
			return port.Errorf(port.CodeConflict, "%s", se.Error())
		}
		switch se.Code() & 0xff {
		case sqlite3.SQLITE_BUSY, sqlite3.SQLITE_LOCKED:
			return port.Errorf(port.CodeUnavailable, "%s", se.Error())
		}
		return port.Errorf(port.CodeInternal, "%s", se.Error())
	}
	return port.Errorf(port.CodeUnavailable, "database: %v", err)
}
