// Package webhook is an EventSink that posts CloudEvents 1.0 signed as Standard Webhooks through plugkit's outbox sink.
package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	plugout "github.com/agents-control/plugkit/outbox"
	"github.com/versatile-sh/connector-engine/port"
)

// Config is the endpoint and the signing secret ("whsec_" base64 or at least 32 raw bytes); an empty secret leaves posts unsigned.
type Config struct {
	URL     string
	Secret  string
	Timeout time.Duration
	Client  *http.Client
}

// Sink posts each event on its own, in order, with the event id as webhook-id so a receiver can drop repeats.
type Sink struct{ next *plugout.WebhookSink }

var _ port.EventSink = (*Sink)(nil)

// New validates cfg: an absolute http(s) URL and a secret plugkit accepts.
func New(cfg Config) (*Sink, error) {
	next, err := plugout.NewWebhookSink(plugout.WebhookConfig{URL: cfg.URL, Secret: cfg.Secret, Mode: plugout.Single, Timeout: cfg.Timeout, Client: cfg.Client})
	if err != nil {
		return nil, port.Errorf(port.CodeInvalidArgument, "%v", err)
	}
	return &Sink{next: next}, nil
}

// Deliver stops at the first failure: a permanent 4xx answer is a port.Rejection of that event, any other failure is unavailable.
func (s *Sink) Deliver(ctx context.Context, events []port.Event) error {
	batch := make([]plugout.Event, len(events))
	for i, e := range events {
		batch[i] = cloudEvent(e)
	}
	n, err := s.next.Deliver(ctx, batch)
	if err == nil {
		return nil
	}
	var status *plugout.StatusError
	if errors.As(err, &status) && status.Code >= 400 && status.Code < 500 && status.Code != http.StatusRequestTimeout && status.Code != http.StatusTooManyRequests {
		return &port.Rejection{Event: events[n].ID, Detail: err.Error()}
	}
	return port.Errorf(port.CodeUnavailable, "deliver event %s: %v", events[n].ID, err)
}

func cloudEvent(e port.Event) plugout.Event {
	ext := map[string]any{"stream": e.Stream, "sequence": e.Sequence}
	if e.TraceParent != "" {
		ext["traceparent"] = e.TraceParent
	}
	out := plugout.Event{ID: e.ID, Source: e.Source, SpecVersion: "1.0", Type: e.Type, Subject: e.Subject, Time: e.Time, Tenant: string(e.Tenant), Extensions: ext}
	if e.Data != nil {
		out.DataContentType = "application/json"
		out.Data, _ = json.Marshal(e.Data)
	}
	return out
}
