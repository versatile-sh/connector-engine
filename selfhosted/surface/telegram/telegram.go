// Package telegram is the Telegram Bot API surface: a private message with an inline keyboard, answered through getUpdates long polling.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/selfhosted/surface"
)

const (
	defaultAPIBase = "https://api.telegram.org"
	pollSeconds    = 25
	shortCall      = 10 * time.Second
)

var _ surface.Channel = (*Channel)(nil)

// Config holds the bot token, the owner's numeric user id, the API base and an optional HTTP client whose timeout outlasts the long poll.
type Config struct {
	Token       string
	OwnerUserID string
	APIBase     string
	HTTP        *http.Client
}

// Channel posts prompts to the owner's private chat with the bot and reads button presses by long polling.
type Channel struct {
	token  string
	owner  int64
	base   string
	http   *http.Client
	poll   int
	offset int64
}

// New validates cfg and returns a Telegram channel.
func New(cfg Config) (*Channel, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("telegram: token is required")
	}
	owner, err := strconv.ParseInt(strings.TrimSpace(cfg.OwnerUserID), 10, 64)
	if err != nil || owner <= 0 {
		return nil, errors.New("telegram: owner user id must be a positive number")
	}
	base := strings.TrimRight(cfg.APIBase, "/")
	if base == "" {
		base = defaultAPIBase
	}
	if u, err := url.Parse(base); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("telegram: api base must be an http or https URL")
	}
	hc := cfg.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: (pollSeconds + 10) * time.Second}
	}
	return &Channel{token: cfg.Token, owner: owner, base: base, http: hc, poll: pollSeconds}, nil
}

// Kind returns "telegram".
func (c *Channel) Kind() string { return "telegram" }

type button struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type keyboard struct {
	InlineKeyboard [][]button `json:"inline_keyboard"`
}

type message struct {
	MessageID int64 `json:"message_id"`
	Chat      struct {
		ID int64 `json:"id"`
	} `json:"chat"`
	Text string `json:"text"`
}

type callbackQuery struct {
	ID   string `json:"id"`
	From struct {
		ID int64 `json:"id"`
	} `json:"from"`
	Message *message `json:"message"`
	Data    string   `json:"data"`
}

type update struct {
	UpdateID      int64          `json:"update_id"`
	CallbackQuery *callbackQuery `json:"callback_query"`
}

// Notify sends the prompt to the owner's private chat with one button per choice.
func (c *Channel) Notify(ctx context.Context, p surface.Prompt) (surface.Ref, error) {
	body := map[string]any{"chat_id": c.owner, "text": text(p)}
	if len(p.Choices) > 0 {
		row := make([]button, 0, len(p.Choices))
		for _, ch := range p.Choices {
			row = append(row, button{Text: surface.Label(ch, p.SessionUntil), CallbackData: surface.ButtonID(ch, p.Key)})
		}
		body["reply_markup"] = keyboard{InlineKeyboard: [][]button{row}}
	}
	var m message
	if err := c.call(ctx, "sendMessage", body, &m); err != nil {
		return "", err
	}
	return surface.Ref(strconv.FormatInt(m.Chat.ID, 10) + ":" + strconv.FormatInt(m.MessageID, 10)), nil
}

// Settle edits the posted message to the prompt text plus the outcome and removes its buttons.
func (c *Channel) Settle(ctx context.Context, ref surface.Ref, p surface.Prompt, o surface.Outcome) error {
	chat, msg, ok := strings.Cut(string(ref), ":")
	if !ok {
		return fmt.Errorf("telegram: malformed message reference %q", ref)
	}
	return c.edit(ctx, chat, msg, text(p), o)
}

// Listen long-polls getUpdates for button presses until ctx ends.
func (c *Channel) Listen(ctx context.Context, click surface.ClickFunc) error {
	for ctx.Err() == nil {
		var ups []update
		err := c.call(ctx, "getUpdates", map[string]any{"offset": c.offset, "timeout": c.poll, "allowed_updates": []string{"callback_query"}}, &ups)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return err
		}
		for _, u := range ups {
			c.offset = max(c.offset, u.UpdateID+1)
			if u.CallbackQuery != nil {
				c.press(ctx, click, u.CallbackQuery)
			}
		}
	}
	return nil
}

