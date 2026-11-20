package console

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

func (c *Console) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		csrf, ok := c.Sessions.Get(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "sign in first")
			return
		}
		if !safeMethod(r.Method) && !csrfOK(csrf, r.Header.Get(csrfHeader)) {
			writeError(w, http.StatusForbidden, "missing or wrong CSRF token")
			return
		}
		next(w, r)
	}
}

func (c *Console) session(w http.ResponseWriter, r *http.Request) {
	csrf, ok := c.Sessions.Get(r)
	out := map[string]any{"owner_exists": c.State.HasOwner(), "signed_in": ok, "mcp_url": c.MCPURL}
	if ok {
		out["csrf"] = csrf
		out["onboarding_dismissed"] = c.State.OnboardingDismissed()
		out["key_protection"] = c.keyProtection(r.Context())
	}
	writeJSON(w, http.StatusOK, out)
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (c *Console) setup(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !readJSON(w, r, &in) {
		return
	}
	err := c.State.CreateOwner(strings.TrimSpace(in.Username), in.Password)
	switch {
	case errors.Is(err, ErrOwnerExists):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		c.fail(w, err)
	default:
		c.signIn(w, http.StatusCreated)
	}
}

func (c *Console) signIn(w http.ResponseWriter, status int) {
	csrf, err := c.Sessions.Start(w)
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, status, map[string]string{"csrf": csrf})
}

func (c *Console) login(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if readJSON(w, r, &in) && c.verify(w, strings.TrimSpace(in.Username), in.Password) {
		c.signIn(w, http.StatusOK)
	}
}

func (c *Console) verify(w http.ResponseWriter, username, password string) bool {
	c.loginMu.Lock()
	now := time.Now()
	if c.passwordFails.full(now, passwordFailsPerMin) {
		c.loginMu.Unlock()
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, apiError{Error: "too many wrong passwords, try again in a minute", Code: "slow_down"})
		return false
	}
	ok, err := c.State.CheckLogin(username, password)
	if !ok && err == nil {
		c.passwordFails.hit(now)
		time.Sleep(500 * time.Millisecond)
	}
	c.loginMu.Unlock()
	switch {
	case errors.Is(err, ErrNoOwner):
		writeError(w, http.StatusConflict, "create the owner account first")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "cannot read the account")
	case !ok:
		writeError(w, http.StatusUnauthorized, "wrong username or password")
	}
	return ok && err == nil
}

func (c *Console) account(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Username   string    `json:"username"`
		LastExport time.Time `json:"last_export,omitzero"`
	}{c.State.OwnerName(), c.State.LastExport()})
}

func (c *Console) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) || !c.verify(w, c.State.OwnerName(), in.Current) {
		return
	}
	if err := c.State.SetPassword(in.Password, c.Sessions.Key(r)); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Console) rename(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := c.State.Rename(strings.TrimSpace(in.Username)); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Console) logout(w http.ResponseWriter, r *http.Request) {
	if err := c.Sessions.End(w, r); err != nil {
		c.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *Console) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	_, err := c.Engine.send(ctx, c.Engine.PublicURL, "", http.MethodGet, "/readyz", nil, 1<<10)
	writeJSON(w, http.StatusOK, map[string]bool{"ready": err == nil && !c.Secrets.Locked()})
}
