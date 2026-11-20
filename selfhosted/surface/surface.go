// Package surface carries approval prompts to the places the owner already is and brings the owner's answer back, behind one Channel interface.
package surface

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Choice is the owner's answer to a held call.
type Choice string

// The answers every surface offers.
const (
	ChoiceOnce    Choice = "once"
	ChoiceSession Choice = "session"
	ChoiceAlways  Choice = "always"
	ChoiceDeny    Choice = "deny"
)

// Valid reports whether c is one of the four answers.
func (c Choice) Valid() bool {
	return c == ChoiceOnce || c == ChoiceSession || c == ChoiceAlways || c == ChoiceDeny
}

// State is how an approval ended, as a surface shows it.
type State string

// The outcomes a message is edited to.
const (
	StateApproved  State = "approved"
	StateSession   State = "approved_for_session"
	StateAlways    State = "always_allowed"
	StateDenied    State = "denied"
	StateExpired   State = "expired"
	StateCancelled State = "cancelled"
)

// Prompt is one held call as a surface shows it; it never carries the call's arguments.
type Prompt struct {
	Key           string    `json:"key"`
	Approval      string    `json:"approval"`
	Connector     string    `json:"connector"`
	ConnectorIcon string    `json:"connector_icon,omitempty"`
	Function      string    `json:"function"`
	FunctionTitle string    `json:"function_title,omitempty"`
	Kind          string    `json:"kind"`
	App           string    `json:"app"`
	ExpiresAt     time.Time `json:"expires_at"`
	Choices       []Choice  `json:"choices"`
	SessionUntil  time.Time `json:"session_until,omitzero"`
	ConsoleURL    string    `json:"console_url"`
}

// Click is an answer a channel received from a person on its platform.
type Click struct {
	Key    string
	Choice Choice
	User   string
}

// Outcome is the end of an approval: what happened, who decided it, through which channel and when.
type Outcome struct {
	State   State     `json:"state"`
	By      string    `json:"by,omitempty"`
	Channel string    `json:"channel,omitempty"`
	At      time.Time `json:"at"`
}

// Ref is a channel's reference to the message it posted for a prompt.
type Ref string

// ClickFunc decides a click and returns the outcome the clicker sees.
type ClickFunc func(ctx context.Context, c Click) (Outcome, error)

// Channel is one place prompts go: Notify posts, Settle edits the post to its outcome, Listen delivers clicks until ctx ends and returns nil at once for a notify-only channel.
type Channel interface {
	Kind() string
	Notify(ctx context.Context, p Prompt) (Ref, error)
	Settle(ctx context.Context, ref Ref, p Prompt, o Outcome) error
	Listen(ctx context.Context, click ClickFunc) error
}

// EventKind tells a requested approval from a settled one.
type EventKind string

// The two kinds of feed events.
const (
	EventRequested EventKind = "requested"
	EventSettled   EventKind = "settled"
)

// Event is one change of an approval on the feed; Outcome is set for a settled one.
type Event struct {
	Kind     EventKind
	Approval string
	Outcome  Outcome
}

// Feed delivers approval events to fn until ctx ends.
type Feed interface {
	Watch(ctx context.Context, fn func(Event)) error
}

// Desk builds a prompt from the engine's record and turns a choice into the engine's decision.
type Desk interface {
	Prompt(ctx context.Context, approval string) (Prompt, error)
	Decide(ctx context.Context, approval string, c Choice, channel, by string) (Outcome, error)
}

// ErrExpired answers a click on a prompt the relay no longer holds.
var ErrExpired = errors.New("this request expired or was already answered; open the console")

// ErrSettled is what a desk answers for an approval that is no longer pending.
var ErrSettled = errors.New("the approval is no longer pending")

// ErrNotOwner answers a click from someone other than the configured owner.
var ErrNotOwner = errors.New("only the owner can answer")

// Label is the English text of a choice, for channels that render buttons; the session choice names how long it lasts when sessionUntil is set.
func Label(c Choice, sessionUntil time.Time) string {
	switch c {
	case ChoiceOnce:
		return "Allow once"
	case ChoiceSession:
		return sessionLabel(time.Until(sessionUntil), sessionUntil.IsZero())
	case ChoiceAlways:
		return "Always allow"
	}
	return "Deny"
}

func sessionLabel(left time.Duration, unknown bool) string {
	switch {
	case unknown || left <= 0:
		return "Allow for this session"
	case left < 90*time.Minute:
		if m := int(left.Round(time.Minute).Minutes()); m > 1 {
			return fmt.Sprintf("Allow for up to %d minutes", m)
		}
		return "Allow for up to 1 minute"
	}
	return fmt.Sprintf("Allow for up to %d hours", int(left.Round(time.Hour).Hours()))
}

// Sentence is the English line a settled message shows.
func Sentence(o Outcome) string {
	what := map[State]string{StateApproved: "Allowed once", StateSession: "Allowed for this session", StateAlways: "Always allowed",
		StateDenied: "Denied", StateExpired: "Expired", StateCancelled: "Cancelled"}[o.State]
	if what == "" {
		what = string(o.State)
	}
	if o.By != "" {
		what += " by " + o.By
	}
	if o.Channel != "" {
		what += " via " + o.Channel
	}
	if !o.At.IsZero() {
		what += " at " + o.At.UTC().Format("2006-01-02 15:04 UTC")
	}
	return what
}

// ButtonID is the short payload a button carries: the choice's first letter and the prompt key.
func ButtonID(c Choice, key string) string { return string(c[0]) + ":" + key }

// ParseButton reads a ButtonID back.
func ParseButton(id string) (Choice, string, bool) {
	if len(id) < 3 || id[1] != ':' {
		return "", "", false
	}
	c := map[byte]Choice{'o': ChoiceOnce, 's': ChoiceSession, 'a': ChoiceAlways, 'd': ChoiceDeny}[id[0]]
	return c, id[2:], c != ""
}
