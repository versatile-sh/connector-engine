package table

// Role is a delegated role of §4.2.
type Role string

// Roles of §4.2.
const (
	Operator        Role = "operator"
	TenantAdmin     Role = "tenant_admin"
	WorkspaceAdmin  Role = "workspace_admin"
	AgentConfigurer Role = "agent_configurer"
	Reviewer        Role = "reviewer"
	Approver        Role = "approver"
	Auditor         Role = "auditor"
)

// Known reports whether r is a role of §4.2.
func Known(r string) bool {
	switch Role(r) {
	case Operator, TenantAdmin, WorkspaceAdmin, AgentConfigurer, Reviewer, Approver, Auditor:
		return true
	}
	return false
}
