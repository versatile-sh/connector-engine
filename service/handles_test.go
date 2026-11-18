package service_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
	st "github.com/agents-control/connector-engine/service/servicetest"
)

func TestASlowUpstreamDoesNotBlockAnotherConnector(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	entered, release := make(chan struct{}), make(chan struct{})
	w.Runtime.OnStart = func(c port.Connector) error {
		if c.Alias == "slow" {
			close(entered)
			<-release
		}
		return nil
	}
	defer close(release)
	go func() {
		_, _ = w.Svc.InstallConnector(w.Ctx, w.admin, service.InstallInput{Workspace: workspace, Alias: "slow", Listing: ref})
	}()
	<-entered
	done := make(chan error, 1)
	go func() {
		_, err := w.Svc.InstallConnector(w.Ctx, w.admin, service.InstallInput{Workspace: workspace, Alias: "fast", Listing: ref})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a connector that is starting blocks every other connector")
	}
}

func TestAnUnavailableOrIdleHandleIsStartedAgain(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.acceptAll()
	w.allowAll("x")
	x := w.agent("x", "s1")
	want(t, w.call(x, "s__search", `{}`).Status, service.OutcomeResult, "first call")
	starts := w.Runtime.Starts
	w.Runtime.Handle("search", func(json.RawMessage) (port.Result, error) {
		return port.Result{}, port.Errorf(port.CodeUnavailable, "session lost")
	})
	want(t, w.call(x, "s__search", `{}`).Code, service.CodeCallFailed, "a lost upstream fails the call")
	w.Runtime.Handle("search", nil)
	want(t, w.call(x, "s__search", `{}`).Status, service.OutcomeResult, "the next call starts the connector again")
	want(t, w.Runtime.Starts, starts+1, "one restart after unavailable")
	stops := w.Runtime.Stops
	w.Clock.Advance(16 * time.Minute)
	st.Ok(w.Svc.InstallConnector(w.Ctx, w.admin, service.InstallInput{Workspace: workspace, Alias: "other", Listing: ref}))
	if w.Runtime.Stops <= stops {
		t.Fatal("an idle handle is stopped")
	}
}

func TestARelistThatLostARaceRunsAgain(t *testing.T) {
	w := newWorld(t, opts{reviewed: true})
	w.acceptAll()
	first := true
	w.Runtime.OnList = func() error {
		if first {
			first = false
			st.Ok(w.Svc.SetConnectorEnabled(w.Ctx, w.admin, w.conn.ID, true))
		}
		return nil
	}
	if _, err := w.Svc.Relist(w.Ctx, tenant, w.conn.ID); err != nil {
		t.Fatalf("a re-list that lost a race to another write must run again: %v", err)
	}
	w.Runtime.OnList = nil
	errs := make(chan error, 8)
	for range 8 {
		go func() {
			_, err := w.Svc.Relist(w.Ctx, tenant, w.conn.ID)
			errs <- err
		}()
	}
	for range 8 {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent re-lists: %v", err)
		}
	}
}
