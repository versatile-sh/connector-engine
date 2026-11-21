import type { CompatStatus } from "../api";
import { Icon, type IconName } from "./Icon";

const GLYPH: Readonly<Record<string, IconName>> = {
  "claude-code": "app-claude-code",
  "codex-cli": "app-codex-cli",
  cursor: "app-cursor",
  "gemini-cli": "app-gemini-cli",
  "vscode-copilot": "app-vscode-copilot",
  "claude-desktop": "app-claude-desktop",
  "chat-mcp": "app-chat-mcp",
  opencode: "app-opencode",
};

const SIZE = { sm: { tile: "size-7 rounded-lg", glyph: "size-4" }, xs: { tile: "size-5 rounded-md text-[0.625rem]", glyph: "size-3" } } as const;

type AppMarkProps = { client: string; name: string; size?: keyof typeof SIZE } & ({ status?: undefined; label?: undefined } | { status: CompatStatus; label: string });

/** One AI app as a small hairline tile with a glyph of the console's icon set, or the first letter of its name for an app without one; given a status it carries a dot (lime works, amber instructions only, muted and struck through does not run) and `label` as its name and tooltip, else it is decoration beside the app's name. */
export function AppMark({ client, name, size = "sm", status, label }: AppMarkProps) {
  const glyph = GLYPH[client];
  const sizes = SIZE[size];
  const quiet = status === "no";
  return (
    <span
      role={status ? "img" : undefined}
      aria-hidden={status ? undefined : true}
      aria-label={label}
      title={label}
      className={`relative grid shrink-0 place-items-center border border-line-strong ${sizes.tile} ${quiet ? "text-muted" : "text-paper"}`}
    >
      {glyph ? <Icon name={glyph} className={sizes.glyph} /> : <span aria-hidden="true" className="font-mono text-xs uppercase">{name.slice(0, 1)}</span>}
      {quiet ? <span aria-hidden="true" className="absolute h-px w-[110%] -rotate-45 bg-muted" /> : null}
      {status ? <span aria-hidden="true" className={`absolute -top-0.5 -right-0.5 size-1.5 rounded-full ring-2 ring-ink-raised ${status === "full" ? "bg-lime" : status === "instructions" ? "bg-amber-400" : "bg-muted"}`} /> : null}
    </span>
  );
}
