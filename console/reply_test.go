package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agents-control/connector-engine/console/sealed"
	"github.com/agents-control/connector-engine/port"
)

func TestFailAnswersTheStatusTheErrorCarries(t *testing.T) {
	c := &Console{}
	for _, tc := range []struct {
		err    error
		status int
		body   apiError
	}{
		{&EngineError{Status: 404, Code: "not_found", Detail: "no such connector"}, 404, apiError{Error: "no such connector", Code: "not_found"}},
		{fmt.Errorf("function f: %w", &EngineError{Status: 503, Code: "unavailable", Detail: "down"}), 502, apiError{Error: "down", Code: "unavailable"}},
		{invalid("from is before to"), 400, apiError{Error: "from is before to"}},
		{fmt.Errorf("listing: %w", invalid("no name")), 400, apiError{Error: "listing: no name"}},
		{notFound("no agent is named x"), 404, apiError{Error: "no agent is named x"}},
		{unreachable(errors.New("example.com answered 500")), 502, apiError{Error: "example.com answered 500"}},
		{port.Errorf(port.CodeInvalidManifest, "skill entry: no id"), 400, apiError{Error: "invalid_manifest: skill entry: no id"}},
		{fmt.Errorf("channel: %w", sealed.ErrLocked), 503, apiError{Error: "channel: " + sealed.ErrLocked.Error()}},
		{errors.New("open /state.json: permission denied"), 500, apiError{Error: "the console could not do this; its log has the cause"}},
	} {
		rec := httptest.NewRecorder()
		c.fail(rec, tc.err)
		var got apiError
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != tc.status || got != tc.body {
			t.Errorf("%v: got %d %+v, want %d %+v", tc.err, rec.Code, got, tc.status, tc.body)
		}
	}
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusTeapot, "x")
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("an error answer is not cached")
	}
}
