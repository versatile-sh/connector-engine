package console

import (
	"slices"
	"testing"
)

func sampleSettings(t *testing.T, name string) map[string]string {
	t.Helper()
	k, _ := kindOf(name)
	host := "ntfy.sh"
	if len(k.Hosts) > 0 {
		host = k.Hosts[0]
	}
	out := map[string]string{}
	for _, f := range k.Fields {
		switch {
		case f.Name == "url":
			out[f.Name] = "https://" + host + "/hook"
		case f.Name == "owner_user_id" && name == "slack":
			out[f.Name] = "U12345"
		case f.Name == "owner_user_id":
			out[f.Name] = "123456"
		default:
			out[f.Name] = f.Placeholder + "tok-" + f.Name
		}
	}
	return out
}

func TestEveryChannelKindDescribesItselfAndBuildsFromItsOwnFields(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range channelKinds {
		if seen[k.Name] || k.Title == "" || len(k.Fields) == 0 || k.New == nil {
			t.Fatalf("kind %q is duplicated or incomplete", k.Name)
		}
		seen[k.Name] = true
		settings, err := checkSettings(k.Name, sampleSettings(t, k.Name))
		if err != nil {
			t.Fatalf("%s: %v", k.Name, err)
		}
		ch, err := DefaultChannel(k.Name, settings)
		if err != nil || ch.Kind() != k.Name {
			t.Fatalf("%s built %v, %v", k.Name, ch, err)
		}
		if _, err := checkSettings(k.Name, map[string]string{}); err == nil {
			t.Errorf("%s accepted no settings", k.Name)
		}
		if h := k.HintField(); h == "" || !slices.Contains(k.Secrets(), h) {
			t.Errorf("%s hints at %q, which is not one of its secrets %v", k.Name, h, k.Secrets())
		}
	}
	for _, want := range []string{"discord", "slack", "telegram", "ntfy", "discord_webhook", "slack_webhook"} {
		if !seen[want] {
			t.Errorf("kind %s is gone", want)
		}
	}
}

func TestOnlyTheChatKindsAreInteractive(t *testing.T) {
	for _, k := range channelKinds {
		want := k.Name == "discord" || k.Name == "slack" || k.Name == "telegram"
		if interactive(k.Name) != want {
			t.Errorf("%s interactive %v", k.Name, interactive(k.Name))
		}
	}
}
