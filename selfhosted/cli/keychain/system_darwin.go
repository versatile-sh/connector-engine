package keychain

import "os"

// System returns the keychain of the unlock keys, under Service.
func System() Keychain { return SystemFor(Service) }

// SystemFor returns the macOS login keychain through /usr/bin/security under service, or nil when it is missing.
func SystemFor(service string) Keychain {
	if _, err := os.Stat(security); err != nil {
		return nil
	}
	return macOS{run: execRun, service: service}
}
