package listing_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/core/listing"
	"github.com/versatile-sh/connector-engine/port"
)

func file(p, content string) port.SkillFile {
	sum := sha256.Sum256([]byte(content))
	return port.SkillFile{Path: p, Bytes: int64(len(content)), SHA256: hex.EncodeToString(sum[:]), Content: []byte(content)}
}

func skill() port.Manifest {
	return port.Manifest{Name: "skills.example/pdf", Version: strings.Repeat("a", 40), Kind: port.KindSkill, Publisher: port.Publisher{Name: "example"},
		Skill: &port.Skill{Name: "pdf", Source: port.SkillSource{Repository: "https://github.com/example/skills", Path: "skills/pdf", Commit: strings.Repeat("a", 40)},
			Licence: port.SkillLicence{SPDX: "MIT", Redistributable: true}, Redistribution: port.RedistributionBundled, HasScripts: true,
			Files: []port.SkillFile{file("SKILL.md", "---\nname: pdf\n---\nUse it."), file("scripts/run.sh", "#!/bin/sh\necho hi\n")}}}
}

func TestASkillListing(t *testing.T) {
	t.Run("when bundled with matching digests it is valid", func(t *testing.T) {
		if err := listing.Validate(skill(), true); err != nil {
			t.Fatal(err)
		}
	})
	cases := map[string]struct {
		edit func(*port.Skill)
		want string
	}{
		"a branch instead of a commit": {func(s *port.Skill) { s.Source.Commit = "main" }, "40-hex"},
		"a climbing path":              {func(s *port.Skill) { s.Files[1].Path = "../etc/passwd" }, "climbs"},
		"an absolute path":             {func(s *port.Skill) { s.Files[1].Path = "/etc/passwd" }, "relative"},
		"a changed byte":               {func(s *port.Skill) { s.Files[0].Content[0] = 'x' }, "sha256"},
		"an oversize file":             {func(s *port.Skill) { s.Files[1].Bytes = listing.MaxSkillFileBytes + 1 }, "at most"},
		"no SKILL.md":                  {func(s *port.Skill) { s.Files = s.Files[1:] }, "SKILL.md"},
		"scripts not declared":         {func(s *port.Skill) { s.HasScripts = false }, "has_scripts"},
		"bundled without a licence":    {func(s *port.Skill) { s.Licence.SPDX = "" }, "metadata_only"},
		"a name unlike the folder":     {func(s *port.Skill) { s.Name = "docx" }, "last segment"},
		"metadata_only with content":   {func(s *port.Skill) { s.Redistribution = port.RedistributionMetadataOnly }, "no file content"},
	}
	for name, c := range cases {
		t.Run("with "+name+" it is refused", func(t *testing.T) {
			m := skill()
			c.edit(m.Skill)
			if err := listing.Validate(m, true); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
	t.Run("when metadata_only it carries paths and digests without content", func(t *testing.T) {
		m := skill()
		m.Skill.Redistribution, m.Skill.Licence = port.RedistributionMetadataOnly, port.SkillLicence{}
		for i := range m.Skill.Files {
			m.Skill.Files[i].Content = nil
		}
		if err := listing.Validate(m, true); err != nil {
			t.Fatal(err)
		}
	})
}

func TestTheSkillDigestIgnoresFileOrder(t *testing.T) {
	a := skill().Skill.Files
	b := []port.SkillFile{a[1], a[0]}
	if listing.SkillDigest(a) != listing.SkillDigest(b) || !strings.HasPrefix(listing.SkillDigest(a), "sha256:") {
		t.Fatal("the bundle digest depends on order")
	}
}
