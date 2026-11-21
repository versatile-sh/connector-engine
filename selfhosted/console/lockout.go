package console

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	passwordFailsPerUserPerMin = 5
	passwordFailsPerIPPerMin   = 30
	maxLockoutKeys             = 1000
	wrongPasswordDelay         = 500 * time.Millisecond
)

type lockout struct {
	mu    sync.Mutex
	ips   map[string]*minuteWindow
	pairs map[string]*minuteWindow
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func pairKey(ip, username string) string { return ip + "\x00" + strings.ToLower(username) }

func (l *lockout) locked(now time.Time, ip, username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if w := l.ips[ip]; w != nil && w.full(now, passwordFailsPerIPPerMin) {
		return true
	}
	w := l.pairs[pairKey(ip, username)]
	return w != nil && w.full(now, passwordFailsPerUserPerMin)
}

func (l *lockout) fail(now time.Time, ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	hitWindow(&l.ips, now, ip)
	hitWindow(&l.pairs, now, pairKey(ip, username))
}

func hitWindow(m *map[string]*minuteWindow, now time.Time, key string) {
	w := (*m)[key]
	if w == nil {
		for k, v := range *m {
			if !v.full(now, 1) {
				delete(*m, k)
			}
		}
		if len(*m) >= maxLockoutKeys {
			return
		}
		w = &minuteWindow{}
		put(m, key, w)
	}
	w.hit(now)
}
