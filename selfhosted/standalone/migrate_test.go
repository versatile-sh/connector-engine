package standalone

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/app"
	"github.com/versatile-sh/connector-engine/internal/plugkit/adapters"
	"github.com/versatile-sh/connector-engine/port"
	secretslocal "github.com/versatile-sh/connector-engine/secrets/local"
	"github.com/versatile-sh/connector-engine/selfhosted"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
	"github.com/versatile-sh/connector-engine/store/storetest"
)

const passphrase = "a passphrase long enough"

func dockerInstall(t *testing.T, dsn string) (string, *console.Console) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	p, err := prepare(shortDir(t))
	if err != nil {
		t.Fatal(err)
	}
	reg := app.Core()
	selfhosted.Register(reg)
	cfg, file, err := engineProfile(p, reg)
	if err != nil {
		t.Fatal(err)
	}
	file.Ports["store"] = adapters.Port{Use: adapters.Uses{"builtin:postgres"}, Config: map[string]any{"databaseUrlRef": "env:CN_TEST_DSN"}}
	file.Ports["secrets"] = adapters.Port{Use: adapters.Uses{"builtin:localfile"}}
	a, err := app.New(ctx, cfg, file, app.Options{Lookup: func(string) string { return dsn }, Log: quiet(), Adapters: reg})
	if err != nil {
		t.Fatal(err)
	}
	l, err := a.Listen()
	if err != nil {
		t.Fatal(err)
	}
	ran := make(chan error, 1)
	go func() { ran <- a.Run(ctx, l) }()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + lis.Addr().String()
	c, closeConsole, err := console.Open(ctx, console.Options{PublicURL: base, EngineURL: "http://" + l.HTTP.Addr().String(), OperatorURL: "http://" + l.Operator.Addr().String(),
		Operator: p.operator, StatePath: p.consoleState, Tenant: "local", Workspace: "default", Person: "owner", AgentTTL: time.Hour, Log: quiet(),
		Unlock: func(context.Context) (port.SecretStore, error) { return secretslocal.Open(p.consoleSecrets) }})
	if err != nil {
		t.Fatal(err)
	}
	srv := &httptest.Server{Listener: lis, Config: &http.Server{Handler: c.Handler()}}
	srv.Start()
	t.Cleanup(func() { srv.Close(); closeConsole(); cancel(); <-ran; a.Close() })
	return base, c
}

func TestImportsTheBackupOfADockerInstall(t *testing.T) {
	pg, err := storetest.StartPostgres()
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(pg.Stop)
	dsn, err := pg.NewDatabase()
	if err != nil {
		t.Fatal(err)
	}
	base, docker := dockerInstall(t, dsn)
	token := seed(t, base)
	if err := callSearch(t, base+"/mcp", token); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	docker.HostHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/export", strings.NewReader(passphrase)))
	if rec.Code != http.StatusOK {
		t.Fatalf("export: %d %s", rec.Code, rec.Body)
	}
	backup := filepath.Join(t.TempDir(), "docker.cebak")
	if err := os.WriteFile(backup, rec.Body.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	data := shortDir(t)
	in := start(t, data, fakeKeychain{})
	release := holdSession(t, in.URL+"/mcp", seed(t, in.URL))
	defer release()
	if code, out := run(t, "import", passphrase+"\n", "--data", data, "--yes", backup); code != 0 {
		t.Fatalf("import: %d %s", code, out)
	}
	deadline := time.Now().Add(30 * time.Second)
	for err = callSearch(t, in.URL+"/mcp", token); err != nil && time.Now().Before(deadline); err = callSearch(t, in.URL+"/mcp", token) {
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("the key of the Docker install after the import: %v", err)
	}
	newBrowser(t, in.URL).signIn("/api/login")
	select {
	case <-in.done:
		t.Fatalf("the import with an attached agent ended the install: %v", in.err)
	default:
	}
}
