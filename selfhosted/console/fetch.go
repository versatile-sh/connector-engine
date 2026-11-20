package console

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/versatile-sh/connector-engine/internal/egress"
)

const (
	fetchTimeout = 15 * time.Second
	maxRedirects = 3
)

// ErrTooLarge is returned when a fetched body passes its limit.
var ErrTooLarge = unreachable(errors.New("the response is larger than the console accepts"))

// Reach says which addresses a server-side fetch may dial: public https only, or, when the person ticked it for one import, local and http too, a loopback host then dialled as LoopbackHost when set.
type Reach struct {
	Local        bool
	LoopbackHost string
}

// CheckURL refuses a URL the reach does not allow: https only unless local is allowed, and never a URL with credentials.
func (r Reach) CheckURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, invalid("%q is not an absolute URL", raw)
	}
	if u.User != nil {
		return nil, invalid("a URL with credentials is refused")
	}
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && r.Local:
	default:
		return nil, invalid("only https URLs are fetched; tick \"local server\" for an http server on this machine or network")
	}
	return u, nil
}

// Transport dials through the egress guard: every hop resolves once and refuses loopback, private, link-local and metadata addresses unless local is allowed, which also maps a loopback host to LoopbackHost.
func (r Reach) Transport() *http.Transport {
	t := egress.Guard{AllowPrivate: r.Local, LoopbackHost: r.LoopbackHost}.Transport()
	t.ResponseHeaderTimeout = fetchTimeout
	return t
}

const maxResponseBytes = 4 << 20

type capped struct{ next http.RoundTripper }

type cappedBody struct {
	io.Reader
	io.Closer
}

// RoundTrip limits every response body to maxResponseBytes; a longer body reads as truncated.
func (c capped) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := c.next.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	resp.Body = cappedBody{Reader: io.LimitReader(resp.Body, maxResponseBytes), Closer: resp.Body}
	return resp, nil
}

// Client follows at most three redirects, each checked against the reach.
func (r Reach) Client() *http.Client {
	return &http.Client{
		Timeout:   fetchTimeout,
		Transport: capped{next: r.Transport()},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return unreachable(errors.New("too many redirects"))
			}
			_, err := r.CheckURL(req.URL.String())
			return err
		},
	}
}

// Get fetches raw and returns at most limit bytes of a 2xx body with its content type.
func (r Reach) Get(ctx context.Context, raw string, limit int64, accept string) ([]byte, string, error) {
	u, err := r.CheckURL(raw)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := r.Client().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, "", unreachable(fmt.Errorf("%s answered %d", u.Host, resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(body)) > limit {
		return nil, "", ErrTooLarge
	}
	return body, resp.Header.Get("Content-Type"), nil
}
