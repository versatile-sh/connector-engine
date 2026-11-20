import { queryOptions } from "@tanstack/react-query";
import { apiQueryOf, type ActivityFilter } from "./activity/activity";
import { api, setCsrf, type ActivityData, type ActivityDetail, type ActivitySettings, type Agent, type Approval, type ApprovalView, type Connector, type NotificationsData, type Session, type Skill } from "./api";

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

/** The key every activity page shares, whatever its filter. */
export const activityKey = ["activity"] as const;

/** The activity page for a filter, its range ending at each fetch, refreshed while it is open. */
export const activityQuery = (filter: ActivityFilter) =>
  queryOptions({
    queryKey: [...activityKey, filter],
    queryFn: () => api<ActivityData>("GET", `/api/activity?${apiQueryOf(filter, new Date())}`),
    refetchInterval: 10_000,
    placeholderData: (previous) => previous,
  });

/** One call in full: its arguments, its result excerpt and the engine's record. */
export const activityCallQuery = (id: string) =>
  queryOptions({ queryKey: ["activity-call", id], queryFn: () => api<ActivityDetail>("GET", `/api/activity/${encodeURIComponent(id)}`) });

/** What the console keeps of each call. */
export const activitySettingsQuery = queryOptions({ queryKey: ["activity-settings"], queryFn: () => api<ActivitySettings>("GET", "/api/activity/settings") });

/** One held call for the /approve page, read with the session or the page's link token; it polls only while the call waits and has not expired. */
export const approvalQuery = (id: string, token: string | null) =>
  queryOptions({
    queryKey: ["approval", id],
    queryFn: () => api<ApprovalView>("GET", `/api/approvals/${encodeURIComponent(id)}${token ? `?t=${encodeURIComponent(token)}` : ""}`),
    refetchInterval: (query) => {
      const data = query.state.data;
      return data?.state === "pending" && Date.parse(data.expires_at) > Date.now() ? 5_000 : false;
    },
    refetchOnWindowFocus: false,
  });

/** The notification channels, the paired computers and the per-app elicitation switches. */
export const notificationsQuery = queryOptions({ queryKey: ["notifications"], queryFn: () => api<NotificationsData>("GET", "/api/notifications") });
