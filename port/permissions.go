package port

import (
	"bytes"
	"encoding/json"
	"time"
)

// PermissionChange is one changed access setting of a tenant; the store stamps Tenant, Revision and Seq (from 1) on append.
// Before and After are JSON values kept as given (compare them as JSON, not bytes); absent or null means the setting did not exist.
type PermissionChange struct {
	Tenant   TenantID        `json:"tenant_id"`
	Revision int64           `json:"revision"`
	Seq      int             `json:"seq"`
	At       time.Time       `json:"at"`
	Actor    string          `json:"actor"`
	Op       string          `json:"op"`
	Kind     string          `json:"kind"`
	Key      string          `json:"key"`
	Before   json.RawMessage `json:"before,omitempty"`
	After    json.RawMessage `json:"after,omitempty"`
}

// PermissionChanges checks changes for an append and returns copies stamped with tenant and position, absent values nil; the store sets Revision.
func PermissionChanges(tenant TenantID, changes []PermissionChange) ([]PermissionChange, error) {
	if len(changes) == 0 {
		return nil, Errorf(CodeInvalidArgument, "an append of access setting changes holds at least one change")
	}
	out := make([]PermissionChange, len(changes))
	for i, c := range changes {
		if c.Kind == "" || c.Key == "" {
			return nil, Errorf(CodeInvalidArgument, "access setting change %d names no kind or key", i+1)
		}
		for _, v := range []*json.RawMessage{&c.Before, &c.After} {
			trimmed := bytes.TrimSpace(*v)
			switch {
			case len(trimmed) == 0 || string(trimmed) == "null":
				*v = nil
			case !json.Valid(trimmed):
				return nil, Errorf(CodeInvalidArgument, "access setting change %d holds a value that is not JSON", i+1)
			default:
				*v = bytes.Clone(trimmed)
			}
		}
		if c.Before == nil && c.After == nil {
			return nil, Errorf(CodeInvalidArgument, "access setting change %d has no value before or after", i+1)
		}
		c.Tenant, c.Seq = tenant, i+1
		out[i] = c
	}
	return out, nil
}
