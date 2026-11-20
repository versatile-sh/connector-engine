/** The owner's choice for one function: hidden, allowed at once, or held for approval on every call. */
export type Access = "off" | "on" | "ask";

export const ACCESS: readonly Access[] = ["off", "on", "ask"];

/** What a function does by its own description; it is shown, never chosen. */
export type Kind = "read" | "write" | "destructive";

export type Session = {
  owner_exists: boolean;
  signed_in: boolean;
  csrf?: string;
  mcp_url: string;
  onboarding_dismissed?: boolean;
  key_protection?: KeyProtection;
};

/** How the root keys of the engine and the console rest: waiting for the host's unlock key, a plaintext file, or wrapped by a key in the OS keychain. */
export type KeyProtection = "" | "locked" | "plaintext" | "wrapped";

export type FunctionView = {
  name: string;
  title?: string;
  description?: string;
  class: string;
  kind: Kind;
  state: string;
  access: Access;
};

/** What a connector item is: the server's instructions, a prompt or a resource. */
export type ItemKind = "instructions" | "prompt" | "resource";

/** One prompt, resource or the server instructions, with the text read before choosing; `access` is the proposal in a preview and the choice once installed. */
export type Item = {
  name: string;
  kind: ItemKind;
  label: string;
  title?: string;
  description?: string;
  preview: string;
  template?: boolean;
  state?: "active" | "quarantined" | "disabled";
  access: Access;
};

export type Connector = {
  id: string;
  alias: string;
  enabled: boolean;
  blocked?: string;
  listing: string;
  version: string;
  title: string;
  description?: string;
  icon?: string;
  publisher?: string;
  remote_url?: string;
  auth?: string[];
  credentials?: string[];
  functions: FunctionView[] | null;
  items?: Item[] | null;
};

export type ProbedTool = {
  name: string;
  title?: string;
  description?: string;
  read_only: boolean;
  destructive: boolean;
  kind: Kind;
  proposed: Access;
};

/** Where a skill folder was read, pinned to a full commit. */
export type SkillSource = { repository: string; path: string; commit: string; url?: string };

/** The licence a catalog states for a skill. */
export type SkillLicence = { spdx?: string; from?: string; redistributable: boolean };

/** What a skill is before or after it is imported: provenance, licence, scripts and files. */
export type SkillFacts = {
  name: string;
  category?: string;
  source: SkillSource;
  licence: SkillLicence;
  redistribution: "bundled" | "metadata_only";
  has_scripts: boolean;
  digest?: string;
  files: { path: string; bytes: number }[];
  keys: string[];
};

/** One access key's choice on one skill. */
export type KeyAccess = { agent: string; access: Access };

/** One installed skill with every access key's choice. */
export type Skill = SkillFacts & {
  id: string;
  alias: string;
  title: string;
  description?: string;
  publisher?: string;
  version: string;
  state: string;
  unbound?: string[];
  access: KeyAccess[];
};

export type Preview = {
  kind: "mcp" | "listing" | "skill";
  skill?: SkillFacts;
  url: string;
  host: string;
  name: string;
  title: string;
  description?: string;
  icon?: string;
  publisher?: string;
  version: string;
  remote_url?: string;
  website_url?: string;
  protocol?: string;
  auth?: string[];
  tools: ProbedTool[];
  items?: Item[] | null;
  warning?: string;
  host_loopback?: boolean;
};

export type ImportSource = { mcp?: string; listing?: string; local?: boolean; header_secret?: boolean };

export type Agent = { name: string; expires_at: string; via_browser?: boolean; new: boolean };

export type AgentToken = { agent: string; token: string; expires_at: string };

export type Approval = {
  id: string;
  agent: string;
  session: string;
  connector: string;
  function: string;
  class?: string;
  state: string;
  created_at: string;
  expires_at: string;
  mode?: string;
};

/** What a call came to: answered, failed, refused as unknown, held for approval, or still running. */
export type CallOutcome = "ok" | "error" | "denied" | "pending" | "running";

export type ActivityCall = {
  id: string;
  at: string;
  agent: string;
  connector?: string;
  alias?: string;
  function: string;
  skill?: string;
  path?: string;
  class?: string;
  outcome: CallOutcome;
  code?: string;
  duration_ms?: number;
  approval?: string;
  approval_state?: string;
  captured: boolean;
};

export type ActivityDetail = ActivityCall & {
  tool?: string;
  session?: string;
  mcp_session?: string;
  rpc_id?: unknown;
  detail?: string;
  kept_arguments: boolean;
  arguments?: string;
  arguments_cut?: boolean;
  kept_result: boolean;
  result?: string;
  result_cut?: boolean;
  result_bytes?: number;
  engine_call?: string;
  call_state?: string;
  finished_at?: string;
};

export type HourCount = { start: string; ok: number; error: number; denied: number; pending: number; running: number };

export type ConnectorCount = { connector?: string; alias?: string; skill?: boolean; total: number; failed: number };

export type ActivityData = {
  from: string;
  to: string;
  calls: ActivityCall[];
  total: number;
  hours: HourCount[];
  connectors: ConnectorCount[];
  agents: string[];
};

export type ActivitySettings = { record_arguments: boolean; record_results: boolean; retention_days: number; cleared_at?: string };

export type ApiError = { status: number; message: string; code?: string | undefined };

let csrfToken = "";

export const setCsrf = (token: string | undefined) => {
  csrfToken = token ?? "";
};

