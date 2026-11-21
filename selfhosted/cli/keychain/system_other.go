//go:build !darwin && !linux && !windows

package keychain

// System returns nil: this OS has no keychain backend.
func System() Keychain { return nil }

// SystemFor returns nil: this OS has no keychain backend.
func SystemFor(string) Keychain { return nil }
