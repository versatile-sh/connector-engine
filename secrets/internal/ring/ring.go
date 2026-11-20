// Package ring is the envelope-encryption key ring shared by the local and memory SecretStore adapters.
package ring

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/agents-control/connector-engine/port"
)

const (
	formatV1   = 1
	headerSize = 5
	keySize    = 32
	nonceSize  = 12
	tagSize    = 16
)

// KeyVersion is one key-encryption key of a tenant, wrapped by the root key.
type KeyVersion struct {
	Version int    `json:"version"`
	Wrapped []byte `json:"wrapped"`
}

// TenantKeys are the key-encryption keys of one tenant; a destroyed tenant keeps none and gets none again.
type TenantKeys struct {
	Destroyed bool         `json:"destroyed,omitempty"`
	Keys      []KeyVersion `json:"keys,omitempty"`
}

// State is everything the ring persists besides the root key.
type State struct {
	RootVersion int                          `json:"root_version"`
	Tenants     map[port.TenantID]TenantKeys `json:"tenants"`
}

// Save persists the state and, when root is not nil, the new root key.
type Save func(s State, root []byte) error

// Ring is a port.SecretStore that wraps one data key per record with a key-encryption key per tenant under one root key.
type Ring struct {
	mu    sync.Mutex
	root  []byte
	state State
	save  Save
}

var _ port.SecretStore = (*Ring)(nil)

// New returns a ring over root; save may be nil for a ring that lives in memory.
func New(root []byte, state State, save Save) (*Ring, error) {
	if len(root) != keySize {
		return nil, port.Errorf(port.CodeInvalidArgument, "the root key is %d bytes", keySize)
	}
	if state.Tenants == nil {
		state.Tenants = map[port.TenantID]TenantKeys{}
	}
	state.RootVersion = max(state.RootVersion, 1)
	return &Ring{root: root, state: state, save: save}, nil
}

// Opens reports whether root unwraps every key-encryption key of state; a state without keys is opened by any root.
func Opens(root []byte, state State) bool {
	r := &Ring{root: root, state: State{RootVersion: max(state.RootVersion, 1)}}
	for tenant, tk := range state.Tenants {
		for _, kv := range tk.Keys {
			if _, err := r.unwrap(tenant, kv); err != nil {
				return false
			}
		}
	}
	return true
}

// NewRootKey returns a random root key.
func NewRootKey() ([]byte, error) {
	k := make([]byte, keySize)
	_, err := rand.Read(k)
	return k, err
}

// WrapRoot seals a root key under an unlock key, so the root can rest on disk without being plaintext.
func WrapRoot(unlock, root []byte) ([]byte, error) { return encrypt(unlock, root, []byte("root")) }

// UnwrapRoot opens what WrapRoot sealed; a wrong unlock key fails.
func UnwrapRoot(unlock, wrapped []byte) ([]byte, error) { return decrypt(unlock, wrapped, []byte("root")) }

