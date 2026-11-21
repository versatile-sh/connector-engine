package app_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func raw(t *testing.T, method, url, token string, body []byte) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, out
}

func TestTheSnapshotRoutesAreOffUnlessConfigured(t *testing.T) {
	e := boot(t, devConfig(t), fmt.Sprintf(localAdapters, "", "memory", ""), map[string]string{"CN_OPERATOR_TOKEN": "op-secret"})
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		if code, body := raw(t, method, e.operator+"/v1/snapshot", "op-secret", []byte(`{}`)); code != http.StatusNotFound && code != http.StatusMethodNotAllowed {
			t.Errorf("%s /v1/snapshot without server.snapshot: %d %s", method, code, body)
		}
	}
}

func TestASnapshotMovesAnEngineWithItsKeysAndStopsTheEngineItRestores(t *testing.T) {
	if pg == nil {
		t.Skip("postgres container unavailable")
	}
	engineOn := func(dir string) (func() *engine, string) {
		dsn, err := pg.NewDatabase()
		if err != nil {
			t.Fatal(err)
		}
		env := map[string]string{"CN_OPERATOR_TOKEN": "op-secret", "CN_DATABASE_URL": dsn}
		adaptersYAML := strings.Replace(fmt.Sprintf(localAdapters, "", "builtin:postgres", ", config: {databaseUrlRef: env:CN_DATABASE_URL}"), "secrets: {use: memory}", "secrets: {use: builtin:localfile}", 1)
		cfg := devConfig(t)
		cfg.Server.Snapshot = true
		cfg.Secrets.Adapter, cfg.Secrets.Dir = "localfile", dir
		return func() *engine { return boot(t, cfg, adaptersYAML, env) }, dsn
	}
	bootOld, _ := engineOn(t.TempDir())
	old := bootOld()
	exercise(t, old)
	ana := old.token(map[string]any{"sub": "ana", "tenant": "t1"})
	if code, _ := raw(t, http.MethodGet, old.operator+"/v1/snapshot", "wrong", nil); code != http.StatusUnauthorized {
		t.Fatalf("a snapshot for a wrong token: %d", code)
	}
	if code, _ := raw(t, http.MethodGet, old.http+"/v1/snapshot", "op-secret", nil); code == http.StatusOK {
		t.Fatal("the public listener serves the snapshot")
	}
	code, snap := raw(t, http.MethodGet, old.operator+"/v1/snapshot", "op-secret", nil)
	if code != http.StatusOK || !strings.Contains(string(snap), `"root"`) || !strings.Contains(string(snap), "local-issuer.json") {
		t.Fatalf("snapshot: %d %.200s", code, snap)
	}

	bootNew, _ := engineOn(t.TempDir())
	fresh := bootNew()
	if code, body := raw(t, http.MethodPut, fresh.operator+"/v1/snapshot", "op-secret", []byte(`{"store":{"store":"memory"}}`)); code != http.StatusBadRequest {
		t.Fatalf("a snapshot of another store: %d %s", code, body)
	}
	if code, body := raw(t, http.MethodPut, fresh.operator+"/v1/snapshot", "op-secret", snap); code != http.StatusOK || !strings.Contains(string(body), `"counts"`) {
		t.Fatalf("restore: %d %s", code, body)
	}
	select {
	case err := <-fresh.done:
		if err != nil {
			t.Fatalf("the restored engine stopped with %v", err)
		}
		fresh.done <- nil
	case <-time.After(10 * time.Second):
		t.Fatal("the restored engine did not stop")
	}
	again := bootNew()
	if code, out := again.call(http.MethodGet, again.http+"/v1/roles?tenant=t1&person=bia", ana, nil); code != http.StatusOK || len(out["items"].([]any)) != 1 {
		t.Fatalf("the old token and roles after the restore: %d %v", code, out)
	}
}
