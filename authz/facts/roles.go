package facts

// Role is a delegated role.
type Role string

// Roles.
const (
	Operator        Role = "operator"
	TenantAdmin     Role = "tenant_admin"
	WorkspaceAdmin  Role = "workspace_admin"
	AgentConfigurer Role = "agent_configurer"
	Reviewer        Role = "reviewer"
	Approver        Role = "approver"
	Auditor         Role = "auditor"
)

// Known reports whether r is a role.
func Known(r string) bool {
	switch Role(r) {
	case Operator, TenantAdmin, WorkspaceAdmin, AgentConfigurer, Reviewer, Approver, Auditor:
		return true
	}
	return false
}
