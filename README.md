# Telegram–Bale Bridge

<p align="center">
  <img src="assets/logo.webp" alt="Telegram–Bale Bridge logo" width="180">
</p>

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-deduplication-003B57?logo=sqlite&logoColor=white)
![License](https://img.shields.io/badge/license-MIT-green)

A reliable, bidirectional message bridge between **one Telegram chat** and **one Bale chat**.

Forward text, photos, videos, documents, audio files, voice messages, GIFs, captions, and photo/video albums in both directions—with retries, ordered delivery, SQLite-backed duplicate prevention, and graceful shutdown.

Built with **Go 1.26**.

* Telegram integration: [`go-telegram/bot`](https://github.com/go-telegram/bot)
* Bale integration: custom HTTP client over Bale's Telegram-compatible Bot API
* No third-party Bale SDK
* No external database required

> [!IMPORTANT]
> The current version supports exactly **one Telegram chat ↔ one Bale chat** mapping. Chat IDs must be numeric.

---

## Features

* Bidirectional Telegram ↔ Bale forwarding
* Text and media support
* Photo, video, and mixed media albums
* Captions preserved during forwarding
* Ordered delivery with one worker per direction
* Automatic retries for transient failures
* `retry_after` support for rate limits
* SQLite-based duplicate prevention
* Protection against forwarding loops
* Automatic temporary-file cleanup
* Graceful SIGINT/SIGTERM shutdown
* Structured logging with `log/slog`
* Docker and Docker Compose support
* Bot tokens are never written to logs

---

## Table of contents

* [Architecture](#architecture)
* [How it works](#how-it-works)
* [Quick start](#quick-start)
* [Configuration](#configuration)
* [Supported message types](#supported-message-types)
* [Platform limits](#platform-limits)
* [Albums](#albums)
* [Duplicate and loop prevention](#duplicate-and-loop-prevention)
* [Delivery behavior](#delivery-behavior)
* [Project structure](#project-structure)
* [Development](#development)
* [Troubleshooting](#troubleshooting)
* [Known limitations](#known-limitations)
* [License](#license)

---

## Architecture

```text
                         Telegram → Bale

┌─────────────┐  getUpdates   ┌──────────────────┐
│  Telegram   │ ◄──────────── │ Telegram poller  │
│   Bot API   │               └────────┬─────────┘
└─────────────┘                        │
                                       │ filter + normalize
                                       ▼
                            ┌──────────────────────┐
                            │    Forward worker    │
                            │  queue + dedup +     │
                            │       retries        │
                            └──────────┬───────────┘
                                       │
                                       │ download to temp file
                                       │ re-upload via send*
                                       ▼
                            ┌──────────────────────┐
                            │   Bale HTTP client   │
                            └──────────┬───────────┘
                                       │
                                       ▼
                            ┌──────────────────────┐
                            │    Bale Bot API      │
                            └──────────────────────┘
```

The reverse direction uses the same pipeline:

```text
Bale poller → filter → normalize → deduplicate → queue → worker
            → download → re-upload → Telegram Bot API
```

---

## How it works

Each direction runs independently using the same processing pipeline.

### 1. Receive

Messages are received through long polling:

* Telegram through `go-telegram/bot`
* Bale through the custom `getUpdates` client

### 2. Filter

The bridge accepts only messages from the configured source chat.

It ignores:

* Messages sent by the bridge bots
* Unsupported message types
* Service messages
* Files larger than the configured platform limit

### 3. Normalize

Telegram and Bale messages are converted into a shared internal `BridgeMessage` model.

This keeps the forwarding logic independent from platform-specific message structures.

### 4. Deduplicate

Before forwarding, the bridge checks the SQLite delivery ledger.

Regular messages use:

```text
source platform + source chat id + source message id + destination platform
```

Albums use their `media_group_id` as the source key.

This prevents duplicate forwarding after retries, crashes, or restarts.

### 5. Queue

Each direction has its own buffered channel.

```text
Telegram → Bale queue
Bale → Telegram queue
```

A single worker consumes each queue, preserving message order within that direction.

The queue capacity is controlled by `QUEUE_SIZE`.

### 6. Download and re-upload

Media file IDs cannot be transferred between different bots or platforms.

The bridge therefore:

1. Downloads the source file
2. Streams it into a per-message temporary directory
3. Uploads it to the destination
4. Removes the temporary files

Temporary files are removed on every exit path, including failed deliveries.

### 7. Retry

Transient failures are retried using the following delays:

```text
1 second → 3 seconds → 10 seconds
```

The bridge retries failures such as:

* HTTP `429`
* HTTP `5xx`
* Network timeouts
* Connection resets
* Temporary transport errors

When the platform returns `retry_after`, that value is honored.

Permanent errors such as bad requests, forbidden access, and missing resources are recorded and skipped.

### 8. Record

The final delivery status is written to SQLite.

This includes successful deliveries and terminal failures such as:

* `file_too_large`
* `queue_full`
* Permanent API errors

---

## Quick start

### Requirements

* Go 1.26 or newer
* A Telegram bot token
* A Bale bot token
* Numeric chat IDs for both chats

Docker is optional.

### 1. Create the bots

#### Telegram

Open [@BotFather](https://t.me/botfather), create a new bot, and copy its token.

#### Bale

Open [@botfather](https://ble.ir/botfather), create a new bot, and copy its token.

### 2. Find the chat IDs

Send a message to each bot, then inspect its latest updates.

Telegram:

```text
https://api.telegram.org/bot<TOKEN>/getUpdates
```

Bale:

```text
https://tapi.bale.ai/bot<TOKEN>/getUpdates
```

Find the numeric value under:

```json
{
  "message": {
    "chat": {
      "id": 123456789
    }
  }
}
```

> [!NOTE]
> Chat usernames such as `@example` are not supported. Use numeric chat IDs.

### 3. Configure the bridge

Copy the example environment file:

```bash
cp .env.example .env
```

Then fill in the required values:

```dotenv
TELEGRAM_BOT_TOKEN=your-telegram-token
TELEGRAM_CHAT_ID=your-telegram-chat-id

BALE_BOT_TOKEN=your-bale-token
BALE_CHAT_ID=your-bale-chat-id
```

See [Configuration](#configuration) for all available options.

### 4. Run locally

```bash
go build -o bridge ./cmd/bridge
./bridge
```

### 5. Run with Docker

```bash
cp .env.example .env
docker compose up -d
```

Follow the logs:

```bash
docker compose logs -f
```

Stop the bridge:

```bash
docker compose down
```

---

## Configuration

All configuration is read from environment variables.

When using Docker Compose, `.env` is loaded automatically.

| Variable                |                    Default | Required | Description                                                       |
| ----------------------- | -------------------------: | :------: | ----------------------------------------------------------------- |
| `TELEGRAM_BOT_TOKEN`    |                          — |    Yes   | Telegram bot token                                                |
| `TELEGRAM_CHAT_ID`      |                          — |    Yes   | Telegram source and destination chat ID                           |
| `BALE_BOT_TOKEN`        |                          — |    Yes   | Bale bot token                                                    |
| `BALE_CHAT_ID`          |                          — |    Yes   | Bale source and destination chat ID                               |
| `BALE_API_BASE_URL`     |     `https://tapi.bale.ai` |    No    | Bale Bot API base URL                                             |
| `TELEGRAM_API_BASE_URL` | `https://api.telegram.org` |    No    | Telegram Bot API base URL, useful for testing or self-hosted APIs |
| `BRIDGE_DIRECTION`      |            `bidirectional` |    No    | Enabled forwarding direction                                      |
| `DATABASE_PATH`         |         `./data/bridge.db` |    No    | SQLite delivery ledger path                                       |
| `TEMP_DIRECTORY`        |               `./data/tmp` |    No    | Temporary media directory                                         |
| `QUEUE_SIZE`            |                      `100` |    No    | Queue capacity for each direction                                 |
| `ALBUM_DELAY`           |                    `700ms` |    No    | Time window used to collect album items                           |
| `LOG_LEVEL`             |                     `INFO` |    No    | Logging verbosity                                                 |

### Bridge direction

`BRIDGE_DIRECTION` accepts one of the following values:

```text
telegram-to-bale
bale-to-telegram
bidirectional
```

### Album delay

`ALBUM_DELAY` accepts Go duration values:

```dotenv
ALBUM_DELAY=700ms
ALBUM_DELAY=2s
```

Bare millisecond values are also accepted:

```dotenv
ALBUM_DELAY=700
```

### Log levels

```text
DEBUG
INFO
WARN
ERROR
```

Invalid or missing required values produce a clear startup error and exit code `1`.

---

## Supported message types

| Message type            | Telegram → Bale | Bale → Telegram |
| ----------------------- | :-------------: | :-------------: |
| Text                    |        ✅        |        ✅        |
| Photo                   |        ✅        |        ✅        |
| Photo with caption      |        ✅        |        ✅        |
| Video                   |        ✅        |        ✅        |
| Video with caption      |        ✅        |        ✅        |
| Document                |        ✅        |        ✅        |
| Document with caption   |        ✅        |        ✅        |
| Audio                   |        ✅        |        ✅        |
| Audio with caption      |        ✅        |        ✅        |
| Voice message           |        ✅*       |        ✅        |
| GIF / animation         |        ✅        |        ✅        |
| Photo album             |        ✅        |        ✅        |
| Video album             |        ✅        |        ✅        |
| Mixed photo/video album |        ✅        |        ✅        |

* Bale renders OGG files up to 1 MB as voice messages. Larger OGG voice files are sent as documents.

### Unsupported message types

The following messages are ignored:

* Stickers
* Contacts
* Locations
* Polls
* Video notes
* Service messages
* Unsupported message variants
* Files larger than the 20 MB download limit

---

## Platform limits

| Item                  |             Telegram |           Bale |
| --------------------- | -------------------: | -------------: |
| Bot API file download |                20 MB |          20 MB |
| Bot API file upload   |                50 MB |          50 MB |
| Photo upload          |                50 MB |          10 MB |
| Voice message         |   Any supported size | OGG up to 1 MB |
| Audio formats         | Any supported format |      MP3 / M4A |
| Video formats         | Any supported format |         MPEG-4 |

The bridge applies a **20 MB per-file maximum**, because that is the download limit shared by both platforms.

Files above this limit are:

1. Not downloaded
2. Recorded as `file_too_large`
3. Skipped without stopping the worker

Photos larger than Bale's 10 MB photo limit are sent as documents.

Bale voice behavior:

```text
OGG ≤ 1 MB       → voice message
OGG > 1 MB       → document
File > 20 MB     → skipped
```

---

## Albums

Messages sharing the same `media_group_id` are treated as one album.

The bridge:

1. Buffers album items in memory
2. Waits for `ALBUM_DELAY`
3. Sorts items by source message ID
4. Places the caption on the first item
5. Sends the collection as one media group

Supported album types:

* Photo albums
* Video albums
* Mixed photo/video albums

The default collection window is:

```dotenv
ALBUM_DELAY=700ms
```

Increasing the value may help when album items arrive slowly, but also adds forwarding latency.

> [!WARNING]
> Album buffers are stored only in memory. An album that is partially collected during a crash cannot be recovered.

---

## Duplicate and loop prevention

The bridge uses three independent protection layers.

### 1. Bot sender filtering

Messages sent by either bridge bot are ignored by the corresponding poller.

### 2. Recent-message tracker

Destination message IDs created by the bridge are stored in memory for five minutes.

If one of those messages appears in incoming updates, it is ignored.

### 3. SQLite delivery ledger

SQLite acts as the final backstop.

The `deliveries` table uses a unique constraint on:

```text
source_platform
source_chat_id
source_key
destination_platform
```

For regular messages, `source_key` is the message ID.

For albums, `source_key` is the `media_group_id`.

This prevents messages from being forwarded again after:

* Worker retries
* Poller restarts
* Process restarts
* Duplicate platform updates

---

## Delivery behavior

### Ordering

Each direction uses one sequential worker.

This preserves source message order within that direction.

Telegram → Bale and Bale → Telegram run independently and concurrently.

### Queue saturation

Each direction has a buffered queue with a capacity defined by `QUEUE_SIZE`.

When the queue is full:

* The new message is rejected
* Its status is recorded as `queue_full`
* The worker continues processing existing messages
* The process remains running

### Crash recovery

Deliveries left in an in-progress state during a crash are marked as failed during the next startup.

Completed deliveries remain deduplicated.

### Graceful shutdown

On `SIGINT` or `SIGTERM`, the bridge:

1. Stops both pollers
2. Flushes buffered albums
3. Closes the input queues
4. Drains active workers
5. Removes temporary files
6. Closes the SQLite database

### Temporary files

Media files are stored in a separate temporary directory for each message.

The configured temporary root is wiped at startup:

```dotenv
TEMP_DIRECTORY=./data/tmp
```

Temporary files are removed after both successful and failed forwarding attempts.

### Logging

The bridge uses Go's structured `log/slog` package.

Supported levels:

```text
DEBUG
INFO
WARN
ERROR
```

Bot tokens and other secrets are never logged.

---

## Project structure

```text
.
├── cmd/
│   └── bridge/
│       └── ...                 Application entry point and wiring
│
├── internal/
│   ├── bale/
│   │   └── ...                 Bale API client, polling, sending, downloading
│   │
│   ├── bridge/
│   │   └── ...                 Message model, workers, albums, retries, loops
│   │
│   ├── config/
│   │   └── ...                 Environment loading and validation
│   │
│   ├── storage/
│   │   └── ...                 SQLite delivery ledger
│   │
│   └── telegram/
│       └── ...                 Telegram integration and normalization
│
├── migrations/
│   └── ...                     Embedded SQLite migrations
│
├── .env.example
├── compose.yml
├── Dockerfile
├── go.mod
└── README.md
```

| Path                 | Responsibility                                                             |
| -------------------- | -------------------------------------------------------------------------- |
| `cmd/bridge/`        | Entry point, dependency wiring, and graceful shutdown                      |
| `internal/bridge/`   | Shared model, queues, workers, retries, album buffering, and loop tracking |
| `internal/config/`   | Environment configuration and validation                                   |
| `internal/bale/`     | Minimal Bale Bot API client                                                |
| `internal/telegram/` | `go-telegram/bot` integration                                              |
| `internal/storage/`  | SQLite delivery ledger                                                     |
| `migrations/`        | Embedded database schema migrations                                        |

---

## Development

Build all packages:

```bash
go build ./...
```

Run static analysis:

```bash
go vet ./...
```

Run the test suite:

```bash
go test ./...
```

Run all validation commands:

```bash
go build ./... &&
go vet ./... &&
go test ./...
```

---

## Troubleshooting

| Symptom                             | Likely cause                                     | Suggested fix                                          |
| ----------------------------------- | ------------------------------------------------ | ------------------------------------------------------ |
| `invalid configuration: ...`        | Missing or invalid environment variables         | Correct `.env` and restart the bridge                  |
| `telegram getMe: ...`               | Invalid Telegram token or network failure        | Verify the token and Telegram API connectivity         |
| `bale getMe: ...`                   | Invalid Bale token or network failure            | Verify the token and Bale API connectivity             |
| Messages move in only one direction | Incorrect `BRIDGE_DIRECTION`                     | Set it to `bidirectional` or the intended direction    |
| No messages are forwarded           | Source chat ID does not match                    | Inspect `getUpdates` and use the exact numeric chat ID |
| Bot receives no group messages      | Bot permissions or privacy settings              | Add the bot correctly and review its group permissions |
| `file_too_large` appears in SQLite  | File exceeds the 20 MB download limit            | Send a smaller file or share an external link          |
| `queue_full` appears in SQLite      | Incoming traffic exceeds worker throughput       | Increase `QUEUE_SIZE` or reduce message volume         |
| Albums are split or incomplete      | Album items arrive outside the collection window | Increase `ALBUM_DELAY`                                 |
| Bale returns rate-limit errors      | Bale is throttling requests                      | The bridge automatically honors `retry_after`          |
| Photos arrive as documents in Bale  | Photo exceeds Bale's 10 MB photo limit           | This is expected fallback behavior                     |

For high-volume Bale traffic, consider Bale's business API:

```text
https://tapi.bale.ai/business/bot<token>/...
```

---

## Known limitations

Current limitations in `v0.1`:

* Only one Telegram/Bale chat pair is supported
* Chat IDs must be numeric
* `@username` chat aliases are not supported
* Album buffers are not persisted
* A partially collected album is lost after a crash
* Message edits are not synchronized
* Message deletions are not synchronized
* Replies and message relationships are not preserved
* Webhooks are not supported
* Long polling is the only update mechanism

---

## Security notes

* Never commit `.env` to version control
* Keep both bot tokens secret
* Restrict bot permissions to only what the bridge requires
* Store the SQLite database and temporary directory on a protected filesystem
* Rotate a bot token immediately if it is exposed
* Review logs before sharing them publicly, even though tokens are intentionally excluded

Example `.gitignore` entries:

```gitignore
.env
data/
bridge
```

---

## License

Released under the [MIT License](LICENSE).
