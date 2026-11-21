package console_test

import (
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

const refusedAnswer = `{"jsonrpc":"2.0","id":1,"error":{"code":-32001,"message":"invalid token"}}`

const searchAnswer = `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"ok"}],"isError":false}}`

func (r *activityRig) acceptOnly(token string, hits *atomic.Int32) {
	r.answer(func(w http.ResponseWriter, req *http.Request) {
		hits.Add(1)
		if req.Header.Get("Authorization") != "Bearer "+token {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(refusedAnswer))
			return
		}
		jsonAnswer(searchAnswer, nil)(w, req)
	})
}

func TestAnUnsignedBearerCannotLabelActivity(t *testing.T) {
	r := newActivityRig(t)
	var hits atomic.Int32
	accepted := r.token
	r.acceptOnly(accepted, &hits)
	r.token = agentToken("someone-else", "their-session")
	resp, _ := r.post(toolCall(1, "notes__search", `{}`))
	if resp.StatusCode != http.StatusUnauthorized || hits.Load() != 1 {
		t.Fatalf("the engine was not asked or accepted a forged bearer: %d, %d hits", resp.StatusCode, hits.Load())
	}
	r.token = accepted
	r.post(toolCall(1, "notes__search", `{}`))
	if row := only(t, r.rows("", 1)); row.Agent != "claude-code" {
		t.Fatalf("a forged bearer labelled activity: %+v", row)
	}
}

func TestAnAcceptedBearerIsLabelled(t *testing.T) {
	r := newActivityRig(t)
	var hits atomic.Int32
	r.acceptOnly(r.token, &hits)
	if resp, _ := r.post(toolCall(1, "notes__search", `{}`)); resp.StatusCode != http.StatusOK {
		t.Fatalf("an accepted bearer: %d", resp.StatusCode)
	}
	if row := only(t, r.rows("", 1)); row.Agent != "claude-code" || row.Outcome != console.OutcomeOK {
		t.Fatalf("row: %+v", row)
	}
}

func TestARevokedKeyIsRefusedWithAndWithoutAValidSignature(t *testing.T) {
	r := newActivityRig(t)
	if err := r.state.PutAgent(console.Agent{Name: "claude-code", Owner: "owner"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.state.RevokeAgent("claude-code", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int32
	r.acceptOnly(r.token, &hits)
	for name, token := range map[string]string{"signed": r.token, "unsigned": agentToken("claude-code", "s")} {
		r.token = token
		if resp, _ := r.post(toolCall(1, "notes__search", `{}`)); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("a revoked %s key: %d", name, resp.StatusCode)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("a revoked key reached the engine %d times", hits.Load())
	}
}
