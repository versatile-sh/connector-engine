package storetest

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/versatile-sh/connector-engine/port"
)

func change(key string, after string) port.PermissionChange {
	return port.PermissionChange{At: t0, Actor: "ana", Op: "createGrant", Kind: "grant", Key: key, After: json.RawMessage(after)}
}

func (e *env) appendChanges(scope port.Scope, changes ...port.PermissionChange) int64 {
	e.t.Helper()
	var rev int64
	e.must(e.write(scope, func(tx port.Tx) (err error) {
		if _, err = tx.LockPermissions(); err != nil {
			return err
		}
		rev, err = tx.AppendPermissionChanges(changes)
		return err
	}))
	return rev
}

func (e *env) changes(scope port.Scope, revision int64, seq, limit int) []port.PermissionChange {
	e.t.Helper()
	var out []port.PermissionChange
	e.must(e.read(scope, func(tx port.Tx) (err error) {
		out, err = tx.ListPermissionChanges(revision, seq, limit)
		return err
	}))
	return out
}

func (e *env) lockedRevision(scope port.Scope) int64 {
	e.t.Helper()
	var rev int64
	e.must(e.write(scope, func(tx port.Tx) (err error) {
		rev, err = tx.LockPermissions()
		return err
	}))
	return rev
}

func positions(cs []port.PermissionChange) [][2]int64 {
	out := [][2]int64{}
	for _, c := range cs {
		out = append(out, [2]int64{c.Revision, int64(c.Seq)})
	}
	return out
}

func revisionGrowsByOnePerWritingTransaction(e *env) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var got []int64
	for range 20 {
		wg.Go(func() {
			e.must(e.write(scopeA, func(tx port.Tx) error {
				held, err := tx.LockPermissions()
				if err != nil {
					return err
				}
				rev, err := tx.AppendPermissionChanges([]port.PermissionChange{change("g", `{"mode":"allow"}`)})
				if err == nil && rev != held+1 {
					err = errors.New("the revision moved while it was held")
				}
				mu.Lock()
				got = append(got, rev)
				mu.Unlock()
				return err
			}))
		})
	}
	wg.Wait()
	slices.Sort(got)
	for i, rev := range got {
		if rev != int64(i+1) {
			e.t.Fatalf("revisions of one tenant are unique and gapless: %v", got)
		}
	}
	if err := e.write(scopeA, func(tx port.Tx) error {
		_, err := tx.LockPermissions()
		_, err2 := tx.AppendPermissionChanges([]port.PermissionChange{change("g", `{}`)})
		return errors.Join(err, err2, errOK)
	}); !errors.Is(err, errOK) {
		e.t.Fatalf("a rolled back append: %v", err)
	}
	e.same(int64(20), e.lockedRevision(scopeA))
	e.same(int64(0), e.lockedRevision(scopeB))
	for _, bad := range [][]port.PermissionChange{nil, {change("", `{}`)}, {change("g", `{not json`)}, {change("g", `null`)}} {
		e.code(e.write(scopeA, func(tx port.Tx) error {
			if _, err := tx.LockPermissions(); err != nil {
				return err
			}
			_, err := tx.AppendPermissionChanges(bad)
			return err
		}), port.CodeInvalidArgument)
	}
	e.code(e.write(scopeA, func(tx port.Tx) error {
		_, err := tx.AppendPermissionChanges([]port.PermissionChange{change("g", `{}`)})
		return err
	}), port.CodeInternal)
	e.same(int64(20), e.lockedRevision(scopeA))
	var revs []int64
	e.must(e.write(scopeA, func(tx port.Tx) error {
		_, err := tx.LockPermissions()
		r1, err1 := tx.AppendPermissionChanges([]port.PermissionChange{change("first", `1`)})
		r2, err2 := tx.AppendPermissionChanges([]port.PermissionChange{change("second", `2`)})
		revs = []int64{r1, r2}
		return errors.Join(err, err1, err2)
	}))
	e.same([]int64{21, 22}, revs)
	e.same([]string{"tafirst", "tasecond"}, keysOf(e.changes(scopeA, 20, 1, 0)))
	e.same([][2]int64{{21, 1}, {22, 1}}, positions(e.changes(scopeA, 20, 1, 0)))
	if err := e.read(scopeA, func(tx port.Tx) error { _, err := tx.LockPermissions(); return err }); err == nil {
		e.t.Fatal("a read transaction locked the revision")
	}
	if err := e.read(scopeA, func(tx port.Tx) error {
		_, err := tx.AppendPermissionChanges([]port.PermissionChange{change("g", `{}`)})
		return err
	}); err == nil {
		e.t.Fatal("a read transaction appended a change")
	}
}

