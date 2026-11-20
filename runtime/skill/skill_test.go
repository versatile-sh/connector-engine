package skill_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/runtime/skill"
)

func file(p string, content []byte) port.SkillFile {
	sum := sha256.Sum256(content)
	return port.SkillFile{Path: p, Bytes: int64(len(content)), SHA256: hex.EncodeToString(sum[:]), Content: content}
}

func manifest() port.Manifest {
	commit := strings.Repeat("b", 40)
	return port.Manifest{Name: "skills.example/pdf", Version: commit, Title: "PDF", Description: "Fills PDF forms.", Kind: port.KindSkill, Publisher: port.Publisher{Name: "example"},
		Skill: &port.Skill{Name: "pdf", Source: port.SkillSource{Repository: "https://github.com/example/skills", Path: "pdf", Commit: commit},
			Licence: port.SkillLicence{SPDX: "Apache-2.0", Redistributable: true}, Redistribution: port.RedistributionBundled, HasScripts: true,
			Files: []port.SkillFile{file("SKILL.md", []byte("---\nname: pdf\n---\nRun scripts/run.sh.")), file("scripts/run.sh", []byte("#!/bin/sh\ntouch /tmp/ran\n")),
				file("assets/logo.bin", []byte{0xff, 0x00, 0xfe})}}}
}

func load(t *testing.T, r *skill.Runtime, h port.Handle, path string) (port.Result, skill.Loaded) {
	t.Helper()
	args, _ := json.Marshal(map[string]string{"path": path})
	res, err := r.Call(context.Background(), h, "pdf", args, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var l skill.Loaded
	if err := json.Unmarshal(res.Structured, &l); err != nil {
		t.Fatal(err)
	}
	return res, l
}

func blocks(t *testing.T, res port.Result) []string {
	t.Helper()
	var b []struct{ Text string }
	if err := json.Unmarshal(res.Content, &b); err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(b))
	for i, x := range b {
		out[i] = x.Text
	}
	return out
}

func TestTheSkillRuntime(t *testing.T) {
	r := skill.New()
	ctx := context.Background()
	h, err := r.Start(ctx, port.Connector{}, manifest(), port.SecretsRef{})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("lists one read-only function pinned to the folder digest", func(t *testing.T) {
		defs, _, err := r.ListFunctions(ctx, h)
		if err != nil || len(defs) != 1 || defs[0].Name != "pdf" {
			t.Fatalf("%v %+v", err, defs)
		}
		var d struct {
			Annotations struct{ ReadOnlyHint bool }
			Meta        map[string]skill.Meta `json:"_meta"`
		}
		_ = json.Unmarshal(defs[0].Raw, &d)
		if !d.Annotations.ReadOnlyHint || !strings.HasPrefix(d.Meta[port.SkillMetaKey].Digest, "sha256:") || len(d.Meta[port.SkillMetaKey].Files) != 3 {
			t.Fatalf("%s", defs[0].Raw)
		}
	})
	t.Run("with no path it serves SKILL.md after an engine header", func(t *testing.T) {
		res, l := load(t, r, h, "")
		b := blocks(t, res)
		if l.Path != "SKILL.md" || len(b) != 2 || !strings.Contains(b[0], "never runs them") || b[1] != string(manifest().Skill.Files[0].Content) {
			t.Fatalf("%+v %q", l, b)
		}
	})
	t.Run("serves a script as its exact text", func(t *testing.T) {
		res, l := load(t, r, h, "scripts/run.sh")
		if l.Content != "#!/bin/sh\ntouch /tmp/ran\n" || l.Encoding != "utf-8" || blocks(t, res)[1] != l.Content {
			t.Fatalf("%+v", l)
		}
	})
	t.Run("serves a binary file as base64 in the structured result", func(t *testing.T) {
		res, l := load(t, r, h, "assets/logo.bin")
		raw, _ := base64.StdEncoding.DecodeString(l.Content)
		if l.Encoding != "base64" || !slices.Equal(raw, []byte{0xff, 0x00, 0xfe}) || len(blocks(t, res)) != 1 {
			t.Fatalf("%+v", l)
		}
	})
	t.Run("answers an unknown path as a tool error", func(t *testing.T) {
		if res, _ := load(t, r, h, "../etc/passwd"); !res.IsError {
			t.Fatal("an unknown path was served")
		}
	})
	t.Run("when metadata_only it serves the source instead of the bytes", func(t *testing.T) {
		m := manifest()
		m.Skill.Redistribution, m.Skill.Licence = port.RedistributionMetadataOnly, port.SkillLicence{}
		for i := range m.Skill.Files {
			m.Skill.Files[i].Content = nil
		}
		h2, err := r.Start(ctx, port.Connector{}, m, port.SecretsRef{})
		if err != nil {
			t.Fatal(err)
		}
		res, l := load(t, r, h2, "")
		if l.Content != "" || !strings.Contains(blocks(t, res)[0], "metadata_only") {
			t.Fatalf("%+v", l)
		}
	})
	t.Run("refuses a manifest whose bytes do not match", func(t *testing.T) {
		m := manifest()
		m.Skill.Files[0].Content = []byte("tampered")
		if _, err := r.Start(ctx, port.Connector{}, m, port.SecretsRef{}); !port.IsCode(err, port.CodeInvalidManifest) {
			t.Fatalf("%v", err)
		}
	})
}

func TestTheSkillRuntimeCannotStartAProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("go list")
	}
	out, err := exec.Command("go", "list", "-deps", "github.com/agents-control/connector-engine/runtime/skill").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if dep == "os/exec" {
			t.Fatalf("runtime/skill imports %s", dep)
		}
	}
}
