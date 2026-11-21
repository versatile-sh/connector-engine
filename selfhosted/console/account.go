package console

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"
)

func (c *Console) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, csrf, ok := c.Sessions.Current(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "sign in first")
			return
		}
		if !safeMethod(r.Method) && !csrfOK(csrf, r.Header.Get(csrfHeader)) {
			writeError(w, http.StatusForbidden, "missing or wrong CSRF token")
			return
		}
		next(w, r.WithContext(WithActor(withUser(r.Context(), u), u.Person)))
	}
}

func (c *Console) session(w http.ResponseWriter, r *http.Request) {
	u, csrf, ok := c.Sessions.Current(r)
	out := map[string]any{"owner_exists": c.State.HasOwner(), "signed_in": ok, "mcp_url": c.MCPURL, "signup_open": c.State.Signup().Enabled && c.State.HasOwner()}
	if c.SetupToken != "" && !c.State.HasOwner() {
		out["setup_token_required"] = true
	}
	if ok {
		out["csrf"] = csrf
		out["user"] = map[string]any{"person": u.Person, "username": u.Username, "role": u.Role}
		out["can"] = u.Capabilities()
		out["people"] = len(c.State.Users())
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
	if !c.setupTokenOK(r) {
		writeJSON(w, http.StatusForbidden, apiError{Error: "the setup token is missing or wrong", Code: "setup_token"})
		return
	}
	var in credentials
	if !readJSON(w, r, &in) {
		return
	}
	u, err := c.State.CreateFirstOwner(strings.TrimSpace(in.Username), in.Password)
	switch {
	case errors.Is(err, ErrOwnerExists):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		c.fail(w, err)
	default:
		if err := c.ReconcileRoles(r.Context()); err != nil {
			go c.untilReady(context.WithoutCancel(r.Context()), "roles", func(ctx context.Context) error { return c.ReconcileRoles(ctx) })
		}
		c.signIn(w, http.StatusCreated, u.Person)
	}
}

func (c *Console) setupTokenOK(r *http.Request) bool {
	if c.SetupToken == "" || c.State.HasOwner() {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(c.SetupToken), []byte(r.Header.Get("X-Setup-Token"))) == 1
}

func (c *Console) signIn(w http.ResponseWriter, status int, person string) {
	csrf, err := c.Sessions.Start(w, person)
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, status, map[string]string{"csrf": csrf})
}

func (c *Console) login(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !readJSON(w, r, &in) {
		return
	}
	if u, ok := c.verify(w, r, strings.TrimSpace(in.Username), in.Password); ok {
		c.signIn(w, http.StatusOK, u.Person)
	}
}

func (c *Console) verify(w http.ResponseWriter, r *http.Request, username, password string) (User, bool) {
	ip := clientIP(r)
	if c.passwordFails.locked(time.Now(), ip, username) {
		w.Header().Set("Retry-After", "60")
		writeJSON(w, http.StatusTooManyRequests, apiError{Error: "too many wrong passwords, try again in a minute", Code: "slow_down"})
		return User{}, false
	}
	u, ok, err := c.State.CheckLogin(username, password)
	if !ok && err == nil {
		c.passwordFails.fail(time.Now(), ip, username)
		time.Sleep(wrongPasswordDelay)
	}
	switch {
	case errors.Is(err, ErrNoOwner):
		writeError(w, http.StatusConflict, "create the owner account first")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "cannot read the account")
	case !ok:
		writeError(w, http.StatusUnauthorized, "wrong username or password")
	}
	return u, ok && err == nil
}

func (c *Console) reauth(w http.ResponseWriter, r *http.Request, password string) bool {
	u, _ := signedInUser(r.Context())
	_, ok := c.verify(w, r, u.Username, password)
	return ok
}

func (c *Console) account(w http.ResponseWriter, r *http.Request) {
	u, _ := signedInUser(r.Context())
	writeJSON(w, http.StatusOK, struct {
		Username   string    `json:"username"`
		LastExport time.Time `json:"last_export,omitzero"`
	}{u.Username, c.State.LastExport()})
}

func (c *Console) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &in) || !c.reauth(w, r, in.Current) {
		return
	}
	u, _ := signedInUser(r.Context())
	if err := c.State.SetPassword(u.Person, in.Password, c.Sessions.Key(r)); err != nil {
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
	u, _ := signedInUser(r.Context())
	if err := c.State.Rename(u.Person, strings.TrimSpace(in.Username)); err != nil {
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
