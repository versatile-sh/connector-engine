// Package discord is the Discord bot surface: a direct message with an embed and buttons, answered through the Gateway websocket.
package discord

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
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"

	"github.com/agents-control/connector-engine/surface"
)

const (
	defaultAPIBase = "https://discord.com/api/v10"
	shortCall      = 10 * time.Second
	ackDeadline    = 3 * time.Second
	colorPending   = 0x5865F2
	colorAllowed   = 0x57F287
	colorDenied    = 0xED4245
	colorClosed    = 0x95A5A6
	outcomeField   = "Outcome"
	ephemeral      = 64
)

var _ surface.Channel = (*Channel)(nil)

// Config holds the bot token, the owner's user id, the REST API base, an optional fixed Gateway URL and an optional HTTP client.
type Config struct {
	Token       string
	OwnerUserID string
	APIBase     string
	GatewayURL  string
	HTTP        *http.Client
}

// Channel posts prompts to the owner's DM with the bot and reads button presses from the Gateway.
type Channel struct {
	token   string
	owner   string
	base    string
	gateway string
	http    *http.Client

	mu sync.Mutex
	dm string
}

// New validates cfg and returns a Discord channel.
func New(cfg Config) (*Channel, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("discord: token is required")
	}
	if _, err := strconv.ParseUint(strings.TrimSpace(cfg.OwnerUserID), 10, 64); err != nil {
		return nil, errors.New("discord: owner user id must be a numeric snowflake")
	}
	base := strings.TrimRight(cfg.APIBase, "/")
	if base == "" {
		base = defaultAPIBase
	}
	if u, err := url.Parse(base); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("discord: api base must be an http or https URL")
	}
	if cfg.GatewayURL != "" {
		if u, err := url.Parse(cfg.GatewayURL); err != nil || (u.Scheme != "wss" && u.Scheme != "ws") || u.Host == "" {
			return nil, errors.New("discord: gateway URL must be a ws or wss URL")
		}
	}
	hc := cfg.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Channel{token: cfg.Token, owner: strings.TrimSpace(cfg.OwnerUserID), base: base, gateway: cfg.GatewayURL, http: hc}, nil
}

// Kind returns "discord".
func (c *Channel) Kind() string { return "discord" }

type field struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type footer struct {
	Text string `json:"text"`
}

type embed struct {
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	Color       int     `json:"color"`
	Fields      []field `json:"fields,omitempty"`
	Footer      *footer `json:"footer,omitempty"`
}

type component struct {
	Type       int         `json:"type"`
	Style      int         `json:"style,omitempty"`
	Label      string      `json:"label,omitempty"`
	CustomID   string      `json:"custom_id,omitempty"`
	Components []component `json:"components,omitempty"`
}

type posted struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
}

func card(p surface.Prompt) embed {
	fn := p.FunctionTitle
	if fn == "" {
		fn = p.Function
	}
	fields := []field{{Name: "App", Value: orDash(markdown.Replace(p.App)), Inline: true}, {Name: "Kind", Value: orDash(markdown.Replace(p.Kind)), Inline: true}}
	if !p.ExpiresAt.IsZero() {
		fields = append(fields, field{Name: "Expires", Value: "<t:" + strconv.FormatInt(p.ExpiresAt.Unix(), 10) + ":R>", Inline: true})
	}
	return embed{Title: "Allow " + markdown.Replace(p.Connector+": "+fn) + "?", Description: "`" + codeSpan.Replace(p.Function) + "`", Color: colorPending, Fields: fields, Footer: &footer{Text: "versatile.sh"}}
}

var markdown = strings.NewReplacer("\\", "\\\\", "*", "\\*", "_", "\\_", "~", "\\~", "`", "\\`", "|", "\\|", "[", "\\[", "]", "\\]", "(", "\\(", ")", "\\)", "<", "\\<", ">", "\\>", "#", "\\#", "@", "\\@", "\n", " ", "\r", " ")

var codeSpan = strings.NewReplacer("`", "'", "\n", " ", "\r", " ")

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func settled(e embed, o surface.Outcome) embed {
	fields := make([]field, 0, len(e.Fields)+1)
	for _, f := range e.Fields {
		if f.Name != outcomeField {
			fields = append(fields, f)
		}
	}
	e.Fields = append(fields, field{Name: outcomeField, Value: surface.Sentence(o)})
	switch o.State {
	case surface.StateApproved, surface.StateSession, surface.StateAlways:
		e.Color = colorAllowed
	case surface.StateDenied:
		e.Color = colorDenied
	default:
		e.Color = colorClosed
	}
	return e
}

func style(ch surface.Choice) int {
	switch ch {
	case surface.ChoiceOnce, surface.ChoiceSession:
		return 3
	case surface.ChoiceAlways:
		return 1
	}
	return 4
}

