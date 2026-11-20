package keychain

import "os/exec"

// System returns the Secret Service (gnome-keyring, KWallet) through secret-tool over D-Bus, or nil when secret-tool is missing.
func System() Keychain {
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return nil
	}
	return secretService{run: execRun}
}
