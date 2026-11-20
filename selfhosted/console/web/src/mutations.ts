import { mutationOptions, type QueryClient, type QueryKey } from "@tanstack/react-query";
import { api, apiFile, setCsrf, type Access, type ProfileMode, type BackupSummary, type SetupResult, type ActivitySettings, type AgentToken, type ApprovalChoice, type ChannelKind, type ChannelView, type Decision, type ImportSource, type PairedDevice, type Session } from "./api";
import { accountQuery, activityKey, activitySettingsQuery, approvalQuery, approvalsQuery, connectorsQuery, keysQuery, notificationsQuery, sessionQuery, skillsQuery } from "./queries";

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

/** Ends the owner's session, forgets every other answer and reads the session again, so the sign-in screen shows at once. */
export const signOut = mutationOptions({
  mutationFn: () => api("POST", "/api/logout"),
  onSettled: async (_data, _error, _variables, _onMutate, { client }) => {
    setCsrf(undefined);
    client.removeQueries({ predicate: (query) => query.queryKey[0] !== sessionQuery.queryKey[0] });
    await client.resetQueries({ queryKey: sessionQuery.queryKey });
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

/** Sets the named access keys, or every key, to off, follow or custom on a connector, in one request that answers each key. */
export const setProfileMode = mutationOptions({
  mutationFn: ({ connector, mode, keys }: { connector: string; mode: ProfileMode; keys?: string[] }) => api<{ keys: KeyResult[] }>("PUT", `/api/connectors/${at(connector, "profiles")}`, keys ? { mode, keys } : { mode }),
  onSettled: (_data, _error, _choice, _onMutate, { client }) => invalidate(client, connectorsQuery.queryKey),
});

/** What one function or item of a batch came to: done without an error. */
export type ItemResult = { name: string; error?: string; code?: string };

/** Sets many functions of a connector to off, on or ask for every key that follows it, in one request that answers each. */
export const setFunctionsAccess = mutationOptions({
  mutationFn: ({ connector, names, access }: { connector: string; names: readonly string[]; access: Access }) => api<{ items: ItemResult[] }>("PUT", `/api/connectors/${at(connector, "functions")}`, { names, access }),
  onSettled: (_data, _error, _choice, _onMutate, { client }) => invalidate(client, connectorsQuery.queryKey),
});

/** Narrows many functions or items for one access key, or resets them to the connector's own access, in one request that answers each; names go in the body since item names hold `/` and `:`. */
export const setProfileAccess = mutationOptions({
  mutationFn: ({ connector, agent, names, access }: { connector: string; agent: string; names: readonly string[]; access: Access | "default" }) =>
    api<{ items: ItemResult[] }>("PUT", `/api/connectors/${at(connector, "profiles", agent)}`, access === "default" ? { names, reset: true } : { names, access }),
  onSettled: (_data, _error, _choice, _onMutate, { client }) => invalidate(client, connectorsQuery.queryKey),
});

/** Gives other access keys the profile one key has on a connector, in one request that answers each key. */
export const copyProfile = mutationOptions({
  mutationFn: ({ connector, from, keys }: { connector: string; from: string; keys: readonly string[] }) => api<{ keys: KeyResult[] }>("POST", `/api/connectors/${encodeURIComponent(connector)}/profiles:copy`, { from, keys }),
  onSettled: (_data, _error, _choice, _onMutate, { client }) => invalidate(client, connectorsQuery.queryKey),
});

/** Forgets a setup's profile waiting on a connector for an access key not made yet. */
export const dropWaiting = mutationOptions({
  mutationFn: ({ connector, agent }: { connector: string; agent: string }) => api("DELETE", `/api/connectors/${at(connector, "waiting", agent)}`),
  onSettled: (_data, _error, _choice, _onMutate, { client }) => invalidate(client, connectorsQuery.queryKey),
});

/** Makes the access keys made from now on start off on a connector, or follow it. */
export const setNewKeysOff = mutationOptions({
  mutationFn: ({ connector, off }: { connector: string; off: boolean }) => api("PUT", `/api/connectors/${at(connector, "new-keys")}`, { off }),
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

/** What one access key's choice in a batch came to: set when it carries no error. */
export type KeyResult = { agent: string; error?: string; code?: string };

/** Sets one access on a skill for every key, or for the keys named, in one request; each key answers its own result. */
export const setSkillAccessAll = mutationOptions({
  mutationFn: ({ skill, access, keys }: { skill: string; access: Access; keys?: string[] }) => api<{ keys: KeyResult[] }>("PUT", `/api/skills/${at(skill, "keys")}`, keys ? { access, keys } : { access }),
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
  mutationFn: ({ name, id }: { name: string; id?: string }) => api<PairedDevice>("POST", "/api/notifications/devices", { name, id }),
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

/** Changes the owner's password; the console ends every other session and keeps this one. */
export const changePassword = mutationOptions({
  mutationFn: (change: { current: string; password: string }) => api("PUT", "/api/account/password", change),
});

/** Renames the owner. */
export const renameOwner = mutationOptions({
  mutationFn: (username: string) => api("PUT", "/api/account/username", { username }),
  onSuccess: (_data, _username, _onMutate, { client }) => invalidate(client, accountQuery.queryKey),
});

/** What POST /api/backup/export takes: a setup needs nothing else; a full backup needs the confirmation, the owner's password and a passphrase. */
export type ExportRequest = { mode: "setup" } | { mode: "full"; confirm: boolean; password: string; passphrase: string };

/** Makes a backup file and answers it to be saved; the account is read again for the date of the last export. */
export const exportBackup = mutationOptions({
  mutationFn: (request: ExportRequest) => apiFile("POST", "/api/backup/export", request),
  onSuccess: (_file, _request, _onMutate, { client }) => invalidate(client, accountQuery.queryKey),
});

/** A backup file and its passphrase, empty for a setup, as a form; a signed-in full import adds the owner's password. */
export type BackupUpload = { file: Blob; passphrase: string; password?: string };

const formOf = ({ file, passphrase, password }: BackupUpload) => {
  const form = new FormData();
  form.append("file", file);
  form.append("passphrase", passphrase);
  if (password !== undefined) form.append("password", password);
  return form;
};

/** Reads what a backup file holds without importing anything. */
export const inspectBackup = mutationOptions({
  mutationFn: (upload: BackupUpload) => api<BackupSummary>("POST", "/api/backup/inspect", formOf(upload)),
});

/** What an import answered: a full backup answers its summary and the console restarts; a setup answers what it did. */
export type ImportOutcome = { kind: "full"; summary: BackupSummary } | { kind: "setup"; result: SetupResult };

/** Imports a backup file; after a setup, connectors, skills and the session are read again, while after a full backup nothing is, as the console is restarting. */
export const importBackup = mutationOptions({
  mutationFn: async (upload: BackupUpload): Promise<ImportOutcome> => {
    const answer = await api<BackupSummary | SetupResult>("POST", "/api/backup/import", formOf(upload));
    return "added" in answer ? { kind: "setup", result: answer } : { kind: "full", summary: answer };
  },
  onSuccess: (outcome, _upload, _onMutate, { client }) => (outcome.kind === "setup" ? invalidate(client, connectorsQuery.queryKey, skillsQuery.queryKey, sessionQuery.queryKey) : undefined),
});
