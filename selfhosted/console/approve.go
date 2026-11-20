package console

import (
	"errors"
	"net/http"
	"time"

	"github.com/versatile-sh/connector-engine/selfhosted/console/sealed"

	"github.com/versatile-sh/connector-engine/selfhosted/surface"
)

func (c *Console) approvalPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := c.Sessions.Get(r); !ok {
		if _, _, err := c.checkLink(r.URL.Query().Get("t"), id); err != nil {
			refuseLink(w, err, "sign in first")
			return
		}
	}
	v, err := c.Approval(r.Context(), id)
	if err != nil {
		c.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func refuseLink(w http.ResponseWriter, err error, refused string) {
	if errors.Is(err, sealed.ErrLocked) {
		writeJSON(w, http.StatusServiceUnavailable, apiError{Error: gatewayLocked, Code: "gateway_locked"})
		return
	}
	writeError(w, http.StatusUnauthorized, refused)
}

func (c *Console) deviceName(id string) string {
	for _, d := range c.State.Devices() {
		if d.ID == id {
			return d.Name
		}
	}
	return id
}

func (c *Console) decideChoice(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Choice surface.Choice `json:"choice"`
		Token  string         `json:"token"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	id := r.PathValue("id")
	if !in.Choice.Valid() {
		writeError(w, http.StatusBadRequest, "the choice is once, session, always or deny")
		return
	}
	channel, note := "console", "console"
	if in.Token != "" {
		device, nonce, err := c.checkLink(in.Token, id)
		if err == nil && !c.surf.links.spend(nonce, time.Now().Add(24*time.Hour)) {
			err = errBadToken
		}
		if err != nil {
			refuseLink(w, err, errBadToken.Error())
			return
		}
		channel, note = "desktop", "desktop:"+c.deviceName(device)
	} else {
		csrf, ok := c.Sessions.Get(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "sign in first")
			return
		}
		if !csrfOK(csrf, r.Header.Get(csrfHeader)) {
			writeError(w, http.StatusForbidden, "missing or wrong CSRF token")
			return
		}
	}
	v, lost, err := c.DecideChoice(r.Context(), id, in.Choice, channel, note)
	if err != nil {
		c.fail(w, err)
		return
	}
	v.Arguments = nil
	writeJSON(w, http.StatusOK, map[string]any{"outcome": v, "already_decided": lost})
}
