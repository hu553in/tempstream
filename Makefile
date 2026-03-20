BUILD_DIR ?= ./dist

GOLANGCI_LINT_CONFIG_URL ?= https://raw.githubusercontent.com/maratori/golangci-lint-config/refs/heads/main/.golangci.yml

COMPOSE_FILES ?= -f docker-compose.yml -f docker-compose.override.dev.yml

.PHONY: ensure-env
ensure-env:
	if [ ! -f .env ]; then cp .env.example .env; fi

.PHONY: pre-commit
pre-commit: lint build

.PHONY: check
check: fmt lint build

.PHONY: install-deps
install-deps:
	go mod download

.PHONY: update-lint-config
update-lint-config:
	@tmp=$$(mktemp); \
	if curl -fsSL $(GOLANGCI_LINT_CONFIG_URL) -o "$$tmp"; then \
		mv "$$tmp" .golangci.yaml && \
		sed -i '' "s|github.com/my/project|github.com/hu553in/tempstream|g" .golangci.yaml; \
	else \
		rm -f "$$tmp"; \
		exit 1; \
	fi

.PHONY: fmt
fmt:
	golangci-lint fmt

.PHONY: lint
lint:
	golangci-lint run

.PHONY: build
build: install-deps
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
