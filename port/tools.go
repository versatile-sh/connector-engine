package port

// ToolFind is the gateway meta-tool that searches the functions granted to the agent.
const ToolFind = "find"

// ToolCall is the gateway meta-tool that calls a function find returned, named in its arguments.
const ToolCall = "call"

// ToolApprovalStatus is the gateway meta-tool that answers the state or the result of a call waiting for approval.
const ToolApprovalStatus = "approval_status"

// ToolApprovalCancel is the gateway meta-tool that withdraws a call waiting for approval.
const ToolApprovalCancel = "approval_cancel"

// ToolSkillList is the gateway meta-tool that lists the skills granted to the agent.
const ToolSkillList = "skill_list"

// ToolSkillLoad is the gateway meta-tool that returns a skill's SKILL.md or one of its files.
const ToolSkillLoad = "skill_load"

// SkillLoad is the arguments of skill_load: the skill skill_list names and a file of its folder, SKILL.md when empty.
type SkillLoad struct {
	Skill string `json:"skill"`
	Path  string `json:"path"`
}

// SkillFunction is the one function of a skill listing, short so that any alias fits the exposed-name limit.
const SkillFunction = "skill"

// ErrorMeta is the _meta key whose value is the code of an error the gateway answered itself, never set on an upstream's result.
const ErrorMeta = "connector-engine/error"
