import type { ChannelKindView } from "../api";
import type { MessageKey, Translate } from "../i18n";

/** One value a channel needs: its settings name, its label, whether it is a secret, and whether it may stay empty. */
export type SetupField = { name: string; label: string; labelKey?: MessageKey; secret: boolean; optional?: boolean; placeholder?: string };

/** How one kind of channel is set up: the steps to follow outside the console and the values to paste back. */
export type Setup = { kind: string; title: string; titleKey?: MessageKey; interactive: boolean; steps: readonly MessageKey[]; fields: readonly SetupField[] };

type Words = { title: MessageKey; steps: readonly MessageKey[]; labels: Readonly<Record<string, MessageKey>> };

const WEBHOOK_STEP: MessageKey = "notify.steps.webhook";

const WORDS: Readonly<Record<string, Words>> = {
  discord: {
    title: "notify.kind.discord",
    steps: ["notify.steps.discord.1", "notify.steps.discord.2", "notify.steps.discord.3", "notify.steps.discord.4"],
    labels: { token: "notify.field.botToken", owner_user_id: "notify.field.discordUser" },
  },
  slack: {
    title: "notify.kind.slack",
    steps: ["notify.steps.slack.1", "notify.steps.slack.2", "notify.steps.slack.3", "notify.steps.slack.4"],
    labels: { app_token: "notify.field.appToken", bot_token: "notify.field.botToken", owner_user_id: "notify.field.slackMember" },
  },
  telegram: {
    title: "notify.kind.telegram",
    steps: ["notify.steps.telegram.1", "notify.steps.telegram.2", "notify.steps.telegram.3", "notify.steps.telegram.4"],
    labels: { token: "notify.field.botToken", owner_user_id: "notify.field.telegramUser" },
  },
  ntfy: { title: "notify.kind.ntfy", steps: ["notify.steps.ntfy.1", "notify.steps.ntfy.2"], labels: { url: "notify.field.topicUrl", token: "notify.field.ntfyToken" } },
  discord_webhook: { title: "notify.kind.discord_webhook", steps: ["notify.steps.discordWebhook.1", WEBHOOK_STEP], labels: { url: "notify.field.webhookUrl" } },
  slack_webhook: { title: "notify.kind.slack_webhook", steps: ["notify.steps.slackWebhook.1", WEBHOOK_STEP], labels: { url: "notify.field.webhookUrl" } },
};

/** The setup of each kind the server lists: its fields, secrets and interactivity come from the server, its translated words from here when the console has them. */
export function setupsOf(kinds: readonly ChannelKindView[]): Setup[] {
  return kinds.map((k) => {
    const words = WORDS[k.kind];
    return {
      kind: k.kind,
      title: k.title,
      titleKey: words?.title,
      interactive: k.interactive,
      steps: words?.steps ?? [],
      fields: k.fields.map((f) => ({ name: f.name, label: f.label, labelKey: words?.labels[f.name], secret: f.secret, optional: f.optional, placeholder: f.placeholder })),
    };
  });
}

/** The title of a channel kind in the viewer's language, else the server's English, else the kind's own name. */
export function kindTitle(t: Translate, kinds: readonly ChannelKindView[], kind: string): string {
  const setup = setupsOf(kinds).find((s) => s.kind === kind);
  return setup ? textOf(t, setup.titleKey, setup.title) : kind;
}

/** A translated text when the console has one, else the server's English. */
export const textOf = (t: Translate, key: MessageKey | undefined, fallback: string) => (key ? t(key) : fallback);

/** The settings body of POST /api/notifications/channels: trimmed values, empty optional ones left out. */
export function settingsFor(setup: Setup, values: Readonly<Record<string, string>>): Record<string, string> {
  const settings: Record<string, string> = {};
  for (const field of setup.fields) {
    const value = (values[field.name] ?? "").trim();
    if (value) settings[field.name] = value;
  }
  return settings;
}

/** The required fields still empty. */
export const missingFields = (setup: Setup, values: Readonly<Record<string, string>>) => setup.fields.filter((field) => !field.optional && !(values[field.name] ?? "").trim());

/** The Slack app manifest to paste in "Create New App > From a manifest": Socket Mode, interactivity, and the two bot scopes the relay uses. */
export const SLACK_MANIFEST = JSON.stringify(
  {
    display_information: { name: "versatile approvals" },
    features: { bot_user: { display_name: "versatile approvals", always_online: false } },
    oauth_config: { scopes: { bot: ["chat:write", "im:write"] } },
    settings: { interactivity: { is_enabled: true }, socket_mode_enabled: true, org_deploy_enabled: false, token_rotation_enabled: false },
  },
  null,
  2,
);
