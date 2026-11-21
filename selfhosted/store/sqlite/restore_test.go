package sqlite_test

import (
	"path/filepath"
	"testing"

	st "github.com/versatile-sh/connector-engine/service/servicetest"
)

func TestEveryRowTheServiceWritesRestoresOnSQLite(t *testing.T) {
	st.RestoreKeepsDecisions(t, open(t, filepath.Join(t.TempDir(), "src.db")), open(t, filepath.Join(t.TempDir(), "dst.db")))
}
