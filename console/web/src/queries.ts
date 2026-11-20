import { queryOptions } from "@tanstack/react-query";
import { api, setCsrf, type Agent, type Approval, type Connector, type Session, type Skill } from "./api";

/** Who is here: whether the owner exists and is signed in, the endpoint, and the guide's dismissal; it also arms the CSRF token. */
export const sessionQuery = queryOptions({
  queryKey: ["session"],
  queryFn: async () => {
    const s = await api<Session>("GET", "/api/session");
    setCsrf(s.csrf);
    return s;
  },
});

/** What GET /api/agents answers: the access keys, the endpoint and the longest life a key may have. */
export type KeysData = { agents: Agent[]; mcp_url: string; max_ttl_hours: number };

/** Every installed connector. */
export const connectorsQuery = queryOptions({
  queryKey: ["connectors"],
  queryFn: () => api<{ connectors: Connector[] }>("GET", "/api/connectors"),
  select: (data) => data.connectors,
});

/** Every installed skill with each access key's choice. */
export const skillsQuery = queryOptions({
  queryKey: ["skills"],
  queryFn: () => api<{ skills: Skill[] }>("GET", "/api/skills"),
  select: (data) => data.skills,
});

/** The access keys an AI app connects with. */
export const keysQuery = queryOptions({ queryKey: ["agents"], queryFn: () => api<KeysData>("GET", "/api/agents") });

/** Calls held for the owner, polled every few seconds. */
export const approvalsQuery = queryOptions({
  queryKey: ["approvals"],
  queryFn: () => api<{ approvals: Approval[] }>("GET", "/api/approvals"),
  select: (data) => data.approvals,
  refetchInterval: 4_000,
});
