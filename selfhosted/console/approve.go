package console

import (
	"errors"
	"net/http"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console/sealed"

	"github.com/versatile-sh/connector-engine/selfhosted/surface"
)

func (c *Console) approvalPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()
	if u, _, ok := c.Sessions.Current(r); ok {
		ctx = WithActor(withUser(ctx, u), u.Person)
	} else if d, _, err := c.checkLink(r.URL.Query().Get("t"), id); err != nil {
		refuseLink(w, err, "sign in first")
		return
	} else {
		ctx = WithActor(ctx, d.Owner)
	}
	err := c.seeApproval(ctx, id)
	var v ApprovalView
	if err == nil {
		v, err = c.Approval(ctx, id)
	}
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
	ctx, channel, note := r.Context(), "console", "console"
	if in.Token != "" {
		device, nonce, err := c.checkLink(in.Token, id)
		if err == nil && !c.surf.links.spend(nonce, time.Now().Add(24*time.Hour)) {
			err = errBadToken
		}
		if err != nil {
			refuseLink(w, err, errBadToken.Error())
			return
		}
		channel, note = "desktop", "desktop:"+device.Name
		ctx = WithActor(ctx, device.Owner)
	} else {
		u, csrf, ok := c.Sessions.Current(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "sign in first")
			return
		}
		if !csrfOK(csrf, r.Header.Get(csrfHeader)) {
			writeError(w, http.StatusForbidden, "missing or wrong CSRF token")
			return
		}
		ctx = WithActor(withUser(ctx, u), u.Person)
		if err := c.seeApproval(ctx, id); err != nil {
			c.fail(w, err)
			return
		}
	}
	v, lost, err := c.DecideChoice(ctx, id, in.Choice, channel, note)
	if notApprover(err) {
		writeJSON(w, http.StatusForbidden, apiError{Error: errNotApprover.Error(), Code: string(port.CodeNotAnApprover)})
		return
	}
	if err != nil {
		c.fail(w, err)
		return
	}
	v.Arguments = nil
	writeJSON(w, http.StatusOK, map[string]any{"outcome": v, "already_decided": lost})
}