func gcm(key []byte) (cipher.AEAD, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

func encrypt(key, plaintext, aad []byte) ([]byte, error) {
	a, err := gcm(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceSize, nonceSize+len(plaintext)+tagSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return a.Seal(nonce, nonce, plaintext, aad), nil
}

func decrypt(key, sealed, aad []byte) ([]byte, error) {
	a, err := gcm(key)
	if err != nil {
		return nil, err
	}
	if len(sealed) < nonceSize+tagSize {
		return nil, fmt.Errorf("sealed value is too short")
	}
	return a.Open(nil, sealed[:nonceSize], sealed[nonceSize:], aad)
}

func kekAAD(tenant port.TenantID, version, root int) []byte {
	return []byte("kek|" + string(tenant) + "|" + strconv.Itoa(version) + "|" + strconv.Itoa(root))
}

func dataAAD(tenant port.TenantID, purpose string) []byte {
	return []byte(purpose + "|" + string(tenant))
}

func (r *Ring) newKey(tenant port.TenantID, version int) (KeyVersion, error) {
	kek, err := NewRootKey()
	if err != nil {
		return KeyVersion{}, err
	}
	wrapped, err := encrypt(r.root, kek, kekAAD(tenant, version, r.state.RootVersion))
	return KeyVersion{Version: version, Wrapped: wrapped}, err
}

func (r *Ring) unwrap(tenant port.TenantID, v KeyVersion) ([]byte, error) {
	return decrypt(r.root, v.Wrapped, kekAAD(tenant, v.Version, r.state.RootVersion))
}

func (r *Ring) persist(next State, root []byte) error {
	if r.save != nil {
		if err := r.save(next, root); err != nil {
			return port.Errorf(port.CodeInternal, "persist keys: %v", err)
		}
	}
	r.state = next
	if root != nil {
		r.root = root
	}
	return nil
}

func (r *Ring) with(tenant port.TenantID, tk TenantKeys) State {
	next := State{RootVersion: r.state.RootVersion, Tenants: make(map[port.TenantID]TenantKeys, len(r.state.Tenants)+1)}
	for k, v := range r.state.Tenants {
		next.Tenants[k] = v
	}
	next.Tenants[tenant] = tk
	return next
}

func (r *Ring) current(tenant port.TenantID, create bool) (KeyVersion, error) {
	tk, known := r.state.Tenants[tenant]
	switch {
	case tk.Destroyed:
		return KeyVersion{}, port.Errorf(port.CodeTenantRemoved, "the keys of tenant %q are destroyed", tenant)
	case len(tk.Keys) > 0:
		return tk.Keys[len(tk.Keys)-1], nil
	case !create || known:
		return KeyVersion{}, port.Errorf(port.CodeNotFound, "tenant %q has no key", tenant)
	}
	kv, err := r.newKey(tenant, 1)
	if err != nil {
		return kv, port.Errorf(port.CodeInternal, "create key: %v", err)
	}
	return kv, r.persist(r.with(tenant, TenantKeys{Keys: []KeyVersion{kv}}), nil)
}

// Seal wraps a fresh data key with the newest key-encryption key of tenant, creating that key on first use.
func (r *Ring) Seal(ctx context.Context, tenant port.TenantID, plaintext []byte) (port.Sealed, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return port.Sealed{}, port.Errorf(port.CodeUnavailable, "%v", err)
	}
	kv, err := r.current(tenant, true)
	if err != nil {
		return port.Sealed{}, err
	}
	return r.seal(tenant, kv, plaintext)
}

func (r *Ring) seal(tenant port.TenantID, kv KeyVersion, plaintext []byte) (port.Sealed, error) {
	kek, err := r.unwrap(tenant, kv)
	if err != nil {
		return port.Sealed{}, port.Errorf(port.CodeInternal, "unwrap key: %v", err)
	}
	dek, err := NewRootKey()
	if err != nil {
		return port.Sealed{}, port.Errorf(port.CodeInternal, "data key: %v", err)
	}
	wrapped, err := encrypt(kek, dek, dataAAD(tenant, "dek"))
	if err != nil {
		return port.Sealed{}, port.Errorf(port.CodeInternal, "wrap data key: %v", err)
	}
	body, err := encrypt(dek, plaintext, dataAAD(tenant, "data"))
	if err != nil {
		return port.Sealed{}, port.Errorf(port.CodeInternal, "seal: %v", err)
	}
	head := make([]byte, headerSize)
	head[0] = formatV1
	binary.BigEndian.PutUint32(head[1:], uint32(kv.Version))
	return port.Sealed{Ciphertext: append(append(head, wrapped...), body...), KeyRef: fmt.Sprintf("local:%s:%d", tenant, kv.Version)}, nil
}

// Open unwraps the data key with the key-encryption key the record names and decrypts it.
func (r *Ring) Open(ctx context.Context, tenant port.TenantID, s port.Sealed) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, port.Errorf(port.CodeUnavailable, "%v", err)
	}
	wrappedSize := nonceSize + keySize + tagSize
	if len(s.Ciphertext) < headerSize+wrappedSize || s.Ciphertext[0] != formatV1 || !strings.HasPrefix(s.KeyRef, "local:") {
		return nil, port.Errorf(port.CodeInvalidArgument, "not a sealed record of this store")
	}
	tk := r.state.Tenants[tenant]
	if tk.Destroyed {
		return nil, port.Errorf(port.CodeTenantRemoved, "the keys of tenant %q are destroyed", tenant)
	}
	version := int(binary.BigEndian.Uint32(s.Ciphertext[1:headerSize]))
	var kv KeyVersion
	for _, k := range tk.Keys {
		if k.Version == version {
			kv = k
		}
	}
	if kv.Version == 0 {
		return nil, port.Errorf(port.CodeNotFound, "tenant %q has no key version %d", tenant, version)
	}
	kek, err := r.unwrap(tenant, kv)
	if err != nil {
		return nil, port.Errorf(port.CodeInternal, "unwrap key: %v", err)
	}
	body := s.Ciphertext[headerSize:]
	dek, err := decrypt(kek, body[:wrappedSize], dataAAD(tenant, "dek"))
	if err != nil {
		return nil, port.Errorf(port.CodePermissionDenied, "the record does not open under the keys of tenant %q", tenant)
	}
	plaintext, err := decrypt(dek, body[wrappedSize:], dataAAD(tenant, "data"))
	if err != nil {
		return nil, port.Errorf(port.CodePermissionDenied, "the record does not open under the keys of tenant %q", tenant)
	}
	return plaintext, nil
}

