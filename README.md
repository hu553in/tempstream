# tempstream

[![CI](https://github.com/hu553in/tempstream/actions/workflows/ci.yml/badge.svg)](https://github.com/hu553in/tempstream/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/hu553in/tempstream)](https://goreportcard.com/report/github.com/hu553in/tempstream)
[![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/hu553in/tempstream)](https://github.com/hu553in/tempstream/blob/main/go.mod)

- [License](./LICENSE)
- [How to contribute](./CONTRIBUTING.md)
- [Code of conduct](./CODE_OF_CONDUCT.md)

tempstream is a small video access service for a single live stream.

It provides:

- a Telegram bot for operators
- an HTTP service for watch links
- MediaMTX for RTMP ingest and HLS output
- Caddy as the public reverse proxy
- SQLite for link storage

The project is intentionally small. It is built as a pragmatic MVP: simple to run, predictable in production,
and easy to reason about.

---

## What it does

- Creates temporary or permanent watch links from Telegram
- Lists active links and disables them on demand
- Exposes a watch page at `/live/stream/{token}`
- Proxies HLS playback through `/play/*` after token validation
- Accepts RTMP publishing through MediaMTX on `live/stream`

A typical operator flow looks like this:

1. Start the stack.
2. Publish video to MediaMTX over RTMP.
3. Create a watch link from Telegram.
4. Open the generated URL in a browser.
5. Disable the link when access should end.

---

## Components

### Telegram bot

The bot is the only admin interface. It supports:

- `/new <duration>` for a temporary link
- `/newperm` for a permanent link
- `/active` to list active links
- `/status` to show stream status
- `/off ID` to disable a link by ID
- `/offlast` to disable the latest active link
- `/whoami` to show the current chat ID

Links returned by the bot include a direct disable action.

### HTTP service

The Go service exposes:

- `/healthz`
- `/live/stream/{token}`
- `/play/*`

`/live/stream/{token}` validates the token, sets a session cookie for playback, and renders the watch page.

`/play/*` validates the playback cookie again and proxies HLS traffic to MediaMTX.

### MediaMTX

MediaMTX:

- accepts RTMP publishing
- remuxes the stream to low-latency HLS
- restricts publishing with username/password authentication

### Caddy

Caddy is the public entry point and reverse-proxies incoming HTTP traffic to the Go service.

---

## Environment variables

| Name                        | Required        | Default       | Description                                                                                                             |
| --------------------------- | --------------- | ------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `HTTP_ADDR`                 | No              | `:8080`       | HTTP listen address for the Go service.                                                                                 |
| `BASE_URL`                  | Yes             | –             | Public base URL used in generated watch links.                                                                          |
| `DB_PATH`                   | No              | `./db.sqlite` | SQLite database path. In Docker Compose, `/data/db.sqlite` is typically used.                                           |
| `TELEGRAM_BOT_TOKEN`        | Yes             | –             | Telegram bot token.                                                                                                     |
| `ALLOWED_CHAT_IDS`          | Yes             | –             | Comma-separated list of Telegram chat IDs allowed to control the bot.                                                   |
| `MEDIAMTX_HLS_BASE_URL`     | Yes             | –             | Internal HLS base URL used by the Go service to probe stream health and proxy playback.                                 |
| `COOKIE_SECURE`             | No              | `true`        | Whether the playback cookie is marked `Secure`. Use `false` for plain local HTTP.                                       |
| `DEFAULT_LINK_TTL`          | No              | `1h`          | Default TTL used when a link is created with a zero duration internally.                                                |
| `LINK_TTL_OPTIONS`          | No              | `30m,1h,3h`   | Comma-separated list of temporary link durations shown in the Telegram bot. If empty, only permanent links are offered. |
| `TIME_ZONE`                 | No              | `UTC`         | IANA time zone used in bot responses, for example `UTC`, `Europe/Berlin`, or `Asia/Omsk`.                               |
| `LOG_LEVEL`                 | No              | `info`        | Log level for the Go service.                                                                                           |
| `MEDIAMTX_PUBLISH_USER`     | Yes for Compose | –             | Username required for RTMP publishing to MediaMTX.                                                                      |
| `MEDIAMTX_PUBLISH_PASSWORD` | Yes for Compose | –             | Password required for RTMP publishing to MediaMTX.                                                                      |

See [.env.example](./.env.example) for a complete example.

---

## Local run

### Docker Compose

1. Copy the example environment:

```bash
make ensure-env
```

2. Fill in:

- `TELEGRAM_BOT_TOKEN`
- `ALLOWED_CHAT_IDS`
- `MEDIAMTX_PUBLISH_USER`
- `MEDIAMTX_PUBLISH_PASSWORD`

3. For phone access on the same Wi-Fi, set `BASE_URL` to your machine's LAN IP:

```env
BASE_URL=http://192.168.1.42
COOKIE_SECURE=false
```

4. Start the stack:

```bash
make start
```

After startup:

- the Go service is available behind Caddy
- MediaMTX listens for RTMP on `:1935`
- MediaMTX serves HLS on `:8888`

### Go only

To run only the Go service:

```bash
make build
dist/tempstream
```

You still need a reachable SQLite path, a Telegram bot token, and a running MediaMTX instance.

---

## Publishing a stream

The configured MediaMTX path is `live/stream`:

```
rtmp://HOST:1935/live/stream?user=MEDIAMTX_PUBLISH_USER&pass=MEDIAMTX_PUBLISH_PASSWORD
```

Example:

```
rtmp://192.168.1.42:1935/live/stream?user=publisher&pass=secret
```

---

## Watching a stream

Create a link from Telegram, then open the returned URL in a browser:

```
http://HOST/live/stream/<token>
```

The watch page:

- validates the token
- sets a playback cookie
- loads HLS from `/play/index.m3u8`

If the link expires or is disabled, playback stops and the page shows a clear error state.

---

## Development

Useful commands:

```bash
make build
make fmt lint
make sqlc
```

Migrations are embedded into the binary with `go:embed`, while `sqlc` uses the same migration directory on disk
as the schema source.

---

## Stack

- Go 1.26
- SQLite
- Docker Compose
- [Caddy](https://caddyserver.com/)
- [MediaMTX](https://github.com/bluenviron/mediamtx)
- [chi](https://go-chi.io/)
- [goose](https://pressly.github.io/goose/)
- [sqlc](https://sqlc.dev/)
- [go-telegram/bot](https://github.com/go-telegram/bot)
- [caarlos0/env](https://github.com/caarlos0/env)
