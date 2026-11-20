package standalone

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
	"github.com/versatile-sh/connector-engine/selfhosted/secrets/hostkey"
)

// Status is what a running install reports on its control socket.
type Status struct {
	PID     int    `json:"pid"`
	Version string `json:"version"`
	URL     string `json:"url"`
	Data    string `json:"data"`
}

var errRunning = errors.New("an install already runs on this data directory")

func listenControl(path string) (net.Listener, error) {
	if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
		c.Close()
		return nil, errRunning
	}
	_ = os.Remove(path)
	return hostkey.Listen(path)
}

func (in *Install) control() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Status{PID: os.Getpid(), Version: in.o.Version, URL: in.URL, Data: in.paths.root})
	})
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		in.stop()
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		h := in.host.Load()
		if h == nil {
			http.Error(w, `{"error":"the console is restarting; try again in a moment"}`, http.StatusServiceUnavailable)
			return
		}
		(*h).ServeHTTP(w, r)
	})
	return mux
}

func request(ctx context.Context, data, method, path string, body io.Reader, out io.Writer) error {
	return console.HostRequest(ctx, layout(data).control, method, path, body, out)
}

func running(ctx context.Context, data string) (Status, bool) {
	var b strings.Builder
	var s Status
	if request(ctx, data, http.MethodGet, "/status", nil, &b) != nil || json.Unmarshal([]byte(b.String()), &s) != nil {
		return s, false
	}
	return s, true
}
