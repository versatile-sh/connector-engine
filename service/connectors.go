package service

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/agents-control/connector-engine/authz"
	"github.com/agents-control/connector-engine/catalog"
	corecatalog "github.com/agents-control/connector-engine/core/catalog"
	"github.com/agents-control/connector-engine/core/grant"
	"github.com/agents-control/connector-engine/core/listing"
	"github.com/agents-control/connector-engine/core/names"
	"github.com/agents-control/connector-engine/core/pinning"
	"github.com/agents-control/connector-engine/port"
)

// InstallInput is one install: the listing, an optional version (newest that exists when empty) and the connector's settings.
type InstallInput struct {
	Workspace       port.WorkspaceID
	Alias           string
	Listing         port.ListingRef
	Version         string
	Config          map[string]string
	CallTimeout     time.Duration
	ApprovalTimeout time.Duration
}

// ConnectorView is a connector with its functions and, when its listing is hidden or quarantined, the reason it is blocked.
type ConnectorView struct {
	Connector port.Connector
	Functions []port.Function
	Blocked   port.Code
}

func canInstall(a corecatalog.Answer) bool {
	ok := a.Visible && a.Tenant.Install && (!a.Workspace.Answered || a.Workspace.Install)
	for _, l := range a.People {
		ok = ok && (!l.Answered || l.Install)
	}
	return ok
}

func wsResource(p port.Principal, w port.WorkspaceID) string {
	return authz.Workspace(string(p.Claims.Tenant), string(w))
}

func (s *Service) connectorOf(ctx context.Context, p port.Principal, id port.ConnectorID, action string) (port.Connector, error) {
	var c port.Connector
	err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) (err error) {
		c, err = tx.GetConnector(id)
		return err
	})
	if err != nil {
		return c, err
	}
	return c, s.authorize(ctx, p, action, wsResource(p, c.Workspace))
}

// InstallConnector installs a visible, installable listing version in a workspace, captures its functions and writes the level (§5.4).
func (s *Service) InstallConnector(ctx context.Context, p port.Principal, in InstallInput) (ConnectorView, error) {
	var out ConnectorView
	tenant := p.Claims.Tenant
	if err := s.authorize(ctx, p, authz.ConnectorInstall, wsResource(p, in.Workspace)); err != nil {
		return out, err
	}
	if err := names.ValidAlias(in.Alias); err != nil {
		return out, err
	}
	var lv port.ListingVersion
	err := s.d.Store.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		v, people, err := s.visibleView(ctx, p, tx, in.Workspace)
		if err != nil {
			return err
		}
		a := v.Resolve(in.Workspace, people, in.Listing)
		version := in.Version
		if version == "" {
			version = a.Version
		}
		status := v.Status(in.Listing, version)
		switch {
		case !a.Visible || !canInstall(a) || !status.Exists():
			return port.Errorf(port.CodeNotFound, "%s not found", in.Listing)
		case status == port.ListingDeprecated:
			return port.Errorf(port.CodeListingDeprecated, "%s@%s is deprecated", in.Listing, version)
		}
		conns, err := tx.ListConnectors(port.ConnectorFilter{Workspace: in.Workspace})
		if err != nil {
			return err
		}
		if slices.ContainsFunc(conns, func(c port.Connector) bool { return c.Alias == in.Alias && c.RemovedAt.IsZero() }) {
			return port.Errorf(port.CodeConflict, "alias %s is taken in %s", in.Alias, in.Workspace)
		}
		lv, err = tx.GetVersion(port.VersionRef{Listing: in.Listing, Version: version})
		return err
	})
	if err != nil {
		return out, err
	}
	if s.d.Verifier != nil {
		if _, err := s.d.Verifier.Verify(ctx, lv.Manifest, nil); err != nil {
			return out, err
		}
	}
	c := port.Connector{ID: port.ConnectorID(s.d.NewID()), Tenant: tenant, Workspace: in.Workspace, Alias: in.Alias, Listing: lv.Ref, Enabled: true,
		Config: in.Config, CallTimeout: in.CallTimeout, ApprovalTimeout: in.ApprovalTimeout}
	fns, meta, err := s.captureFunctions(ctx, c, lv.Manifest)
	if err != nil {
		return out, err
	}
	c.InstructionsHash = pinning.InstructionsHash(meta.Instructions)
	c.LastRelistAt = s.now()
	level := listing.InstallLevel(lv.Manifest.TrustLevel)
	err = s.write(ctx, port.Scope{Tenant: tenant}, "InstallConnector", func(tx port.Tx) error {
		saved, err := tx.PutConnector(c, 0)
		if err != nil {
			return err
		}
		out.Connector = saved
		for _, f := range fns {
			f = pinning.Captured(f, lv.Manifest.TrustLevel)
			if err := tx.PutFunction(f); err != nil {
				return err
			}
			out.Functions = append(out.Functions, f)
			if f.State == port.FunctionQuarantined {
				if err := s.emit(tx, tenant, string(c.ID), evFunctionQuarantined, catalog.FunctionQuarantined{Connector: string(c.ID), Function: f.Name, Reason: "unreviewed", NewHash: string(f.DefinitionHash)}); err != nil {
					return err
				}
			}
		}
		base := port.Grant{Tenant: tenant, ScopeKind: port.ScopeWorkspace, ScopeID: string(in.Workspace), Connector: c.ID, CreatedBy: actor(p), Origin: port.OriginLevel}
		_, rows := grant.SetLevel(nil, base, level, s.cfg.LevelModes)
		for _, g := range rows {
			if err := s.putGrant(tx, s.newGrant(g), actor(p)); err != nil {
				return err
			}
		}
		return s.record(tx, tenant, actor(p), string(c.ID), evConnectorInstalled,
			catalog.ConnectorInstalled{Connector: string(c.ID), Listing: in.Listing.String(), Version: lv.Ref.Version, Level: string(level)})
	})
	return out, err
}

