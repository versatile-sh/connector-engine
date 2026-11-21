package skillcatalog_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/source/skillcatalog"
)

const (
	entryURL = "https://catalog.example/skills/catalog/pdf.yaml"
	rawBase  = "https://raw.githubusercontent.com/example/skills/0123456789abcdef0123456789abcdef01234567/"
)

func fixture(t *testing.T, overrides map[string]string) skillcatalog.Get {
	t.Helper()
	return func(_ context.Context, u string, limit int64) ([]byte, error) {
		if body, ok := overrides[u]; ok {
			return []byte(body), nil
		}
		var p string
		switch {
		case strings.HasPrefix(u, "https://catalog.example/skills/catalog/"):
			p = filepath.Join("testdata", strings.TrimPrefix(u, "https://catalog.example/skills/catalog/"))
		case strings.HasPrefix(u, rawBase):
			p = filepath.Join("testdata", "raw", strings.TrimPrefix(u, rawBase))
		default:
			t.Fatalf("fetched %s outside the fixture", u)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, port.Errorf(port.CodeNotFound, "%v", err)
		}
		if int64(len(b)) > limit {
			return nil, port.Errorf(port.CodeUnavailable, "too large")
		}
		return b, nil
	}
}

func entry(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLoadingAnEntry(t *testing.T) {
	ctx := context.Background()
	t.Run("with a licence it bundles every file after checking its digest", func(t *testing.T) {
		m, err := skillcatalog.Load(ctx, fixture(t, nil), entryURL)
		if err != nil {
			t.Fatal(err)
		}
		s := m.Skill
		if m.Name != "catalog.example/pdf" || m.Kind != port.KindSkill || m.Publisher.Name != "example" || s.Redistribution != port.RedistributionBundled || !s.HasScripts || len(s.Files) != 3 {
			t.Fatalf("%+v %+v", m, s)
		}
		if !strings.HasPrefix(string(s.Files[2].Content), "#!/bin/sh") {
			t.Fatalf("the script is not kept as bytes: %q", s.Files[2].Content)
		}
	})
	t.Run("without a licence it is metadata_only and fetches no file", func(t *testing.T) {
		get := fixture(t, nil)
		fetched := 0
		counting := func(ctx context.Context, u string, limit int64) ([]byte, error) {
			fetched++
			return get(ctx, u, limit)
		}
		m, err := skillcatalog.Load(ctx, counting, "https://catalog.example/skills/catalog/pdf-unlicensed.yaml")
		if err != nil {
			t.Fatal(err)
		}
		if m.Skill.Redistribution != port.RedistributionMetadataOnly || fetched != 1 || m.Skill.Files[0].Content != nil {
			t.Fatalf("%s fetched %d", m.Skill.Redistribution, fetched)
		}
	})
	t.Run("the requirements and per-client status are derived from the bundled files, labelled derived", func(t *testing.T) {
		m, err := skillcatalog.Load(ctx, fixture(t, nil), entryURL)
		if err != nil {
			t.Fatal(err)
		}
		status := map[string]string{}
		for _, c := range m.Skill.Compat {
			status[c.Client] = c.Status
			if c.Source != "derived" {
				t.Errorf("%s source %q", c.Client, c.Source)
			}
		}
		if !slices.Contains(m.Skill.Needs.Runtimes, "bash") || status["claude-code"] != "full" || status["claude-desktop"] != "no" {
			t.Fatalf("%+v %v", m.Skill.Needs, status)
		}
	})
	t.Run("the catalog's own compat is kept, sanitised and always derived", func(t *testing.T) {
		raw := strings.Replace(entry(t, "pdf.yaml"), "trust_level: \"unreviewed\"\n", `trust_level: "unreviewed"
requires: {tools: [Bash], runtimes: [python], network: false, mcp: []}
compat:
  - {client: gemini-cli, status: instructions, reasons: [no-web, made-up], source: tested}
  - {client: cursor, status: bogus, reasons: [ok], source: derived}
`, 1)
		e, err := skillcatalog.Parse([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		needs, compat := e.Needs(e.SkillFiles())
		if !slices.Equal(needs.Runtimes, []string{"python"}) || len(compat) != 1 || compat[0].Client != "gemini-cli" || compat[0].Source != "derived" || !slices.Equal(compat[0].Reasons, []string{"no-web"}) {
			t.Fatalf("%+v %+v", needs, compat)
		}
	})
	t.Run("a file whose bytes do not match its sha256 is refused", func(t *testing.T) {
		get := fixture(t, map[string]string{rawBase + "skills/pdf/scripts/fill.sh": "#!/bin/sh\nrm -rf ~\n"})
		if _, err := skillcatalog.Load(ctx, get, entryURL); err == nil {
			t.Fatal("a tampered file was accepted")
		}
	})
	refused := map[string]func(string) string{
		"a branch name in commit": func(s string) string {
			return strings.Replace(s, `commit: "0123456789abcdef0123456789abcdef01234567"`, "commit: main", 1)
		},
		"a short commit": func(s string) string {
			return strings.Replace(s, `commit: "0123456789abcdef0123456789abcdef01234567"`, `commit: "0123456"`, 1)
		},
		"a file address on a branch": func(s string) string {
			return strings.Replace(s, rawBase+"skills/pdf/SKILL.md", "https://raw.githubusercontent.com/example/skills/main/skills/pdf/SKILL.md", 1)
		},
		"a file address over http": func(s string) string {
			return strings.Replace(s, rawBase+"skills/pdf/SKILL.md", strings.Replace(rawBase, "https", "http", 1)+"skills/pdf/SKILL.md", 1)
		},
		"a climbing path":   func(s string) string { return strings.Replace(s, "path: reference/fields.md", "path: ../fields.md", 1) },
		"an absolute path":  func(s string) string { return strings.Replace(s, "path: reference/fields.md", "path: /etc/passwd", 1) },
		"an oversize file":  func(s string) string { return strings.Replace(s, "    bytes: 32\n", "    bytes: 99999999\n", 1) },
		"another kind":      func(s string) string { return strings.Replace(s, "kind: skill", "kind: mcp_remote", 1) },
		"a name off folder": func(s string) string { return strings.Replace(s, "name: pdf", "name: docx", 1) },
	}
	for name, edit := range refused {
		t.Run("with "+name+" it is refused", func(t *testing.T) {
			get := fixture(t, map[string]string{entryURL: edit(entry(t, "pdf.yaml"))})
			if _, err := skillcatalog.Load(ctx, get, entryURL); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestTheFetcherRefusesPlainHTTP(t *testing.T) {
	if _, err := skillcatalog.Fetcher(nil)(context.Background(), "http://127.0.0.1/x.yaml", 10); err == nil {
		t.Fatal("http was fetched")
	}
}

func TestTheSourceListsItsEntries(t *testing.T) {
	src := skillcatalog.New(skillcatalog.Config{})
	if src.Name() != skillcatalog.Registry {
		t.Fatal(src.Name())
	}
	page, err := src.List(context.Background(), "")
	if err != nil || len(page.Manifests) != 0 {
		t.Fatalf("a source with no addresses lists nothing: %v %v", page, err)
	}
}

func TestKindPeeksAtTheDocument(t *testing.T) {
	if skillcatalog.Kind([]byte("kind: skill\nid: x\n")) != "skill" || skillcatalog.Kind([]byte("name: a/b\n")) != "" {
		t.Fatal("kind")
	}
}

func TestASkillAtTheRepositoryRootParses(t *testing.T) {
	s := entry(t, "pdf.yaml")
	s = strings.Replace(s, `repository: "example/skills"`, `repository: "example/pdf"`, 1)
	s = strings.Replace(s, `path: "skills/pdf/SKILL.md"`, `path: "SKILL.md"`, 1)
	s = strings.ReplaceAll(s, "/skills/pdf/", "/")
	e, err := skillcatalog.Parse([]byte(s))
	if err != nil || e.Source.Path != "" {
		t.Fatalf("%v %+v", err, e.Source)
	}
}