// Notify opens the owner's DM once and posts the prompt as an embed with one button per choice.
func (c *Channel) Notify(ctx context.Context, p surface.Prompt) (surface.Ref, error) {
	dm, err := c.dmChannel(ctx)
	if err != nil {
		return "", err
	}
	body := map[string]any{"embeds": []embed{card(p)}}
	if len(p.Choices) > 0 {
		row := component{Type: 1}
		for _, ch := range p.Choices {
			row.Components = append(row.Components, component{Type: 2, Style: style(ch), Label: surface.Label(ch), CustomID: surface.ButtonID(ch, p.Key)})
		}
		body["components"] = []component{row}
	}
	var m posted
	if err := c.do(ctx, http.MethodPost, c.base+"/channels/"+dm+"/messages", body, &m); err != nil {
		return "", err
	}
	return surface.Ref(dm + "/" + m.ID), nil
}

func (c *Channel) dmChannel(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dm != "" {
		return c.dm, nil
	}
	var ch struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, c.base+"/users/@me/channels", map[string]string{"recipient_id": c.owner}, &ch); err != nil {
		return "", err
	}
	if ch.ID == "" {
		return "", errors.New("discord: DM channel has no id")
	}
	c.dm = ch.ID
	return c.dm, nil
}

// Settle edits the posted message to the outcome and removes its buttons.
func (c *Channel) Settle(ctx context.Context, ref surface.Ref, p surface.Prompt, o surface.Outcome) error {
	ch, msg, ok := strings.Cut(string(ref), "/")
	if !ok {
		return fmt.Errorf("discord: malformed message reference %q", ref)
	}
	return c.edit(ctx, ch, msg, settled(card(p), o))
}

func (c *Channel) edit(ctx context.Context, ch, msg string, e embed) error {
	return c.do(ctx, http.MethodPatch, c.base+"/channels/"+ch+"/messages/"+msg, map[string]any{"embeds": []embed{e}, "components": []component{}}, nil)
}

