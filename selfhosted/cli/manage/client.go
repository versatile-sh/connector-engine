// Package manage is the management side of vsh: it signs in to the console as a user and drives the same HTTP API the web console uses.
package manage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/cli/command"
	"github.com/versatile-sh/connector-engine/selfhosted/standalone"
)

const (
	defaultURL    = "http://127.0.0.1:7465"
	sessionCookie = "cn_console"
)

// Client calls the console's API at Base as the signed-in user of Session.
type Client struct {
	Base    string
	Session Session
	HTTP    *http.Client
}

// Locate returns the console address: flag, then $VSH_URL, then the install running on the default data directory, then 127.0.0.1:7465.
func Locate(ctx context.Context, flag string, getenv func(string) string) string {
	if v := strings.TrimSpace(flag); v != "" {
		return strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(getenv("VSH_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	if data, err := standalone.DataDir(getenv); err == nil {
		if s, ok := standalone.Running(ctx, data); ok {
			return strings.TrimRight(s.URL, "/")
		}
	}
	return defaultURL
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (c *Client) request(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, r)
	if err != nil {
		return nil, command.Fail(command.ExitUsage, "set --url or $VSH_URL to the console's address, such as "+defaultURL, "the console address %q: %v", c.Base, err)
	}
	req.Header.Set("Origin", c.Base)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Session.Cookie != "" {
		req.Header.Set("Cookie", sessionCookie+"="+c.Session.Cookie)
		req.Header.Set("X-CSRF-Token", c.Session.CSRF)
	}
	resp, err := c.http().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, command.Fail(command.ExitUnreachable, "start it with 'vsh up', or set --url or $VSH_URL to the console's address", "cannot reach the console at %s", c.Base)
	}
	return resp, nil
}

// Do sends body as JSON with method to path and decodes the JSON answer into out; a refusal becomes an error with its exit code and next step.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	resp, err := c.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return refusal(resp.StatusCode, raw)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("the console answered %s %s with something else than JSON: is %s a connector-engine console?", method, path, c.Base)
	}
	return nil
}

func refusal(status int, raw []byte) error {
	var e struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	msg := strings.TrimSpace(string(raw))
	if json.Unmarshal(raw, &e) == nil && e.Error != "" {
		msg = e.Error
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	switch status {
	case http.StatusUnauthorized:
		if e.Code == "" && msg == "sign in first" {
			return command.Fail(command.ExitAuth, "run 'vsh login'", "not signed in, or the session ended")
		}
		return command.Fail(command.ExitAuth, "", "%s", msg)
	case http.StatusForbidden:
		return command.Fail(command.ExitAuth, "ask an owner of this console for the permission", "%s", msg)
	case http.StatusConflict:
		if msg == "create the owner account first" {
			return command.Fail(command.ExitFailure, "open the console in a browser to create the owner account", "%s", msg)
		}
	case http.StatusMisdirectedRequest:
		return command.Fail(command.ExitUsage, "use the console's own address, as 'vsh up' or the browser shows it", "the console refused the address %s", msg)
	}
	return command.Fail(command.ExitFailure, "", "%s", msg)
}

// Login signs in with username and password as the web does and returns the new session.
func (c *Client) Login(ctx context.Context, username, password string) (Session, error) {
	resp, err := c.request(ctx, http.MethodPost, "/api/login", map[string]string{"username": username, "password": password})
	if err != nil {
		return Session{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized {
			return Session{}, command.Fail(command.ExitAuth, "check the username and password; 'vsh reset-password' sets a new one on this machine", "wrong username or password")
		}
		return Session{}, refusal(resp.StatusCode, raw)
	}
	var out struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal(raw, &out)
	s := Session{CSRF: out.CSRF, User: username}
	for _, ck := range resp.Cookies() {
		if ck.Name == sessionCookie {
			s.Cookie = ck.Value
		}
	}
	if s.Cookie == "" || s.CSRF == "" {
		return Session{}, fmt.Errorf("the console at %s gave no session", c.Base)
	}
	return s, nil
}

func esc(s string) string { return url.PathEscape(s) }

var errNoSession = errors.New("not signed in")
