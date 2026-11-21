package console

import (
	"fmt"

	"github.com/versatile-sh/connector-engine/selfhosted/surface"
	"github.com/versatile-sh/connector-engine/selfhosted/surface/discord"
	"github.com/versatile-sh/connector-engine/selfhosted/surface/slack"
	"github.com/versatile-sh/connector-engine/selfhosted/surface/telegram"
	"github.com/versatile-sh/connector-engine/selfhosted/surface/webhook"
)

var channelKinds = append([]surface.Kind{discord.Kind, slack.Kind, telegram.Kind}, webhook.Kinds...)

func kindOf(name string) (surface.Kind, bool) {
	for _, k := range channelKinds {
		if k.Name == name {
			return k, true
		}
	}
	return surface.Kind{}, false
}

// DefaultChannel builds the built-in approval channel of a kind from its stored settings.
func DefaultChannel(kind string, s map[string]string) (surface.Channel, error) {
	k, ok := kindOf(kind)
	if !ok {
		return nil, fmt.Errorf("channel kind %q is not built in", kind)
	}
	return k.New(s)
}
