package app

import "testing"

func TestTheVaultClientGivesUpOnAHungServer(t *testing.T) {
	if vaultHTTP.Timeout <= 0 {
		t.Fatal("a 1Password Connect server that never answers would hang the engine at boot")
	}
}
