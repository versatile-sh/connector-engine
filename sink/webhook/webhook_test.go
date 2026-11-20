package webhook_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	plugout "github.com/agents-control/plugkit/outbox"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/sink/webhook"
)

var (
	ctx    = context.Background()
	secret = "whsec_" + "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE="
	t0     = time.Date(2026, 11, 18, 3, 4, 5, 0, time.UTC)
)

type received struct {
	header http.Header
	body   []byte
}

func serve(t *testing.T, status func(n int) int) (*httptest.Server, *[]received) {
	t.Helper()
	var got []received
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = append(got, received{r.Header.Clone(), body})
		w.WriteHeader(status(len(got)))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func events() []port.Event {
	return []port.Event{
		{ID: "e1", Type: "connector.tenant.created", Source: "connector-engine/ta", Tenant: "ta", Subject: "ta", Stream: "connector-engine/ta", Sequence: 1, Time: t0, TraceParent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", Data: map[string]any{"tenant": "ta"}},
		{ID: "e2", Type: "connector.connector.installed", Source: "connector-engine/ta", Tenant: "ta", Stream: "connector-engine/ta", Sequence: 2, Time: t0},
	}
}

func TestDeliverPostsSignedCloudEvents(t *testing.T) {
	srv, got := serve(t, func(int) int { return http.StatusNoContent })
	s, err := webhook.New(webhook.Config{URL: srv.URL, Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Deliver(ctx, events()); err != nil {
		t.Fatal(err)
	}
	key, _ := plugout.ParseKey(secret)
	if len(*got) != 2 {
		t.Fatalf("posts: %d", len(*got))
	}
	for i, r := range *got {
		if err := plugout.Verify(key, r.header, r.body, time.Now(), time.Minute); err != nil {
			t.Fatalf("post %d does not verify: %v", i, err)
		}
	}
	first := (*got)[0]
	var ce map[string]any
	if err := json.Unmarshal(first.body, &ce); err != nil {
		t.Fatal(err)
	}
	data, _ := ce["data"].(map[string]any)
	if first.header.Get("webhook-id") != "e1" || first.header.Get("Content-Type") != "application/cloudevents+json" || first.header.Get("traceparent") == "" ||
		ce["specversion"] != "1.0" || ce["type"] != "connector.tenant.created" || ce["source"] != "connector-engine/ta" || ce["tenant"] != "ta" ||
		ce["sequence"] != float64(1) || ce["stream"] != "connector-engine/ta" || ce["datacontenttype"] != "application/json" || data["tenant"] != "ta" {
		t.Fatalf("headers %v body %s", first.header, first.body)
	}
	var second map[string]any
	_ = json.Unmarshal((*got)[1].body, &second)
	if _, hasData := second["data"]; hasData {
		t.Fatalf("an event without data carries none: %s", (*got)[1].body)
	}
}

func TestAWrongSecretDoesNotVerify(t *testing.T) {
	srv, got := serve(t, func(int) int { return http.StatusOK })
	s, _ := webhook.New(webhook.Config{URL: srv.URL, Secret: secret})
	if err := s.Deliver(ctx, events()[:1]); err != nil {
		t.Fatal(err)
	}
	other, _ := plugout.ParseKey("0123456789abcdef0123456789abcdef0123")
	if err := plugout.Verify(other, (*got)[0].header, (*got)[0].body, time.Now(), time.Minute); err == nil {
		t.Fatal("a signature verified under another key")
	}
}

func TestStatusesMapToRejectionOrUnavailable(t *testing.T) {
	for status, rejected := range map[int]bool{400: true, 404: true, 410: true, 408: false, 429: false, 500: false, 503: false, 302: false} {
		srv, got := serve(t, func(n int) int {
			if n == 2 {
				return status
			}
			return http.StatusOK
		})
		s, _ := webhook.New(webhook.Config{URL: srv.URL, Secret: secret})
		err := s.Deliver(ctx, events())
		var rej *port.Rejection
		switch {
		case rejected && (!asRejection(err, &rej) || rej.Event != "e2"):
			t.Errorf("status %d: want a rejection of e2, got %v", status, err)
		case !rejected && !port.IsCode(err, port.CodeUnavailable):
			t.Errorf("status %d: want unavailable, got %v", status, err)
		}
		if len(*got) != 2 {
			t.Errorf("status %d: %d posts", status, len(*got))
		}
	}
}

func asRejection(err error, target **port.Rejection) bool {
	r, ok := err.(*port.Rejection)
	*target = r
	return ok
}

func TestNewFailsFast(t *testing.T) {
	for _, cfg := range []webhook.Config{{URL: ""}, {URL: "ftp://h"}, {URL: "http://h", Secret: "short"}} {
		if _, err := webhook.New(cfg); !port.IsCode(err, port.CodeInvalidArgument) {
			t.Errorf("accepted %+v", cfg)
		}
	}
}