func jsonValue(e *env, raw json.RawMessage) any {
	if raw == nil {
		return "absent"
	}
	var v any
	e.must(json.Unmarshal(raw, &v))
	return v
}

func permissionValuesAsGiven(e *env) {
	given := []port.PermissionChange{
		{At: t0.Add(time.Microsecond), Actor: "ana", Op: "updateGrant", Kind: "grant", Key: "agent/x/c1/*", Before: json.RawMessage(` {"z":1, "a":[1.5,"é",{"k":null}]} `), After: json.RawMessage(`"deny"`)},
		{At: t0, Actor: "system", Op: "revokeGrant", Kind: "grant", Key: "user/u1/c1/f", Before: json.RawMessage(`[true]`), After: json.RawMessage(`null`)},
	}
	e.same(int64(1), e.appendChanges(scopeA, given...))
	got := e.changes(scopeA, 0, 0, 0)
	if len(got) != 2 {
		e.t.Fatalf("got %d changes", len(got))
	}
	values := [][2]any{{jsonValue(e, given[0].Before), "deny"}, {[]any{true}, "absent"}}
	for i, c := range got {
		w := given[i]
		if c.Tenant != "ta" || c.Revision != 1 || c.Seq != i+1 || !c.At.Equal(w.At) || c.Actor != w.Actor || c.Op != w.Op || c.Kind != w.Kind || c.Key != w.Key {
			e.t.Fatalf("change %d: %+v", i, c)
		}
		if v := [2]any{jsonValue(e, c.Before), jsonValue(e, c.After)}; !reflect.DeepEqual(values[i], v) {
			e.t.Fatalf("change %d holds %v, want %v", i, v, values[i])
		}
	}
}

func permissionChangesPage(e *env) {
	e.appendChanges(scopeA, change("a", `1`), change("b", `2`))
	e.appendChanges(scopeA, change("c", `3`))
	e.appendChanges(scopeA, change("d", `4`), change("e", `5`), change("f", `6`))
	all := e.changes(scopeA, 0, 0, 0)
	e.same([][2]int64{{1, 1}, {1, 2}, {2, 1}, {3, 1}, {3, 2}, {3, 3}}, positions(all))
	var paged []port.PermissionChange
	rev, seq := int64(0), 0
	for {
		page := e.changes(scopeA, rev, seq, 2)
		if len(page) == 0 {
			break
		}
		paged = append(paged, page...)
		rev, seq = page[len(page)-1].Revision, page[len(page)-1].Seq
	}
	e.same(all, paged)
	e.same([][2]int64{{3, 1}, {3, 2}, {3, 3}}, positions(e.changes(scopeA, 3, 0, 0)))
	e.same([][2]int64{{3, 2}}, positions(e.changes(scopeA, 3, 1, 1)))
}

func permissionLogIsTenantScoped(e *env) {
	e.appendChanges(scopeA, change("a", `1`))
	e.appendChanges(inst, change("i", `1`))
	both := port.Scope{Tenant: "ta", Installation: true}
	e.same([]string{"taa"}, keysOf(e.changes(both, 0, 0, 0)))
	e.same([]string{"i"}, keysOf(e.changes(inst, 0, 0, 0)))
	e.same([]string{}, keysOf(e.changes(scopeB, 0, 0, 0)))
	e.same(int64(1), e.appendChanges(scopeB, change("b", `1`)))
	e.same(int64(2), e.appendChanges(both, change("a2", `1`)))
	e.same([]string{"taa", "taa2"}, keysOf(e.changes(scopeA, 0, 0, 0)))
	e.same([]string{"tbb"}, keysOf(e.changes(scopeB, 0, 0, 0)))
	e.same([]string{"i"}, keysOf(e.changes(inst, 0, 0, 0)))
}

