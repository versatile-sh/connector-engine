// Package desktop shows a native notification on the host and opens the approval link when the owner clicks it.
package desktop

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

const appName = "versatile.sh"

const powershellAppID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`

const dialogScript = `on run argv
set r to display dialog (item 3 of argv) with title (item 2 of argv) buttons {"Ignore", "Open"} default button "Open" giving up after 600
return button returned of r
end run`

// Notifier runs the host's notification tool for GOOS through Run, finding optional tools with LookPath.
type Notifier struct {
	GOOS     string
	Run      func(ctx context.Context, name string, args ...string) ([]byte, error)
	LookPath func(string) (string, error)
}

// Default returns a Notifier for this host that runs real commands.
func Default() Notifier {
	return Notifier{
		GOOS: runtime.GOOS,
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
		LookPath: exec.LookPath,
	}
}

// Show displays title and body, waits until the notification is clicked or dismissed where the platform allows, and opens link on click.
func (n Notifier) Show(ctx context.Context, title, body, link string) error {
	if n.Run == nil {
		return errors.New("desktop: no command runner")
	}
	if u, err := url.Parse(link); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("desktop: link must be an http or https URL")
	}
	switch n.GOOS {
	case "linux":
		out, err := n.Run(ctx, "notify-send", "--app-name="+appName, "--wait", "--action=open=Open", "--", title, body)
		if err != nil {
			return fmt.Errorf("desktop: notify-send: %w", err)
		}
		if strings.TrimSpace(string(out)) == "open" {
			return n.open(ctx, "xdg-open", link)
		}
		return nil
	case "darwin":
		if n.LookPath != nil {
			if _, err := n.LookPath("terminal-notifier"); err == nil {
				if _, err := n.Run(ctx, "terminal-notifier", "-title", title, "-message", body, "-open", link, "-group", "versatile"); err != nil {
					return fmt.Errorf("desktop: terminal-notifier: %w", err)
				}
				return nil
			}
		}
		out, err := n.Run(ctx, "osascript", "-e", dialogScript, appName, title, body)
		if err != nil {
			return fmt.Errorf("desktop: osascript: %w", err)
		}
		if strings.TrimSpace(string(out)) == "Open" {
			return n.open(ctx, "open", link)
		}
		return nil
	case "windows":
		if _, err := n.Run(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", toastScript(title, body, link)); err != nil {
			return fmt.Errorf("desktop: powershell: %w", err)
		}
		return nil
	}
	return fmt.Errorf("desktop: notifications are not supported on %q", n.GOOS)
}

func (n Notifier) open(ctx context.Context, opener, link string) error {
	if _, err := n.Run(ctx, opener, link); err != nil {
		return fmt.Errorf("desktop: %s: %w", opener, err)
	}
	return nil
}

func toastScript(title, body, link string) string {
	l := xmlLiteral(link)
	doc := `<toast activationType="protocol" launch="` + l + `"><visual><binding template="ToastGeneric"><text>` + xmlLiteral(title) +
		`</text><text>` + xmlLiteral(body) + `</text></binding></visual><actions><action content="Open" activationType="protocol" arguments="` + l + `"/></actions></toast>`
	return strings.Join([]string{
		"$ErrorActionPreference = 'Stop'",
		"[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null",
		"[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null",
		"$x = New-Object Windows.Data.Xml.Dom.XmlDocument",
		"$x.LoadXml('" + doc + "')",
		"$t = New-Object Windows.UI.Notifications.ToastNotification $x",
		"[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('" + powershellAppID + "').Show($t)",
	}, "\n")
}

func xmlLiteral(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', strings.ContainsRune(" .,:/?=_-#%+@~", r):
			b.WriteRune(r)
		case r < 0x20 && r != '\t' && r != '\n' && r != '\r':
			b.WriteByte(' ')
		default:
			b.WriteString("&#" + strconv.Itoa(int(r)) + ";")
		}
	}
	return b.String()
}
