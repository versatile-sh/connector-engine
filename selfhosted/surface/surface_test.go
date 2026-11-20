package surface

import (
	"testing"
	"time"
)

func TestTheSessionChoiceNamesHowLongItLasts(t *testing.T) {
	for _, c := range []struct {
		left    time.Duration
		unknown bool
		want    string
	}{
		{8*time.Hour - time.Second, false, "Allow for up to 8 hours"},
		{2 * time.Hour, false, "Allow for up to 2 hours"},
		{59*time.Minute + 50*time.Second, false, "Allow for up to 60 minutes"},
		{10 * time.Second, false, "Allow for up to 1 minute"},
		{0, false, "Allow for this session"},
		{time.Hour, true, "Allow for this session"},
	} {
		if got := sessionLabel(c.left, c.unknown); got != c.want {
			t.Errorf("sessionLabel(%v, %v) = %q, want %q", c.left, c.unknown, got, c.want)
		}
	}
	if got := Label(ChoiceOnce, time.Now().Add(time.Hour)); got != "Allow once" {
		t.Errorf("once: %q", got)
	}
}
