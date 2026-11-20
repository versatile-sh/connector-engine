package atomicfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/versatile-sh/connector-engine/internal/atomicfile"
)

func TestWriteReplacesTheFileWithOwnerOnlyMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key.json")
	for _, content := range []string{"old", "new"} {
		if err := atomicfile.Write(path, []byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	st, _ := os.Stat(path)
	if err != nil || string(got) != "new" || st.Mode().Perm() != 0o600 {
		t.Fatalf("content %q, mode %v, err %v", got, st.Mode().Perm(), err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("the temporary file is left behind: %v", err)
	}
}
