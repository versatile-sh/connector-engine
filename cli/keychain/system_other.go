//go:build !darwin && !linux && !windows

package keychain

// System returns nil: this OS has no keychain backend.
func System() Keychain { return nil }