func keysOf(cs []port.PermissionChange) []string {
	out := []string{}
	for _, c := range cs {
		out = append(out, string(c.Tenant)+c.Key)
	}
	return out
}

func consistentReadSeesOneRevision(e *env) {
	e.appendChanges(scopeA, change("a", `1`))
	done := make(chan struct{})
	e.must(e.read(port.Scope{Tenant: "ta", Consistent: true}, func(tx port.Tx) error {
		first, err := tx.ListPermissionChanges(0, 0, 0)
		if err != nil {
			return err
		}
		go func() {
			defer close(done)
			e.appendChanges(scopeA, change("b", `2`))
		}()
		select {
		case <-done:
		case <-time.After(300 * time.Millisecond):
		}
		second, err := tx.ListPermissionChanges(0, 0, 0)
		e.same(positions(first), positions(second))
		return err
	}))
	<-done
	e.same([][2]int64{{1, 1}, {2, 1}}, positions(e.changes(scopeA, 0, 0, 0)))
}

func restoreKeepsTheLivePermissionLog(e *env) {
	src, ok := e.s.(port.Snapshotter)
	if !ok {
		e.t.Skip("the store takes no snapshot")
	}
	e.must(e.write(inst, func(tx port.Tx) error {
		return errors.Join(tx.PutTenant(port.Tenant{ID: "ta", State: port.TenantActive, CreatedAt: t0}), tx.PutTenant(port.Tenant{ID: "tb", State: port.TenantActive, CreatedAt: t0}))
	}))
	e.appendChanges(scopeA, change("a1", `1`))
	e.appendChanges(scopeA, change("a2", `2`), change("a3", `3`))
	e.appendChanges(scopeB, change("b-old", `1`))
	snap, err := src.Snapshot(ctx)
	e.must(err)

	s, _ := e.fresh()
	dst := s.(port.Snapshotter)
	live := &env{t: e.t, s: s}
	live.appendChanges(scopeB, change("b-live", `1`))
	live.appendChanges(scopeB, change("b-live2", `1`))
	_, err = dst.Restore(ctx, snap, "ops")
	e.must(err)
	e.same([]string{"taa1", "taa2", "taa3"}, keysOf(live.changes(scopeA, 0, 0, 0)))
	e.same([]string{"tbb-live", "tbb-live2"}, keysOf(live.changes(scopeB, 0, 0, 0)))
	e.same(int64(2), live.lockedRevision(scopeA))
	e.same(int64(2), live.lockedRevision(scopeB))
	e.same(int64(3), live.appendChanges(scopeA, change("a4", `4`)))

	broken := cloneSnapshot(snap)
	broken.Tables["permission_changes"] = append(broken.Tables["permission_changes"], broken.Tables["permission_changes"][0])
	_, err = dst.Restore(ctx, broken, "ops")
	e.code(err, port.CodeInvalidArgument)
	heads := [][]json.RawMessage{{}, {json.RawMessage(`{"tenant_id":"ta","revision":1}`)}}
	if snap.Store == "memory" {
		heads = [][]json.RawMessage{{json.RawMessage(`{}`)}, {json.RawMessage(`{"ta":1}`)}}
	}
	for _, rows := range heads {
		behind := cloneSnapshot(snap)
		behind.Tables["permission_head"] = rows
		_, err = dst.Restore(ctx, behind, "ops")
		e.code(err, port.CodeInvalidArgument)
	}
	e.same([]string{"taa1", "taa2", "taa3", "taa4"}, keysOf(live.changes(scopeA, 0, 0, 0)))

	e.appendChanges(scopeA, change("a-new", `1`))
	_, err = src.Restore(ctx, snap, "ops")
	e.must(err)
	e.same([]string{"taa1", "taa2", "taa3", "taa-new"}, keysOf(e.changes(scopeA, 0, 0, 0)))
	e.same(int64(3), e.lockedRevision(scopeA))
}
