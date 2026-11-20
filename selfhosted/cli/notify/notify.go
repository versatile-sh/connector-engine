// Package notify is the host helper of the person profile: it follows the console's approval stream and shows each prompt as a native notification.
package notify

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
	"github.com/versatile-sh/connector-engine/selfhosted/surface/desktop"
)

const (
	defaultConsole = "http://127.0.0.1:7465"
	retryMin       = time.Second
	retryMax       = 30 * time.Second
)

// ErrUnpaired is returned when the console refuses the device key.
var ErrUnpaired = errors.New("the console refused this key: pair this computer again in Notifications")

// Shower shows one notification and returns once it is dismissed or clicked.
type Shower interface {
	Show(ctx context.Context, title, body, link string) error
}

// Helper follows one console's approval stream with one device key.
type Helper struct {
	Console string
	Key     string
	Show    Shower
	HTTP    *http.Client
	Log     io.Writer
	Retry   time.Duration

	mu    sync.Mutex
	shown map[string]context.CancelFunc
}

// Command parses the flags of cnctl notify and runs the helper until ctx ends.
func Command(ctx context.Context, args []string, getenv func(string) string, stderr io.Writer) int {
	fs := flag.NewFlagSet("cnctl notify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	home, _ := os.UserHomeDir()
	base := fs.String("console", cmp.Or(getenv("CN_CONSOLE"), defaultConsole), "address of the local console")
	keyFile := fs.String("key-file", cmp.Or(getenv("CN_NOTIFY_KEY_FILE"), filepath.Join(home, ".config", "versatile", "notify.key")), "file holding the device key from Notifications")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	raw, err := os.ReadFile(*keyFile)
	if err != nil || strings.TrimSpace(string(raw)) == "" {
		fmt.Fprintf(stderr, "cnctl notify: read the device key: %v\n", err)
		return 1
	}
	if st, err := os.Stat(*keyFile); err == nil && st.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(stderr, "cnctl notify: %s is readable by others; run chmod 600 on it\n", *keyFile)
	}
	h := &Helper{Console: strings.TrimRight(*base, "/"), Key: strings.TrimSpace(string(raw)), Show: desktop.Default(), Log: stderr}
	if err := h.Run(ctx); err != nil {
		fmt.Fprintln(stderr, "cnctl notify:", err)
		return 1
	}
	return 0
}

// Run follows the stream and reconnects after a loss until ctx ends or the key is refused.
func (h *Helper) Run(ctx context.Context) error {
	if h.HTTP == nil {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.ResponseHeaderTimeout = 30 * time.Second
		h.HTTP = &http.Client{Transport: t}
	}
	if h.Log == nil {
		h.Log = io.Discard
	}
	wait := retryMin
	if h.Retry > 0 {
		wait = h.Retry
	}
	fmt.Fprintf(h.Log, "following approvals of %s\n", h.Console)
	for ctx.Err() == nil {
		err := h.follow(ctx)
		if errors.Is(err, ErrUnpaired) {
			return err
		}
		if ctx.Err() != nil {
			break
		}
		fmt.Fprintf(h.Log, "console stream lost: %v; retrying in %s\n", err, wait)
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
		wait = min(wait*2, retryMax)
	}
	return nil
}

func (h *Helper) follow(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.Console+"/api/notify/stream", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+h.Key)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := h.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnpaired
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the console answered %d", resp.StatusCode)
	}
	err = console.ReadEvents(resp.Body, func(data []byte) {
		var ev console.HubEvent
		if json.Unmarshal(data, &ev) == nil {
			h.handle(ctx, ev)
		}
	})
	if err != nil {
		return err
	}
	return errors.New("the stream ended")
}

// Text is the title and body of a prompt's notification.
func Text(ev console.HubEvent) (string, string) {
	p := ev.Prompt
	fn := cmp.Or(p.FunctionTitle, p.Function)
	return fmt.Sprintf("Allow %s: %s?", p.Connector, fn),
		fmt.Sprintf("%s wants to run %s. Click to answer before %s.", p.App, p.Function, p.ExpiresAt.Local().Format("15:04"))
}

func (h *Helper) handle(ctx context.Context, ev console.HubEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.shown == nil {
		h.shown = map[string]context.CancelFunc{}
	}
	switch ev.Type {
	case "prompt":
		if ev.Prompt == nil || ev.Link == "" || h.shown[ev.Approval] != nil {
			return
		}
		nctx, cancel := context.WithDeadline(ctx, ev.Prompt.ExpiresAt)
		h.shown[ev.Approval] = cancel
		title, body := Text(ev)
		go func() {
			if err := h.Show.Show(nctx, title, body, ev.Link); err != nil && nctx.Err() == nil {
				fmt.Fprintf(h.Log, "notification failed: %v\n", err)
			}
		}()
	case "settled":
		if cancel := h.shown[ev.Approval]; cancel != nil {
			cancel()
			delete(h.shown, ev.Approval)
		}
	}
}
