package remote

import (
	"net/http"
	"strings"
	"testing"
)

type scripted []*http.Response

func (s *scripted) RoundTrip(*http.Request) (*http.Response, error) {
	r := (*s)[0]
	*s = (*s)[1:]
	return r, nil
}

func answer(status int, location string) *http.Response {
	h := http.Header{}
	if location != "" {
		h.Set("Location", location)
	}
	return &http.Response{StatusCode: status, Header: h, Body: http.NoBody}
}

func TestTheRedirectNoteIsClearedByTheNextAnswer(t *testing.T) {
	c := &capture{next: &scripted{answer(307, "https://moved.example/x"), answer(200, "")}}
	req, _ := http.NewRequest(http.MethodGet, "http://up.example/", nil)
	if _, err := c.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if n := c.note(); !strings.Contains(n, "307") || !strings.Contains(n, "moved.example") {
		t.Fatalf("note after a redirect: %q", n)
	}
	if _, err := c.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if n := c.note(); n != "" {
		t.Fatalf("note after a 200: %q", n)
	}
}
