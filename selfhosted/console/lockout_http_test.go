package console_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func (r *accountRig) loginFrom(addr, user, pass string) int {
	host := r.srv.Listener.Addr().String()
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(fmt.Sprintf(`{"username":%q,"password":%q}`, user, pass)))
	req.Host = host
	req.RemoteAddr = addr
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+host)
	rec := httptest.NewRecorder()
	r.c.Handler().ServeHTTP(rec, req)
	return rec.Code
}

func TestOneAttackerCannotLockOtherClientsOut(t *testing.T) {
	r := newAccountRig(t)
	for range 40 {
		r.loginFrom("203.0.113.9:4000", "me", "wrong")
	}
	if got := r.loginFrom("203.0.113.9:4001", "me", "wrong"); got != http.StatusTooManyRequests {
		t.Fatalf("the attacker is not limited: %d", got)
	}
	if got := r.loginFrom("198.51.100.7:5000", "me", "correct horse battery"); got != http.StatusOK {
		t.Fatalf("another client was refused after the attack: %d", got)
	}
}

func TestWrongLoginsDoNotSerialise(t *testing.T) {
	r := newAccountRig(t)
	var wg sync.WaitGroup
	start := time.Now()
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.loginFrom(fmt.Sprintf("203.0.113.%d:4000", i+1), "nobody", "wrong")
		}()
	}
	wg.Wait()
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("twelve concurrent wrong logins (serial sleeps alone need 6s) took %v: they are serialised", took)
	}
}
