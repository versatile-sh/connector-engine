SHELL    := /bin/bash
PROJECT  ?= connector-engine
PROFILE  ?= $(if $(wildcard selfhosted/selfhosted.mk),person,headless)
COMPOSE_FILE := $(if $(filter headless,$(PROFILE)),deploy/compose.yaml,selfhosted/deploy/compose.yaml)
COMPOSE  ?= docker compose -f $(COMPOSE_FILE) -p $(PROJECT)
IMAGE    ?= connector-engine:local
HEADLESS_IMAGE ?= connector-engine-headless:local
export CN_IMAGE := $(IMAGE)
export CN_HEADLESS_IMAGE := $(HEADLESS_IMAGE)
RUN_IMAGE := $(if $(filter headless,$(PROFILE)),$(HEADLESS_IMAGE),$(IMAGE))
ARGS     ?= --help
-include $(dir $(COMPOSE_FILE)).env
CONSOLE_PORT ?= 7465
export CONSOLE_PORT
ENGINE   := $(if $(filter headless,$(PROFILE)),engine-headless,engine)

.PHONY: up down logs ps cli admin build test vendor layering

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
	docker run --rm -i --network host -e CN_ENDPOINT=$(or $(CN_ENDPOINT),http://127.0.0.1:$(CONSOLE_PORT)/mcp) -e CN_AGENT_TOKEN --entrypoint /usr/local/bin/vsh $(RUN_IMAGE) $(ARGS)

admin:
	docker run --rm -i --network $(PROJECT)_default -e VSH_ADMIN_URL=http://$(ENGINE):8081 \
		-e VSH_ADMIN_TOKEN="$$($(COMPOSE) --profile $(PROFILE) exec -T postgres cat /run/connector-engine/operator-token)" \
		--entrypoint /usr/local/bin/vsh $(RUN_IMAGE) admin $(ARGS)

build: vendor
	$(COMPOSE) --profile $(PROFILE) build

vendor:
	@if command -v go >/dev/null 2>&1; then go mod vendor; fi

layering:
	scripts/check-layering.sh

test: layering
	go test -race -timeout 30m ./...
	@if [ -d selfhosted/console/web ]; then cd selfhosted/console/web && npm ci --no-audit --no-fund && npm run typecheck; fi
	go run ./cmd/connector-engine --config deploy/profiles/headless/connector-engine.yaml --adapters deploy/profiles/headless/adapters.yaml adapters check
	@if [ -d selfhosted ]; then go run ./selfhosted/cmd/connector-engine --config selfhosted/deploy/profiles/person/connector-engine.yaml --adapters selfhosted/deploy/profiles/person/adapters.yaml adapters check; fi

-include selfhosted/selfhosted.mk
