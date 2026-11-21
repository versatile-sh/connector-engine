import type { ClientCompat, CompatWarning, SkillFacts } from "../api";
import type { MessageKey, Translate } from "../i18n";

/** The AI apps the compatibility rules know, as the server's table names them. */
export const APPS: readonly { id: string; name: string }[] = [
  { id: "claude-code", name: "Claude Code" },
  { id: "codex-cli", name: "Codex CLI" },
  { id: "cursor", name: "Cursor" },
  { id: "gemini-cli", name: "Gemini CLI" },
  { id: "vscode-copilot", name: "VS Code Copilot" },
  { id: "opencode", name: "opencode / oh-my-pi" },
  { id: "claude-desktop", name: "Claude Desktop" },
  { id: "chat-mcp", name: "ChatGPT and other chat-only MCP clients" },
];

const REASON: Record<string, MessageKey> = {
  ok: "compat.reason.ok",
  "runs-bundled-code": "compat.reason.runs",
  "no-shell": "compat.reason.shell",
  "no-files": "compat.reason.files",
  "no-web": "compat.reason.web",
};

const MISSING: Record<string, MessageKey> = { "no-shell": "compat.missing.shell", "no-files": "compat.missing.files", "no-web": "compat.missing.web" };

/** The one line that says why an app has its status, from the reasons the server derived. */
export function reasonText(entry: Pick<ClientCompat, "status" | "reasons">, t: Translate): string {
  if (entry.status === "full") return t("compat.reason.ok");
  const keys = entry.reasons.map((reason) => REASON[reason]).filter((key): key is MessageKey => key !== undefined);
  return keys.length > 0 ? keys.map((key) => t(key)).join(" · ") : t("compat.reason.unknown");
}

/** The warning for an app behind the gateway: what it can still do with the skill, and what it cannot. */
export function warningText(warning: Pick<CompatWarning, "name" | "reasons">, t: Translate): string {
  if (warning.reasons.includes("runs-bundled-code")) return t("compat.warn.scripts", { app: warning.name });
  const missing = warning.reasons.map((reason) => MISSING[reason]).filter((key): key is MessageKey => key !== undefined);
  return t("compat.warn.partial", { app: warning.name, missing: missing.map((key) => t(key)).join(", ") });
}

/** What the grid card flags: `runs` when the skill ships scripts, `shell` when its instructions need a shell, else nothing. */
export function codeTag(skill: Pick<SkillFacts, "has_scripts" | "needs">): "runs" | "shell" | undefined {
  if (skill.has_scripts) return "runs";
  const needsShell = skill.needs.tools.some((tool) => tool === "Bash" || tool === "BashOutput" || tool === "KillShell") || skill.needs.runtimes.length > 0;
  return needsShell ? "shell" : undefined;
}

/** The tags of the Requires row: tools, runtimes, `network` and each MCP server as `mcp:name`. */
export function requireTags(needs: SkillFacts["needs"]): string[] {
  return [...needs.tools, ...needs.runtimes, ...(needs.network ? ["network"] : []), ...needs.mcp.map((name) => `mcp:${name}`)];
}
