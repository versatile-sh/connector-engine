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
};

export type FunctionView = {
  name: string;
  title?: string;
  description?: string;
  class: string;
  kind: Kind;
  state: string;
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
  warning?: string;
};

export type ImportSource = { mcp?: string; listing?: string; local?: boolean; header_secret?: boolean };

export type Agent = { name: string; expires_at: string; via_browser?: boolean; new: boolean };

export type AgentToken = { agent: string; token: string; expires_at: string; mcp_url: string };

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

export async function api<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (method !== "GET" && csrfToken) headers["X-CSRF-Token"] = csrfToken;
  const response = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: "same-origin",
  });
  const text = await response.text();
  const data: unknown = text ? JSON.parse(text) : undefined;
  if (!response.ok) {
    const message =
      typeof data === "object" && data !== null && "error" in data && typeof data.error === "string"
        ? data.error
        : `${response.status} ${response.statusText}`;
    const code = typeof data === "object" && data !== null && "code" in data && typeof data.code === "string" ? data.code : undefined;
    const failure: ApiError = { status: response.status, message, code };
    throw failure;
  }
  return data as T;
}

export const iconURL = (raw: string | undefined) => (raw ? `/api/icon?u=${encodeURIComponent(raw)}` : undefined);
