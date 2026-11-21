package sqlite_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/store/sqlite"
	st "github.com/versatile-sh/connector-engine/service/servicetest"
)

func TestEveryRowTheServiceWritesRestoresOnSQLite(t *testing.T) {
	st.RestoreKeepsDecisions(t, open(t, filepath.Join(t.TempDir(), "src.db")), open(t, filepath.Join(t.TempDir(), "dst.db")))
}

func TestASnapshotsSchemaIsComparedWithTheKindThatWroteIt(t *testing.T) {
	defer sqlite.SetWriterSchema("postgres", 7)()
	restoreAt := func(kind string, schema int) error {
		var snap port.Snapshot
		if err := json.Unmarshal([]byte(postgresSnapshot), &snap); err != nil {
			t.Fatal(err)
		}
		snap.Store, snap.Schema = kind, schema
		_, err := open(t, filepath.Join(t.TempDir(), "store.db")).Restore(ctx, snap, "ops")
		return err
	}
	if err := restoreAt("postgres", 5); err != nil {
		t.Fatalf("a Postgres snapshot of an older Postgres schema: %v", err)
	}
	for _, c := range []struct {
		kind   string
		schema int
	}{{"postgres", 8}, {"sqlite", 5}} {
		if err := restoreAt(c.kind, c.schema); !port.IsCode(err, port.CodeInvalidArgument) {
			t.Fatalf("a %s snapshot of schema %d: %v", c.kind, c.schema, err)
		}
	}
}
