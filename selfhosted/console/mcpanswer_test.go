package console_test

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

func TestAnAppThatAcceptsEveryElicitationApprovesNothing(t *testing.T) {
	for _, era := range []struct{ name, version string }{{"session era", "2025-11-25"}, {"stateless era", ""}} {
		t.Run(era.name, func(t *testing.T) {
			r := newSeededRig(t, `{"notifications":{"elicit":{"claude-code":true}}}`, func(c *config.Config) { c.Approval.Timeout = 2 * time.Second })
			var asked atomic.Int32
			cs := r.connect(era.version, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				asked.Add(1)
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"decision": "once", "approval": "once"}}, nil
			})

			got := within(t, r.call(cs, "create", map[string]any{"n": 1}), 8*time.Second)
			if got.err == nil && !got.res.IsError && textOf(got.res) == "ran create" {
				t.Fatal("the call ran on the app's own answer")
			}
			if asked.Load() != 0 || r.up.count("create") != 0 {
				t.Fatalf("asked %d times, upstream ran %d times", asked.Load(), r.up.count("create"))
			}
			r.eventually("the approval expiring", func() bool {
				var rows struct{ Items []port.Approval }
				if err := r.eng.Read(context.Background(), "GET", "/v1/approvals", nil, &rows); err != nil {
					t.Fatal(err)
				}
				return len(rows.Items) == 1 && rows.Items[0].State == port.ApprovalExpired && rows.Items[0].DecidedBy == ""
			})
			if r.up.count("create") != 0 {
				t.Fatal("an expired approval was forwarded")
			}
		})
	}
}

func TestAStateFileWithTheOldInAppSwitchLoadsAndDropsIt(t *testing.T) {
	path := t.TempDir() + "/state.json"
	if err := os.WriteFile(path, []byte(`{"notifications":{"elicit":{"claude-code":true}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := console.OpenState(path, "owner")
	if err != nil {
		t.Fatalf("an old state file did not load: %v", err)
	}
	if err := state.DismissOnboarding(); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(path); err != nil || strings.Contains(string(raw), "elicit") {
		t.Fatalf("the old switch was written back: %s %v", raw, err)
	}
}
