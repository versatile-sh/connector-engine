// Package hostkey is a SecretStore whose root key rests wrapped by an unlock key the host delivers at start, from its OS keychain, over a unix socket inside the container.
package hostkey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/secrets/local"
)

// Op names a request on the unlock socket.
type Op string

// The requests: deliver the unlock key, keep a plaintext root on a host without a keychain, wrap a plaintext root, unwrap it back, or report.
const (
	OpUnlock    Op = "unlock"
	OpPlaintext Op = "plaintext"
	OpMigrate   Op = "migrate"
	OpRevert    Op = "revert"
	OpStatus    Op = "status"
)

// Locked is the protection reported before the store is open.
const Locked = "locked"

// Request is one request on the unlock socket; Key is the hex unlock key of unlock and migrate.
type Request struct {
	Op  Op     `json:"op"`
	Key string `json:"key,omitempty"`
}

// Reply is the answer to a request: the protection of the root key (locked, plaintext or wrapped) or an error.
type Reply struct {
	Protection string `json:"protection"`
	Error      string `json:"error,omitempty"`
}

// Store is a local.Store reached through the unlock socket; Open returns it once the store is open.
type Store struct {
	*local.Store
	dir   string
	lis   net.Listener
	mu    sync.Mutex
	ready chan struct{}
	done  chan struct{}
}

// Open listens on socket and returns once the store in dir is open: at once for a plaintext root, after the host delivers the unlock key or the plaintext choice otherwise.
func Open(ctx context.Context, dir, socket string, log *slog.Logger) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(socket), 0o700); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(socket); err == nil && info.Mode()&os.ModeSocket != 0 {
		_ = os.Remove(socket)
	}
	lis, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	s := &Store{dir: dir, lis: lis, ready: make(chan struct{}), done: make(chan struct{})}
	if err := os.Chmod(socket, 0o600); err != nil {
		lis.Close()
		return nil, err
	}
	if local.FormOf(dir) == local.Plaintext {
		st, err := local.Open(dir)
		if err != nil {
			lis.Close()
			return nil, err
		}
		s.opened(st)
	} else {
		log.Warn("waiting for the host to deliver the unlock key", "socket", socket, "form", local.FormOf(dir))
	}
	go s.serve()
	select {
	case <-s.ready:
		return s, nil
	case <-ctx.Done():
		s.Close()
		return nil, ctx.Err()
	}
}

// Close stops the unlock socket.
func (s *Store) Close() {
	s.lis.Close()
	<-s.done
}

func (s *Store) opened(st *local.Store) {
	s.Store = st
	close(s.ready)
}

func (s *Store) serve() {
	defer close(s.done)
	for {
		conn, err := s.lis.Accept()
		if err != nil {
			return
		}
		s.answer(conn)
	}
}

func (s *Store) answer(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	var req Request
	reply := Reply{}
	if err := json.NewDecoder(io.LimitReader(conn, 4096)).Decode(&req); err != nil {
		reply.Error = "malformed request"
	} else {
		reply = s.handle(req)
	}
	_ = json.NewEncoder(conn).Encode(reply)
}

func (s *Store) handle(req Request) Reply {
	s.mu.Lock()
	defer s.mu.Unlock()
	var key []byte
	if req.Op == OpUnlock || req.Op == OpMigrate {
		var err error
		if key, err = local.ParseUnlockKey(req.Key); err != nil {
			return s.reply(err)
		}
	}
	if s.Store == nil {
		return s.reply(s.unlock(req.Op, key))
	}
	switch req.Op {
	case OpUnlock:
		if s.Form() == local.Wrapped && !s.WrappedBy(key) {
			return s.reply(errors.New("this is not the unlock key the root key is wrapped with"))
		}
	case OpMigrate:
		return s.reply(s.Migrate(key))
	case OpRevert:
		return s.reply(s.Revert())
	case OpPlaintext, OpStatus:
	default:
		return s.reply(fmt.Errorf("unknown request %q", req.Op))
	}
	return s.reply(nil)
}

func (s *Store) unlock(op Op, key []byte) error {
	var st *local.Store
	var err error
	switch op {
	case OpUnlock:
		st, err = local.OpenWrapped(s.dir, key)
	case OpPlaintext:
		if local.FormOf(s.dir) != local.Fresh {
			return errors.New("the root key is wrapped: deliver its unlock key")
		}
		st, err = local.Open(s.dir)
	case OpStatus:
		return nil
	default:
		return errors.New("the store is locked: deliver the unlock key first")
	}
	if err != nil {
		return err
	}
	s.opened(st)
	return nil
}

func (s *Store) reply(err error) Reply {
	r := Reply{Protection: Locked}
	if s.Store != nil {
		r.Protection = string(s.Form())
	}
	if err != nil {
		r.Error = err.Error()
	}
	return r
}

var _ port.SecretStore = (*Store)(nil)

// Send delivers one request on socket, retrying the connection until ctx ends, and returns the reply; a reply error is returned as an error too.
func Send(ctx context.Context, socket string, req Request) (Reply, error) {
	var d net.Dialer
	for {
		conn, err := d.DialContext(ctx, "unix", socket)
		if err == nil {
			defer conn.Close()
			if err := json.NewEncoder(conn).Encode(req); err != nil {
				return Reply{}, err
			}
			var r Reply
			if err := json.NewDecoder(io.LimitReader(conn, 4096)).Decode(&r); err != nil {
				return Reply{}, err
			}
			if r.Error != "" {
				return r, errors.New(r.Error)
			}
			return r, nil
		}
		select {
		case <-ctx.Done():
			return Reply{}, fmt.Errorf("unlock socket %s: %w", socket, err)
		case <-time.After(200 * time.Millisecond):
		}
	}
}
