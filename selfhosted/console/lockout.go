package console

import (
	"strings"
	"time"
)

const (
	passwordFailsPerUserPerMin = 5
	passwordFailsPerMin        = 30
	maxLockoutNames            = 1000
)

type lockout struct {
	names map[string]*minuteWindow
	all   minuteWindow
}

func (l *lockout) locked(now time.Time, username string) bool {
	if l.all.full(now, passwordFailsPerMin) {
		return true
	}
	w := l.names[strings.ToLower(username)]
	return w != nil && w.full(now, passwordFailsPerUserPerMin)
}

func (l *lockout) fail(now time.Time, username string) {
	l.all.hit(now)
	name := strings.ToLower(username)
	w := l.names[name]
	if w == nil {
		for k, v := range l.names {
			if !v.full(now, 1) {
				delete(l.names, k)
			}
		}
		if len(l.names) >= maxLockoutNames {
			return
		}
		w = &minuteWindow{}
		put(&l.names, name, w)
	}
	w.hit(now)
}
