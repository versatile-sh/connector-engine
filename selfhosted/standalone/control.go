package standalone

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
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
	lis, err := net.Listen("unix", path)
	if err != nil {
		if c, derr := net.DialTimeout("unix", path, time.Second); derr == nil {
			c.Close()
			return nil, errRunning
		}
		if info, lerr := os.Lstat(path); lerr != nil || info.Mode()&os.ModeSocket == 0 {
			return nil, err
		}
		_ = os.Remove(path)
		if lis, err = net.Listen("unix", path); err != nil {
			return nil, err
		}
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			lis.Close()
			return nil, err
		}
	}
	return peerListener{Listener: lis, uid: os.Geteuid()}, nil
}

type peerListener struct {
	net.Listener
	uid int
}

// Accept returns the next connection whose peer runs as the owning user and closes the others; an OS without peer credentials relies on the 0700 data directory.
func (l peerListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if uid, err := peerUID(c); err == nil && (uid < 0 || uid == l.uid) {
			return c, nil
		}
		c.Close()
	}
}

func (in *Install) control() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Status{PID: os.Getpid(), Version: in.o.Version, URL: in.URL, Data: in.paths.root})
	})
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_ = http.NewResponseController(w).Flush()
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

// Running returns the status of the install that runs on the data directory data, if one does.
func Running(ctx context.Context, data string) (Status, bool) { return running(ctx, data) }
