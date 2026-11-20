package surface

import (
	"errors"
	"net/url"
	"strings"

	"golang.org/x/net/websocket"
)

const redacted = "[redacted]"

// Scrub returns an error's message without the address a url.Error or a websocket.DialError carries, and with every secret replaced.
func Scrub(err error, secrets ...string) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	var de *websocket.DialError
	if errors.As(err, &de) {
		err = de.Err
	}
	return ScrubText(err.Error(), secrets...)
}

// ScrubText replaces every secret in s, skipping empty ones and a bare "/".
func ScrubText(s string, secrets ...string) string {
	for _, sec := range secrets {
		if sec != "" && sec != "/" {
			s = strings.ReplaceAll(s, sec, redacted)
		}
	}
	return s
}
