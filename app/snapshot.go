package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/versatile-sh/connector-engine/port"
	secretslocal "github.com/versatile-sh/connector-engine/secrets/local"
	"github.com/versatile-sh/connector-engine/server"
)

const maxSnapshotBytes = 512 << 20

var errRestored = errors.New("restored")

// Snapshot is a copy of an engine for a backup: the build that took it, every row of its store and, when its secrets adapter keeps them in a directory, its keys in plaintext.
type Snapshot struct {
	Build string             `json:"build"`
	Store port.Snapshot      `json:"store"`
	Keys  *secretslocal.Keys `json:"keys,omitempty"`
}

type keyStore interface {
	Keys() (secretslocal.Keys, error)
	Restore(k secretslocal.Keys, then func() error) error
}

func (a *App) snapshotRoutes(mux *http.ServeMux) {
	if !a.Config.Server.Snapshot {
		return
	}
	mux.HandleFunc("GET /v1/snapshot", a.operatorOnly(a.getSnapshot))
	mux.HandleFunc("PUT /v1/snapshot", a.operatorOnly(a.putSnapshot))
}

func (a *App) operatorOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bearer, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if _, err := a.operatorAuthn()(r.Context(), bearer); err != nil || a.operator == "" {
			http.Error(w, "the snapshot takes the operator token", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (a *App) snapshotter() (port.Snapshotter, error) {
	s, ok := a.Store.(port.Snapshotter)
	if !ok {
		return nil, port.Errorf(port.CodeConflict, "the store adapter takes no snapshot")
	}
	return s, nil
}

func (a *App) getSnapshot(w http.ResponseWriter, r *http.Request) {
	store, err := a.snapshotter()
	snap := Snapshot{Build: a.build}
	if err == nil {
		snap.Store, err = store.Snapshot(r.Context())
	}
	if ks, ok := a.secrets.(keyStore); ok && err == nil {
		var k secretslocal.Keys
		if k, err = ks.Keys(); err == nil {
			snap.Keys = &k
		}
	}
	if err != nil {
		writeProblem(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(snap)
}

func (a *App) putSnapshot(w http.ResponseWriter, r *http.Request) {
	var snap Snapshot
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSnapshotBytes)).Decode(&snap); err != nil {
		writeProblem(w, port.Errorf(port.CodeInvalidArgument, "the body is not a snapshot: %v", err))
		return
	}
	store, err := a.snapshotter()
	if err == nil {
		restore := func() error { return store.Restore(r.Context(), snap.Store) }
		ks, ok := a.secrets.(keyStore)
		switch {
		case ok != (snap.Keys != nil):
			err = port.Errorf(port.CodeInvalidArgument, "the snapshot's keys and this engine's secrets adapter do not match: restore keys only into a key store kept in a directory")
		case ok:
			err = ks.Restore(*snap.Keys, restore)
		default:
			err = restore()
		}
	}
	if err != nil {
		a.log.ErrorContext(r.Context(), "snapshot restore refused", "error", err)
		writeProblem(w, err)
		return
	}
	a.log.InfoContext(r.Context(), "snapshot restored: the engine stops so that it starts again on the restored keys")
	w.WriteHeader(http.StatusNoContent)
	a.restoredOnce.Do(func() { close(a.restored) })
}

func writeProblem(w http.ResponseWriter, err error) {
	p := server.ProblemOf(err)
	w.Header().Set("Content-Type", server.ProblemMediaType)
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}
