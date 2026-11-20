// Package skillcatalog reads skill catalog entries at https addresses and turns each into a skill listing whose files are fetched and checked against their digests.
package skillcatalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/agents-control/connector-engine/core/listing"
	"github.com/agents-control/connector-engine/internal/egress"
	"github.com/agents-control/connector-engine/internal/fetch"
	"github.com/agents-control/connector-engine/internal/importer"
	"github.com/agents-control/connector-engine/port"
)

// Registry is the name recorded as the provenance of a skill catalog import.
const Registry = "skillcatalog"

// MaxEntryBytes is the largest catalog entry document read.
const MaxEntryBytes = 256 << 10

var entryID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

// Get fetches one address and returns its body, refusing more than limit bytes.
type Get func(ctx context.Context, rawURL string, limit int64) ([]byte, error)

// Entry is one catalog entry document: a skill folder pinned to a commit, its licence and its files.
type Entry struct {
	Kind        string       `json:"kind"`
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Category    string       `json:"category"`
	Source      EntrySource  `json:"source"`
	License     EntryLicense `json:"license"`
	HasScripts  bool         `json:"has_scripts"`
	Files       []EntryFile  `json:"files"`
}

// EntrySource is where the catalog read the skill folder.
type EntrySource struct {
	Repository string `json:"repository"`
	Path       string `json:"path"`
	Commit     string `json:"commit"`
	URL        string `json:"url"`
}

// EntryLicense is the licence the catalog found for the skill and whether it may be redistributed.
type EntryLicense struct {
	SPDX            string `json:"spdx"`
	From            string `json:"from"`
	Redistributable bool   `json:"redistributable"`
}

// EntryFile is one file of the folder with its size, digest and raw address pinned to the commit.
type EntryFile struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	URL    string `json:"url"`
}

// Kind returns the kind a YAML or JSON document declares, empty when it declares none.
func Kind(data []byte) string {
	var k struct {
		Kind string `json:"kind"`
	}
	_ = importer.Decode(data, &k)
	return k.Kind
}

func invalid(format string, args ...any) error {
	return port.Errorf(port.CodeInvalidManifest, "skill entry: "+format, args...)
}

// Parse reads an entry strictly and refuses branch names, unsafe paths, oversize files and file addresses not pinned to the commit.
func Parse(data []byte) (Entry, error) {
	raw, err := importer.YAMLToJSON(data)
	if err != nil {
		return Entry{}, err
	}
	var e Entry
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&e); err != nil {
		return Entry{}, invalid("%v", err)
	}
	switch {
	case e.Kind != string(port.KindSkill):
		return Entry{}, invalid("kind %q is not skill", e.Kind)
	case !entryID.MatchString(e.ID):
		return Entry{}, invalid("id %q is lowercase letters, digits, dots, dashes and underscores", e.ID)
	case !listing.SkillName(e.Name) || path.Base(e.Source.Path) != e.Name:
		return Entry{}, invalid("name %q is an Agent Skills name equal to the folder of source.path %q", e.Name, e.Source.Path)
	case !listing.FullCommit(e.Source.Commit):
		return Entry{}, invalid("source.commit %q is not a full 40-hex commit", e.Source.Commit)
	case listing.SkillPath(e.Source.Path) != nil:
		return Entry{}, invalid("source.path %q is not a clean relative path", e.Source.Path)
	case len(e.Files) == 0 || len(e.Files) > listing.MaxSkillFiles:
		return Entry{}, invalid("an entry lists 1 to %d files", listing.MaxSkillFiles)
	}
	total := int64(0)
	for _, f := range e.Files {
		if err := listing.SkillPath(f.Path); err != nil {
			return Entry{}, err
		}
		if f.Bytes < 0 || f.Bytes > listing.MaxSkillFileBytes {
			return Entry{}, invalid("file %s has %d bytes; at most %d", f.Path, f.Bytes, listing.MaxSkillFileBytes)
		}
		total += f.Bytes
		if err := pinned(f.URL, e.Source, f.Path); err != nil {
			return Entry{}, err
		}
	}
	if total > listing.MaxSkillBytes {
		return Entry{}, invalid("the files have %d bytes; at most %d", total, listing.MaxSkillBytes)
	}
	return e, nil
}

func pinned(raw string, s EntrySource, file string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return invalid("file %s is fetched from a plain https address, not %q", file, raw)
	}
	if !strings.HasSuffix(u.EscapedPath(), "/"+s.Commit+"/"+s.Path+"/"+file) || strings.Contains(u.Path, "/../") {
		return invalid("file %s address %q is not pinned to commit %s at %s", file, raw, s.Commit, s.Path)
	}
	return nil
}

// Redistribution is bundled for a redistributable licence with an SPDX id and metadata_only otherwise.
func (e Entry) Redistribution() port.Redistribution {
	if e.License.SPDX != "" && e.License.Redistributable {
		return port.RedistributionBundled
	}
	return port.RedistributionMetadataOnly
}