type frame struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d,omitempty"`
	S  *int64          `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
}

type interaction struct {
	ID            string `json:"id"`
	ApplicationID string `json:"application_id"`
	Type          int    `json:"type"`
	Token         string `json:"token"`
	ChannelID     string `json:"channel_id"`
	Member        *struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	} `json:"member"`
	User *struct {
		ID string `json:"id"`
	} `json:"user"`
	Data struct {
		CustomID string `json:"custom_id"`
	} `json:"data"`
	Message *struct {
		ID        string  `json:"id"`
		ChannelID string  `json:"channel_id"`
		Embeds    []embed `json:"embeds"`
	} `json:"message"`
}

// Listen connects to the Gateway, keeps it alive and handles button presses until ctx ends.
func (c *Channel) Listen(ctx context.Context, click surface.ClickFunc) error {
	err := c.listen(ctx, click)
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func (c *Channel) listen(ctx context.Context, click surface.ClickFunc) error {
	gw, err := c.gatewayURL(ctx)
	if err != nil {
		return err
	}
	cfg, err := websocket.NewConfig(gw, "https://versatile.sh")
	if err != nil {
		return errors.New("discord gateway: invalid URL")
	}
	ws, err := cfg.DialContext(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return errors.New("discord gateway: dial: " + c.scrub(err))
	}
	stop := context.AfterFunc(ctx, func() { ws.Close() })
	defer stop()
	defer ws.Close()

	var hello frame
	if err := websocket.JSON.Receive(ws, &hello); err != nil || hello.Op != 10 {
		if ctx.Err() != nil {
			return nil
		}
		return errors.New("discord gateway: no HELLO")
	}
	var h struct {
		Interval int64 `json:"heartbeat_interval"`
	}
	if err := json.Unmarshal(hello.D, &h); err != nil || h.Interval <= 0 {
		return errors.New("discord gateway: invalid HELLO")
	}
	identify := map[string]any{"token": c.token, "intents": 0, "properties": map[string]string{"os": "linux", "browser": "versatile.sh", "device": "versatile.sh"}}
	if err := c.send(ws, 2, identify); err != nil {
		return errors.New("discord gateway: identify failed")
	}

	var seq atomic.Int64
	seq.Store(-1)
	var acked, zombie atomic.Bool
	acked.Store(true)
	beat := func() error {
		var d any
		if s := seq.Load(); s >= 0 {
			d = s
		}
		return c.send(ws, 1, d)
	}
	beatCtx, stopBeat := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer wg.Wait()
	defer ws.Close()
	defer stopBeat()
	wg.Go(func() {
		t := time.NewTicker(time.Duration(h.Interval) * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-beatCtx.Done():
				return
			case <-t.C:
				if !acked.Swap(false) {
					zombie.Store(true)
					ws.Close()
					return
				}
				if beat() != nil {
					ws.Close()
					return
				}
			}
		}
	})

	for {
		var f frame
		if err := websocket.JSON.Receive(ws, &f); err != nil {
			switch {
			case ctx.Err() != nil:
				return nil
			case zombie.Load():
				return errors.New("discord gateway: heartbeat not acknowledged")
			case errors.Is(err, io.EOF):
				return errors.New("discord gateway: connection closed")
			}
			return errors.New("discord gateway: read: " + c.scrub(err))
		}
		if f.S != nil {
			seq.Store(*f.S)
		}
		switch f.Op {
		case 0:
			if f.T == "INTERACTION_CREATE" {
				var in interaction
				if json.Unmarshal(f.D, &in) == nil && in.Type == 3 {
					wg.Go(func() { c.press(ctx, click, in) })
				}
			}
		case 1:
			_ = beat()
		case 7:
			return errors.New("discord gateway: reconnect requested")
		case 9:
			return errors.New("discord gateway: invalid session")
		case 11:
			acked.Store(true)
		}
	}
}

func (c *Channel) gatewayURL(ctx context.Context) (string, error) {
	raw := c.gateway
	if raw == "" {
		var g struct {
			URL string `json:"url"`
		}
		if err := c.do(ctx, http.MethodGet, c.base+"/gateway/bot", nil, &g); err != nil {
			return "", err
		}
		raw = g.URL
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "wss" && u.Scheme != "ws") {
		return "", errors.New("discord gateway: invalid URL")
	}
	q := u.Query()
	q.Set("v", "10")
	q.Set("encoding", "json")
	u.RawQuery = q.Encode()
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

func (c *Channel) send(ws *websocket.Conn, op int, d any) error {
	return websocket.JSON.Send(ws, map[string]any{"op": op, "d": d})
}

func (c *Channel) press(ctx context.Context, click surface.ClickFunc, in interaction) {
	user := ""
	if in.Member != nil {
		user = in.Member.User.ID
	} else if in.User != nil {
		user = in.User.ID
	}
	callback := c.base + "/interactions/" + url.PathEscape(in.ID) + "/" + url.PathEscape(in.Token) + "/callback"
	if user != c.owner {
		cctx, cancel := context.WithTimeout(ctx, ackDeadline)
		defer cancel()
		_ = c.do(cctx, http.MethodPost, callback, map[string]any{"type": 4, "data": map[string]any{"content": surface.ErrNotOwner.Error(), "flags": ephemeral}}, nil)
		return
	}
	actx, cancel := context.WithTimeout(ctx, ackDeadline)
	err := c.do(actx, http.MethodPost, callback, map[string]int{"type": 6}, nil)
	cancel()
	if err != nil {
		return
	}
	choice, key, ok := surface.ParseButton(in.Data.CustomID)
	if !ok {
		c.followup(ctx, in, surface.ErrExpired.Error())
		return
	}
	o, err := click(ctx, surface.Click{Key: key, Choice: choice, User: user})
	if err != nil {
		c.followup(ctx, in, err.Error())
		return
	}
	if in.Message == nil {
		return
	}
	e := embed{Color: colorPending}
	if len(in.Message.Embeds) > 0 {
		e = in.Message.Embeds[0]
	}
	ch := in.Message.ChannelID
	if ch == "" {
		ch = in.ChannelID
	}
	cctx, cancel := context.WithTimeout(ctx, shortCall)
	defer cancel()
	_ = c.edit(cctx, ch, in.Message.ID, settled(e, o))
}

func (c *Channel) followup(ctx context.Context, in interaction, msg string) {
	cctx, cancel := context.WithTimeout(ctx, shortCall)
	defer cancel()
	_ = c.do(cctx, http.MethodPost, c.base+"/webhooks/"+url.PathEscape(in.ApplicationID)+"/"+url.PathEscape(in.Token), map[string]any{"content": msg, "flags": ephemeral}, nil)
}

func (c *Channel) do(ctx context.Context, method, target string, body, out any) error {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("discord: encode request: %w", err)
		}
		rd = bytes.NewReader(raw)
	}
	op := c.opName(method, target)
	req, err := http.NewRequestWithContext(ctx, method, target, rd)
	if err != nil {
		return errors.New(op + ": invalid request")
	}
	req.Header.Set("Authorization", "Bot "+c.token)
	req.Header.Set("User-Agent", "DiscordBot (https://versatile.sh, 1)")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New(op + ": " + c.scrub(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		var e struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&e)
		return fmt.Errorf("%s: HTTP %d %s", op, resp.StatusCode, c.scrubText(e.Message))
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s: decode response: %w", op, err)
	}
	return nil
}

func (c *Channel) opName(method, target string) string {
	path := strings.TrimPrefix(target, c.base)
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) > 0 && (parts[0] == "interactions" || parts[0] == "webhooks") {
		return "discord " + method + " /" + parts[0]
	}
	return "discord " + method + " " + path
}

func (c *Channel) scrub(err error) string { return surface.Scrub(err, c.token) }

func (c *Channel) scrubText(s string) string { return surface.ScrubText(s, c.token) }
