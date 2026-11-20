// Package authz names the permissions and resources the engine asks an Authorizer about.
package authz

import "strings"

// Actions: one per kind of act; the engine names the permission and the authorization service decides.
const (
	SourceSync          = "source.sync"
	ListingImport       = "listing.import"
	ListingCreateInst   = "listing.create.installation"
	ListingCreatePriv   = "listing.create.private"
	ListingStatusInst   = "listing.status.installation"
	ListingStatusPriv   = "listing.status.private"
	ListingReview       = "listing.review"
	ListingStarter      = "listing.starter"
	OfferPut            = "offer.put"
	TenantCreate        = "tenant.create"
	TenantRemove        = "tenant.remove"
	CatalogRead         = "catalog.read"
	CatalogWrite        = "catalog.write"
	ConnectorRead       = "connector.read"
	ConnectorInstall    = "connector.install"
	ConnectorManage     = "connector.manage"
	FunctionManage      = "function.manage"
	OverlayManage       = "overlay.manage"
	BindingWrite        = "binding.write"
	GrantTenant         = "grant.tenant"
	GrantWorkspace      = "grant.workspace"
	GrantAgent          = "grant.agent"
	GrantUser           = "grant.user"
	PauseClear          = "pause.clear"
	ApprovalDecide      = "approval.decide"
	ApprovalCancel      = "approval.cancel"
	ApprovalRead        = "approval.read"
	CredentialOwn       = "credential.own"
	CredentialWorkspace = "credential.workspace"
	SessionEnd          = "session.end"
	AgentRemove         = "agent.remove"
	PersonRemove        = "person.remove"
	AuditRead           = "audit.read"
	SourceRead          = "source.read"
	ClientRegister      = "client.register"
	KeyRotate           = "key.rotate"
	RoleInstallation    = "role.installation"
	RoleManage          = "role.manage"
)

// Installation is the resource of installation-wide acts.
const Installation = "installation"

// Tenant is the resource of one tenant.
func Tenant(t string) string { return "t/" + t }

// Workspace is the resource of one workspace of a tenant.
func Workspace(t, w string) string { return Tenant(t) + "/w/" + w }

// Agent is the resource of one agent of a workspace.
func Agent(t, w, a string) string { return Workspace(t, w) + "/a/" + a }

// Person is the resource of one person of a tenant.
func Person(t, p string) string { return Tenant(t) + "/p/" + p }

// Covers reports whether a role scope covers a resource: the same string, or a parent of it.
func Covers(scope, resource string) bool {
	return scope == resource || scope == Installation || strings.HasPrefix(resource, scope+"/")
}
