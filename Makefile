COMPOSE  ?= docker compose -f deploy/compose.yaml
PROFILE  ?= person
IMAGE    ?= connector-engine:local
ARGS     ?= --help
CONSOLE_PORT ?= 7465
export CONSOLE_PORT
ENGINE   := $(if $(filter headless,$(PROFILE)),engine-headless,engine)

.PHONY: up down logs ps cli admin build test vendor check

up: vendor
	$(COMPOSE) --profile $(PROFILE) up -d --build
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
	docker run --rm -i --network connector-engine_default -e CNCTL_URL=http://$(ENGINE):8081 \
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

check: vendor
	scripts/console-check.sh
