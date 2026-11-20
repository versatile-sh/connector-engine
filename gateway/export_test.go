package gateway_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/agents-control/connector-engine/cli/skills"
	"github.com/agents-control/connector-engine/port"
)

func (w *world) exporter(agent string) func(args ...string) (string, error) {
	env := map[string]string{"CN_ENDPOINT": w.srv.URL, "CN_AGENT_TOKEN": w.token(w.claims(agent, "export"))}
	cmd := skills.New(func(k string) string { return env[k] })
	return func(args ...string) (string, error) {
		var out, errw bytes.Buffer
		err := cmd.Run(context.Background(), append([]string{"export"}, args...), &out, &errw)
		return out.String(), err
	}
}

func TestCnctlSkillsExport(t *testing.T) {
	w := newWorld(t, options{cfg: skillsOn})
	w.installSkill(port.ModeAllow, "a1")
	export := w.exporter("a1")

	t.Run("writes the exact bytes and a manifest of what it wrote", func(t *testing.T) {
		dir := t.TempDir()
		out, err := export("--to", dir)
		if err != nil {
			t.Fatal(err)
		}
		for p, want := range map[string]string{"pdf/SKILL.md": skillMD, "pdf/scripts/fill.sh": script} {
			got, err := os.ReadFile(filepath.Join(dir, p))
			if err != nil || sha256.Sum256(got) != sha256.Sum256([]byte(want)) {
				t.Fatalf("%s: %v %q", p, err, got)
			}
		}
		var m skills.Manifest
		raw, _ := os.ReadFile(filepath.Join(dir, skills.ManifestFile))
		if json.Unmarshal(raw, &m) != nil || !slices.Equal(m.Files, []string{"pdf/SKILL.md", "pdf/scripts/fill.sh"}) || !strings.Contains(out, "warning:") {
			t.Fatalf("%s %s", raw, out)
		}
	})
	t.Run("with --prune removes only files an earlier export listed", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := export("--to", dir); err != nil {
			t.Fatal(err)
		}
		old, mine := filepath.Join(dir, "pdf", "old.md"), filepath.Join(dir, "pdf", "mine.md")
		for _, p := range []string{old, mine} {
			if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		var m skills.Manifest
		raw, _ := os.ReadFile(filepath.Join(dir, skills.ManifestFile))
		_ = json.Unmarshal(raw, &m)
		m.Files = append(m.Files, "pdf/old.md")
		raw, _ = json.Marshal(m)
		_ = os.WriteFile(filepath.Join(dir, skills.ManifestFile), raw, 0o644)
		if _, err := export("--to", dir, "--prune"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(old); !os.IsNotExist(err) {
			t.Fatal("a file the manifest listed was kept")
		}
		if _, err := os.Stat(mine); err != nil {
			t.Fatal("a file the user wrote was pruned")
		}
	})
	t.Run("never overwrites a file it did not write", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.MkdirAll(filepath.Join(dir, "pdf"), 0o755)
		_ = os.WriteFile(filepath.Join(dir, "pdf", "SKILL.md"), []byte("mine"), 0o644)
		if _, err := export("--to", dir); err == nil {
			t.Fatal("an unowned file was overwritten")
		}
	})
	t.Run("never writes through a symlink", func(t *testing.T) {
		dir, elsewhere := t.TempDir(), t.TempDir()
		if err := os.Symlink(elsewhere, filepath.Join(dir, "pdf")); err != nil {
			t.Skip(err)
		}
		if _, err := export("--to", dir); err == nil {
			t.Fatal("wrote through a symlink")
		}
		if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
			t.Fatalf("files landed outside the folder: %v", entries)
		}
	})
	t.Run("an agent without the skill exports nothing", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := w.exporter("a2")("--to", dir, "--skill", "pdf"); err == nil {
			t.Fatal("a skill was exported for an agent without it")
		}
	})
}
