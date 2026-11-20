package surface_test

import (
	"errors"
	"net/url"
	"testing"

	"github.com/versatile-sh/connector-engine/selfhosted/surface"
)

func TestScrubDropsTheAddressAndEverySecret(t *testing.T) {
	err := &url.Error{Op: "Post", URL: "https://hooks.example/T0K3N?sig=abc", Err: errors.New("dial T0K3N refused")}
	if got := surface.Scrub(err, "T0K3N", "", "/"); got != "dial [redacted] refused" {
		t.Fatalf("got %q", got)
	}
	if got := surface.ScrubText("a/b", "/", ""); got != "a/b" {
		t.Fatalf("a bare slash or an empty secret is not a secret: %q", got)
	}
}
