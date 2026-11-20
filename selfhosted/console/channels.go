package console

import (
	"fmt"

	"github.com/versatile-sh/connector-engine/selfhosted/surface"
	"github.com/versatile-sh/connector-engine/selfhosted/surface/discord"
	"github.com/versatile-sh/connector-engine/selfhosted/surface/slack"
	"github.com/versatile-sh/connector-engine/selfhosted/surface/telegram"
	"github.com/versatile-sh/connector-engine/selfhosted/surface/webhook"
)

// DefaultChannel builds the built-in approval channel of a kind from its stored settings.
func DefaultChannel(kind string, s map[string]string) (surface.Channel, error) {
	switch kind {
	case "discord":
		return discord.New(discord.Config{Token: s["token"], OwnerUserID: s["owner_user_id"]})
	case "slack":
		return slack.New(slack.Config{AppToken: s["app_token"], BotToken: s["bot_token"], OwnerUserID: s["owner_user_id"]})
	case "telegram":
		return telegram.New(telegram.Config{Token: s["token"], OwnerUserID: s["owner_user_id"]})
	case "ntfy", "discord_webhook", "slack_webhook":
		return webhook.New(webhook.Config{Kind: kind, URL: s["url"], Token: s["token"]})
	}
	return nil, fmt.Errorf("channel kind %q is not built in", kind)
}
