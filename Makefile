BUILD_DIR ?= ./dist

COMPOSE_FILES ?= -f docker-compose.yml -f docker-compose.override.dev.yml

.PHONY: ensure-env
ensure-env:
	if [ ! -f .env ]; then cp .env.example .env; fi

.PHONY: pre-commit
pre-commit: build lint check-deps

.PHONY: check
check: build fmt lint check-deps

.PHONY: install-deps
install-deps:
	go mod download

.PHONY: fmt
fmt:
	golangci-lint fmt

.PHONY: lint
lint:
	golangci-lint run

.PHONY: check-deps
check-deps: install-deps
	go tool govulncheck ./...

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
