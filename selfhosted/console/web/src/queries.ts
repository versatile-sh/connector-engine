import { infiniteQueryOptions, keepPreviousData, queryOptions } from "@tanstack/react-query";
import { activityViewOf, apiQueryOf, KEPT_DAYS, type ActivityFilter } from "./activity/activity";
import type { Scope } from "./people/viewer";
import { api, setCsrf, type Account, type LinkFacts, type PeopleData, type ActivityData, type ActivityDetail, type ActivitySettings, type Agent, type Approval, type ApprovalView, type Connector, type Effect, type NotificationsData, type PrunePeriod, type PruneResult, type Session, type Skill, type SlotView } from "./api";

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

/** The access keys a scope of the Access page shows: the user's own, as `keysPageQuery`, or everyone's under the same key. */
export const scopedKeysQuery = (scope: Scope) =>
  scope === "mine" ? keysPageQuery : queryOptions({ queryKey: [...keysPageQuery.queryKey, "everyone"], queryFn: () => api<KeysData>("GET", "/api/agents?scope=everyone") });

/** Calls held for the signed-in user, polled every few seconds. */
export const approvalsQuery = queryOptions({
  queryKey: ["approvals"],
  queryFn: () => api<{ approvals: Approval[] }>("GET", "/api/approvals"),
  select: (data) => data.approvals,
  refetchInterval: 4_000,
});

/** The held calls a scope shows: the user's own, as `approvalsQuery`, or everyone's under the same key, so one invalidation reads both again. */
export const scopedApprovalsQuery = (scope: Scope) =>
  scope === "mine"
    ? approvalsQuery
    : queryOptions({
        queryKey: [...approvalsQuery.queryKey, "everyone"],
        queryFn: () => api<{ approvals: Approval[] }>("GET", "/api/approvals?scope=everyone"),
        select: (data) => data.approvals,
        refetchInterval: 4_000,
      });

/** The key every activity page shares, whatever its filter. */
export const activityKey = ["activity"] as const;

/** The activity for a filter, a page of calls at a time under one key per filter, so changing the filter keeps the old rows until the new ones arrive; refreshed every few seconds while only the first page is loaded. */
export const activityQuery = (filter: ActivityFilter, keptDays = KEPT_DAYS) =>
  infiniteQueryOptions({
    queryKey: [...activityKey, filter, keptDays],
    queryFn: ({ pageParam }) => api<ActivityData>("GET", `/api/activity?${apiQueryOf(filter, new Date(), pageParam, keptDays)}`),
    initialPageParam: "",
    getNextPageParam: (last) => last.next_cursor || undefined,
    select: activityViewOf,
    refetchInterval: (query) => ((query.state.data?.pages.length ?? 0) > 1 ? false : ACTIVITY_POLL_MS),
    placeholderData: keepPreviousData,
  });

/** How often the first page of the activity is read again. */
export const ACTIVITY_POLL_MS = 10_000;

/** How many calls a period holds, counted by the console without deleting any. */
export const activityPruneQuery = (period: PrunePeriod) =>
  queryOptions({ queryKey: ["activity-prune", period], queryFn: () => api<PruneResult>("POST", "/api/activity/prune", { ...period, dry_run: true }), staleTime: 0, gcTime: 0 });

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

/** The notification channels and the paired computers. */
export const notificationsQuery = queryOptions({ queryKey: ["notifications"], queryFn: () => api<NotificationsData>("GET", "/api/notifications") });

/** The owner's name and when the last full backup was made. */
export const accountQuery = queryOptions({ queryKey: ["account"], queryFn: () => api<Account>("GET", "/api/account") });

/** The people of this console, the links still waiting and the newest changes. */
export const peopleQuery = queryOptions({ queryKey: ["people"], queryFn: () => api<PeopleData>("GET", "/api/people") });

/** What a join or reset link says, read once from its token; a link that no longer works answers 410. */
export const linkQuery = (token: string) =>
  queryOptions({ queryKey: ["link", token], queryFn: () => api<LinkFacts>("POST", "/api/invite/inspect", { token }), staleTime: Infinity, refetchOnWindowFocus: false });
