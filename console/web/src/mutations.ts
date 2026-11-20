import { mutationOptions, type QueryClient, type QueryKey } from "@tanstack/react-query";
import { api, setCsrf, type Access, type ActivitySettings, type AgentToken, type ApprovalChoice, type ChannelKind, type ChannelView, type Decision, type ImportSource, type PairedDevice, type Session } from "./api";
import { activityKey, activitySettingsQuery, approvalQuery, approvalsQuery, connectorsQuery, keysQuery, notificationsQuery, sessionQuery, skillsQuery } from "./queries";

const invalidate = (client: QueryClient, ...keys: QueryKey[]) => Promise.all(keys.map((queryKey) => client.invalidateQueries({ queryKey })));

const at = (...segments: string[]) => segments.map(encodeURIComponent).join("/");

/** Creates the owner on first visit or signs them in; the answer arms the CSRF token and the session is read again. */
export const signIn = mutationOptions({
  mutationFn: ({ mode, ...credentials }: { mode: "setup" | "login"; username: string; password: string }) => api<{ csrf: string }>("POST", `/api/${mode}`, credentials),
  onSuccess: (result, _credentials, _onMutate, { client }) => {
    setCsrf(result.csrf);
    return invalidate(client, sessionQuery.queryKey);
  },
});

/** Ends the owner's session and forgets every answer the console gave it. */
export const signOut = mutationOptions({
  mutationFn: () => api("POST", "/api/logout"),
  onSettled: (_data, _error, _variables, _onMutate, { client }) => {
    setCsrf(undefined);
    client.clear();
  },
});

/** Hides the first-run guide for good, at once, and shows it again if the console refuses. */
export const dismissGuide = mutationOptions({
  mutationFn: () => api("POST", "/api/onboarding/dismiss"),
  onMutate: (_variables, { client }) => {
    client.setQueryData<Session>(sessionQuery.queryKey, (session) => session && { ...session, onboarding_dismissed: true });
  },
  onError: (_error, _variables, _onMutate, { client }) => invalidate(client, sessionQuery.queryKey),
});

/** Allows or denies an AI app asking to connect by OAuth; allowing takes the owner's password. */
export const decideConsent = mutationOptions({
  mutationFn: ({ request, approve, password }: { request: string; approve: boolean; password: string }) =>
    api<{ redirect: string }>("POST", `/api/oauth/requests/${at(request)}`, approve ? { approve, password } : { approve }),
});

/** Makes an access key; its token is in this answer only. */
export const mintKey = mutationOptions({
  mutationFn: (key: { name: string; ttl_hours: number }) => api<AgentToken>("POST", "/api/agents", key),
  onSuccess: (_token, _key, _onMutate, { client }) => invalidate(client, keysQuery.queryKey, sessionQuery.queryKey),
});

/** Revokes an access key for good. */
export const revokeKey = mutationOptions({
  mutationFn: (name: string) => api("DELETE", `/api/agents/${at(name)}`),
  onSuccess: (_data, _name, _onMutate, { client }) => invalidate(client, sessionQuery.queryKey, keysQuery.queryKey),
});

/** Sets one function of a connector to off, on or ask. */
export const setFunctionAccess = mutationOptions({
  mutationFn: ({ connector, name, access }: { connector: string; name: string; access: Access }) => api("PUT", `/api/connectors/${at(connector, "functions", name)}`, { access }),
  onSettled: (_data, _error, _choice, _onMutate, { client }) => invalidate(client, connectorsQuery.queryKey),
});

/** Sets one item of a connector to off, on or ask, by name in the body since names hold `/` and `:`. */
export const setItemAccess = mutationOptions({
  mutationFn: ({ connector, name, access }: { connector: string; name: string; access: Access }) => api("PUT", `/api/connectors/${at(connector, "items")}`, { name, access }),
  onSettled: (_data, _error, _choice, _onMutate, { client }) => invalidate(client, connectorsQuery.queryKey),
});

/** Seals a secret header onto a connector's calls. */
export const sealHeader = mutationOptions({
  mutationFn: ({ connector, header, value }: { connector: string; header: string; value: string }) => api("POST", `/api/connectors/${at(connector, "headers")}`, { headers: { [header]: value } }),
  onSuccess: (_data, _header, _onMutate, { client }) => invalidate(client, connectorsQuery.queryKey),
});

/** Starts the sign-in with a connector's provider; the answer is the address to send the browser to. */
export const connectOAuth = mutationOptions({
  mutationFn: ({ connector, clientID, clientSecret }: { connector: string; clientID: string; clientSecret: string }) =>
    api<{ url: string }>("POST", `/api/connectors/${at(connector, "oauth")}`, { client_id: clientID, client_secret: clientSecret }),
});

/** Removes a connector with its functions, items and credentials; skills name the connectors they need, so they are read again too. */
export const removeConnector = mutationOptions({
  mutationFn: (id: string) => api("DELETE", `/api/connectors/${at(id)}`),
  onSuccess: (_data, _id, _onMutate, { client }) => invalidate(client, sessionQuery.queryKey, connectorsQuery.queryKey, skillsQuery.queryKey),
});