// UpdateResult is what an update or a re-list found: the changes, the functions it quarantined and the tenant rows left unmatched.
type UpdateResult struct {
	Connector           port.Connector
	Changes             []pinning.Change
	Quarantined         []string
	TenantRowsUnmatched []string
}

// UpdateConnector pins a newer listing version by an explicit act; changed and new functions are quarantined (§5.4, §9).
func (s *Service) UpdateConnector(ctx context.Context, p port.Principal, id port.ConnectorID, version string) (UpdateResult, error) {
	c, err := s.connectorOf(ctx, p, id, authz.ConnectorManage)
	if err != nil {
		return UpdateResult{}, err
	}
	if version != "" && version != c.Listing.Version {
		var status port.ListingStatus
		var ok bool
		err := s.d.Store.Read(ctx, port.Scope{Tenant: c.Tenant}, func(tx port.Tx) error {
			v, people, err := s.visibleView(ctx, p, tx, c.Workspace)
			if err != nil {
				return err
			}
			a := v.Resolve(c.Workspace, people, c.Listing.Listing)
			status, ok = v.Status(c.Listing.Listing, version), a.Visible && canInstall(a)
			return nil
		})
		switch {
		case err != nil:
			return UpdateResult{}, err
		case !ok || !status.Exists():
			return UpdateResult{}, port.Errorf(port.CodeNotFound, "%s@%s not found", c.Listing.Listing, version)
		case status == port.ListingDeprecated:
			return UpdateResult{}, port.Errorf(port.CodeListingDeprecated, "%s@%s is deprecated", c.Listing.Listing, version)
		}
	}
	return s.reconcile(ctx, c.Tenant, id, version, actor(p))
}

// Relist captures the connector's pinned version again and quarantines drift; the worker and A8 call it (§9).
func (s *Service) Relist(ctx context.Context, tenant port.TenantID, id port.ConnectorID) (UpdateResult, error) {
	return s.reconcile(ctx, tenant, id, "", "worker")
}

