package local_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/auth/local"
	"github.com/agents-control/connector-engine/port"
	"github.com/go-jose/go-jose/v4"
)

func TestIssueNeedsAnExpiry(t *testing.T) {
	i, err := local.New("iss", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := i.Issue(port.Claims{Tenant: "ta"}); !port.IsCode(err, port.CodeInvalidArgument) {
		t.Fatalf("got %v", err)
	}
	if tok, err := i.Issue(port.Claims{Tenant: "ta", ExpiresAt: time.Now().Add(time.Minute)}); err != nil || tok == "" {
		t.Fatalf("issue: %q %v", tok, err)
	}
}

func TestHandlerPublishesOnlyThePublicKey(t *testing.T) {
	i, _ := local.New("iss", nil)
	rec := httptest.NewRecorder()
	i.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/jwks.json", nil))
	var set jose.JSONWebKeySet
	if err := json.Unmarshal(rec.Body.Bytes(), &set); err != nil || len(set.Keys) != 1 || !set.Keys[0].IsPublic() || set.Keys[0].Algorithm != "EdDSA" {
		t.Fatalf("key set: %s %v", rec.Body, err)
	}
}
