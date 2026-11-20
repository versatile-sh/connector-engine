package keychain

import "os"

// System returns the macOS login keychain through /usr/bin/security, or nil when it is missing.
func System() Keychain {
	if _, err := os.Stat(security); err != nil {
		return nil
	}
	return macOS{run: execRun}
}
