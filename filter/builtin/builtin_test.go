package builtin_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/filter/builtin"
	"github.com/versatile-sh/connector-engine/port"
)

func brazil(t *testing.T) builtin.Filter {
	t.Helper()
	f, err := builtin.New(builtin.DefaultLocales...)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func run(t *testing.T, text string) (string, map[string]int) {
	t.Helper()
	content, _ := json.Marshal([]map[string]string{{"type": "text", "text": text}})
	res, found, err := brazil(t).Filter(context.Background(), port.Result{Content: content})
	if err != nil {
		t.Fatal(err)
	}
	var blocks []map[string]string
	if err := json.Unmarshal(res.Content, &blocks); err != nil {
		t.Fatal(err)
	}
	return blocks[0]["text"], found
}

func TestBrazilianDocumentsNeedCheckDigits(t *testing.T) {
	cases := []struct {
		name string
		text string
		kind string
		want int
	}{
		{"formatted CPF", "titular 529.982.247-25 ativo", "cpf", 1},
		{"bare CPF", "cpf 52998224725", "cpf", 1},
		{"second valid CPF", "111.444.777-35", "cpf", 1},
		{"CPF with a wrong check digit", "529.982.247-26", "cpf", 0},
		{"CPF of equal digits", "111.111.111-11", "cpf", 0},
		{"formatted CNPJ", "empresa 11.222.333/0001-81", "cnpj", 1},
		{"bare CNPJ", "11222333000181", "cnpj", 1},
		{"CNPJ with a wrong check digit", "11.222.333/0001-82", "cnpj", 0},
		{"CNPJ of equal digits", "00.000.000/0000-00", "cnpj", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, found := run(t, c.text)
			if found[c.kind] != c.want {
				t.Fatalf("%s findings = %d, want %d (%v)", c.kind, found[c.kind], c.want, found)
			}
			if c.want == 0 && out != c.text {
				t.Fatalf("a false positive was masked: %q", out)
			}
			if c.want > 0 && !strings.Contains(out, "[REDACTED:"+c.kind+"]") {
				t.Fatalf("not masked: %q", out)
			}
		})
	}
	if out, found := run(t, "CNPJ 11.222.333/0001-81"); found["cpf"] != 0 || strings.Count(out, "REDACTED") != 1 {
		t.Fatalf("a CNPJ is not also a CPF: %q %v", out, found)
	}
}

func TestOtherKinds(t *testing.T) {
	cases := []struct {
		name string
		text string
		kind string
		want int
	}{
		{"email", "write to ana.souza+x@mail.example.com now", "email", 1},
		{"not an email", "user at host dot com", "email", 0},
		{"card with spaces", "pay 4111 1111 1111 1111 today", "card", 1},
		{"card with dashes", "5500-0000-0000-0004", "card", 1},
		{"card failing Luhn", "4111 1111 1111 1112", "card", 0},
		{"Brazilian mobile", "ligue (11) 98765-4321", "phone", 1},
		{"Brazilian landline", "11 3456-7890", "phone", 1},
		{"international number", "call +55 11 98765 4321 please", "phone", 1},
		{"US number", "555-123-4567", "phone", 1},
		{"epoch seconds", "created 1700000000", "phone", 0},
		{"date", "on 2026-11-18", "phone", 0},
		{"aws key", "key AKIAIOSFODNN7EXAMPLE end", "secret", 1},
		{"github token", "ghp_" + strings.Repeat("a1", 18), "secret", 1},
		{"slack token", "xoxb-1234567890-abcdefghij", "secret", 1},
		{"jwt", "eyJhbGciOiJIUzI1NiIsInR5cCI6.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI.SflKxwRJSMeKKF2QT4fwpM", "secret", 1},
		{"private key", "-----BEGIN RSA PRIVATE KEY-----\n" + strings.Repeat("MIIEowIBAAKCAQEA", 8) + "\n-----END RSA PRIVATE KEY-----", "secret", 1},
		{"plain prose", "the quick brown fox AKIA short", "secret", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, found := run(t, c.text)
			if found[c.kind] != c.want {
				t.Fatalf("%s findings = %d, want %d: %q %v", c.kind, found[c.kind], c.want, out, found)
			}
			if c.want == 0 && out != c.text {
				t.Fatalf("a false positive was masked: %q", out)
			}
		})
	}
}

func TestStructuredContentAndCounts(t *testing.T) {
	structured := json.RawMessage(`{"owner":{"email":"a@b.example.org","cpf":"529.982.247-25"},"notes":["call (11) 98765-4321","fine"],"count":3,"nested":{"count":12345678909}}`)
	content := json.RawMessage(`[{"type":"text","text":"a@b.example.org and c@d.example.org"},{"type":"image","data":"4111111111111111","mimeType":"image/png"}]`)
	res, found, err := brazil(t).Filter(context.Background(), port.Result{Content: content, Structured: structured, IsError: true})
	if err != nil {
		t.Fatal(err)
	}
	if found["email"] != 3 || found["cpf"] != 1 || found["phone"] != 1 {
		t.Fatalf("counts %v", found)
	}
	for _, leak := range []string{"a@b.example.org", "c@d.example.org", "529.982.247-25", "98765-4321"} {
		if strings.Contains(string(res.Content)+string(res.Structured), leak) {
			t.Fatalf("%q leaked: %s | %s", leak, res.Content, res.Structured)
		}
	}
	if !strings.Contains(string(res.Content), "4111111111111111") || !strings.Contains(string(res.Structured), `"count":3`) || !strings.Contains(string(res.Structured), `12345678909`) || !res.IsError {
		t.Fatalf("only strings are masked, and image data is not text: %s | %s", res.Content, res.Structured)
	}
	for kind, n := range found {
		if n == 0 || strings.ContainsAny(kind, "@0123456789") {
			t.Fatalf("counts carry kinds, never values: %v", found)
		}
	}
}

func TestCleanResultIsReturnedUntouched(t *testing.T) {
	in := port.Result{Content: json.RawMessage(`[ {"type":"text","text":"nothing here"} ]`), Structured: json.RawMessage(`{"b":1,  "a":2}`)}
	out, found, err := brazil(t).Filter(context.Background(), in)
	if err != nil || len(found) != 0 || string(out.Content) != string(in.Content) || string(out.Structured) != string(in.Structured) {
		t.Fatalf("%v %v %s %s", err, found, out.Content, out.Structured)
	}
	if _, _, err := brazil(t).Filter(context.Background(), port.Result{Content: json.RawMessage(`{`)}); !port.IsCode(err, port.CodeInvalidArgument) {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestLocalePatternSetsAreChosenByName(t *testing.T) {
	if got := builtin.DefaultLocales; len(got) != 1 || got[0] != "br" {
		t.Fatalf("the default locales changed: %v", got)
	}
	content, _ := json.Marshal([]map[string]string{{"type": "text", "text": "cpf 529.982.247-25"}})
	none, err := builtin.New()
	if err != nil {
		t.Fatal(err)
	}
	res, found, err := none.Filter(context.Background(), port.Result{Content: content})
	if err != nil || found["cpf"] != 0 || string(res.Content) != string(content) {
		t.Fatalf("no locale masks no CPF: %s %v %v", res.Content, found, err)
	}
	if _, found := run(t, "cpf 529.982.247-25"); found["cpf"] != 1 {
		t.Fatalf("the br set masks a CPF: %v", found)
	}
	if _, err := builtin.New("xx"); err == nil {
		t.Fatal("an unknown locale is refused")
	}
}
