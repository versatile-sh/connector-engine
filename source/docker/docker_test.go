package docker_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/versatile-sh/connector-engine/internal/sourcetest"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/source/docker"
)

func TestImportsTheCatalog(t *testing.T) {
	got := sourcetest.Import(t, docker.New("testdata", nil), "com.docker.mcp/github-official")
	if len(got) != 2 {
		t.Fatalf("imported %d", len(got))
	}
	gh := got["com.docker.mcp/github-official"]
	if gh.Kind != port.KindMCPPackage || gh.Artifact.Type != "oci" || gh.Artifact.Ref != "mcp/github-mcp-server" || gh.Version != "0ba9e31" ||
		len(gh.Egress.AllowHosts) != 1 || gh.Egress.AllowHosts[0] != "api.github.com:443" || gh.Title != "GitHub Official" {
		t.Fatalf("image mapping: %+v", gh)
	}
	if len(gh.Config) != 2 || gh.Config[0].Key != "GITHUB_PERSONAL_ACCESS_TOKEN" || !gh.Config[0].Sensitive || gh.Config[1].Sensitive || len(gh.Auth) != 1 || gh.Auth[0].Kind != port.CredentialOAuth2User {
		t.Fatalf("config mapping: %+v %+v", gh.Config, gh.Auth)
	}
	nr := got["com.docker.mcp/notion-remote"]
	if nr.Kind != port.KindMCPRemote || nr.Remote.URL != "https://mcp.notion.example/mcp" || nr.Version != "latest" {
		t.Fatalf("remote mapping: %+v", nr)
	}
}

func TestRefusesAServerWithoutImageOrRemote(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "x"), 0o755)
	os.WriteFile(filepath.Join(dir, "x", "server.yaml"), []byte("name: x\n"), 0o600)
	if _, err := docker.New(dir, nil).List(context.Background(), ""); !port.IsCode(err, port.CodeInvalidManifest) {
		t.Fatalf("got %v", err)
	}
}
