package keychain

import "os/exec"

// System returns the keychain of the unlock keys, under Service.
func System() Keychain { return SystemFor(Service) }

// SystemFor returns the Secret Service (gnome-keyring, KWallet) through secret-tool over D-Bus under service, or nil when secret-tool is missing.
func SystemFor(service string) Keychain {
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return nil
	}
	return secretService{run: execRun, service: service}
}
