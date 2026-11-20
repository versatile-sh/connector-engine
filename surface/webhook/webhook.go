// Package webhook is the notify-only surface for ntfy topics and Discord or Slack incoming webhooks, which link to the console instead of taking clicks.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/surface"
)

// The webhook kinds a Config accepts.
const (
	KindNtfy           = "ntfy"
	KindDiscordWebhook = "discord_webhook"
	KindSlackWebhook   = "slack_webhook"
)

const answerHint = "Open the console on this computer to answer."

var _ surface.Channel = (*Channel)(nil)

// Config holds the webhook kind (ntfy, discord_webhook or slack_webhook), its URL, an optional ntfy access token and an optional HTTP client.
type Config struct {
	Kind  string
	URL   string
	Token string
	HTTP  *http.Client
}

// Channel posts prompts to one incoming webhook and never receives answers.
type Channel struct {
	kind  string
	url   *url.URL
	token string
	http  *http.Client
}

// New validates cfg and returns a webhook channel.
func New(cfg Config) (*Channel, error) {
	switch cfg.Kind {
	case KindNtfy, KindDiscordWebhook, KindSlackWebhook:
	default:
		return nil, fmt.Errorf("webhook: unknown kind %q", cfg.Kind)
	}
	u, err := url.Parse(strings.TrimSpace(cfg.URL))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("webhook: URL must be an http or https URL")
	}
	hc := cfg.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Channel{kind: cfg.Kind, url: u, token: cfg.Token, http: hc}, nil
}

// Kind returns the configured webhook kind.
func (c *Channel) Kind() string { return c.kind }

func title(p surface.Prompt) string {
	fn := p.FunctionTitle
	if fn == "" {
		fn = p.Function
	}
	return "Allow " + p.Connector + ": " + fn + "?"
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func expiry(p surface.Prompt) string { return p.ExpiresAt.UTC().Format("2006-01-02 15:04 UTC") }

func body(p surface.Prompt) string {
	return "Connector: " + p.Connector + "\nFunction: " + p.Function + "\nApp: " + p.App + "\nExpires: " + expiry(p) + "\n\n" + answerHint
}

type field struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type embed struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	URL         string            `json:"url,omitempty"`
	Color       int               `json:"color"`
	Fields      []field           `json:"fields"`
	Footer      map[string]string `json:"footer"`
}

func card(p surface.Prompt) embed {
	desc := answerHint
	if p.ConsoleURL != "" {
		desc = "[Open in console](" + p.ConsoleURL + ")\n" + answerHint
	}
	return embed{Title: title(p), Description: desc, URL: p.ConsoleURL, Color: 0x5865F2, Footer: map[string]string{"text": "versatile.sh"},
		Fields: []field{{Name: "App", Value: orDash(p.App), Inline: true}, {Name: "Expires", Value: "<t:" + strconv.FormatInt(p.ExpiresAt.Unix(), 10) + ":R>", Inline: true}}}
}

// Notify posts the prompt with a link to the console; only discord_webhook returns a reference, since the others cannot be edited.
func (c *Channel) Notify(ctx context.Context, p surface.Prompt) (surface.Ref, error) {
	switch c.kind {
	case KindNtfy:
		h := http.Header{"Title": {mime.QEncoding.Encode("utf-8", title(p))}, "Tags": {"raised_hand"}}
		if p.ConsoleURL != "" {
			h.Set("Click", p.ConsoleURL)
			h.Set("Actions", "view, Open in console, "+p.ConsoleURL)
		}
		if c.token != "" {
			h.Set("Authorization", "Bearer "+c.token)
		}
		return "", c.send(ctx, http.MethodPost, c.url, h, []byte(body(p)), nil)
	case KindDiscordWebhook:
		u := *c.url
		q := u.Query()
		q.Set("wait", "true")
		u.RawQuery = q.Encode()
		var m struct {
			ID string `json:"id"`
		}
		if err := c.sendJSON(ctx, http.MethodPost, &u, map[string]any{"embeds": []embed{card(p)}}, &m); err != nil {
			return "", err
		}
		return surface.Ref(m.ID), nil
	}
	blocks := []any{
		map[string]any{"type": "header", "text": map[string]any{"type": "plain_text", "text": clip(title(p), 150)}},
		map[string]any{"type": "section", "text": map[string]any{"type": "plain_text", "text": body(p)}},
	}
	if p.ConsoleURL != "" {
		blocks = append(blocks, map[string]any{"type": "actions", "elements": []any{map[string]any{
			"type": "button", "text": map[string]any{"type": "plain_text", "text": "Open in console"}, "url": p.ConsoleURL, "action_id": "open_console",
		}}})
	}
	return "", c.sendJSON(ctx, http.MethodPost, c.url, map[string]any{"text": title(p) + " " + answerHint, "blocks": blocks}, nil)
}

// Settle edits a discord_webhook message to the outcome and does nothing for the other kinds.
func (c *Channel) Settle(ctx context.Context, ref surface.Ref, p surface.Prompt, o surface.Outcome) error {
	if c.kind != KindDiscordWebhook || ref == "" {
		return nil
	}
	u := *c.url
	u.Path = strings.TrimRight(u.Path, "/") + "/messages/" + url.PathEscape(string(ref))
	u.RawPath = ""
	e := card(p)
	e.Description = surface.Sentence(o)
	e.Color = 0x95A5A6
	switch o.State {
	case surface.StateApproved, surface.StateSession, surface.StateAlways:
		e.Color = 0x57F287
	case surface.StateDenied:
		e.Color = 0xED4245
	}
	e.Fields = append(e.Fields, field{Name: "Outcome", Value: surface.Sentence(o)})
	return c.sendJSON(ctx, http.MethodPatch, &u, map[string]any{"embeds": []embed{e}}, nil)
}

// Listen returns nil at once because incoming webhooks deliver no answers.
func (c *Channel) Listen(context.Context, surface.ClickFunc) error { return nil }

func (c *Channel) sendJSON(ctx context.Context, method string, u *url.URL, v, out any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("%s: encode request: %w", c.kind, err)
	}
	return c.send(ctx, method, u, http.Header{"Content-Type": {"application/json"}}, raw, out)
}

func (c *Channel) send(ctx context.Context, method string, u *url.URL, h http.Header, raw []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(raw))
	if err != nil {
		return errors.New(c.kind + ": invalid request")
	}
	for k, v := range h {
		req.Header[k] = v
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New(c.kind + ": " + c.scrub(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s: HTTP %d", c.kind, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("%s: decode response: %w", c.kind, err)
	}
	return nil
}

func (c *Channel) scrub(err error) string {
	return surface.Scrub(err, c.url.String(), c.url.Path, c.url.RawQuery, c.token)
}
