# tempstream

[![CI](https://github.com/hu553in/tempstream/actions/workflows/ci.yml/badge.svg)](https://github.com/hu553in/tempstream/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/hu553in/tempstream)](https://goreportcard.com/report/github.com/hu553in/tempstream)
[![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/hu553in/tempstream)](https://github.com/hu553in/tempstream/blob/main/go.mod)

tempstream is a video access service for a single live stream.

## What it does

- Creates temporary or permanent watch links from Telegram
- Lists active links and disables them on demand
- Exposes a watch page at `/live/stream/{token}`
- Proxies HLS playback through `/play/*` after token validation
- Accepts RTMP publishing through MediaMTX on `live/stream`
- Stores links in SQLite
- Runs as a Compose stack with the Go service, MediaMTX, and Caddy

A typical operator flow looks like this:

1. Start the stack.
2. Publish video to MediaMTX over RTMP.
3. Create a watch link from Telegram.
4. Open the generated URL in a browser.
5. Disable the link when access should end.

## Requirements

- Go 1.26+ for source builds
- Docker and Docker Compose for the full stack
- Telegram bot token
- Telegram chat IDs allowed to control the bot
- Public `BASE_URL` reachable by viewers
- RTMP publisher credentials for MediaMTX

## Setup

```bash
make ensure-env
```

Fill in these values in `.env`:

- `TELEGRAM_BOT_TOKEN`
- `ALLOWED_CHAT_IDS`
- `BASE_URL`
- `MEDIAMTX_PUBLISH_USER`
- `MEDIAMTX_PUBLISH_PASSWORD`

For phone access on the same Wi-Fi, set `BASE_URL` to the machine's LAN IP and disable secure
cookies for local HTTP:

```env
BASE_URL=http://192.168.1.42
COOKIE_SECURE=false
```

## Configuration

| Name                        | Required              | Default       | Description                                       |
| --------------------------- | --------------------- | ------------- | ------------------------------------------------- |
| `HTTP_ADDR`                 | No                    | `:8080`       | HTTP listen address for the Go service            |
| `HTTP_TRUSTED_PROXY_COUNT`  | No                    | `1`           | Trusted reverse proxy count for `X-Forwarded-For` |
| `BASE_URL`                  | Yes                   | -             | Public base URL used in generated watch links     |
| `DB_PATH`                   | No                    | `./db.sqlite` | SQLite database path                              |
| `TELEGRAM_BOT_TOKEN`        | Yes                   | -             | Telegram bot token                                |
| `ALLOWED_CHAT_IDS`          | Yes                   | -             | Comma-separated Telegram chat IDs                 |
| `MEDIAMTX_HLS_BASE_URL`     | Yes                   | -             | Internal HLS base URL for stream probing/playback |
| `COOKIE_SECURE`             | No                    | `true`        | Marks playback cookies as `Secure`                |
| `DEFAULT_LINK_TTL`          | No                    | `1h`          | Default temporary link duration                   |
| `LINK_TTL_OPTIONS`          | No                    | `30m,1h,3h`   | Temporary link durations shown in Telegram        |
| `TIME_ZONE`                 | No                    | `UTC`         | IANA time zone used in bot responses              |
| `LOG_LEVEL`                 | No                    | `info`        | Log level for the Go service                      |
| `MEDIAMTX_PUBLISH_USER`     | Yes in Docker Compose | -             | RTMP publish username                             |
| `MEDIAMTX_PUBLISH_PASSWORD` | Yes in Docker Compose | -             | RTMP publish password                             |

See `.env.example` for a complete example.

## Usage

Service commands:

```bash
make start
make stop
make restart
```

The Compose stack uses:

- `tempstream` - Go HTTP service and Telegram bot
- `mediamtx` - RTMP ingest and HLS output
- `caddy` - public reverse proxy

The bot is the operator interface:

- `/new <duration>` for a temporary link
- `/newperm` for a permanent link
- `/active` to list active links
- `/status` to show stream status
- `/off ID` to disable a link by ID
- `/offlast` to disable the latest active link
- `/whoami` to show the current chat ID

Links returned by the bot include a direct disable action.

## Streaming

The configured MediaMTX path is `live/stream`:

```text
rtmp://HOST:1935/live/stream?user=MEDIAMTX_PUBLISH_USER&pass=MEDIAMTX_PUBLISH_PASSWORD
```

Example:

```text
rtmp://192.168.1.42:1935/live/stream?user=publisher&pass=secret
```

Create a link from Telegram, then open the returned URL:

```text
http://HOST/live/stream/<token>
```

## Runtime behavior

- The Go service exposes `/healthz`, `/live/stream/{token}`, and `/play/*`
- `/live/stream/{token}` validates the token, sets a playback cookie, and renders the watch page
- `/play/*` validates the playback cookie again and proxies HLS traffic to MediaMTX
- MediaMTX accepts RTMP and remuxes the stream to low-latency HLS
- Caddy is the public entry point and reverse-proxies traffic to the Go service
- In Docker Compose, SQLite data lives in the `tempstream_data` volume at `/data/db.sqlite`
- If a link expires or is disabled, playback stops and the page shows a clear error state

Without Docker, build only the Go service:

```bash
make build
dist/tempstream
```

A reachable SQLite path, a Telegram bot token, and a running MediaMTX instance are still required.

## Development

```bash
make install-deps
make build
make check
```

Focused checks:

```bash
make fmt
make lint
make check-deps
```

Generated SQL:

```bash
make sqlc
```

Migrations are embedded into the binary with `go:embed`, while `sqlc` uses the same migration
directory on disk as the schema source.
