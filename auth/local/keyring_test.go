package local_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/agents-control/connector-engine/auth/local"
	"github.com/agents-control/connector-engine/port"
	secretsmemory "github.com/agents-control/connector-engine/secrets/memory"
)

func published(t *testing.T, i *local.Issuer) []string {
	t.Helper()
	rec := httptest.NewRecorder()
	i.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/jwks", nil))
	var set jose.JSONWebKeySet
	if err := json.Unmarshal(rec.Body.Bytes(), &set); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, k := range set.Keys {
		ids = append(ids, k.KeyID)
	}
	return ids
}

func signedBy(t *testing.T, i *local.Issuer, at time.Time) string {
	t.Helper()
	tok, err := i.Issue(port.Claims{Tenant: "ta", ExpiresAt: at.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := jwt.ParseSigned(tok, []jose.SignatureAlgorithm{jose.EdDSA})
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Headers[0].KeyID
}

func TestTheIssuerKeySurvivesARestartAndRotatesWithAGracePeriod(t *testing.T) {
	ctx, secrets := context.Background(), secretsmemory.New()
	at := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return at }
	ring := local.Keyring{Path: filepath.Join(t.TempDir(), "local-issuer.json"), Secrets: secrets, RotateEvery: 30 * 24 * time.Hour, Keep: 24 * time.Hour}
	open := func() *local.Issuer {
		t.Helper()
		i, err := local.Open(ctx, "iss", clock, ring)
		if err != nil {
			t.Fatal(err)
		}
		return i
	}
	first := open()
	kid := signedBy(t, first, at)
	if again := open(); signedBy(t, again, at) != kid || len(published(t, again)) != 1 {
		t.Fatalf("a restart changed the signing key: %v", published(t, again))
	}
	at = at.Add(31 * 24 * time.Hour)
	rotated := open()
	if signedBy(t, rotated, at) == kid {
		t.Fatal("a key older than RotateEvery still signs")
	}
	if ids := published(t, rotated); len(ids) != 2 || ids[0] != kid {
		t.Fatalf("the retired key is not published during the grace period: %v", ids)
	}
	at = at.Add(25 * time.Hour)
	if ids := published(t, open()); len(ids) != 1 || ids[0] == kid {
		t.Fatalf("the retired key outlived Keep: %v", ids)
	}
}