/** What POST /api/import takes: the previewed source with the owner's choices for a connector, or the access and digest for a skill. */
export type ImportRequest = ImportSource & ({ choices: Record<string, Access> } | { access: Access; digest: string | undefined });

/** Installs what a preview showed, bound to what was shown. */
export const installImport = mutationOptions({
  mutationFn: (request: ImportRequest) => api<{ connector?: string; skill?: string }>("POST", "/api/import", request),
  onSuccess: (_installed, _request, _onMutate, { client }) => invalidate(client, connectorsQuery.queryKey, skillsQuery.queryKey, sessionQuery.queryKey),
});

/** Sets one access key's choice on a skill. */
export const setSkillAccess = mutationOptions({
  mutationFn: ({ skill, agent, access }: { skill: string; agent: string; access: Access }) => api("PUT", `/api/skills/${at(skill, "keys", agent)}`, { access }),
  onSettled: (_data, _error, _choice, _onMutate, { client }) => invalidate(client, skillsQuery.queryKey),
});

/** Removes an installed skill; the session is read again, as after any install or removal. */
export const removeSkill = mutationOptions({
  mutationFn: (id: string) => api("DELETE", `/api/skills/${at(id)}`),
  onSuccess: (_data, _id, _onMutate, { client }) => invalidate(client, skillsQuery.queryKey, sessionQuery.queryKey),
});

/** Answers a held call, with the page's link token when there is no session. The record it answers replaces the cached one; signed in, the held list is read again, and connectors and skills too after Always. */
export const decideApproval = mutationOptions({
  mutationFn: ({ id, choice, token }: { id: string; choice: ApprovalChoice; token?: string | null }) =>
    api<Decision>("POST", `/api/approvals/${at(id, "decide")}`, token ? { choice, token } : { choice }),
  onSuccess: (decision, { id }, _onMutate, { client }) => {
    client.setQueryData(approvalQuery(id, null).queryKey, decision.outcome);
  },
  onSettled: (_data, _error, { choice, token }, _onMutate, { client }) =>
    token ? undefined : invalidate(client, approvalsQuery.queryKey, ...(choice === "always" ? [connectorsQuery.queryKey, skillsQuery.queryKey] : [])),
});

/** What the console keeps of each call. */
export type Recording = Pick<ActivitySettings, "record_arguments" | "record_results">;

/** Saves what the console keeps of each call; the answer is the settings now. */
export const saveRecording = mutationOptions({
  mutationFn: (recording: Recording) => api<ActivitySettings>("PUT", "/api/activity/settings", recording),
  onSuccess: (saved, _recording, _onMutate, { client }) => {
    client.setQueryData(activitySettingsQuery.queryKey, saved);
  },
});

/** Deletes every recorded call. */
export const deleteActivity = mutationOptions({
  mutationFn: () => api("DELETE", "/api/activity"),
  onSuccess: (_data, _variables, _onMutate, { client }) => invalidate(client, activityKey, activitySettingsQuery.queryKey),
});

/** Turns MCP elicitation on or off for one app. */
export const setElicit = mutationOptions({
  mutationFn: ({ name, elicit }: { name: string; elicit: boolean }) => api("PUT", `/api/notifications/apps/${at(name)}`, { elicit }),
  onSettled: (_data, _error, _choice, _onMutate, { client }) => invalidate(client, notificationsQuery.queryKey),
});

/** Adds a notification channel; its secrets are never shown again. */
export const addChannel = mutationOptions({
  mutationFn: (channel: { kind: ChannelKind; label: string; settings: Record<string, string> }) => api<ChannelView>("POST", "/api/notifications/channels", channel),
  onSuccess: (_channel, _input, _onMutate, { client }) => invalidate(client, notificationsQuery.queryKey),
});

/** Sends a test message through a channel. */
export const testChannel = mutationOptions({
  mutationFn: (id: string) => api<{ ok: boolean; error?: string }>("POST", `/api/notifications/channels/${at(id, "test")}`),
});

/** Removes a notification channel. */
export const removeChannel = mutationOptions({
  mutationFn: (id: string) => api("DELETE", `/api/notifications/channels/${at(id)}`),
  onSuccess: (_data, _id, _onMutate, { client }) => invalidate(client, notificationsQuery.queryKey),
});

/** Pairs a computer for desktop popups; its key is in this answer only. */
export const pairDevice = mutationOptions({
  mutationFn: (name: string) => api<PairedDevice>("POST", "/api/notifications/devices", { name }),
  onSuccess: (_paired, _name, _onMutate, { client }) => invalidate(client, notificationsQuery.queryKey),
});

/** Names a paired computer again; an empty name is the console's default. */
export const renameDevice = mutationOptions({
  mutationFn: ({ id, name }: { id: string; name: string }) => api("PATCH", `/api/notifications/devices/${at(id)}`, { name }),
  onSettled: (_data, _error, _rename, _onMutate, { client }) => invalidate(client, notificationsQuery.queryKey),
});

/** Revokes a paired computer's key. */
export const revokeDevice = mutationOptions({
  mutationFn: (id: string) => api("DELETE", `/api/notifications/devices/${at(id)}`),
  onSuccess: (_data, _id, _onMutate, { client }) => invalidate(client, notificationsQuery.queryKey),
});
