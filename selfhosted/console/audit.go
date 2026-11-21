package console

import (
	"slices"
	"time"
)

const maxAudit = 500

// AuditEntry is one change to people or access: who did what to whom, with details that never hold a secret.
type AuditEntry struct {
	At     time.Time         `json:"at"`
	Actor  string            `json:"actor"`
	Action string            `json:"action"`
	Target string            `json:"target"`
	Detail map[string]string `json:"detail,omitempty"`
}

func record(d *stateFile, actor, action, target string, detail map[string]string) {
	d.Audit = append(d.Audit, AuditEntry{At: time.Now().UTC().Truncate(time.Second), Actor: actor, Action: action, Target: target, Detail: detail})
	if over := len(d.Audit) - maxAudit; over > 0 {
		d.Audit = slices.Clone(d.Audit[over:])
	}
}

// Audit returns up to n of the newest console audit entries, newest first.
func (s *State) Audit(n int) []AuditEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]AuditEntry{}, s.data.Audit...)
	slices.Reverse(out)
	return out[:min(n, len(out))]
}
