import type { ChannelKind } from "../api";
import type { MessageKey } from "../i18n";

/** One value a channel needs: its settings name, its label, whether it is a secret, and whether it may stay empty. */
export type SetupField = { name: string; label: MessageKey; secret: boolean; optional?: boolean; placeholder?: string };

/** How one kind of channel is set up: the steps to follow outside the console and the values to paste back. */
export type Setup = { kind: ChannelKind; title: MessageKey; interactive: boolean; steps: readonly MessageKey[]; fields: readonly SetupField[] };

const WEBHOOK_URL: SetupField = { name: "url", label: "notify.field.webhookUrl", secret: true, placeholder: "https://" };

/** The setup of each channel kind. */
export const SETUP: Record<ChannelKind, Setup> = {
  discord: {
    kind: "discord",
    title: "notify.kind.discord",
    interactive: true,
    steps: ["notify.steps.discord.1", "notify.steps.discord.2", "notify.steps.discord.3", "notify.steps.discord.4"],
    fields: [
      { name: "token", label: "notify.field.botToken", secret: true },
      { name: "owner_user_id", label: "notify.field.discordUser", secret: false },
    ],
  },
  slack: {
    kind: "slack",
    title: "notify.kind.slack",
    interactive: true,
    steps: ["notify.steps.slack.1", "notify.steps.slack.2", "notify.steps.slack.3", "notify.steps.slack.4"],
    fields: [
      { name: "app_token", label: "notify.field.appToken", secret: true, placeholder: "xapp-" },
      { name: "bot_token", label: "notify.field.botToken", secret: true, placeholder: "xoxb-" },
      { name: "owner_user_id", label: "notify.field.slackMember", secret: false },
    ],
  },
  telegram: {
    kind: "telegram",
    title: "notify.kind.telegram",
    interactive: true,
    steps: ["notify.steps.telegram.1", "notify.steps.telegram.2", "notify.steps.telegram.3", "notify.steps.telegram.4"],
    fields: [
      { name: "token", label: "notify.field.botToken", secret: true },
      { name: "owner_user_id", label: "notify.field.telegramUser", secret: false },
    ],
  },
  ntfy: {
    kind: "ntfy",
    title: "notify.kind.ntfy",
    interactive: false,
    steps: ["notify.steps.ntfy.1", "notify.steps.ntfy.2"],
    fields: [
      { name: "url", label: "notify.field.topicUrl", secret: true, placeholder: "https://ntfy.sh/" },
      { name: "token", label: "notify.field.ntfyToken", secret: true, optional: true },
    ],
  },
  discord_webhook: { kind: "discord_webhook", title: "notify.kind.discord_webhook", interactive: false, steps: ["notify.steps.discordWebhook.1", "notify.steps.webhook"], fields: [WEBHOOK_URL] },
  slack_webhook: { kind: "slack_webhook", title: "notify.kind.slack_webhook", interactive: false, steps: ["notify.steps.slackWebhook.1", "notify.steps.webhook"], fields: [WEBHOOK_URL] },
};

/** Every channel kind in the order the picker shows them. */
export const SETUPS: readonly Setup[] = Object.values(SETUP);

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
