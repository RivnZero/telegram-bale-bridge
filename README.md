# Telegram–Bale Bridge

A bidirectional bridge between **one Telegram chat** and **one Bale chat**. Text, photos, videos, documents, audio, voice messages, GIFs/animations, captions, and photo/video albums are forwarded in both directions with retries, SQLite-based duplicate prevention, and graceful shutdown.

Written in **Go 1.26**. The Telegram side uses [`go-telegram/bot`](https://github.com/go-telegram/bot); the Bale side is a small custom HTTP client over Bale's Telegram-compatible Bot API — no third-party Bale SDK.

---

## How it works

```
┌─────────────┐   getUpdates    ┌──────────────────┐
│  Telegram   │ ◄────────────── │  Telegram poller │
│  Bot API    │                 └────────┬─────────┘
└─────────────┘                          │ normalize
                                         ▼
                              ┌──────────────────────┐
                              │   forward worker     │
                              │  (queue + retry)     │
                              └──────────┬───────────┘
                                         │ download → temp file
                                         │ sendMediaGroup / send*
                                         ▼
                              ┌──────────────────────┐
                              │   Bale HTTP client   │
                              └──────────┬───────────┘
                                         │ HTTPS
                                         ▼
┌─────────────┐                  ┌──────────────────┐
│  Bale       │ ──────────────►  │  Bale poller     │
│  Bot API    │   getUpdates     └──────────────────┘
└─────────────┘
```

Both directions run the same pipeline:

1. **Receive** — long polling (`getUpdates`, Telegram via `go-telegram/bot`).
2. **Filter** — only messages from the configured source chat; messages sent by the bridge bots are ignored; unsupported types are skipped.
3. **Normalize** — both platforms' message shapes are converted into one internal `BridgeMessage`.
4. **Deduplicate** — SQLite unique key (`source platform + chat + message id`; albums key on `media_group_id`) prevents re-forwarding after retries or restarts.
5. **Queue** — one buffered channel per direction (capacity `QUEUE_SIZE`); one worker per direction preserves message order.
6. **Download → re-upload** — media is streamed to a per-message temp directory, then re-uploaded to the destination (file ids are not transferable between bots).
7. **Retry** — transient failures (HTTP 429/5xx, network timeouts/resets) retry after 1 s, 3 s, 10 s, honoring `retry_after`. Permanent errors (bad request, forbidden, not found) are recorded and skipped.
8. **Record** — the delivery lands in SQLite; temp files are removed on every exit path.

### Albums

Messages sharing a `media_group_id` are buffered in memory for `ALBUM_DELAY` (default 700 ms), then sent as a single media group, ordered by source message id, with the caption on the first item. Photo/video albums are supported.

### Loop prevention

Three layers: bot-sender check (ignore the bridge's own messages), a 5-minute in-memory tracker of recently sent destination message ids, and SQLite as the final backstop.

---

## Quick start

### 1. Create the bots

- Telegram: talk to [@BotFather](https://t.me/botfather) → new bot → copy the token.
- Bale: talk to [@botfather](https://ble.ir/botfather) → new bot → copy the token.

> Chat ids must be **numeric**. To find a chat id, message your bot and check the update via `https://api.telegram.org/bot<TOKEN>/getUpdates` (Telegram) or `https://tapi.bale.ai/bot<TOKEN>/getUpdates` (Bale).

### 2. Configure

```bash
cp .env.example .env
```

Fill in the values (see [Configuration](#configuration)).

### 3. Run locally

```bash
go build -o bridge ./cmd/bridge
./bridge
```

### 4. Or run with Docker

```bash
cp .env.example .env
docker compose up -d
docker compose logs -f
```

---

## Configuration

All configuration is read from environment variables (`.env` is loaded automatically by `docker compose`).

| Variable | Default | Required | Description |
|---|---|---|---|
| `TELEGRAM_BOT_TOKEN` | — | yes | Telegram bot token |
| `TELEGRAM_CHAT_ID` | — | yes | Telegram source/target chat (numeric id) |
| `BALE_BOT_TOKEN` | — | yes | Bale bot token |
| `BALE_CHAT_ID` | — | yes | Bale source/target chat (numeric id) |
| `BALE_API_BASE_URL` | `https://tapi.bale.ai` | no | Bale Bot API endpoint override |
| `TELEGRAM_API_BASE_URL` | `https://api.telegram.org` | no | Telegram Bot API endpoint override (testing/self-hosted) |
| `BRIDGE_DIRECTION` | `bidirectional` | no | `telegram-to-bale`, `bale-to-telegram`, or `bidirectional` |
| `DATABASE_PATH` | `./data/bridge.db` | no | SQLite delivery ledger |
| `TEMP_DIRECTORY` | `./data/tmp` | no | Media temp directory (wiped at startup) |
| `QUEUE_SIZE` | `100` | no | Per-direction queue capacity |
| `ALBUM_DELAY` | `700ms` | no | Album collection window (`700ms`, `2s`, or bare milliseconds) |
| `LOG_LEVEL` | `INFO` | no | `DEBUG`, `INFO`, `WARN`, `ERROR` |

Invalid values produce a clear error and exit code 1.

---

## Supported message types

| Type | Telegram → Bale | Bale → Telegram |
|---|---|---|
| Text | ✓ | ✓ |
| Photo (+ caption) | ✓ | ✓ |
| Video (+ caption) | ✓ | ✓ |
| Document (+ caption) | ✓ | ✓ |
| Audio (+ caption) | ✓ | ✓ |
| Voice (OGG) | ✓* | ✓ |
| GIF / animation | ✓ | ✓ |
| Photo albums | ✓ | ✓ |
| Video albums | ✓ | ✓ |
| Mixed photo/video albums | ✓ | ✓ |

\* Bale renders OGG voice up to 1 MB as a voice message; larger voice files are sent as documents.

Not forwarded (ignored): stickers, contacts, locations, poll/video-note messages, service messages, and anything with a file over the download limit.

---

## Platform limits

| Item | Telegram | Bale |
|---|---|---|
| File download (bot API) | 20 MB | 20 MB |
| File upload (bot API) | 50 MB | 50 MB |
| Photo upload (bot API) | 50 MB | 10 MB |
| Voice message | any size | OGG ≤ 1 MB (1–20 MB sent as file) |
| Audio format | any | MP3 / M4A |
| Video format | any | MPEG-4 |

The bridge enforces a **20 MB per-file cap** (the download limit on both platforms); larger files are recorded as `file_too_large` and skipped. Photos over Bale's 10 MB cap are sent as documents.

---

## Behavior details

- **Ordering** — one worker per direction consumes messages sequentially.
- **Dedup** — the `deliveries` table has a unique constraint on `(source_platform, source_chat_id, source_key, destination_platform)`; restarts never re-forward. Rows left mid-flight at crash time are marked `failed` on startup.
- **Queue full** — new messages are rejected and recorded as `queue_full`; the worker keeps running.
- **Shutdown** — SIGINT/SIGTERM stops polling, flushes buffered albums, drains the workers, removes temp files, and closes the database.
- **Logging** — structured `log/slog`; bot tokens are never logged.

---

## Project layout

```text
cmd/bridge/          entry point, wiring, graceful shutdown
internal/bridge/     normalized model, worker, album buffer, retry, loop tracker
internal/config/     environment configuration and validation
internal/bale/       minimal Bale Bot API client, poller, sender, receiver
internal/telegram/   go-telegram/bot wrapper: polling, normalize, send, download
internal/storage/    SQLite delivery ledger
migrations/          embedded SQL migration
```

---

## Development

```bash
go build ./...
go vet ./...
go test ./...
```

The test suite includes an end-to-end test that runs the full wiring against mock Telegram and Bale HTTP servers — no real tokens needed.

---

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `invalid configuration: ...` | Missing/incorrect env vars. Fix `.env` and restart. |
| `telegram getMe: ...` / `bale getMe: ...` | Token rejected or unreachable; verify the token and network. |
| Messages forwarded in only one direction | Check `BRIDGE_DIRECTION`. |
| Nothing forwarded at all | Confirm the source chat id matches the chat your bot actually reads (`getUpdates`). |
| `file_too_large` in the ledger | Media exceeds the 20 MB platform download limit. |
| Bale rate limits | Bale throttles engagement-based; honor `retry_after` (built in). For bulk traffic use Bale's business API (`https://tapi.bale.ai/business/bot<token>/...`). |

---

## Known limitations (v0.1)

- One chat pair only; no multi-mapping.
- Numeric chat ids only (no `@username` aliases).
- An album partially collected at crash time is lost (buffer is in-memory only).
- Message edits and deletions are not synchronized.
- Webhooks are not supported (long polling only).

---

## License

MIT
