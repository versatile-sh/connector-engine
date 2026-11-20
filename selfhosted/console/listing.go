package console

import (
	"bytes"
	"cmp"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	"github.com/versatile-sh/connector-engine/internal/importer"
	"github.com/versatile-sh/connector-engine/port"
)

const maxListingBytes = 256 << 10

var slugUnsafe = regexp.MustCompile(`[^a-z0-9-]+`)

// ParseManifest reads a listing in the engine's Manifest format, YAML or JSON.
func ParseManifest(data []byte) (port.Manifest, error) {
	raw, err := importer.YAMLToJSON(data)
	if err != nil {
		return port.Manifest{}, invalid("the listing is not YAML: %w", err)
	}
	var m port.Manifest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return port.Manifest{}, invalid("the listing is not an engine manifest: %w", err)
	}
	if m.Name == "" || m.Kind == "" {
		return port.Manifest{}, invalid("the listing has no name or kind")
	}
	return m, nil
}

// Untrusted keeps what a third-party listing may say about itself and drops what only a review gives: trust level, review, signature, provenance and declared function classes and hashes.
func Untrusted(m port.Manifest) port.Manifest {
	m.Owner, m.Source, m.Review, m.Signature, m.TrustLevel, m.InstructionsHash = "", nil, nil, nil, "", ""
	fns := make([]port.FunctionDef, len(m.Functions))
	for i, f := range m.Functions {
		fns[i] = port.FunctionDef{Name: f.Name, Title: f.Title, Description: f.Description, InputSchema: f.InputSchema, OutputSchema: f.OutputSchema, Annotations: f.Annotations, MetaUI: f.MetaUI}
	}
	m.Functions = fns
	return m
}

func manifestFrom(rawURL string, body []byte) (port.Manifest, error) {
	m, err := ParseManifest(body)
	if err != nil {
		return port.Manifest{}, err
	}
	m.Icon = ResolveIcon(rawURL, m.Icon)
	return Untrusted(m), nil
}

// ResolveIcon makes a relative icon reference absolute against the listing URL it came from; absolute and data URIs pass unchanged.
func ResolveIcon(listingURL, icon string) string {
	if icon == "" || strings.HasPrefix(icon, "data:") {
		return icon
	}
	ref, err := url.Parse(icon)
	if err != nil || ref.IsAbs() {
		return icon
	}
	base, err := url.Parse(listingURL)
	if err != nil {
		return icon
	}
	return base.ResolveReference(ref).String()
}

// Slug lowers s to [a-z0-9-] with no leading, trailing or doubled dash.
func Slug(s string) string {
	s = slugUnsafe.ReplaceAllString(strings.ToLower(s), "-")
	return strings.Trim(s, "-")
}

// ManifestFor turns a probed MCP server into a private remote listing; headerSecret declares that calls carry a header the owner configures.
func ManifestFor(p Probed, headerSecret bool) (port.Manifest, error) {
	u, err := url.Parse(p.URL)
	if err != nil {
		return port.Manifest{}, invalid("%w", err)
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "loopback"
	}
	if !strings.Contains(host, ".") {
		host += ".local"
	}
	name := Slug(p.Name)
	if name == "" {
		name = Slug(strings.Trim(u.Path, "/"))
	}
	if name == "" {
		name = "mcp"
	}
	version := p.Version
	if version == "" {
		version = "1.0.0"
	}
	m := port.Manifest{
		Name: host + "/" + name, Version: version, Title: firstOf(p.Title, p.Name), Description: p.Description, Icon: p.Icon,
		Kind: port.KindMCPRemote, Remote: &port.Remote{URL: p.URL, Transport: "streamable-http"}, Publisher: port.Publisher{Name: u.Host},
	}
	if headerSecret {
		m.Auth = []port.AuthSpec{{Kind: port.CredentialCustomHeaders, OwnerKindsAllowed: []port.OwnerKind{port.OwnerWorkspace}}}
	}
	return m, nil
}

var firstOf = cmp.Or[string]
