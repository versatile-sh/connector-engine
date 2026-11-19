package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/agents-control/connector-engine/port"
)

const maxListingBytes = 256 << 10

var slugUnsafe = regexp.MustCompile(`[^a-z0-9-]+`)

// ParseManifest reads a listing in the engine's Manifest format, YAML or JSON.
func ParseManifest(data []byte) (port.Manifest, error) {
	var generic any
	if err := yaml.Unmarshal(data, &generic); err != nil {
		return port.Manifest{}, fmt.Errorf("the listing is not YAML: %w", err)
	}
	raw, err := json.Marshal(generic)
	if err != nil {
		return port.Manifest{}, fmt.Errorf("the listing has keys that are not strings: %w", err)
	}
	var m port.Manifest
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return port.Manifest{}, fmt.Errorf("the listing is not an engine manifest: %w", err)
	}
	if m.Name == "" || m.Kind == "" {
		return port.Manifest{}, fmt.Errorf("the listing has no name or kind")
	}
	return m, nil
}

// FetchManifest downloads a listing over public https only; a deep link never reaches a local address.
func FetchManifest(ctx context.Context, rawURL string) (port.Manifest, error) {
	body, _, err := Reach{}.Get(ctx, rawURL, maxListingBytes, "application/yaml, application/json, text/plain")
	if err != nil {
		return port.Manifest{}, err
	}
	return ParseManifest(body)
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
		return port.Manifest{}, err
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

func firstOf(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