const isApiError = (value: unknown): value is ApiError =>
  typeof value === "object" && value !== null && "status" in value && "message" in value;

export const errorText = (error: unknown) => (isApiError(error) ? error.message : String(error));

/** The machine code the console put on a refusal, such as `wrong_password`, when it gave one. */
export const errorCode = (error: unknown) => (isApiError(error) ? error.code : undefined);

/** The HTTP status of a refusal, when the error is one. */
export const errorStatus = (error: unknown) => (isApiError(error) ? error.status : undefined);

const parse = (text: string): unknown => (text ? JSON.parse(text) : undefined);

async function send(method: string, path: string, body?: unknown): Promise<Response> {
  const headers: Record<string, string> = {};
  const form = body instanceof FormData;
  if (body !== undefined && !form) headers["Content-Type"] = "application/json";
  if (method !== "GET" && csrfToken) headers["X-CSRF-Token"] = csrfToken;
  const response = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : form ? body : JSON.stringify(body),
    credentials: "same-origin",
  });
  if (response.ok) return response;
  const data = parse(await response.text());
  const message =
    typeof data === "object" && data !== null && "error" in data && typeof data.error === "string"
      ? data.error
      : `${response.status} ${response.statusText}`;
  const code = typeof data === "object" && data !== null && "code" in data && typeof data.code === "string" ? data.code : undefined;
  const failure: ApiError = { status: response.status, message, code };
  throw failure;
}

/** Calls the console's API with a JSON body, or a form when the body is `FormData`, and reads the JSON answer. */
export async function api<T>(method: string, path: string, body?: unknown): Promise<T> {
  return parse(await (await send(method, path, body)).text()) as T;
}

/** The file name a `Content-Disposition: attachment` header gives, if it gives one. */
export const attachmentName = (header: string | null) => header?.match(/filename="?([^";]+)"?/)?.[1];

/** A file the console answered with: its bytes and the name it gave. */
export type Download = { blob: Blob; name: string | undefined };

/** Calls the console's API and reads the answer as a file. */
export async function apiFile(method: string, path: string, body?: unknown): Promise<Download> {
  const response = await send(method, path, body);
  return { blob: await response.blob(), name: attachmentName(response.headers.get("Content-Disposition")) };
}

export const iconURL = (raw: string | undefined) => (raw ? `/api/icon?u=${encodeURIComponent(raw)}` : undefined);

/** An answer to a held call: allow it once, for the app's session, from now on, or deny it. */
export type ApprovalChoice = "once" | "session" | "always" | "deny";

/** One held call as GET /api/approvals/{id} shows it, arguments included, to the local page only. */
export type ApprovalView = {
  id: string;
  state: "pending" | "approved" | "denied" | "expired" | "cancelled" | (string & {});
  mode: "ask" | "ask_once";
  connector: { id: string; alias: string; title: string; icon?: string };
  function: string;
  function_title?: string;
  kind: Kind;
  skill?: boolean;
  app: string;
  session: string;
  created_at: string;
  expires_at: string;
  arguments?: unknown;
  may_have_run: boolean;
  reaches_outside: boolean;
  choices: ApprovalChoice[];
  decided_by?: string;
  decided_at?: string;
  channel?: string;
};

/** What POST /api/approvals/{id}/decide answers: the record now, and whether someone else had decided it first. */
export type Decision = { outcome: ApprovalView; already_decided: boolean };

/** Where a notification can go: chat bots that take clicks, and notify-only pushes and webhooks. */
export type ChannelKind = "discord" | "slack" | "telegram" | "ntfy" | "discord_webhook" | "slack_webhook";

export type ChannelView = {
  id: string;
  kind: ChannelKind;
  label: string;
  interactive: boolean;
  owner?: string;
  secret_hint?: string;
  created_at: string;
  status: "connected" | "connecting" | "error" | "notify_only";
  error?: string;
};

export type DeviceView = { id: string; name: string; created_at: string; last_seen?: string };

/** What GET /api/notifications answers; it never carries a secret. */
export type NotificationsData = { channels: ChannelView[]; devices: DeviceView[]; apps: { name: string; elicit: boolean }[] };

/** A computer just paired for desktop popups: its key, shown once, and the command that uses it. */
export type PairedDevice = { device: DeviceView; key: string; command: string };

/** What GET /api/account answers; `last_export` is the zero time when there was none. */
export type Account = { username: string; last_export?: string };

/** A full backup carries every credential; a setup carries none and can be shared. */
export type BackupKind = "full" | "setup";

/** One connector of a setup: where it comes from, each choice, and whether it is installed here already. */
export type SetupConnector = { name: string; mcp?: string; listing?: string; transport?: string; functions: Record<string, Access> | null; items: Record<string, Access> | null; exists: boolean };

/** One skill of a setup, and whether it is installed here already. */
export type SetupSkill = { name: string; listing: string; digest?: string; access: Access; exists: boolean };

/** What POST /api/backup/inspect answers about a file before anything is imported. */
export type BackupSummary = {
  kind: BackupKind;
  build?: string;
  created_at?: string;
  counts: { connectors?: number; skills?: number; keys?: number; oauth_clients?: number } | null;
  replaces: boolean;
  setup?: { connectors: SetupConnector[] | null; skills: SetupSkill[] | null; notifications: string[] | null; stripped: string[] | null };
};

/** What importing a setup did: added, kept because installed, failed with why, and the connectors to sign in to. */
export type SetupResult = { added: string[] | null; kept: string[] | null; failed: Record<string, string> | null; sign_ins: string[] | null };
