SHELL    := /bin/bash
PROJECT  ?= connector-engine
COMPOSE  ?= docker compose -f deploy/compose.yaml -p $(PROJECT)
CNCTL    ?= bin/cnctl
PROFILE  ?= person
IMAGE    ?= connector-engine:local
export CN_IMAGE := $(IMAGE)
ARGS     ?= --help
-include deploy/.env
CONSOLE_PORT ?= 7465
export CONSOLE_PORT
ENGINE   := $(if $(filter headless,$(PROFILE)),engine-headless,engine)

KEYS     = COMPOSE="$(COMPOSE) --profile person" CNCTL="$(CNCTL)" PROJECT="$(PROJECT)" scripts/keychain.sh

.PHONY: up down logs ps cli admin build test vendor check cnctl unlock keychain-status keychain-migrate keychain-revert keychain-recovery keychain-import notify

up: vendor
	$(COMPOSE) --profile $(PROFILE) up -d --build
	@if [ "$(PROFILE)" = person ]; then $(MAKE) --no-print-directory unlock; fi
	@if [ "$(PROFILE)" = person ]; then echo "console: http://127.0.0.1:$(CONSOLE_PORT)   agent endpoint: http://127.0.0.1:$(CONSOLE_PORT)/mcp"; fi

down:
	$(COMPOSE) --profile $(PROFILE) down

logs:
	$(COMPOSE) --profile $(PROFILE) logs -f

ps:
	$(COMPOSE) --profile $(PROFILE) ps

cli:
	docker run --rm -i --network host -e CN_ENDPOINT=$(or $(CN_ENDPOINT),http://127.0.0.1:$(CONSOLE_PORT)/mcp) -e CN_AGENT_TOKEN --entrypoint /usr/local/bin/cnctl $(IMAGE) $(ARGS)

admin:
	docker run --rm -i --network $(PROJECT)_default -e CNCTL_URL=http://$(ENGINE):8081 \
		-e CNCTL_TOKEN="$$($(COMPOSE) --profile $(PROFILE) exec -T postgres cat /run/connector-engine/operator-token)" \
		--entrypoint /usr/local/bin/cnctl $(IMAGE) admin $(ARGS)

build: vendor
	$(COMPOSE) --profile $(PROFILE) build

vendor:
	@if command -v go >/dev/null 2>&1; then go mod vendor; fi

test:
	go test -race ./...
	cd console/web && npm ci --no-audit --no-fund && npm run typecheck
	go run ./cmd/connector-engine --config deploy/profiles/person/connector-engine.yaml --adapters deploy/profiles/person/adapters.yaml adapters check
	go run ./cmd/connector-engine --config deploy/profiles/headless/connector-engine.yaml --adapters deploy/profiles/headless/adapters.yaml adapters check

cnctl:
	@if command -v go >/dev/null 2>&1; then go build -o bin/cnctl ./cmd/cnctl; else echo "no Go on this host: the keychain is skipped" >&2; fi

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
	scripts/console-check.sh

notify: cnctl
	$(CNCTL) notify --console http://127.0.0.1:$(CONSOLE_PORT)