func (s *Service) reconcile(ctx context.Context, tenant port.TenantID, id port.ConnectorID, version, by string) (UpdateResult, error) {
	var res UpdateResult
	var c port.Connector
	var pinned []port.Function
	var m port.Manifest
	err := s.d.Store.Read(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		var err error
		if c, err = tx.GetConnector(id); err != nil {
			return err
		}
		if pinned, err = tx.ListFunctions(id); err != nil {
			return err
		}
		ref := c.Listing
		if version != "" {
			ref.Version = version
		}
		lv, err := tx.GetVersion(ref)
		m = lv.Manifest
		return err
	})
	if err != nil {
		return res, err
	}
	explicit := version != ""
	next := c
	next.Listing.Version = m.Version
	captured, meta, err := s.captureFunctions(ctx, next, m)
	if err != nil {
		return res, err
	}
	merged, changes := pinning.Reconcile(pinned, captured)
	merged, changes = s.refreshFunctions(merged, changes, captured, pinned, m)
	err = s.write(ctx, port.Scope{Tenant: tenant}, "Reconcile", func(tx port.Tx) error {
		cur, err := tx.GetConnector(id)
		if err != nil {
			return err
		}
		if cur.Revision != c.Revision {
			return port.Errorf(port.CodeRevisionMismatch, "connector %s changed during the capture", id)
		}
		if !cur.RemovedAt.IsZero() {
			return port.Errorf(port.CodeNotFound, "connector %s was removed", id)
		}
		byName := map[string]port.Function{}
		for _, f := range merged {
			byName[f.Name] = f
		}
		for _, ch := range changes {
			f := byName[ch.Function]
			if err := s.applyChange(tx, cur, &f, ch, m, explicit, by); err != nil {
				return err
			}
			byName[ch.Function] = f
		}
		for _, f := range merged {
			if err := tx.PutFunction(byName[f.Name]); err != nil {
				return err
			}
			if byName[f.Name].State == port.FunctionQuarantined && slices.ContainsFunc(changes, func(ch pinning.Change) bool { return ch.Function == f.Name }) {
				res.Quarantined = append(res.Quarantined, f.Name)
			}
		}
		res.Changes = changes
		return s.finishReconcile(tx, &cur, &res, m, meta, explicit, by, byName)
	})
	return res, err
}

