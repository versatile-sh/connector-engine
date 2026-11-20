// Package registry is the ListingSource over the official MCP registry API v0.1, or any registry that speaks it.
package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/internal/egress"
	"github.com/versatile-sh/connector-engine/internal/fetch"
	"github.com/versatile-sh/connector-engine/internal/importer"
	"github.com/versatile-sh/connector-engine/port"
)

// DefaultURL is the official registry.
const DefaultURL = "https://registry.modelcontextprotocol.io"

// Config selects the registry and what to read from it.
type Config struct {
	Name              string
	BaseURL           string
	UpdatedSince      time.Time
	PageSize          int
	NamespaceVerified bool
	Token             string
	HTTP              *http.Client
	Guard             egress.Guard
	Now               func() time.Time
}

// Source reads GET /v0.1/servers with a cursor and updated_since, and one version at a time.
type Source struct{ cfg Config }

// New returns a source; NamespaceVerified records the registry's namespace proof as publisher.verified_by, and only an official registry should set it.
func New(cfg Config) *Source {
	if cfg.Name == "" {
		cfg.Name = "mcp-registry"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultURL
	}
	if cfg.HTTP == nil {
		cfg.HTTP = cfg.Guard.Client()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Source{cfg: cfg}
}

func (s *Source) headers() map[string]string {
	h := map[string]string{"Accept": "application/json"}
	if s.cfg.Token != "" {
		h["Authorization"] = "Bearer " + s.cfg.Token
	}
	return h
}

// Name is the registry name recorded as provenance.
func (s *Source) Name() string { return s.cfg.Name }

type entry struct {
	Server serverJSON `json:"server"`
	Meta   map[string]struct {
		Status string `json:"status"`
	} `json:"_meta"`
}

type serverJSON struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Icons       []struct {
		Src string `json:"src"`
	} `json:"icons"`
	Packages []struct {
		RegistryType string      `json:"registryType"`
		Identifier   string      `json:"identifier"`
		FileSHA256   string      `json:"fileSha256"`
		Variables    []fieldJSON `json:"environmentVariables"`
	} `json:"packages"`
	Remotes []struct {
		Type    string      `json:"type"`
		URL     string      `json:"url"`
		Headers []fieldJSON `json:"headers"`
	} `json:"remotes"`
}

type fieldJSON struct {
	Name       string `json:"name"`
	IsRequired bool   `json:"isRequired"`
	IsSecret   bool   `json:"isSecret"`
}

const officialMeta = "io.modelcontextprotocol.registry/official"

func (e entry) status() string { return e.Meta[officialMeta].Status }

func (s *Source) manifest(e entry) (port.Manifest, bool) {
	sv := e.Server
	m := port.Manifest{Name: sv.Name, Version: sv.Version, Title: sv.Title, Description: sv.Description}
	m.Publisher.Name, _, _ = strings.Cut(sv.Name, "/")
	if s.cfg.NamespaceVerified {
		m.Publisher.VerifiedBy = s.cfg.Name
	}
	if len(sv.Icons) > 0 {
		m.Icon = sv.Icons[0].Src
	}
	var fields []fieldJSON
	switch {
	case len(sv.Remotes) > 0:
		r := sv.Remotes[0]
		m.Kind, m.Remote, fields = port.KindMCPRemote, &port.Remote{URL: r.URL, Transport: r.Type}, r.Headers
	case len(sv.Packages) > 0:
		p := sv.Packages[0]
		m.Kind, m.Artifact, fields = port.KindMCPPackage, &port.Artifact{Type: p.RegistryType, Ref: p.Identifier, Digest: p.FileSHA256}, p.Variables
	default:
		return m, false
	}
	for _, f := range fields {
		m.Config = append(m.Config, port.ConfigField{Key: f.Name, Type: "string", Sensitive: f.IsSecret, Required: f.IsRequired})
	}
	m = importer.Imported(m, s.cfg.Name, s.cfg.Now())
	m.Source.Status = e.status()
	return m, true
}

// List returns one page of servers; deleted servers and servers with neither a remote nor a package are left out.
func (s *Source) List(ctx context.Context, cursor string) (port.ListingPage, error) {
	q := url.Values{"version": {"latest"}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if s.cfg.PageSize > 0 {
		q.Set("limit", strconv.Itoa(s.cfg.PageSize))
	}
	if !s.cfg.UpdatedSince.IsZero() {
		q.Set("updated_since", s.cfg.UpdatedSince.UTC().Format(time.RFC3339))
	}
	body, err := fetch.Do(ctx, s.cfg.HTTP, http.MethodGet, s.cfg.BaseURL+"/v0.1/servers?"+q.Encode(), nil, s.headers())
	if err != nil {
		return port.ListingPage{}, err
	}
	var res struct {
		Servers  []entry `json:"servers"`
		Metadata struct {
			NextCursor string `json:"nextCursor"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return port.ListingPage{}, port.Errorf(port.CodeUnavailable, "registry list: %v", err)
	}
	page := port.ListingPage{Next: res.Metadata.NextCursor}
	for _, e := range res.Servers {
		if m, ok := s.manifest(e); ok && e.status() != "deleted" {
			page.Manifests = append(page.Manifests, m)
		}
	}
	return page, nil
}

// Fetch reads one version of a server; an empty version means latest.
func (s *Source) Fetch(ctx context.Context, name, version string) (port.Manifest, error) {
	if version == "" {
		version = "latest"
	}
	u := s.cfg.BaseURL + "/v0.1/servers/" + url.PathEscape(name) + "/versions/" + url.PathEscape(version)
	body, err := fetch.Do(ctx, s.cfg.HTTP, http.MethodGet, u, nil, s.headers())
	if err != nil {
		return port.Manifest{}, err
	}
	var e entry
	if err := json.Unmarshal(body, &e); err != nil {
		return port.Manifest{}, port.Errorf(port.CodeUnavailable, "registry version: %v", err)
	}
	m, ok := s.manifest(e)
	if !ok {
		return port.Manifest{}, port.Errorf(port.CodeInvalidManifest, "%s %s has neither a remote nor a package", name, version)
	}
	return m, nil
}
