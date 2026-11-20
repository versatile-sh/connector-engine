package skillcatalog_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/source/skillcatalog"
)

func TestARealCatalogParses(t *testing.T) {
	dir := os.Getenv("CN_SKILLS_CATALOG")
	if dir == "" {
		t.Skip("CN_SKILLS_CATALOG names a skills catalog checkout")
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "skills", "*.yaml"))
	bundled := 0
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		e, err := skillcatalog.Parse(raw)
		if err != nil && strings.Contains(err.Error(), "at most") {
			t.Logf("over the size limits: %s: %v", filepath.Base(p), err)
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(p), err)
			continue
		}
		if e.Redistribution() == port.RedistributionBundled {
			bundled++
		}
	}
	t.Logf("%d entries, %d bundled", len(paths), bundled)
}
