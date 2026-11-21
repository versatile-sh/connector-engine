package console

import (
	"encoding/base64"
	"testing"
)

func TestBearerClaimsAreReadOnlyFromAnUnambiguousPayload(t *testing.T) {
	bearer := func(payload string) string {
		return "Bearer h." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".s"
	}
	if c, ok := bearerClaims(bearer(`{"agent":"a","session":"s","iat":5,"Other":1}`)); !ok || c.Agent != "a" || c.Session != "s" {
		t.Fatalf("a plain payload is read: %+v %v", c, ok)
	}
	for _, payload := range []string{
		`{"agent":"a","agent":"b"}`,
		`{"agent":"a","Agent":"b"}`,
		`{"Agent":"b"}`,
		`{"agent":"a","SESSION":"x"}`,
		`{"agent":"a"} {}`,
		`["agent"]`,
	} {
		if c, ok := bearerClaims(bearer(payload)); ok {
			t.Errorf("%s was read as %+v", payload, c)
		}
	}
}
