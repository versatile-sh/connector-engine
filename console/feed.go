package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/surface"
)

const (
	feedRetryMin = time.Second
	feedRetryMax = 30 * time.Second
	feedReplay   = time.Minute
)

type engineFeed struct {
	c      *Console
	client http.Client
	since  time.Time
	fresh  bool
}

func (f *engineFeed) Watch(ctx context.Context, fn func(surface.Event)) error {
	f.since, f.fresh = time.Now(), f.c.State.Cursor() == 0
	seeded := false
	wait := feedRetryMin
	for ctx.Err() == nil {
		if !seeded {
			if list, err := f.c.Approvals(ctx); err == nil {
				seeded = true
				for _, a := range list {
					fn(surface.Event{Kind: surface.EventRequested, Approval: string(a.ID)})
				}
			}
		}
		err := f.stream(ctx, fn)
		if ctx.Err() != nil {
			return nil
		}
		f.c.logger().Warn("approval watch interrupted", "error", err, "retry_in", wait)
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
		wait = min(wait*2, feedRetryMax)
	}
	return nil
}

func (f *engineFeed) stream(ctx context.Context, fn func(surface.Event)) error {
	tok, err := f.c.Engine.personToken(ctx, false)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(f.c.Engine.PublicURL, "/")+"/v1/approvals:watch"+q("after", strconv.FormatInt(f.c.State.Cursor(), 10)), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		_, _ = f.c.Engine.personToken(ctx, true)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("watch answered %d", resp.StatusCode)
	}
	err = ReadEvents(resp.Body, func(data []byte) {
		var ev port.Event
		if json.Unmarshal(data, &ev) == nil {
			f.handle(ctx, ev, fn)
			_ = f.c.State.SetCursor(ev.Sequence)
		}
	})
	if err != nil {
		return err
	}
	return fmt.Errorf("the watch stream ended")
}

func (f *engineFeed) handle(ctx context.Context, ev port.Event, fn func(surface.Event)) {
	id, _ := ev.Data["approval"].(string)
	if id == "" {
		return
	}
	if ev.Type == "connector.approval.requested" {
		if !f.fresh || ev.Time.After(f.since.Add(-feedReplay)) {
			fn(surface.Event{Kind: surface.EventRequested, Approval: id})
		}
		return
	}
	switch ev.Type {
	case "connector.approval.decided", "connector.approval.expired", "connector.approval.cancelled":
	default:
		return
	}
	if !f.c.surf.holds(id) {
		return
	}
	o := surface.Outcome{State: surface.StateCancelled, At: ev.Time}
	if v, err := f.c.Approval(ctx, id); err == nil {
		o = f.c.Outcome(v)
		if o.At.IsZero() {
			o.At = ev.Time
		}
	} else if ev.Type == "connector.approval.expired" {
		o.State = surface.StateExpired
	}
	fn(surface.Event{Kind: surface.EventSettled, Approval: id, Outcome: o})
}
