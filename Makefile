.DEFAULT_GOAL := check

BUILD_DIR ?= ./dist

COMPOSE_FILES ?= -f compose.yaml -f compose.override.dev.yaml

PRETTIER := bunx prettier -u
ACTIONLINT := bunx github-actionlint
TAPLO := bunx @taplo/cli
PREK ?= prek

.PHONY: ensure-build-dir
ensure-build-dir:
	mkdir -p $(BUILD_DIR)

.PHONY: ensure-env
ensure-env:
	if [ ! -f .env ]; then cp .env.example .env; fi

.PHONY: check-workflows
check-workflows:
	$(ACTIONLINT)

.PHONY: check-renovate
check-renovate:
	bunx --package renovate renovate-config-validator --strict --no-global renovate.json

.PHONY: check-hooks
check-hooks:
	$(PREK) validate-config prek.toml

.PHONY: check
check: lint check-hooks build check-generated check-deps check-vulns check-config check-renovate check-workflows

.PHONY: check-fix
check-fix: lint-fix
	$(MAKE) check

.PHONY: install-deps
install-deps:
	go mod download

.PHONY: lint
lint:
	$(PRETTIER) -c .
	$(TAPLO) fmt --check
	golangci-lint fmt --diff
	golangci-lint run

.PHONY: lint-fix
lint-fix:
	$(PRETTIER) -w .
	$(TAPLO) fmt
	golangci-lint fmt
	golangci-lint run --fix

.PHONY: check-deps
check-deps: install-deps
	go mod tidy -diff
	go mod verify

.PHONY: check-generated
check-generated: install-deps
	go tool sqlc diff

.PHONY: check-vulns
check-vulns: install-deps
	go tool govulncheck ./...

.PHONY: check-config
check-config:
	docker compose $(COMPOSE_FILES) config --quiet --no-interpolate --no-env-resolution
	@image=$$(docker compose -f compose.yaml config --images --no-interpolate --no-env-resolution | grep '^caddy:' | head -1); \
		test -n "$$image"; \
		docker run --rm --network none --entrypoint /bin/sh \
			-e BASE_URL=tempstream.example.com \
			-v "$(CURDIR)/Caddyfile:/etc/caddy/Caddyfile:ro" \
			"$$image" -ec \
			'caddy fmt --diff /etc/caddy/Caddyfile >/dev/null && caddy validate --config /etc/caddy/Caddyfile'

.PHONY: build
build: ensure-build-dir install-deps
	CGO_ENABLED=0 GOFLAGS="-buildvcs=false" \
	go build -trimpath -ldflags="-s -w" -o $(BUILD_DIR)/tempstream ./cmd/tempstream

.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)

.PHONY: sqlc
sqlc:
	go tool sqlc generate

.PHONY: start
start: ensure-env
	docker compose $(COMPOSE_FILES) \
	up -d --build --wait --remove-orphans

.PHONY: stop
stop: ensure-env
	docker compose $(COMPOSE_FILES) \
	down --remove-orphans

.PHONY: restart
restart: stop start
