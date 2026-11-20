package sqlite

import (
	"encoding/json"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

var dueFloor = time.Date(1, 1, 2, 0, 0, 0, 0, time.UTC)

func dueOf(k string, v any) (port.WorkKind, string, time.Time) {
	var kind port.WorkKind
	var at time.Time
	id := k
	switch x := v.(type) {
	case port.Approval:
		switch x.State {
		case port.ApprovalPending:
			kind, at = port.WorkApprovalExpiry, x.ExpiresAt
		case port.ApprovalApproved:
			kind, at = port.WorkApprovedRecord, x.DecidedAt
		case port.ApprovalExecuting:
			kind, at = port.WorkExecuteDeadline, x.ExecutionStartedAt.Add(x.CallDeadline)
		default:
			if x.ResultRef != nil || x.Arguments != nil {
				kind, at = port.WorkResultRetention, x.ResultExpiresAt
			}
		}
	case port.Call:
		if x.State == port.CallStarted {
			kind, at = port.WorkCallDeadline, x.Deadline
		}
	case port.Grant:
		kind, at = port.WorkGrantExpiry, x.ExpiresAt
	case port.Pause:
		if x.Paused {
			kind, at, id = port.WorkPauseExpiry, x.ExpiresAt, string(x.ID)
		}
	}
	if kind == "" || !at.After(dueFloor) {
		return "", "", time.Time{}
	}
	return kind, id, at
}

func decodeDue(kind string, doc []byte) (any, error) {
	switch kind {
	case kindApproval:
		return decodeAs[port.Approval](doc)
	case kindCall:
		return decodeAs[port.Call](doc)
	case kindGrant:
		return decodeAs[port.Grant](doc)
	case kindPause:
		return decodeAs[port.Pause](doc)
	}
	return nil, nil
}

func decodeAs[V any](doc []byte) (any, error) {
	var v V
	err := json.Unmarshal(doc, &v)
	return v, err
}

func (t *tx) track(kind, k string, owner port.TenantID, v any) error {
	if kind != kindApproval && kind != kindCall && kind != kindGrant && kind != kindPause {
		return nil
	}
	if err := t.untrack(kind, k); err != nil {
		return err
	}
	work, id, at := dueOf(k, v)
	if work == "" {
		return nil
	}
	_, err := t.exec(`INSERT INTO due_work (rec_kind, k, tenant_id, kind, id, due) VALUES (?3, ?4, ?5, ?6, ?7, ?8)`, kind, k, string(owner), string(work), id, stamp(at))
	return err
}

func (t *tx) untrack(kind, k string) error {
	_, err := t.exec(`DELETE FROM due_work WHERE rec_kind = ?3 AND k = ?4 AND (tenant_id = ?1 OR ?2)`, kind, k)
	return err
}
