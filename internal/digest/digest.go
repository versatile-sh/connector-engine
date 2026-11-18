// Package digest computes and compares the SHA-256 digests listings carry.
package digest

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

// Sum is the lowercase hexadecimal SHA-256 of b.
func Sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Matches reports whether want, with or without a sha256: prefix, is the digest of b.
func Matches(want string, b []byte) bool {
	want = strings.TrimPrefix(strings.ToLower(want), "sha256:")
	return subtle.ConstantTimeCompare([]byte(want), []byte(Sum(b))) == 1
}
