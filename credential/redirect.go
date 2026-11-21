package credential

import (
	"fmt"
	"net/url"
)

const maxNoteHost = 255

// RedirectNote words a 3xx answer for an operator: the status, and the Location host only when it is a short, plain hostname.
// Userinfo, path, query and any host with other characters are never printed.
func RedirectNote(status int, location string) string {
	note := fmt.Sprintf("the upstream answered %d", status)
	if host := plainHost(location); host != "" {
		note += " and redirected to " + host
	}
	return note
}

func plainHost(location string) string {
	u, err := url.Parse(location)
	if err != nil || u.Host == "" || len(u.Host) > maxNoteHost {
		return ""
	}
	for i := 0; i < len(u.Host); i++ {
		c := u.Host[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == ':' || c == '[' || c == ']') {
			return ""
		}
	}
	return u.Host
}
