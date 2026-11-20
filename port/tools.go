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

// SkillFileView is a skill file without its bytes, as a skill definition or a load result names it.
type SkillFileView struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// SkillMeta is what a skill function definition carries under SkillMetaKey; its digest pins the whole folder.
type SkillMeta struct {
	Digest         string          `json:"digest"`
	Redistribution Redistribution  `json:"redistribution"`
	HasScripts     bool            `json:"has_scripts"`
	Licence        SkillLicence    `json:"licence"`
	Source         SkillSource     `json:"source"`
	Requires       []string        `json:"requires,omitempty"`
	Files          []SkillFileView `json:"files"`
}

// SkillLoaded is the structured result of skill_load: the file named, its bytes as text or base64, and the folder's files.
type SkillLoaded struct {
	Skill          string          `json:"skill"`
	Path           string          `json:"path"`
	Bytes          int64           `json:"bytes"`
	SHA256         string          `json:"sha256"`
	Encoding       string          `json:"encoding,omitempty"`
	Content        string          `json:"content,omitempty"`
	Redistribution Redistribution  `json:"redistribution"`
	Source         string          `json:"source,omitempty"`
	Files          []SkillFileView `json:"files"`
}

// SkillFunction is the one function of a skill listing, short so that any alias fits the exposed-name limit.
const SkillFunction = "skill"

// ErrorMeta is the _meta key whose value is the code of an error the gateway answered itself, never set on an upstream's result.
const ErrorMeta = "connector-engine/error"