func (s *Service) finishReconcile(tx port.Tx, cur *port.Connector, res *UpdateResult, m port.Manifest, meta port.ListMeta, explicit bool, by string, fns map[string]port.Function) error {
	from := cur.Listing.Version
	cur.Listing.Version, cur.LastRelistAt = m.Version, s.now()
	if h := pinning.InstructionsHash(meta.Instructions); h != cur.InstructionsHash {
		if err := s.emit(tx, cur.Tenant, string(cur.ID), evInstructions, catalog.InstructionsChanged{Connector: string(cur.ID), OldHash: string(cur.InstructionsHash), NewHash: string(h)}); err != nil {
			return err
		}
		cur.Config = withPending(cur.Config, string(h))
	}
	saved, err := tx.PutConnector(*cur, cur.Revision)
	if err != nil {
		return err
	}
	res.Connector = saved
	if !explicit {
		return nil
	}
	rows, err := tx.ListGrants(port.GrantFilter{ScopeKind: port.ScopeTenant, Listing: &saved.Listing.Listing})
	if err != nil {
		return err
	}
	for _, g := range rows {
		if n, ok := strings.CutPrefix(g.Selector, port.SelectorFunction); ok {
			if f, present := fns[n]; !present || f.State == port.FunctionRemoved {
				res.TenantRowsUnmatched = append(res.TenantRowsUnmatched, n)
			}
		}
	}
	slices.Sort(res.TenantRowsUnmatched)
	return s.record(tx, cur.Tenant, by, string(cur.ID), evConnectorUpdated, catalog.ConnectorUpdated{Connector: string(cur.ID), FromVersion: from, ToVersion: m.Version,
		Quarantined: nonNil(res.Quarantined), TenantRowsUnmatched: nonNil(res.TenantRowsUnmatched)})
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

const pendingInstructions = "_instructions_pending"

func withPending(cfg map[string]string, hash string) map[string]string {
	out := map[string]string{pendingInstructions: hash}
	for k, v := range cfg {
		out[k] = v
	}
	return out
}

func (s *Service) refreshFunctions(merged []port.Function, changes []pinning.Change, captured, pinned []port.Function, m port.Manifest) ([]port.Function, []pinning.Change) {
	cap := map[string]port.Function{}
	for _, f := range captured {
		cap[f.Name] = f
	}
	old := map[string]port.Function{}
	for _, f := range pinned {
		old[f.Name] = f
	}
	for i, f := range merged {
		c, present := cap[f.Name]
		if !present || f.State == port.FunctionRemoved {
			continue
		}
		o, existed := old[f.Name]
		changed := slices.ContainsFunc(changes, func(ch pinning.Change) bool { return ch.Function == f.Name })
		if existed && !changed && o.DefinitionHash == c.DefinitionHash && o.UIResourceHash != c.UIResourceHash && o.State != port.FunctionRemoved {
			q, _ := pinning.Changed(f, f.Definition, f.DefinitionHash)
			f = q
			changes = append(changes, pinning.Change{Function: f.Name, Kind: pinning.ChangeChanged, OldHash: o.DefinitionHash, NewHash: c.DefinitionHash,
				Effects: pinning.Effects{DropSessionRows: true, Cancel: port.ReasonQuarantined}})
			changed = true
		}
		f.UIResourceHash, f.Visibility, f.OpenWorld = c.UIResourceHash, c.Visibility, c.OpenWorld
		f.ReadsPrivate, f.ReturnsThirdParty = c.ReadsPrivate, c.ReturnsThirdParty
		if changed || !existed {
			f.ClassSource = ""
			f.Class, f.ClassSource = classOf(f, m)
		} else if f.ClassSource != port.ClassSourceAdmin {
			class, source := classOf(f, m)
			if f.Class.Raises(class) {
				var eff pinning.Effects
				f, eff = pinning.Reclassify(f, class, source)
				changes = append(changes, pinning.Change{Function: f.Name, Kind: "reclassified", OldHash: f.DefinitionHash, NewHash: f.DefinitionHash, Effects: eff})
			} else {
				f.Class, f.ClassSource = class, source
			}
		}
		merged[i] = f
	}
	return merged, changes
}

func (s *Service) applyChange(tx port.Tx, c port.Connector, f *port.Function, ch pinning.Change, m port.Manifest, explicit bool, by string) error {
	fn := ch.Function
	if ch.Effects.DropSessionRows {
		if err := s.dropSessionRows(tx, c.ID, fn, by); err != nil {
			return err
		}
	}
	if ch.Effects.Cancel != "" {
		if _, err := s.cancelPending(tx, port.ApprovalFilter{Connector: c.ID}, ch.Effects.Cancel, func(a port.Approval) bool { return a.Function == fn }); err != nil {
			return err
		}
	}
	switch ch.Kind {
	case pinning.ChangeRemoved:
		return s.emit(tx, c.Tenant, string(c.ID), evFunctionRemoved, catalog.FunctionRemoved{Connector: string(c.ID), Function: fn})
	case pinning.ChangeChanged, pinning.ChangeNew:
		if err := s.emitQuarantine(tx, c, f, ch); err != nil {
			return err
		}
		if explicit && ch.Kind == pinning.ChangeChanged && pinning.AutoAccept(*f, m.Review, s.cfg.AutoAcceptReviewed) {
			acc, err := pinning.Accept(*f, f.DefinitionHash, "review", s.now())
			if err != nil {
				return err
			}
			*f = acc
			return s.record(tx, c.Tenant, "review", string(c.ID), evFunctionAccepted, catalog.FunctionAccepted{Connector: string(c.ID), Function: fn, Hash: string(f.DefinitionHash),
				Class: string(f.Class), ClassSource: string(f.ClassSource), By: "review"})
		}
	}
	return nil
}

func (s *Service) emitQuarantine(tx port.Tx, c port.Connector, f *port.Function, ch pinning.Change) error {
	ev := catalog.FunctionQuarantined{Connector: string(c.ID), Function: ch.Function, Reason: string(ch.Kind), OldHash: string(ch.OldHash), NewHash: string(ch.NewHash)}
	if ch.Kind == pinning.ChangeChanged {
		old, err := tx.GetFunction(c.ID, ch.Function)
		if err == nil {
			ev.ChangedFields, _ = pinning.ChangedFields(old.Definition, f.Definition)
		}
	}
	return s.record(tx, c.Tenant, "system", string(c.ID), evFunctionQuarantined, ev)
}

// SetConnectorEnabled enables or disables a connector; disabling cancels its pending approvals (§7.1 step 1, A7).
func (s *Service) SetConnectorEnabled(ctx context.Context, p port.Principal, id port.ConnectorID, enabled bool) (port.Connector, error) {
	c, err := s.connectorOf(ctx, p, id, authz.ConnectorManage)
	if err != nil {
		return c, err
	}
	err = s.write(ctx, port.Scope{Tenant: c.Tenant}, "SetConnectorEnabled", func(tx port.Tx) error {
		if c, err = tx.GetConnector(id); err != nil {
			return err
		}
		c.Enabled = enabled
		if c, err = tx.PutConnector(c, c.Revision); err != nil {
			return err
		}
		if !enabled {
			if _, err := s.cancelPending(tx, port.ApprovalFilter{Connector: id}, port.ReasonConnectorDisabled, nil); err != nil {
				return err
			}
		}
		return s.record(tx, c.Tenant, actor(p), string(id), evEnabledChanged, catalog.EnabledChanged{Connector: string(id), Enabled: enabled, By: actor(p)})
	})
	return c, err
}

// RemoveConnector revokes the bindings, grants and credentials of a connector and cancels its pending approvals; rows stay for audit (§5.4).
func (s *Service) RemoveConnector(ctx context.Context, p port.Principal, id port.ConnectorID) error {
	c, err := s.connectorOf(ctx, p, id, authz.ConnectorManage)
	if err != nil {
		return err
	}
	err = s.write(ctx, port.Scope{Tenant: c.Tenant}, "RemoveConnector", func(tx port.Tx) error {
		if c, err = tx.GetConnector(id); err != nil {
			return err
		}
		c.Enabled, c.RemovedAt = false, s.now()
		if _, err = tx.PutConnector(c, c.Revision); err != nil {
			return err
		}
		if _, err := s.cancelPending(tx, port.ApprovalFilter{Connector: id}, port.ReasonConnectorDisabled, nil); err != nil {
			return err
		}
		binds, err := tx.ListBindings(port.BindingFilter{Connector: id})
		if err != nil {
			return err
		}
		for _, b := range binds {
			if err := s.removeBinding(tx, b, actor(p)); err != nil {
				return err
			}
		}
		grants, err := tx.ListGrants(port.GrantFilter{Connector: id})
		if err != nil {
			return err
		}
		for _, g := range grants {
			if err := s.revokeGrant(tx, g, actor(p), "connector_removed"); err != nil {
				return err
			}
		}
		creds, err := tx.ListCredentials(port.CredentialFilter{Connector: id})
		if err != nil {
			return err
		}
		for _, cr := range creds {
			if err := s.revokeCredentialRow(tx, cr, actor(p)); err != nil {
				return err
			}
		}
		return s.record(tx, c.Tenant, actor(p), string(id), evConnectorRemoved, catalog.ConnectorRef{Connector: string(id)})
	})
	if err == nil {
		s.stopConnector(ctx, c)
	}
	return err
}

// SetConnectorLevel replaces the level rows of the workspace layer of a connector (§6.4 G3) and returns the rows it wrote.
func (s *Service) SetConnectorLevel(ctx context.Context, p port.Principal, id port.ConnectorID, level port.Level) ([]port.Grant, error) {
	c, err := s.connectorOf(ctx, p, id, authz.GrantWorkspace)
	if err != nil {
		return nil, err
	}
	var written []port.Grant
	err = s.write(ctx, port.Scope{Tenant: c.Tenant}, "SetConnectorLevel", func(tx port.Tx) error {
		layer, err := tx.ListGrants(port.GrantFilter{ScopeKind: port.ScopeWorkspace, ScopeID: string(c.Workspace), Connector: id})
		if err != nil {
			return err
		}
		base := port.Grant{Tenant: c.Tenant, ScopeKind: port.ScopeWorkspace, ScopeID: string(c.Workspace), Connector: id, CreatedBy: actor(p), Origin: port.OriginLevel}
		return s.applyLevel(tx, layer, base, level, actor(p), &written)
	})
	return written, err
}

func (s *Service) applyLevel(tx port.Tx, layer []port.Grant, base port.Grant, level port.Level, by string, written *[]port.Grant) error {
	removed, added := grant.SetLevel(layer, base, level, s.cfg.LevelModes)
	for _, g := range removed {
		if err := s.revokeGrant(tx, g, by, "level_changed"); err != nil {
			return err
		}
	}
	for _, g := range added {
		g = s.newGrant(g)
		if err := s.putGrant(tx, g, by); err != nil {
			return err
		}
		*written = append(*written, g)
	}
	return nil
}

// GetConnector returns a connector with its functions; a person who may not see its listing gets not found (§5.5).
func (s *Service) GetConnector(ctx context.Context, p port.Principal, id port.ConnectorID) (ConnectorView, error) {
	var out ConnectorView
	vs, err := s.ListConnectors(ctx, p, "")
	if err != nil {
		return out, err
	}
	i := slices.IndexFunc(vs, func(v ConnectorView) bool { return v.Connector.ID == id })
	if i < 0 {
		return out, port.Errorf(port.CodeNotFound, "connector %s not found", id)
	}
	return vs[i], nil
}

// ListConnectors returns the connectors of a workspace (all when empty); admins see every one with the blocked mark, others only those whose listing they see.
func (s *Service) ListConnectors(ctx context.Context, p port.Principal, workspace port.WorkspaceID) ([]ConnectorView, error) {
	if err := s.authorize(ctx, p, authz.ConnectorRead, s.tenantResource(p)); err != nil {
		return nil, err
	}
	var out []ConnectorView
	err := s.d.Store.Read(ctx, port.Scope{Tenant: p.Claims.Tenant}, func(tx port.Tx) error {
		conns, err := tx.ListConnectors(port.ConnectorFilter{Workspace: workspace})
		if err != nil {
			return err
		}
		blocked, err := s.blockedReasons(tx, p.Claims.Tenant, nil)
		if err != nil {
			return err
		}
		for _, c := range conns {
			admin := s.authorize(ctx, p, authz.ConnectorManage, wsResource(p, c.Workspace)) == nil
			if !admin {
				v, people, err := s.visibleView(ctx, p, tx, c.Workspace)
				if err != nil {
					return err
				}
				if !v.Resolve(c.Workspace, people, c.Listing.Listing).Visible {
					continue
				}
			}
			fns, err := tx.ListFunctions(c.ID)
			if err != nil {
				return err
			}
			out = append(out, ConnectorView{Connector: c, Functions: fns, Blocked: blocked[c.ID]})
		}
		return nil
	})
	return out, err
}
