package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestBothPoolsRunWALWithFullSyncAndOnlyTheWriterWrites(t *testing.T) {
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for name, db := range map[string]*sql.DB{"writer": s.writer, "reader": s.reader} {
		var mode string
		var sync, busy, readOnly int
		for q, into := range map[string]any{"PRAGMA journal_mode": &mode, "PRAGMA synchronous": &sync, "PRAGMA busy_timeout": &busy, "PRAGMA query_only": &readOnly} {
			if err := db.QueryRow(q).Scan(into); err != nil {
				t.Fatal(err)
			}
		}
		if mode != "wal" || sync != 2 || busy != 5000 || (readOnly == 1) != (name == "reader") {
			t.Fatalf("%s: journal_mode=%s synchronous=%d busy_timeout=%d query_only=%d", name, mode, sync, busy, readOnly)
		}
	}
}
