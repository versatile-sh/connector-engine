CNCTL    ?= bin/cnctl
SEED_ADDR ?= 127.0.0.1:7477
ACCOUNT   = COMPOSE="docker compose -f selfhosted/deploy/compose.yaml -p $(PROJECT) --profile person" UNLOCK="$(MAKE) --no-print-directory unlock" MODE="$(MODE)" YES="$(YES)" selfhosted/scripts/account.sh
KEYS      = COMPOSE="docker compose -f selfhosted/deploy/compose.yaml -p $(PROJECT) --profile person" CNCTL="$(CNCTL)" PROJECT="$(PROJECT)" selfhosted/scripts/keychain.sh

.PHONY: check seed cnctl unlock keychain-status keychain-migrate keychain-revert keychain-recovery keychain-import notify reset-password export import

cnctl:
	@if command -v go >/dev/null 2>&1; then go build -o bin/cnctl ./selfhosted/cmd/cnctl; else echo "no Go on this host: the keychain is skipped" >&2; fi

unlock: cnctl
	@$(KEYS) unlock

keychain-status:
	@$(KEYS) status

keychain-migrate: cnctl
	@$(KEYS) migrate

keychain-revert:
	@$(KEYS) revert

keychain-recovery: cnctl
	@$(KEYS) recovery

keychain-import: cnctl
	@$(KEYS) import

check: vendor
	selfhosted/scripts/console-check.sh

seed:
	cd selfhosted/console/web && npm ci --no-audit --no-fund && npm run build
	CN_SEED_ADDR=$(SEED_ADDR) go test ./selfhosted/console -run '^TestSeedPlayground$$' -count=1 -timeout 0 -v

notify: cnctl
	$(CNCTL) notify --console http://127.0.0.1:$(CONSOLE_PORT)

reset-password:
	@$(ACCOUNT) reset-password

export:
	@$(ACCOUNT) export

import:
	@$(ACCOUNT) import "$(FILE)"
