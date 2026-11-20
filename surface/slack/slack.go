// Package slack is the Slack app surface: a direct message with Block Kit buttons, answered through Socket Mode.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"

	"github.com/agents-control/connector-engine/surface"
)

const (
	defaultAPIBase = "https://slack.com/api"
	shortCall      = 10 * time.Second
	outcomeBlock   = "outcome"
	actionsBlock   = "choices"
)

var _ surface.Channel = (*Channel)(nil)

// Config holds the app-level token for Socket Mode, the bot token, the owner's member id, the Web API base and an optional HTTP client.
type Config struct {
	AppToken    string
	BotToken    string
	OwnerUserID string
	APIBase     string
	HTTP        *http.Client
}

// Channel posts prompts to the owner's DM with the app and reads button presses from Socket Mode.
type Channel struct {
	app   string
	bot   string
	owner string
	base  string
	http  *http.Client

	mu sync.Mutex
	dm string
}

// New validates cfg and returns a Slack channel.
func New(cfg Config) (*Channel, error) {
	if !strings.HasPrefix(cfg.AppToken, "xapp-") {
		return nil, errors.New("slack: app token must be an app-level token (xapp-)")
	}
	if !strings.HasPrefix(cfg.BotToken, "xoxb-") {
		return nil, errors.New("slack: bot token must be a bot token (xoxb-)")
	}
	owner := strings.TrimSpace(cfg.OwnerUserID)
	if owner == "" || strings.ContainsAny(owner, " <>@") {
		return nil, errors.New("slack: owner user id is required, like U012ABCDEF")
	}
	base := strings.TrimRight(cfg.APIBase, "/")
	if base == "" {
		base = defaultAPIBase
	}
	if u, err := url.Parse(base); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("slack: api base must be an http or https URL")
	}
	hc := cfg.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Channel{app: cfg.AppToken, bot: cfg.BotToken, owner: owner, base: base, http: hc}, nil
}

// Kind returns "slack".
func (c *Channel) Kind() string { return "slack" }

type block = map[string]any

func plain(s string) map[string]any {
	return map[string]any{"type": "plain_text", "text": s, "emoji": true}
}

func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

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

func fallback(p surface.Prompt) string {
	return title(p) + " App: " + p.App + ". Expires " + p.ExpiresAt.UTC().Format("2006-01-02 15:04 UTC") + "."
}

func content(p surface.Prompt) []block {
	exp := p.ExpiresAt.UTC().Format("2006-01-02 15:04 UTC")
	meta := "App: *" + escape(p.App) + "*  ·  Expires <!date^" + strconv.FormatInt(p.ExpiresAt.Unix(), 10) + "^{date_short_pretty} {time}|" + exp + ">"
	section := "Connector: " + p.Connector + "\nFunction: " + p.Function
	if p.Kind != "" {
		section += "\nKind: " + p.Kind
	}
	return []block{
		{"type": "header", "text": plain(clip(title(p), 150))},
		{"type": "section", "text": plain(section)},
		{"type": "context", "elements": []any{map[string]any{"type": "mrkdwn", "text": meta}}},
	}
}

func buttons(p surface.Prompt) block {
	var els []any
	for _, ch := range p.Choices {
		b := map[string]any{"type": "button", "text": plain(surface.Label(ch)), "action_id": surface.ButtonID(ch, p.Key), "value": surface.ButtonID(ch, p.Key)}
		switch ch {
		case surface.ChoiceOnce, surface.ChoiceSession:
			b["style"] = "primary"
		case surface.ChoiceDeny:
			b["style"] = "danger"
		}
		els = append(els, b)
	}
	return block{"type": "actions", "block_id": actionsBlock, "elements": els}
}

func settled(blocks []block, o surface.Outcome) []block {
	out := make([]block, 0, len(blocks)+1)
	for _, b := range blocks {
		if b["type"] == "actions" || b["block_id"] == outcomeBlock {
			continue
		}
		out = append(out, b)
	}
	return append(out, block{"type": "context", "block_id": outcomeBlock, "elements": []any{map[string]any{"type": "plain_text", "text": surface.Sentence(o)}}})
}

// Notify opens the owner's DM once and posts the prompt with one button per choice.
func (c *Channel) Notify(ctx context.Context, p surface.Prompt) (surface.Ref, error) {
	dm, err := c.dmChannel(ctx)
	if err != nil {
		return "", err
	}
	blocks := content(p)
	if len(p.Choices) > 0 {
		blocks = append(blocks, buttons(p))
	}
	var r struct {
		Channel string `json:"channel"`
		TS      string `json:"ts"`
	}
	if err := c.api(ctx, c.bot, "chat.postMessage", map[string]any{"channel": dm, "text": fallback(p), "blocks": blocks}, &r); err != nil {
		return "", err
	}
	if r.Channel == "" {
		r.Channel = dm
	}
	return surface.Ref(r.Channel + ":" + r.TS), nil
}

func (c *Channel) dmChannel(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dm != "" {
		return c.dm, nil
	}
	var r struct {
		Channel struct {
			ID string `json:"id"`
		} `json:"channel"`
	}
	if err := c.api(ctx, c.bot, "conversations.open", map[string]any{"users": c.owner}, &r); err != nil {
		return "", err
	}
	if r.Channel.ID == "" {
		return "", errors.New("slack conversations.open: no channel id")
	}
	c.dm = r.Channel.ID
	return c.dm, nil
}

