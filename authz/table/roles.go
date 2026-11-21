package table

import "github.com/versatile-sh/connector-engine/authz/facts"

// Role is a delegated role.
type Role = facts.Role

// Roles.
const (
	Operator        = facts.Operator
	TenantAdmin     = facts.TenantAdmin
	WorkspaceAdmin  = facts.WorkspaceAdmin
	AgentConfigurer = facts.AgentConfigurer
	Reviewer        = facts.Reviewer
	Approver        = facts.Approver
	Auditor         = facts.Auditor
)

// Settings are the installation keys the evaluation reads: permission.destructiveMax and exposure.onTrifecta.
type Settings = facts.Settings
