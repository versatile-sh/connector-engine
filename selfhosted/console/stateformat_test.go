package console

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRaw(t *testing.T, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAFileFromBeforeVersioningUpgradesToTheCurrentFormatOnItsFirstWrite(t *testing.T) {
	path := writeRaw(t, `{"owner":{"username":"ann","salt":"c2FsdA==","hash":"aGFzaA==","iterations":1},"agents":{"old":{"name":"old"}},"pending_profiles":{"conn":[{"Agent":"slotkey"}]}}`)
	s, err := OpenState(path, "p-ann")
	if err != nil {
		t.Fatal(err)
	}
	if s.data.Format != stateFormat || len(s.data.Users) != 1 || s.data.Owner != nil || s.data.Agents["old"].Owner != "p-ann" || s.data.Pending != nil || len(s.data.Slots) != 1 {
		t.Fatalf("not upgraded: %+v", s.data)
	}
	if err := s.update(func(d *stateFile) error { d.OnboardingDismissed = true; return nil }); err != nil {
		t.Fatal(err)
	}
	var on stateFile
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &on); err != nil || on.Format != stateFormat || on.Owner != nil {
		t.Fatalf("the file on disk: %v %s", err, raw)
	}
}

func TestACurrentFileIsNotMigratedAgain(t *testing.T) {
	path := writeRaw(t, `{"format":1,"agents":{"k":{"name":"k"}},"users":{"p1":{"Person":"p1","Role":"owner"}}}`)
	s, err := OpenState(path, "p-other")
	if err != nil {
		t.Fatal(err)
	}
	if s.data.Agents["k"].Owner != "" {
		t.Fatalf("a current file went through the legacy step: owner %q", s.data.Agents["k"].Owner)
	}
}

func TestANewerFileIsRefusedWithAClearMessageAndLeftUntouched(t *testing.T) {
	raw := `{"format":99,"users":{}}`
	path := writeRaw(t, raw)
	_, err := OpenState(path, "p")
	if err == nil || !strings.Contains(err.Error(), "format 99") || !strings.Contains(err.Error(), "up to format") {
		t.Fatalf("got %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != raw {
		t.Fatal("the refused file was rewritten")
	}
}

func TestAFreshStateIsWrittenInTheCurrentFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := OpenState(path, "p")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.update(func(d *stateFile) error { d.OnboardingDismissed = true; return nil }); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"format": 1`) {
		t.Fatalf("no format in %s", raw)
	}
}