// Settle updates the posted message to the outcome and removes its buttons.
func (c *Channel) Settle(ctx context.Context, ref surface.Ref, p surface.Prompt, o surface.Outcome) error {
	ch, ts, ok := strings.Cut(string(ref), ":")
	if !ok {
		return fmt.Errorf("slack: malformed message reference %q", ref)
	}
	return c.update(ctx, ch, ts, fallback(p)+" "+surface.Sentence(o), settled(content(p), o))
}

func (c *Channel) update(ctx context.Context, ch, ts, text string, blocks []block) error {
	return c.api(ctx, c.bot, "chat.update", map[string]any{"channel": ch, "ts": ts, "text": text, "blocks": blocks}, nil)
}

type envelope struct {
	EnvelopeID string          `json:"envelope_id"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
}

type action struct {
	Type        string              `json:"type"`
	User        struct{ ID string } `json:"user"`
	ResponseURL string              `json:"response_url"`
	Channel     struct{ ID string } `json:"channel"`
	Container   struct {
		ChannelID string `json:"channel_id"`
		MessageTS string `json:"message_ts"`
	} `json:"container"`
	Message *struct {
		TS     string  `json:"ts"`
		Text   string  `json:"text"`
		Blocks []block `json:"blocks"`
	} `json:"message"`
	Actions []struct {
		ActionID string `json:"action_id"`
		Value    string `json:"value"`
	} `json:"actions"`
}

// Listen opens a Socket Mode connection, acknowledges every envelope and handles button presses until ctx ends.
func (c *Channel) Listen(ctx context.Context, click surface.ClickFunc) error {
	var open struct {
		URL string `json:"url"`
	}
	if err := c.api(ctx, c.app, "apps.connections.open", nil, &open); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	cfg, err := websocket.NewConfig(open.URL, "https://versatile.sh")
	if err != nil {
		return errors.New("slack socket mode: invalid URL")
	}
	ws, err := cfg.DialContext(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return errors.New("slack socket mode: dial: " + c.scrub(err, open.URL))
	}
	stop := context.AfterFunc(ctx, func() { ws.Close() })
	defer stop()
	var wg sync.WaitGroup
	defer wg.Wait()
	defer ws.Close()
	for {
		var env envelope
		if err := websocket.JSON.Receive(ws, &env); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, io.EOF) {
				return errors.New("slack socket mode: connection closed")
			}
			return errors.New("slack socket mode: read: " + c.scrub(err, open.URL))
		}
		if env.EnvelopeID != "" {
			if err := websocket.JSON.Send(ws, map[string]string{"envelope_id": env.EnvelopeID}); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return errors.New("slack socket mode: ack: " + c.scrub(err, open.URL))
			}
		}
		switch env.Type {
		case "disconnect":
			return errors.New("slack socket mode: server asked to reconnect")
		case "interactive":
			var a action
			if json.Unmarshal(env.Payload, &a) == nil && a.Type == "block_actions" && len(a.Actions) > 0 {
				wg.Go(func() { c.press(ctx, click, a) })
			}
		}
	}
}

func (c *Channel) press(ctx context.Context, click surface.ClickFunc, a action) {
	if a.User.ID != c.owner {
		c.respond(ctx, a.ResponseURL, surface.ErrNotOwner.Error())
		return
	}
	choice, key, ok := surface.ParseButton(a.Actions[0].ActionID)
	if !ok {
		c.respond(ctx, a.ResponseURL, surface.ErrExpired.Error())
		return
	}
	o, err := click(ctx, surface.Click{Key: key, Choice: choice, User: a.User.ID})
	if err != nil {
		c.respond(ctx, a.ResponseURL, err.Error())
		return
	}
	ch, ts := a.Container.ChannelID, a.Container.MessageTS
	if ch == "" {
		ch = a.Channel.ID
	}
	var blocks []block
	text := surface.Sentence(o)
	if a.Message != nil {
		blocks = a.Message.Blocks
		if ts == "" {
			ts = a.Message.TS
		}
		if a.Message.Text != "" {
			text = a.Message.Text + " " + text
		}
	}
	if ch == "" || ts == "" {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, shortCall)
	defer cancel()
	_ = c.update(cctx, ch, ts, text, settled(blocks, o))
}

func (c *Channel) respond(ctx context.Context, responseURL, text string) {
	u, err := url.Parse(responseURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, shortCall)
	defer cancel()
	raw, _ := json.Marshal(map[string]any{"response_type": "ephemeral", "replace_original": false, "text": text})
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, responseURL, bytes.NewReader(raw))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func (c *Channel) api(ctx context.Context, token, method string, body, out any) error {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("slack %s: encode request: %w", method, err)
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/"+method, rd)
	if err != nil {
		return errors.New("slack " + method + ": invalid request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("slack " + method + ": " + c.scrub(err))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return errors.New("slack " + method + ": read response: " + c.scrub(err))
	}
	var r struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("slack %s: HTTP %d", method, resp.StatusCode)
	}
	if !r.OK {
		return errors.New("slack " + method + ": " + c.scrubText(r.Error))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("slack %s: decode response: %w", method, err)
	}
	return nil
}

func (c *Channel) scrub(err error, secrets ...string) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	var de *websocket.DialError
	if errors.As(err, &de) {
		err = de.Err
	}
	return c.scrubText(err.Error(), secrets...)
}

func (c *Channel) scrubText(s string, secrets ...string) string {
	for _, sec := range append([]string{c.app, c.bot}, secrets...) {
		if sec != "" {
			s = strings.ReplaceAll(s, sec, "[redacted]")
		}
	}
	return s
}
