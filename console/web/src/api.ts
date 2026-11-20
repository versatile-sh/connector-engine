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

export type Preview = {
  kind: "mcp" | "listing";
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

export type Agent = { name: string; expires_at: string };

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

export type ApiError = { status: number; message: string };

let csrfToken = "";

export const setCsrf = (token: string | undefined) => {
  csrfToken = token ?? "";
};

const isApiError = (value: unknown): value is ApiError =>
  typeof value === "object" && value !== null && "status" in value && "message" in value;

export const errorText = (error: unknown) => (isApiError(error) ? error.message : String(error));

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
    const failure: ApiError = { status: response.status, message };
    throw failure;
  }
  return data as T;
}

export const iconURL = (raw: string | undefined) => (raw ? `/api/icon?u=${encodeURIComponent(raw)}` : undefined);
