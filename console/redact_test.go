package console_test

import (
	"strings"
	"testing"

	"github.com/agents-control/connector-engine/console"
)

func TestRedactHidesSecretsAndKeepsTheRest(t *testing.T) {
	in := `{"Authorization":"Bearer abc","client_secret":"s3","tokens":["a","b"],"max_tokens":12,"pin_password":1234,"has_secret":true,"author":"Ana","items":[{"apiKey":"k","n":1}],` +
		`"text":"use ghp_abcdefghijklmnopqrstuvwxyz0123 or eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTYifQ.c2lnbmF0dXJlMTIz or AKIAABCDEFGHIJKLMNOP","html":"<b>hi</b>"}`
	got, ok := console.Redact([]byte(in))
	if !ok {
		t.Fatal("valid JSON did not redact")
	}
	for _, leaked := range []string{"abc", "s3", `"a"`, `"k"`, "1234", "ghp_", "eyJhbGciOiJIUzI1NiJ9", "AKIAABCDEFGHIJKLMNOP"} {
		if strings.Contains(got, leaked) {
			t.Errorf("kept %s in %s", leaked, got)
		}
	}
	for _, kept := range []string{`"max_tokens":12`, `"has_secret":true`, `"author":"Ana"`, `"n":1`, `"html":"<b>hi</b>"`} {
		if !strings.Contains(got, kept) {
			t.Errorf("lost %s in %s", kept, got)
		}
	}
	if _, ok := console.Redact([]byte(`{"password":"x"`)); ok {
		t.Error("broken JSON is not passed through")
	}
}

func TestRedactLooksInsideStringsAndSessionKeys(t *testing.T) {
	in := `{"text":"{\"password\":\"hunter2x\",\"q\":\"ok\"}","log":"sent \"password\": \"swordfish\" to x","session_id":"blockscout-abc","sid":"s9","nested":"[{\"api_key\":\"k9k9k9\"}]","session_count":3}`
	got, ok := console.Redact([]byte(in))
	if !ok {
		t.Fatal("valid JSON did not redact")
	}
	for _, leaked := range []string{"hunter2x", "swordfish", "blockscout-abc", "s9", "k9k9k9"} {
		if strings.Contains(got, leaked) {
			t.Errorf("kept %s in %s", leaked, got)
		}
	}
	for _, kept := range []string{`ok`, `"session_count":3`} {
		if !strings.Contains(got, kept) {
			t.Errorf("lost %s in %s", kept, got)
		}
	}
}