// SkillFiles are the entry's files without content.
func (e Entry) SkillFiles() []port.SkillFile {
	out := make([]port.SkillFile, len(e.Files))
	for i, f := range e.Files {
		out[i] = port.SkillFile{Path: f.Path, Bytes: f.Bytes, SHA256: f.SHA256, URL: f.URL}
	}
	return out
}

// Digest is the bundle digest the entry declares, over its paths and sha256 values.
func (e Entry) Digest() string { return listing.SkillDigest(e.SkillFiles()) }

func publisher(repository, fallback string) string {
	u, err := url.Parse(repository)
	if err != nil {
		return fallback
	}
	if owner, _, _ := strings.Cut(strings.Trim(u.Path, "/"), "/"); owner != "" {
		return owner
	}
	return fallback
}

// Manifest turns an entry read at entryURL into a skill listing; a bundled entry fetches every file and checks its size and sha256.
func Manifest(ctx context.Context, get Get, entryURL string, e Entry) (port.Manifest, error) {
	u, err := url.Parse(entryURL)
	if err != nil || u.Hostname() == "" {
		return port.Manifest{}, invalid("entry address %q is not absolute", entryURL)
	}
	files := e.SkillFiles()
	if e.Redistribution() == port.RedistributionBundled {
		for i := range files {
			body, err := get(ctx, files[i].URL, files[i].Bytes)
			if err != nil {
				return port.Manifest{}, port.Errorf(port.CodeUnavailable, "skill file %s: %v", files[i].Path, err)
			}
			sum := sha256.Sum256(body)
			if int64(len(body)) != files[i].Bytes || hex.EncodeToString(sum[:]) != files[i].SHA256 {
				return port.Manifest{}, invalid("file %s does not match its declared size and sha256", files[i].Path)
			}
			files[i].Content = body
		}
	}
	hasScripts := e.HasScripts
	for _, f := range files {
		hasScripts = hasScripts || listing.LooksExecutable(f)
	}
	m := port.Manifest{Name: strings.ToLower(u.Hostname()) + "/" + e.ID, Version: e.Source.Commit, Title: e.Title, Description: e.Description, Kind: port.KindSkill,
		Publisher: port.Publisher{Name: publisher(e.Source.Repository, u.Hostname())}, Licence: e.License.SPDX,
		Skill: &port.Skill{Name: e.Name, Category: e.Category, Source: port.SkillSource{Repository: e.Source.Repository, Path: e.Source.Path, Commit: e.Source.Commit, URL: e.Source.URL},
			Licence: port.SkillLicence{SPDX: e.License.SPDX, From: e.License.From, Redistributable: e.License.Redistributable}, Redistribution: e.Redistribution(),
			HasScripts: hasScripts, Files: files}}
	return m, listing.Validate(m, true)
}

// Load fetches the entry at entryURL, parses it and builds its listing.
func Load(ctx context.Context, get Get, entryURL string) (port.Manifest, error) {
	data, err := get(ctx, entryURL, MaxEntryBytes)
	if err != nil {
		return port.Manifest{}, err
	}
	e, err := Parse(data)
	if err != nil {
		return port.Manifest{}, err
	}
	return Manifest(ctx, get, entryURL, e)
}

// Config names the entry addresses of a source; the engine ships none.
type Config struct {
	URLs  []string
	HTTP  *http.Client
	Guard egress.Guard
	Now   func() time.Time
}

// Fetcher is a Get over client that accepts https addresses only.
func Fetcher(client *http.Client) Get {
	return func(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
		if u, err := url.Parse(rawURL); err != nil || u.Scheme != "https" {
			return nil, port.Errorf(port.CodeInvalidArgument, "only https addresses are fetched, not %q", rawURL)
		}
		body, err := fetch.Do(ctx, client, http.MethodGet, rawURL, nil, nil)
		if err == nil && int64(len(body)) > limit {
			return nil, port.Errorf(port.CodeUnavailable, "%s: the body exceeds %d bytes", rawURL, limit)
		}
		return body, err
	}
}

// New returns a ListingSource over the entries of cfg, read on every list.
func New(cfg Config) *importer.Static {
	if cfg.HTTP == nil {
		cfg.HTTP = cfg.Guard.Client()
	}
	get := Fetcher(cfg.HTTP)
	return &importer.Static{Registry: Registry, Now: cfg.Now, Load: func(ctx context.Context) ([]port.Manifest, error) {
		out := make([]port.Manifest, 0, len(cfg.URLs))
		for _, u := range cfg.URLs {
			m, err := Load(ctx, get, u)
			if err != nil {
				return nil, err
			}
			out = append(out, m)
		}
		return out, nil
	}}
}
