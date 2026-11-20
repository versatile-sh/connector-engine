package console_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

func TestTheAppAsksTheOwnerThroughElicitation(t *testing.T) {
	for _, era := range []struct{ name, version string }{{"session era", "2025-11-25"}, {"stateless era", ""}} {
		t.Run(era.name, func(t *testing.T) {
			r := newRig(t)
			var mu sync.Mutex
			var asked []string
			answer := "once"
			action := "accept"
			handler := func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				mu.Lock()
				defer mu.Unlock()
				asked = append(asked, req.Params.Message)
				return &mcp.ElicitResult{Action: action, Content: map[string]any{"decision": answer}}, nil
			}
			count := func() int {
				mu.Lock()
				defer mu.Unlock()
				return len(asked)
			}
			cs := r.connect(era.version, handler)

			t.Run("off for the app, the call waits for another surface", func(t *testing.T) {
				pending := r.call(cs, "create", map[string]any{"n": 1})
				r.approveFirstPending()
				ran(t, within(t, pending, 8*time.Second), "create")
				if count() != 0 {
					t.Fatal("asked while elicitation is off")
				}
			})
			r.b.ok("PUT", "/api/notifications/apps/claude-code", map[string]any{"elicit": true}, nil)

			t.Run("allow once answers the call before the hold window ends", func(t *testing.T) {
				start := time.Now()
				ran(t, within(t, r.call(cs, "create", map[string]any{"n": 2}), 8*time.Second), "create")
				mu.Lock()
				msg := strings.Join(asked, "|")
				mu.Unlock()
				if count() != 1 || !strings.Contains(msg, "Allow Notes: create?") || !strings.Contains(msg, "claude-code") || time.Since(start) > 8*time.Second {
					t.Fatalf("asked %d times: %q", count(), msg)
				}
				if r.up.count("create") != 2 {
					t.Fatalf("upstream ran %d times", r.up.count("create"))
				}
				r.eventually("the elicited call in Activity", func() bool {
					var p console.ActivityPage
					r.b.ok("GET", "/api/activity", nil, &p)
					ok := 0
					for _, call := range p.Calls {
						if call.Function == "create" && call.Outcome == console.OutcomeOK {
							ok++
						}
					}
					return ok == 2
				})
			})

			t.Run("a destructive function is never asked in the app", func(t *testing.T) {
				before := count()
				pending := r.call(cs, "purge", map[string]any{})
				time.Sleep(600 * time.Millisecond)
				r.approveFirstPending()
				ran(t, within(t, pending, 8*time.Second), "purge")
				if count() != before {
					t.Fatal("a destructive function was asked in the app")
				}
			})

			t.Run("declining denies the call", func(t *testing.T) {
				mu.Lock()
				action = "decline"
				mu.Unlock()
				got := within(t, r.call(cs, "create", map[string]any{"n": 3}), 8*time.Second)
				if got.err == nil && !got.res.IsError {
					t.Fatalf("a declined call ran: %+v", got.res)
				}
				if r.up.count("create") != 2 {
					t.Fatalf("upstream ran %d times", r.up.count("create"))
				}
			})
		})
	}
}