func (c *Channel) press(ctx context.Context, click surface.ClickFunc, q *callbackQuery) {
	if q.From.ID != c.owner {
		c.answer(ctx, q.ID, surface.ErrNotOwner.Error())
		return
	}
	choice, key, ok := surface.ParseButton(q.Data)
	if !ok {
		c.answer(ctx, q.ID, surface.ErrExpired.Error())
		return
	}
	o, err := click(ctx, surface.Click{Key: key, Choice: choice, User: strconv.FormatInt(q.From.ID, 10)})
	if err != nil {
		c.answer(ctx, q.ID, err.Error())
		return
	}
	c.answer(ctx, q.ID, surface.Sentence(o))
	if q.Message != nil {
		_ = c.edit(ctx, strconv.FormatInt(q.Message.Chat.ID, 10), strconv.FormatInt(q.Message.MessageID, 10), q.Message.Text, o)
	}
}

func (c *Channel) answer(ctx context.Context, id, msg string) {
	if r := []rune(msg); len(r) > 200 {
		msg = string(r[:199]) + "…"
	}
	cctx, cancel := context.WithTimeout(ctx, shortCall)
	defer cancel()
	_ = c.call(cctx, "answerCallbackQuery", map[string]any{"callback_query_id": id, "text": msg}, nil)
}

func (c *Channel) edit(ctx context.Context, chat, msg, base string, o surface.Outcome) error {
	cctx, cancel := context.WithTimeout(ctx, shortCall)
	defer cancel()
	err := c.call(cctx, "editMessageText", map[string]any{
		"chat_id": chat, "message_id": msg, "text": base + "\n\n" + surface.Sentence(o),
		"reply_markup": keyboard{InlineKeyboard: [][]button{}},
	}, nil)
	if err != nil && strings.Contains(err.Error(), "message is not modified") {
		return nil
	}
	return err
}

func text(p surface.Prompt) string {
	fn := p.Function
	if p.FunctionTitle != "" && p.FunctionTitle != p.Function {
		fn = p.FunctionTitle + " (" + p.Function + ")"
	}
	return "Approval needed\nConnector: " + p.Connector + "\nFunction: " + fn + "\nApp: " + p.App +
		"\nExpires: " + p.ExpiresAt.UTC().Format("2006-01-02 15:04 UTC")
}

type reply struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

func (c *Channel) call(ctx context.Context, method string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/bot"+c.token+"/"+method, bytes.NewReader(raw))
	if err != nil {
		return errors.New("telegram " + method + ": " + c.scrub(err))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("telegram " + method + ": " + c.scrub(err))
	}
	defer resp.Body.Close()
	var r reply
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("telegram %s: HTTP %d", method, resp.StatusCode)
	}
	if !r.OK {
		return errors.New("telegram " + method + ": " + c.scrubText(fmt.Sprintf("%d %s", r.ErrorCode, r.Description)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(r.Result, out); err != nil {
		return fmt.Errorf("telegram %s: decode result: %w", method, err)
	}
	return nil
}

func (c *Channel) scrub(err error) string { return surface.Scrub(err, c.token) }

func (c *Channel) scrubText(s string) string { return surface.ScrubText(s, c.token) }

// Kind describes the Telegram channel for the console's settings page and factory.
var Kind = surface.Kind{
	Name:        "telegram",
	Title:       "Telegram",
	Interactive: true,
	Fields: []surface.Field{
		{Name: "token", Label: "Bot token", Secret: true, Hint: true},
		{Name: "owner_user_id", Label: "Your Telegram user id"},
	},
	Owner: regexp.MustCompile(`^[0-9]{1,24}$`),
	New: func(s map[string]string) (surface.Channel, error) {
		return New(Config{Token: s["token"], OwnerUserID: s["owner_user_id"]})
	},
}
