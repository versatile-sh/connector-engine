// Package skill is the ConnectorRuntime that serves the pinned files of a skill listing as text; it starts no process and opens no connection.
package skill

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agents-control/connector-engine/core/listing"
	"github.com/agents-control/connector-engine/internal/handles"
	"github.com/agents-control/connector-engine/port"
)

// Runtime answers a skill listing from the bytes stored in its manifest.
type Runtime struct {
	started *handles.Registry[port.Manifest]
}

var _ port.ConnectorRuntime = (*Runtime)(nil)

// New returns a skill runtime.
func New() *Runtime { return &Runtime{started: handles.New[port.Manifest]("skill")} }

// Describe states a runtime with no process and no egress.
func (*Runtime) Describe() port.RuntimeCapabilities {
	return port.RuntimeCapabilities{Kinds: []port.Kind{port.KindSkill}, Isolation: "none", Egress: "none", UnaryOnly: true}
}

// Start keeps the manifest of a valid skill listing.
func (r *Runtime) Start(_ context.Context, _ port.Connector, m port.Manifest, _ port.SecretsRef) (port.Handle, error) {
	if m.Kind != port.KindSkill {
		return "", port.Errorf(port.CodeInvalidManifest, "the skill runtime serves skill listings")
	}
	if err := listing.Validate(m, true); err != nil {
		return "", err
	}
	return r.started.Add(m), nil
}

// FileView is a skill file without its bytes, as a listing or a load result names it.
type FileView struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// Meta is what a skill function definition carries under port.SkillMetaKey; its digest pins the whole folder.
type Meta struct {
	Digest         string              `json:"digest"`
	Redistribution port.Redistribution `json:"redistribution"`
	HasScripts     bool                `json:"has_scripts"`
	Licence        port.SkillLicence   `json:"licence"`
	Source         port.SkillSource    `json:"source"`
	Requires       []string            `json:"requires,omitempty"`
	Files          []FileView          `json:"files"`
}

func views(files []port.SkillFile) []FileView {
	out := make([]FileView, len(files))
	for i, f := range files {
		out[i] = FileView{Path: f.Path, Bytes: f.Bytes, SHA256: f.SHA256}
	}
	return out
}

// Definition is the one function a skill listing has: its name, description, a path argument and the pinned folder in _meta.
func Definition(m port.Manifest) (port.RawDefinition, error) {
	s := m.Skill
	no := false
	def := map[string]any{
		"name": port.SkillFunction, "title": m.Title, "description": m.Description,
		"inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "A file of the skill folder; SKILL.md when empty."}}},
		"annotations": map[string]any{"readOnlyHint": true, "openWorldHint": &no},
		"_meta": map[string]any{port.SkillMetaKey: Meta{Digest: listing.SkillDigest(s.Files), Redistribution: s.Redistribution, HasScripts: s.HasScripts,
			Licence: s.Licence, Source: s.Source, Requires: s.Requires, Files: views(s.Files)}},
	}
	raw, err := json.Marshal(def)
	return port.RawDefinition{Name: port.SkillFunction, Raw: raw}, err
}

// ListFunctions returns the skill's one definition.
func (r *Runtime) ListFunctions(_ context.Context, h port.Handle) ([]port.RawDefinition, port.ListMeta, error) {
	m, err := r.started.Get(h)
	if err != nil {
		return nil, port.ListMeta{}, err
	}
	d, err := Definition(m)
	return []port.RawDefinition{d}, port.ListMeta{}, err
}

// Loaded is the structured result of a load: the file named, its bytes as text or base64, and the folder's files.
type Loaded struct {
	Skill          string              `json:"skill"`
	Path           string              `json:"path"`
	Bytes          int64               `json:"bytes"`
	SHA256         string              `json:"sha256"`
	Encoding       string              `json:"encoding,omitempty"`
	Content        string              `json:"content,omitempty"`
	Redistribution port.Redistribution `json:"redistribution"`
	Source         string              `json:"source,omitempty"`
	Files          []FileView          `json:"files"`
}

// Call returns one file of the folder as data; nothing of the folder is ever run.
func (r *Runtime) Call(_ context.Context, h port.Handle, function string, arguments json.RawMessage, _ time.Time) (port.Result, error) {
	m, err := r.started.Get(h)
	if err != nil {
		return port.Result{}, err
	}
	s := m.Skill
	if function != port.SkillFunction {
		return port.Result{}, port.Errorf(port.CodeNotFound, "skill %s has no function %s", s.Name, function)
	}
	var in struct {
		Path string `json:"path"`
	}
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &in); err != nil {
			return port.Result{}, port.Errorf(port.CodeInvalidArgument, "skill arguments: %v", err)
		}
	}
	if in.Path == "" {
		in.Path = listing.SkillEntry
	}
	out := Loaded{Skill: s.Name, Path: in.Path, Redistribution: s.Redistribution, Source: firstOf(s.Source.URL, s.Source.Repository), Files: views(s.Files)}
	var f *port.SkillFile
	for i := range s.Files {
		if s.Files[i].Path == in.Path {
			f = &s.Files[i]
		}
	}
	if f == nil {
		return result(out, header(m, out, "This skill has no file "+in.Path+"."), "", true)
	}
	out.Bytes, out.SHA256 = f.Bytes, f.SHA256
	if s.Redistribution == port.RedistributionMetadataOnly {
		return result(out, header(m, out, "This skill is listed without its files: its licence does not allow redistribution (metadata_only). Read it at "+out.Source+"."), "", false)
	}
	body := string(f.Content)
	out.Encoding, out.Content = "utf-8", body
	if !utf8.Valid(f.Content) {
		out.Encoding, out.Content = "base64", base64.StdEncoding.EncodeToString(f.Content)
		body = ""
	}
	return result(out, header(m, out, ""), body, false)
}

func header(m port.Manifest, l Loaded, note string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Skill %s, file %s, served by the connector gateway as pinned data.", m.Skill.Name, l.Path)
	if note != "" {
		b.WriteString(" " + note)
	}
	b.WriteString("\nFiles of this skill (load one with skill_load and its path):")
	for _, f := range l.Files {
		fmt.Fprintf(&b, "\n- %s (%d bytes)", f.Path, f.Bytes)
	}
	if m.Skill.HasScripts {
		b.WriteString("\nScripts are served as text; the gateway never runs them. Run one only in your own sandbox, with your user's approval.")
	}
	b.WriteString("\nTools this skill names are served as <connector>__<tool>; when they are not listed, search for them with find.")
	if l.Encoding == "base64" {
		b.WriteString("\nThis file is binary; its bytes are in the structured result as base64.")
	}
	return b.String()
}

func result(l Loaded, head, body string, isError bool) (port.Result, error) {
	blocks := []map[string]string{{"type": "text", "text": head}}
	if body != "" {
		blocks = append(blocks, map[string]string{"type": "text", "text": body})
	}
	content, err := json.Marshal(blocks)
	if err != nil {
		return port.Result{}, err
	}
	structured, err := json.Marshal(l)
	return port.Result{Content: content, Structured: structured, IsError: isError}, err
}

func firstOf(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// Stop forgets the instance.
func (r *Runtime) Stop(_ context.Context, h port.Handle) error {
	r.started.Remove(h)
	return nil
}
