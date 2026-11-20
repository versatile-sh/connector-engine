package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
)

const maxHostSecret = 4 << 10

// HostRequest sends one request to the host handler served on the unix socket and copies the answer to out; a refusal is returned as its error message.
func HostRequest(ctx context.Context, socket, method, path string, body io.Reader, out io.Writer) error {
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}}
	req, err := http.NewRequestWithContext(ctx, method, "http://console"+path, body)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		if json.Unmarshal(raw, &e) != nil || e.Error == "" {
			e.Error = strings.TrimSpace(string(raw))
		}
		return errors.New(e.Error)
	}
	_, err = io.Copy(out, resp.Body)
	return err
}

// HostHandler serves what only the local machine may ask, through a socket that only the host reaches: a password reset (an optional username line, then the password), a backup export and an import.
func (c *Console) HostHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /password", c.resetPassword)
	mux.HandleFunc("POST /export", c.hostExport)
	mux.HandleFunc("POST /inspect", c.hostInspect)
	mux.HandleFunc("POST /import", c.hostImport)
	return mux
}

func readSecret(r io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxHostSecret+1))
	if err == nil && len(raw) > maxHostSecret {
		err = invalid("the secret is longer than %d bytes", maxHostSecret)
	}
	return bytes.TrimRight(raw, "\r\n"), err
}

func (c *Console) resetPassword(w http.ResponseWriter, r *http.Request) {
	body, err := readSecret(r.Body)
	username, password, named := bytes.Cut(body, []byte("\n"))
	if !named {
		username, password = nil, body
	}
	var u User
	if err == nil {
		u, err = c.State.HostUser(strings.TrimSpace(string(username)))
	}
	if err == nil {
		err = c.State.SetPassword(u.Person, string(bytes.TrimRight(password, "\r")), "")
	}
	if err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
