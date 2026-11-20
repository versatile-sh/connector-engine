import { queryOptions } from "@tanstack/react-query";
import { apiQueryOf, type ActivityFilter } from "./activity/activity";
import { api, setCsrf, type Account, type ActivityData, type ActivityDetail, type ActivitySettings, type Agent, type Approval, type ApprovalView, type Connector, type Effect, type NotificationsData, type Session, type Skill, type SlotView } from "./api";

/** Who is here: whether the owner exists and is signed in, the endpoint, and the guide's dismissal; it also arms the CSRF token. */
export const sessionQuery = queryOptions({
  queryKey: ["session"],
  queryFn: async () => {
    const s = await api<Session>("GET", "/api/session");
    setCsrf(s.csrf);
    return s;
  },
});

/** What GET /api/agents answers: the access keys and the longest life a key may have. */
export type KeysData = { agents: Agent[]; max_ttl_hours: number };

/** Every installed connector. */
export const connectorsQuery = queryOptions({
  queryKey: ["connectors"],
  queryFn: () => api<{ connectors: Connector[] }>("GET", "/api/connectors"),
  select: (data) => data.connectors,
});

/** What the engine gives one access key on each function and item of a connector; its key sits under the connectors' one, so every connector change reads it again. */
export const profileEffectsQuery = (connector: string, agent: string) =>
  queryOptions({
    queryKey: [...connectorsQuery.queryKey, connector, "effects", agent],
    queryFn: () => api<{ effects: Effect[] }>("GET", `/api/connectors/${encodeURIComponent(connector)}/profiles/${encodeURIComponent(agent)}`),
    select: (data) => data.effects,
  });

/** The permission sets no access key has claimed yet. */
export const slotsQuery = queryOptions({
  queryKey: ["slots"],
  queryFn: () => api<{ slots: SlotView[] }>("GET", "/api/slots"),
  select: (data) => data.slots,
});

/** Every installed skill with each access key's choice. */
export const skillsQuery = queryOptions({
  queryKey: ["skills"],
  queryFn: () => api<{ skills: Skill[] }>("GET", "/api/skills"),
  select: (data) => data.skills,
});

/** The access keys an AI app connects with. */
export const keysQuery = queryOptions({ queryKey: ["agents"], queryFn: () => api<KeysData>("GET", "/api/agents") });

/** The access keys as the Access page reads them: a list that arrives with new keys tells the console they were seen, so the flags show this once; a failed mark only leaves them for the next visit. */
export const keysPageQuery = queryOptions({
  queryKey: [...keysQuery.queryKey, "page"],
  queryFn: async () => {
    const data = await api<KeysData>("GET", "/api/agents");
    if (data.agents.some((key) => key.new)) await api("POST", "/api/agents/seen").catch(() => undefined);
    return data;
  },
});

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

/** How long after an approval is decided the console may still be writing an "always allow", as the console's own wait. */
export const ALWAYS_WAIT_MS = 3 * 60_000;

/** Whether the /approve page reads a held call again: while it waits and has not expired, and while an allowed call's "always allow" may still be written and has not failed. */
export function approvalPolls(data: ApprovalView | undefined, now: number): boolean {
  if (!data) return false;
  if (data.state === "pending") return Date.parse(data.expires_at) > now;
  const allowed = ["approved", "executing", "executed"].includes(data.state);
  return allowed && !data.always_error && data.decided_at !== undefined && now - Date.parse(data.decided_at) < ALWAYS_WAIT_MS;
}

/** One held call for the /approve page, read with the session or the page's link token; it polls as `approvalPolls` says. */
export const approvalQuery = (id: string, token: string | null) =>
  queryOptions({
    queryKey: ["approval", id],
    queryFn: () => api<ApprovalView>("GET", `/api/approvals/${encodeURIComponent(id)}${token ? `?t=${encodeURIComponent(token)}` : ""}`),
    refetchInterval: (query) => (approvalPolls(query.state.data, Date.now()) ? 5_000 : false),
    refetchOnWindowFocus: false,
  });

/** The notification channels, the paired computers and the per-app elicitation switches. */
export const notificationsQuery = queryOptions({ queryKey: ["notifications"], queryFn: () => api<NotificationsData>("GET", "/api/notifications") });

/** The owner's name and when the last full backup was made. */
export const accountQuery = queryOptions({ queryKey: ["account"], queryFn: () => api<Account>("GET", "/api/account") });
