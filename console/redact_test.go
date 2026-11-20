package console_test

import (
	"strings"
	"testing"

	"github.com/agents-control/connector-engine/console"
)

func TestRedactHidesSecretsAndKeepsTheRest(t *testing.T) {
	in := `{"Authorization":"Bearer abc","client_secret":"s3","tokens":["a","b"],"max_tokens":12,"author":"Ana","items":[{"apiKey":"k","n":1}],` +
		`"text":"use ghp_abcdefghijklmnopqrstuvwxyz0123 or eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTYifQ.c2lnbmF0dXJlMTIz or AKIAABCDEFGHIJKLMNOP","html":"<b>hi</b>"}`
	got, ok := console.Redact([]byte(in))
	if !ok {
		t.Fatal("valid JSON did not redact")
	}
	for _, leaked := range []string{"abc", "s3", `"a"`, `"k"`, "ghp_", "eyJhbGciOiJIUzI1NiJ9", "AKIAABCDEFGHIJKLMNOP"} {
		if strings.Contains(got, leaked) {
			t.Errorf("kept %s in %s", leaked, got)
		}
	}
	for _, kept := range []string{`"max_tokens":12`, `"author":"Ana"`, `"n":1`, `"html":"<b>hi</b>"`} {
		if !strings.Contains(got, kept) {
			t.Errorf("lost %s in %s", kept, got)
		}
	}
	if _, ok := console.Redact([]byte(`{"password":"x"`)); ok {
		t.Error("broken JSON is not passed through")
	}
}
