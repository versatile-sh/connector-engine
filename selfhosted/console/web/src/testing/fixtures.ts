import type { QueryKey } from "@tanstack/react-query";
import type { Account, ActivityData, ActivitySettings, Approval, Capability, Connector, NotificationsData, PeopleData, Session, Skill, SlotView } from "../api";
import type { KeysData } from "../queries";

/** Every capability a lone Owner holds. */
export const OWNER_CAN = [
  "people.manage",
  "people.read",
  "people.reset",
  "backup.full",
  "connectors.manage",
  "skills.manage",
  "setup.manage",
  "activity.settings",
  "activity.delete",
  "keys.any",
  "approvals.any",
  "activity.any",
  "keys.own",
  "approvals.own",
  "activity.own",
  "notifications.own",
  "signin.own",
  "account.own",
] as const satisfies readonly Capability[];

/** What a Member may do. */
export const MEMBER_CAN = ["keys.own", "approvals.own", "activity.own", "notifications.own", "signin.own", "account.own"] as const satisfies readonly Capability[];

/** What an Admin may do. */
export const ADMIN_CAN = OWNER_CAN.filter((can) => !["people.manage", "backup.full", "activity.delete"].includes(can));

/** The session of the only user of an install: its Owner. */
export const ONE_OWNER = {
  owner_exists: true,
  signed_in: true,
  csrf: "c",
  mcp_url: "http://127.0.0.1:7465/mcp",
  onboarding_dismissed: true,
  key_protection: "wrapped",
  user: { person: "owner", username: "alice", role: "owner" },
  can: [...OWNER_CAN],
  people: 1,
} as const satisfies Session;

/** The same Owner once a second user exists. */
export const TEAM_OWNER = { ...ONE_OWNER, people: 3 } as const satisfies Session;

/** A Member of a team of three. */
export const TEAM_MEMBER = { ...ONE_OWNER, user: { person: "p-bob", username: "bob", role: "member" }, can: [...MEMBER_CAN], people: 3 } as const satisfies Session;

/** An Admin of a team of three. */
export const TEAM_ADMIN = { ...ONE_OWNER, user: { person: "p-carol", username: "carol", role: "admin" }, can: [...ADMIN_CAN], people: 3 } as const satisfies Session;

const ALICE = { person: "owner", username: "alice" };

/** One connector with a function of each access and a sign-in. */
export const CONNECTOR = {
  id: "c1",
  alias: "gmail",
  enabled: true,
  listing: "acme/gmail",
  version: "1.0.0",
  title: "Gmail",
  publisher: "Acme",
  remote_url: "https://mcp.acme.test/gmail",
  auth: ["oauth2_user"],
  credentials: ["oauth2_user:active"],
  functions: [
    { name: "list_messages", class: "read", kind: "read", state: "active", access: "on" },
    { name: "send_message", class: "write", kind: "write", state: "active", access: "ask" },
    { name: "delete_message", class: "write", kind: "destructive", state: "active", access: "off" },
  ],
  items: [],
  profiles: [{ agent: "laptop", mode: "follow" }],
  slots: [],
  credential_mode: "shared",
  credential_modes: ["shared", "per_person"],
} as const satisfies Connector;

/** The access keys of the install. */
export const KEYS = { agents: [{ name: "laptop", expires_at: "2999-01-01T00:00:00Z", new: false, owner: ALICE }], max_ttl_hours: 720 } as const satisfies KeysData;

/** One call held for approval. */
export const APPROVAL = {
  id: "a1",
  agent: "laptop",
  session: "s-0123456789",
  connector: "c1",
  function: "send_message",
  class: "write",
  state: "pending",
  created_at: "2999-01-01T00:00:00Z",
  expires_at: "2999-01-01T00:05:00Z",
  owner: ALICE,
  choices: ["once", "always", "deny"],
} as const satisfies Approval;

/** A day of activity with one call. */
export const ACTIVITY = {
  from: "2026-11-19T00:00:00Z",
  to: "2026-11-20T00:00:00Z",
  calls: [{ id: "k1", at: "2026-11-19T12:00:00Z", agent: "laptop", connector: "c1", alias: "gmail", function: "list_messages", class: "read", outcome: "ok", duration_ms: 120, captured: true, owner: ALICE }],
  total: 1,
  hours: [],
  connectors: [{ connector: "c1", alias: "gmail", total: 1, failed: 0 }],
  agents: ["laptop"],
} as const satisfies ActivityData;

const SETTINGS = { record_arguments: false, record_results: false, retention_days: 30 } as const satisfies ActivitySettings;

const NOTIFICATIONS = { channels: [], devices: [], apps: [] } as const satisfies NotificationsData;

const ACCOUNT = { username: "alice", last_export: "2026-11-19T00:00:00Z" } as const satisfies Account;

/** The people of a team of three, a pending invite and one change. */
export const PEOPLE = {
  people: [
    { person: "owner", username: "alice", role: "owner", created_at: "2026-11-01T00:00:00Z", keys: 1, you: true },
    { person: "p-carol", username: "carol", role: "admin", created_at: "2026-11-02T00:00:00Z", keys: 0, you: false },
    { person: "p-bob", username: "bob", role: "member", created_at: "2026-11-03T00:00:00Z", keys: 3, you: false },
  ],
  invites: [{ id: "i1", purpose: "join", role: "member", person: "", created_by: "owner", created_at: "2026-11-19T00:00:00Z", expires_at: "2026-11-22T00:00:00Z" }],
  audit: [{ at: "2026-11-03T00:00:00Z", actor: "owner", action: "user.created", target: "p-bob" }],
} as const satisfies PeopleData;

const SKILLS: Skill[] = [];

const SLOTS: SlotView[] = [];

/** The answers every page of the console reads, by query key, for a session. */
export const pageData = (session: Session): readonly (readonly [QueryKey, unknown])[] => [
  [["session"], session],
  [["connectors"], { connectors: [CONNECTOR] }],
  [["agents"], KEYS],
  [["agents", "page"], KEYS],
  [["approvals"], { approvals: [APPROVAL] }],
  [["activity", { range: "24h" }], ACTIVITY],
  [["activity-settings"], SETTINGS],
  [["notifications"], NOTIFICATIONS],
  [["account"], ACCOUNT],
  [["skills"], { skills: SKILLS }],
  [["slots"], { slots: SLOTS }],
  [["people"], PEOPLE],
];
