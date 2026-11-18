// Package fetch performs bounded HTTP requests for the source and runtime adapters.
package fetch

import (
	"context"
	"io"
	"net/http"

	"github.com/agents-control/connector-engine/port"
)

// MaxBody is the largest response body Do reads.
const MaxBody = 32 << 20

// Do sends one request and returns the body of a 2xx answer; 404 is a not_found error and any other status an unavailable one.
func Do(ctx context.Context, c *http.Client, method, url string, body io.Reader, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, port.Errorf(port.CodeInvalidArgument, "request %s: %v", url, err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, port.Errorf(port.CodeUnavailable, "%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody+1))
	if err != nil {
		return nil, port.Errorf(port.CodeUnavailable, "%s %s: %v", method, url, err)
	}
	if len(data) > MaxBody {
		return nil, port.Errorf(port.CodeUnavailable, "%s %s: the body exceeds %d bytes", method, url, MaxBody)
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, port.Errorf(port.CodeNotFound, "%s %s: %s", method, url, resp.Status)
	case resp.StatusCode/100 != 2:
		return nil, port.Errorf(port.CodeUnavailable, "%s %s: %s", method, url, resp.Status)
	}
	return data, nil
}
