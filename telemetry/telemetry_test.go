package telemetry_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/gateway"
	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
	sinkmemory "github.com/agents-control/connector-engine/sink/memory"
	"github.com/agents-control/connector-engine/store/memory"
	"github.com/agents-control/connector-engine/telemetry"
)

type core struct {
	gateway.Core
	out service.Outcome
	err error
}

func (c core) Call(context.Context, port.Principal, service.CallRequest) (service.Outcome, error) {
	return c.out, c.err
}

func scrape(t *testing.T, m *telemetry.Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	return rec.Body.String()
}

func TestCallsAreCountedByOutcomeAndNeverLeakArguments(t *testing.T) {
	store := memory.New()
	m := telemetry.New(store, time.Now)
	p := port.Principal{Kind: port.PrincipalAgent, Claims: port.Claims{Tenant: "t1", Agent: "x"}}
	req := service.CallRequest{Name: "s__search", Arguments: []byte(`{"secret":"hunter2"}`)}
	for _, c := range []core{{out: service.Outcome{Status: service.OutcomeResult}}, {out: service.Outcome{Status: service.OutcomeResult}},
		{out: service.Outcome{Status: service.OutcomePending}}, {err: errors.New("boom")}} {
		_, _ = m.Core(c).Call(context.Background(), p, req)
	}
	got := scrape(t, m)
	for _, want := range []string{
		`connector_calls_total{outcome="result",tenant="t1"} 2`, `connector_calls_total{outcome="pending_approval",tenant="t1"} 1`,
		`connector_calls_total{outcome="failed",tenant="t1"} 1`, `connector_gateway_call_duration_seconds_count{tenant="t1"} 4`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
	if strings.Contains(got, "hunter2") || strings.Contains(got, "search") {
		t.Fatal("arguments or function names leaked into the metrics")
	}
}

func TestApprovalsAreCountedByState(t *testing.T) {
	m := telemetry.New(memory.New(), time.Now)
	next := sinkmemory.New()
	sink := m.Sink(next)
	events := []port.Event{
		{Type: "connector.approval.requested", Tenant: "t1"}, {Type: "connector.approval.decided", Tenant: "t1", Data: map[string]any{"decision": "approve"}},
		{Type: "connector.approval.decided", Tenant: "t1", Data: map[string]any{"decision": "deny"}}, {Type: "connector.approval.expired", Tenant: "t1"},
		{Type: "connector.call.finished", Tenant: "t1"},
	}
	if err := sink.Deliver(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	got := scrape(t, m)
	for _, state := range []string{"pending", "approved", "denied", "expired"} {
		if !strings.Contains(got, `connector_approvals_total{state="`+state+`",tenant="t1"} 1`) {
			t.Errorf("state %s missing in\n%s", state, got)
		}
	}
	if len(next.Events()) != 5 {
		t.Fatalf("every event is forwarded: %d", len(next.Events()))
	}
	next.FailWith(func([]port.Event) error { return errors.New("down") })
	_ = sink.Deliver(context.Background(), events[:1])
	if !strings.Contains(scrape(t, m), `connector_approvals_total{state="pending",tenant="t1"} 1`) {
		t.Fatal("a failed delivery is not counted")
	}
}

func TestOutboxLagIsTheAgeOfTheOldestPendingEvent(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	m := telemetry.New(store, func() time.Time { return now })
	if !strings.Contains(scrape(t, m), "connector_outbox_lag_seconds 0\n") {
		t.Fatal("an empty outbox has no lag")
	}
	if err := store.Write(context.Background(), port.Scope{Installation: true}, func(tx port.Tx) error {
		_, err := tx.Emit(port.Event{Type: "connector.tenant.created", Time: now.Add(-90 * time.Second)})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(scrape(t, m), "connector_outbox_lag_seconds 90\n") {
		t.Fatalf("lag:\n%s", scrape(t, m))
	}
}
