package console

import (
	"bytes"
	"io"
	"net/http"
)

const maxHostSecret = 4 << 10

// HostHandler serves what only the local machine may ask, through a socket that only docker compose exec reaches: a password reset, a backup export and an import.
func (c *Console) HostHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /password", c.resetPassword)
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
	password, err := readSecret(r.Body)
	if err == nil {
		err = c.State.SetPassword(string(password), "")
	}
	if err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
