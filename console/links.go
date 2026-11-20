package console

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/versatile-sh/connector-engine/console/sealed"
)

const (
	linkKind  = "link"
	stateKind = "mrtr"
)

var errBadToken = errors.New("the link is not valid or has expired")

const gatewayLocked = "the gateway is locked: run make unlock on this computer, then open the link again"

type signingKey struct {
	mu  sync.Mutex
	key []byte
}

func (c *Console) linkKey() ([]byte, error) {
	c.signing.mu.Lock()
	defer c.signing.mu.Unlock()
	if c.signing.key != nil {
		return c.signing.key, nil
	}
	if c.Secrets.Locked() {
		return nil, sealed.ErrLocked
	}
	ctx := context.Background()
	stored, legacy := c.State.linkKeyRecords()
	var hexKey string
	if stored != nil {
		opened, err := c.Secrets.Open(ctx, *stored)
		if err != nil {
			return nil, err
		}
		hexKey = opened
	} else {
		if hexKey = legacy; hexKey == "" {
			b := make([]byte, 32)
			rand.Read(b)
			hexKey = hex.EncodeToString(b)
		}
		v, err := c.Secrets.Seal(ctx, hexKey)
		if err != nil {
			return nil, err
		}
		if err := c.State.putSealedLinkKey(v); err != nil {
			return nil, err
		}
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, err
	}
	c.signing.key = key
	return key, nil
}

func (c *Console) sign(kind string, expires time.Time, fields ...string) (string, error) {
	key, err := c.linkKey()
	if err != nil {
		return "", err
	}
	payload := strings.Join(append([]string{kind, strconv.FormatInt(expires.Unix(), 10), randomID()[:16]}, fields...), "\x1f")
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(payload)) + "." + enc.EncodeToString(mac.Sum(nil)), nil
}

func (c *Console) open(kind, token string) (nonce string, fields []string, err error) {
	key, err := c.linkKey()
	if err != nil {
		return "", nil, err
	}
	enc := base64.RawURLEncoding
	body, sig, ok := strings.Cut(token, ".")
	payload, err1 := enc.DecodeString(body)
	got, err2 := enc.DecodeString(sig)
	if !ok || err1 != nil || err2 != nil {
		return "", nil, errBadToken
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	parts := strings.Split(string(payload), "\x1f")
	if !hmac.Equal(mac.Sum(nil), got) || len(parts) < 3 || parts[0] != kind {
		return "", nil, errBadToken
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().After(time.Unix(exp, 0)) {
		return "", nil, errBadToken
	}
	return parts[2], parts[3:], nil
}

// LinkToken signs a single-use link that decides one approval from one paired device until the approval expires.
func (c *Console) LinkToken(approval, device string, expires time.Time) (string, error) {
	return c.sign(linkKind, expires, approval, device)
}

func (c *Console) checkLink(token, approval string) (device string, nonce string, err error) {
	nonce, f, err := c.open(linkKind, token)
	if err != nil {
		return "", "", err
	}
	if len(f) != 2 || f[0] != approval || c.surf.spent(nonce) || !slices.ContainsFunc(c.State.Devices(), func(d Device) bool { return d.ID == f[1] }) {
		return "", "", errBadToken
	}
	return f[1], nonce, nil
}

type spentSet struct {
	mu   sync.Mutex
	used map[string]time.Time
}

func (s *spentSet) spend(nonce string, until time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.used == nil {
		s.used = map[string]time.Time{}
	}
	expire(s.used, now, func(v time.Time) time.Time { return v })
	if _, ok := s.used[nonce]; ok {
		return false
	}
	s.used[nonce] = until
	return true
}

func (s *spentSet) has(nonce string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.used[nonce]
	return ok
}
