// Package console is the local web console of a person install: owner account, import of MCP servers and listings, grants and agent tokens, all through the engine's HTTP API.
package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"
)

const personTokenTTL = 10 * time.Minute

// EngineError is a refusal of the engine: its HTTP status, its code and its detail.
type EngineError struct {
	Status int    `json:"status"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

// Error reads as the status, the code and the detail.
func (e *EngineError) Error() string {
	return fmt.Sprintf("engine %d %s: %s", e.Status, e.Code, e.Detail)
}

// Engine calls the engine's public listener as the local person and its operator listener with the operator token.
type Engine struct {
	PublicURL   string
	OperatorURL string
	Operator    string
	Tenant      string
	Workspace   string
	Person      string
	HTTP        *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

type bodyReader interface {
	readBody(io.Reader) error
}

var engineHTTP = &http.Client{Timeout: 90 * time.Second}

func (e *Engine) client() *http.Client {
	if e.HTTP != nil {
		return e.HTTP
	}
	return engineHTTP
}

func (e *Engine) do(ctx context.Context, base, bearer, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := e.client().Do(req)
	if err != nil {
		return unreachable(fmt.Errorf("engine unreachable: %w", err))
	}
	defer resp.Body.Close()
	if br, ok := out.(bodyReader); ok && resp.StatusCode < 300 {
		return br.readBody(resp.Body)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		ee := &EngineError{Status: resp.StatusCode}
		if json.Unmarshal(raw, ee) != nil || ee.Code == "" {
			ee.Detail = strings.TrimSpace(string(raw))
		}
		ee.Status = resp.StatusCode
		return ee
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// Mint asks the local issuer for a token with these claims, living ttl.
func (e *Engine) Mint(ctx context.Context, claims map[string]any, ttl time.Duration) (string, error) {
	body := map[string]any{"ttl": ttl.String()}
	maps.Copy(body, claims)
	var out struct {
		Token string `json:"token"`
	}
	if err := e.do(ctx, e.PublicURL, "", http.MethodPost, "/local/token", body, &out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", unreachable(errors.New("the local issuer returned no token"))
	}
	return out.Token, nil
}

func (e *Engine) personToken(ctx context.Context, fresh bool) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !fresh && e.token != "" && time.Until(e.expires) > time.Minute {
		return e.token, nil
	}
	tok, err := e.Mint(ctx, map[string]any{"sub": e.Person, "tenant": e.Tenant}, personTokenTTL)
	if err != nil {
		return "", err
	}
	e.token, e.expires = tok, time.Now().Add(personTokenTTL)
	return tok, nil
}

// Call runs one operation of the public API as the local person; a refused token is minted again once, since the issuer key changes when the engine restarts.
func (e *Engine) Call(ctx context.Context, method, path string, in, out any) error {
	tok, err := e.personToken(ctx, false)
	if err != nil {
		return err
	}
	err = e.do(ctx, e.PublicURL, tok, method, path, in, out)
	if ee := (*EngineError)(nil); errors.As(err, &ee) && ee.Status == http.StatusUnauthorized {
		if tok, err = e.personToken(ctx, true); err != nil {
			return err
		}
		return e.do(ctx, e.PublicURL, tok, method, path, in, out)
	}
	return err
}

// EnsureTenant creates the local tenant through the operator listener; creating an existing tenant is accepted.
func (e *Engine) EnsureTenant(ctx context.Context) error {
	return e.do(ctx, e.OperatorURL, e.Operator, http.MethodPut, "/v1/tenants/"+e.Tenant, nil, nil)
}