// Rotate makes a new root key and re-wraps every key-encryption key, or adds a key-encryption key to one tenant; old records still open.
func (r *Ring) Rotate(ctx context.Context, scope port.KeyScope) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return port.Errorf(port.CodeUnavailable, "%v", err)
	}
	if scope.Root {
		return r.rotateRoot()
	}
	tk, ok := r.state.Tenants[scope.Tenant]
	switch {
	case tk.Destroyed:
		return port.Errorf(port.CodeTenantRemoved, "the keys of tenant %q are destroyed", scope.Tenant)
	case !ok:
		return port.Errorf(port.CodeNotFound, "tenant %q has no key", scope.Tenant)
	}
	kv, err := r.newKey(scope.Tenant, tk.Keys[len(tk.Keys)-1].Version+1)
	if err != nil {
		return port.Errorf(port.CodeInternal, "create key: %v", err)
	}
	tk.Keys = append(append([]KeyVersion(nil), tk.Keys...), kv)
	return r.persist(r.with(scope.Tenant, tk), nil)
}

func (r *Ring) rotateRoot() error {
	root, err := NewRootKey()
	if err != nil {
		return port.Errorf(port.CodeInternal, "root key: %v", err)
	}
	next := State{RootVersion: r.state.RootVersion + 1, Tenants: make(map[port.TenantID]TenantKeys, len(r.state.Tenants))}
	for tenant, tk := range r.state.Tenants {
		keys := make([]KeyVersion, len(tk.Keys))
		for i, kv := range tk.Keys {
			kek, err := r.unwrap(tenant, kv)
			if err != nil {
				return port.Errorf(port.CodeInternal, "unwrap key: %v", err)
			}
			wrapped, err := encrypt(root, kek, kekAAD(tenant, kv.Version, next.RootVersion))
			if err != nil {
				return port.Errorf(port.CodeInternal, "wrap key: %v", err)
			}
			keys[i] = KeyVersion{Version: kv.Version, Wrapped: wrapped}
		}
		next.Tenants[tenant] = TenantKeys{Destroyed: tk.Destroyed, Keys: keys}
	}
	return r.persist(next, root)
}

// Destroy deletes every key-encryption key of tenant for good: its sealed records can no longer be opened (crypto-shredding).
func (r *Ring) Destroy(ctx context.Context, tenant port.TenantID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return port.Errorf(port.CodeUnavailable, "%v", err)
	}
	return r.persist(r.with(tenant, TenantKeys{Destroyed: true}), nil)
}
