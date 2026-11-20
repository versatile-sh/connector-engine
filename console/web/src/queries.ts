import { queryOptions } from "@tanstack/react-query";
import { api, type Agent, type Approval, type Connector } from "./api";

/** What GET /api/agents answers: the access keys, the endpoint and the longest life a key may have. */
export type KeysData = { agents: Agent[]; mcp_url: string; max_ttl_hours: number };

/** Every installed connector. */
export const connectorsQuery = queryOptions({
  queryKey: ["connectors"],
  queryFn: () => api<{ connectors: Connector[] }>("GET", "/api/connectors"),
  select: (data) => data.connectors,
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
