package gateway

import (
	"context"
	"net/http"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
)

type deliveries struct {
	mu  sync.Mutex
	ids []port.ApprovalID
}

func (d *deliveries) add(id port.ApprovalID) {
	d.mu.Lock()
	d.ids = append(d.ids, id)
	d.mu.Unlock()
}

func (d *deliveries) take() []port.ApprovalID {
	d.mu.Lock()
	defer d.mu.Unlock()
	ids := d.ids
	d.ids = nil
	return ids
}

func deliveriesOf(req mcp.Request) *deliveries {
	if extra := req.GetExtra(); extra != nil && extra.TokenInfo != nil {
		d, _ := extra.TokenInfo.Extra[deliveryExtra].(*deliveries)
		return d
	}
	return nil
}

func owed(d *deliveries, out service.Outcome) {
	if d != nil && out.Approval != nil && out.Approval.State.Terminal() && (out.Status == service.OutcomeResult || out.Status == service.OutcomeError) {
		d.add(out.Approval.ID)
	}
}

func (g *Gateway) delivering(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		info := auth.TokenInfoFromContext(r.Context())
		if info == nil || r.Context().Err() != nil {
			return
		}
		d, _ := info.Extra[deliveryExtra].(*deliveries)
		p, _ := info.Extra[principalExtra].(port.Principal)
		if d == nil {
			return
		}
		ctx := context.WithoutCancel(r.Context())
		for _, id := range d.take() {
			if err := g.core.MarkDelivered(ctx, p, id); err != nil {
				g.cfg.Logger.Error("mark delivered", "approval", id, "error", err)
			}
		}
	})
}
