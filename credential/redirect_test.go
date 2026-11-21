package credential

import (
	"strings"
	"testing"
)

func TestRedirectNote(t *testing.T) {
	for _, tc := range []struct{ name, location, want string }{
		{"absolute", "https://api.example.com/v2/x?y=1", "the upstream answered 307 and redirected to api.example.com"},
		{"ipv6 with port", "http://[::1]:8080/p", "the upstream answered 307 and redirected to [::1]:8080"},
		{"userinfo is never printed", "https://user:secret@host.example/p", "the upstream answered 307 and redirected to host.example"},
		{"long host", "https://" + strings.Repeat("a", 5000) + ".example/", "the upstream answered 307"},
		{"bidi character", "https://evil" + string(rune(0x202e)) + ".example/", "the upstream answered 307"},
		{"relative", "/elsewhere", "the upstream answered 307"},
		{"empty", "", "the upstream answered 307"},
		{"malformed", "http://[::1", "the upstream answered 307"},
		{"javascript", "javascript:alert(1)", "the upstream answered 307"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := RedirectNote(307, tc.location); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
