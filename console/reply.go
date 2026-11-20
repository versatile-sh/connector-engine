package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/agents-control/connector-engine/port"
)

var codeStatus = map[port.Code]int{
	port.CodeInvalidArgument: http.StatusBadRequest,
	port.CodeInvalidManifest: http.StatusBadRequest,
	port.CodeNotFound:        http.StatusNotFound,
	port.CodeConflict:        http.StatusConflict,
	port.CodeUnavailable:     http.StatusBadGateway,
}

type apiError struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

type statusError struct {
	status int
	err    error
}

func (e *statusError) Error() string { return e.err.Error() }

func (e *statusError) Unwrap() error { return e.err }

func invalid(format string, args ...any) error {
	return &statusError{status: http.StatusBadRequest, err: fmt.Errorf(format, args...)}
}

func notFound(format string, args ...any) error {
	return &statusError{status: http.StatusNotFound, err: fmt.Errorf(format, args...)}
}

func unreachable(err error) error {
	if err == nil {
		return nil
	}
	return &statusError{status: http.StatusBadGateway, err: err}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, apiError{Error: message})
}

func (c *Console) fail(w http.ResponseWriter, err error) {
	var ee *EngineError
	var se *statusError
	switch {
	case errors.As(err, &ee):
		status := ee.Status
		if status >= 500 {
			status = http.StatusBadGateway
		}
		writeJSON(w, status, apiError{Error: ee.Detail, Code: ee.Code})
	case errors.As(err, &se):
		writeError(w, se.status, err.Error())
	case codeStatus[port.CodeOf(err)] != 0:
		writeError(w, codeStatus[port.CodeOf(err)], err.Error())
	default:
		c.Log.Error("console request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "the console could not do this; its log has the cause")
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "the body is JSON")
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAPIBody)).Decode(into); err != nil {
		writeError(w, http.StatusBadRequest, "the body is not valid JSON")
		return false
	}
	return true
}
