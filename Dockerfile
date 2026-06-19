# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26.1
ARG DEBIAN_VERSION=bookworm

FROM golang:${GO_VERSION}-${DEBIAN_VERSION} AS builder
WORKDIR /src

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
  go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN --mount=type=cache,target=/root/.cache/go-build \
  CGO_ENABLED=0 GOFLAGS="-buildvcs=false" \
  go build -trimpath -ldflags="-s -w" -o /dist/tempstream ./cmd/tempstream

FROM debian:${DEBIAN_VERSION}-slim AS runner

RUN --mount=type=cache,target=/var/cache/apt \
  --mount=type=cache,target=/var/lib/apt/lists \
  apt-get update && \
  apt-get install -y --no-install-recommends \
  ca-certificates \
  curl

RUN groupadd --gid 10001 app \
  && useradd --uid 10001 --gid 10001 -M app \
  && install -d -m 0750 -o app -g app /app

COPY --from=builder --chown=app:app /dist/tempstream /app/tempstream

WORKDIR /app
USER app
EXPOSE 8080
ENTRYPOINT ["/app/tempstream"]
