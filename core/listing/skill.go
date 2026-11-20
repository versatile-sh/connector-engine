package listing

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/agents-control/connector-engine/port"
)

// Limits of one skill listing: files, bytes of one file and bytes of the whole folder.
const (
	MaxSkillFiles     = 256
	MaxSkillFileBytes = 1 << 20
	MaxSkillBytes     = 4 << 20
)

// SkillEntry is the file every skill folder holds.
const SkillEntry = "SKILL.md"

var (
	skillName  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)
	fullCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sha256Hex  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	scriptExts = []string{".sh", ".bash", ".zsh", ".py", ".js", ".mjs", ".cjs", ".ts", ".rb", ".pl", ".php", ".ps1", ".bat", ".cmd", ".exe"}
)

// SkillName reports whether n is an Agent Skills name: lowercase letters, digits and inner hyphens, at most 64.
func SkillName(n string) bool { return skillName.MatchString(n) && !strings.Contains(n, "--") }

// FullCommit reports whether c is a full 40-hex commit id, never a branch, a tag or a short id.
func FullCommit(c string) bool { return fullCommit.MatchString(c) }

// SkillPath refuses a file path that is empty, absolute, not clean, climbs with .. or uses a backslash or a control byte.
func SkillPath(p string) error {
	switch {
	case p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\"):
		return port.Errorf(port.CodeInvalidManifest, "skill file path %q is not a relative slash path", p)
	case path.Clean(p) != p || p == "." || slices.Contains(strings.Split(p, "/"), ".."):
		return port.Errorf(port.CodeInvalidManifest, "skill file path %q is not clean or climbs out of the folder", p)
	case strings.ContainsFunc(p, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		return port.Errorf(port.CodeInvalidManifest, "skill file path %q has a control character", p)
	}
	return nil
}

// SkillDigest is the bundle digest: SHA-256 over the sorted path and sha256 pairs of the files.
func SkillDigest(files []port.SkillFile) string {
	lines := make([]string, len(files))
	for i, f := range files {
		lines[i] = f.Path + "\x00" + f.SHA256 + "\n"
	}
	slices.Sort(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// LooksExecutable reports whether a file is under scripts/, has a script extension or starts with a shebang.
func LooksExecutable(f port.SkillFile) bool {
	return strings.HasPrefix(f.Path, "scripts/") || slices.Contains(scriptExts, strings.ToLower(path.Ext(f.Path))) || bytes.HasPrefix(f.Content, []byte("#!"))
}

func lintSkill(s *port.Skill, bad func(string, ...any)) {
	if s == nil {
		bad("a skill listing carries skill")
		return
	}
	if !SkillName(s.Name) {
		bad("skill.name %q is lowercase letters, digits and hyphens, at most 64", s.Name)
	}
	if s.Source.Repository == "" || s.Source.Path == "" || path.Base(s.Source.Path) != s.Name {
		bad("skill.source names the repository and a folder path whose last segment is %q", s.Name)
	}
	if !FullCommit(s.Source.Commit) {
		bad("skill.source.commit %q is a full 40-hex commit", s.Source.Commit)
	}
	if s.Redistribution != port.RedistributionBundled && s.Redistribution != port.RedistributionMetadataOnly {
		bad("skill.redistribution %q is bundled or metadata_only", s.Redistribution)
	}
	if s.Redistribution == port.RedistributionBundled && (s.Licence.SPDX == "" || !s.Licence.Redistributable) {
		bad("a skill without a redistributable licence is metadata_only")
	}
	if len(s.Files) == 0 || len(s.Files) > MaxSkillFiles {
		bad("a skill has 1 to %d files, not %d", MaxSkillFiles, len(s.Files))
	}
	seen, total, entry := map[string]bool{}, int64(0), false
	for _, f := range s.Files {
		if err := SkillPath(f.Path); err != nil {
			bad("%s", err.(*port.Error).Detail)
			continue
		}
		if seen[f.Path] {
			bad("skill file %q is listed twice", f.Path)
		}
		seen[f.Path], entry, total = true, entry || f.Path == SkillEntry, total+f.Bytes
		if f.Bytes < 0 || f.Bytes > MaxSkillFileBytes {
			bad("skill file %q has %d bytes; at most %d", f.Path, f.Bytes, MaxSkillFileBytes)
		}
		if !sha256Hex.MatchString(f.SHA256) {
			bad("skill file %q carries a 64-hex sha256", f.Path)
		}
		if LooksExecutable(f) && !s.HasScripts {
			bad("skill file %q looks executable and has_scripts is false", f.Path)
		}
		switch {
		case s.Redistribution == port.RedistributionMetadataOnly && f.Content != nil:
			bad("a metadata_only skill carries no file content (%s)", f.Path)
		case s.Redistribution == port.RedistributionBundled && int64(len(f.Content)) != f.Bytes:
			bad("skill file %q has %d bytes of content, not %d", f.Path, len(f.Content), f.Bytes)
		case s.Redistribution == port.RedistributionBundled:
			if sum := sha256.Sum256(f.Content); hex.EncodeToString(sum[:]) != f.SHA256 {
				bad("skill file %q does not match its sha256", f.Path)
			}
		}
	}
	if !entry {
		bad("a skill folder holds %s", SkillEntry)
	}
	if total > MaxSkillBytes {
		bad("a skill has at most %d bytes, not %d", MaxSkillBytes, total)
	}
}
