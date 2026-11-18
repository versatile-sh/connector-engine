// Package docker is the ListingSource over a checkout of the Docker MCP catalog: one server.yaml per server.
package docker

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agents-control/connector-engine/internal/importer"
	"github.com/agents-control/connector-engine/port"
)

// Registry is the name recorded as the provenance of a catalog import.
const Registry = "docker-mcp-catalog"

type serverYAML struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	About struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Icon        string `json:"icon"`
	} `json:"about"`
	Source struct {
		Commit string `json:"commit"`
	} `json:"source"`
	Run struct {
		AllowHosts []string `json:"allowHosts"`
	} `json:"run"`
	Config struct {
		Secrets []struct {
			Name string `json:"name"`
			Env  string `json:"env"`
		} `json:"secrets"`
		Env []struct {
			Name string `json:"name"`
		} `json:"env"`
	} `json:"config"`
	OAuth  []struct{ Provider string } `json:"oauth"`
	Remote struct {
		Transport string `json:"transport_type"`
		URL       string `json:"url"`
	} `json:"remote"`
}

// New returns a source that reads every server.yaml under dir, in path order; now may be nil.
func New(dir string, now func() time.Time) *importer.Static {
	return &importer.Static{Registry: Registry, Now: now, Load: func(context.Context) ([]port.Manifest, error) { return load(dir) }}
}

func load(dir string) ([]port.Manifest, error) {
	var out []port.Manifest
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "server.yaml" {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var s serverYAML
		if err := importer.Decode(data, &s); err != nil {
			return port.Errorf(port.CodeInvalidManifest, "%s: %v", path, err)
		}
		m, err := manifest(s)
		if err != nil {
			return port.Errorf(port.CodeInvalidManifest, "%s: %v", path, err)
		}
		out = append(out, m)
		return nil
	})
	if err != nil && !port.IsCode(err, port.CodeInvalidManifest) {
		return nil, port.Errorf(port.CodeNotFound, "catalog directory: %v", err)
	}
	return out, err
}

func manifest(s serverYAML) (port.Manifest, error) {
	if s.Name == "" {
		return port.Manifest{}, port.Errorf(port.CodeInvalidManifest, "a server.yaml carries a name")
	}
	m := port.Manifest{
		Name:        "com.docker.mcp/" + s.Name,
		Version:     s.Source.Commit,
		Title:       s.About.Title,
		Description: s.About.Description,
		Icon:        s.About.Icon,
		Publisher:   port.Publisher{Name: "docker"},
	}
	switch {
	case s.Remote.URL != "":
		m.Kind, m.Remote = port.KindMCPRemote, &port.Remote{URL: s.Remote.URL, Transport: s.Remote.Transport}
	case s.Image != "":
		m.Kind, m.Artifact = port.KindMCPPackage, &port.Artifact{Type: "oci", Ref: s.Image}
		if m.Version == "" {
			_, m.Version, _ = strings.Cut(s.Image, ":")
		}
	default:
		return m, port.Errorf(port.CodeInvalidManifest, "%s has neither an image nor a remote", s.Name)
	}
	if m.Version == "" {
		m.Version = "latest"
	}
	if len(s.Run.AllowHosts) > 0 {
		m.Egress = &port.Egress{AllowHosts: s.Run.AllowHosts}
	}
	for _, sec := range s.Config.Secrets {
		m.Config = append(m.Config, port.ConfigField{Key: sec.Env, Type: "string", Sensitive: true, Required: true})
	}
	for _, e := range s.Config.Env {
		m.Config = append(m.Config, port.ConfigField{Key: e.Name, Type: "string"})
	}
	for range s.OAuth {
		m.Auth = append(m.Auth, port.AuthSpec{Kind: port.CredentialOAuth2User})
	}
	return m, nil
}
