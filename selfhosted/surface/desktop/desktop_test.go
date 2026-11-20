package desktop

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const link = "http://127.0.0.1:7465/approve/ap-1?t=tok"

type recorder struct {
	argv [][]string
	out  map[string]string
}

func (r *recorder) run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.argv = append(r.argv, append([]string{name}, args...))
	return []byte(r.out[name]), nil
}

func notifier(goos string, out map[string]string, onPath bool) (Notifier, *recorder) {
	r := &recorder{out: out}
	look := func(string) (string, error) { return "", errors.New("not found") }
	if onPath {
		look = func(s string) (string, error) { return "/usr/local/bin/" + s, nil }
	}
	return Notifier{GOOS: goos, Run: r.run, LookPath: look}, r
}

func joined(argv [][]string) []string {
	var out []string
	for _, a := range argv {
		out = append(out, strings.Join(a, " | "))
	}
	return out
}

func TestLinuxOpensOnClick(t *testing.T) {
	n, r := notifier("linux", map[string]string{"notify-send": "open\n"}, false)
	if err := n.Show(context.Background(), "-Allow Notes", "create_note", link); err != nil {
		t.Fatal(err)
	}
	want := []string{"notify-send | --app-name=versatile.sh | --wait | --action=open=Open | -- | -Allow Notes | create_note", "xdg-open | " + link}
	if got := joined(r.argv); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("argv = %q", got)
	}
}

func TestLinuxDismissOpensNothing(t *testing.T) {
	n, r := notifier("linux", map[string]string{"notify-send": ""}, false)
	if err := n.Show(context.Background(), "t", "b", link); err != nil || len(r.argv) != 1 {
		t.Fatalf("err=%v argv=%q", err, r.argv)
	}
}

func TestDarwinTerminalNotifier(t *testing.T) {
	n, r := notifier("darwin", nil, true)
	if err := n.Show(context.Background(), "Title", "Body", link); err != nil {
		t.Fatal(err)
	}
	want := "terminal-notifier | -title | Title | -message | Body | -open | " + link + " | -group | versatile"
	if got := joined(r.argv); len(got) != 1 || got[0] != want {
		t.Fatalf("argv = %q", got)
	}
}

func TestDarwinDialogKeepsTextOutOfScript(t *testing.T) {
	n, r := notifier("darwin", map[string]string{"osascript": "Open\n"}, false)
	title := `Evil" & (do shell script "rm -rf ~") & "`
	if err := n.Show(context.Background(), title, "body", link); err != nil {
		t.Fatal(err)
	}
	if len(r.argv) != 2 {
		t.Fatalf("argv = %q", r.argv)
	}
	osa := r.argv[0]
	if osa[0] != "osascript" || osa[1] != "-e" || osa[2] != dialogScript || osa[3] != appName || osa[4] != title || osa[5] != "body" {
		t.Fatalf("osascript argv = %q", osa)
	}
	if strings.Contains(osa[2], "Evil") {
		t.Fatal("title interpolated into the script")
	}
	if strings.Join(r.argv[1], " ") != "open "+link {
		t.Fatalf("open argv = %q", r.argv[1])
	}
}

func TestDarwinIgnoreOpensNothing(t *testing.T) {
	n, r := notifier("darwin", map[string]string{"osascript": "Ignore\n"}, false)
	if err := n.Show(context.Background(), "t", "b", link); err != nil || len(r.argv) != 1 {
		t.Fatalf("err=%v argv=%q", err, r.argv)
	}
}

func TestWindowsToastCannotBreakOutOfLiteral(t *testing.T) {
	n, r := notifier("windows", nil, false)
	title := "it's \"quoted\" \u2019smart\u2018 $(Remove-Item C:\\) `whoami` \u201a\u201b <b>&amp;"
	if err := n.Show(context.Background(), title, "body'; Stop-Computer; '", link); err != nil {
		t.Fatal(err)
	}
	argv := r.argv[0]
	if len(argv) != 5 || argv[0] != "powershell" || argv[1] != "-NoProfile" || argv[3] != "-Command" {
		t.Fatalf("argv = %q", argv)
	}
	script := argv[4]
	start := strings.Index(script, "$x.LoadXml('") + len("$x.LoadXml('")
	end := strings.Index(script[start:], "')") + start
	lit := script[start:end]
	for _, bad := range []string{"'", "\u2018", "\u2019", "\u201a", "\u201b", "`", "$", "<b>", "&amp;", "\n"} {
		if strings.Contains(lit, bad) {
			t.Errorf("literal contains %q: %s", bad, lit)
		}
	}
	if !strings.Contains(lit, `activationType="protocol" launch="`) || !strings.Contains(lit, "&#39;") || !strings.Contains(lit, "&#8217;") {
		t.Fatalf("literal = %s", lit)
	}
}

func TestRejectsNonHTTPLinkAndUnknownOS(t *testing.T) {
	n, r := notifier("linux", nil, false)
	if err := n.Show(context.Background(), "t", "b", "file:///etc/passwd"); err == nil || len(r.argv) != 0 {
		t.Fatalf("err=%v argv=%q", err, r.argv)
	}
	n, _ = notifier("plan9", nil, false)
	if err := n.Show(context.Background(), "t", "b", link); err == nil {
		t.Fatal("unknown OS accepted")
	}
}

func TestDefaultUsesHost(t *testing.T) {
	d := Default()
	if d.GOOS == "" || d.Run == nil || d.LookPath == nil {
		t.Fatalf("Default = %+v", d)
	}
}
