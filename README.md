# ⚡ bully

> This is a failed experiment. The downloaded files were all corrupt. It is going to be an interesting case study for AI generated code and how it could still get this wrong.

**Zero-config torrent downloader for macOS.** Paste a magnet link, walk away, come back to a completed download.

```
╭──────────────────────────────────────────────────────────╮
│  Paste magnet link here and press Enter...               │
╰──────────────────────────────────────────────────────────╯

╭─ ⬇  ubuntu-24.04-desktop-amd64.iso ─────────────────────╮
│  ████████████████████████░░░░░░░░░░░░░░  62.3%           │
│  3.2 MB/s  •  14 peers  •  8 seeds  •  eta 2m15s         │
╰──────────────────────────────────────────────────────────╯

╭─ ⏳  debian-12.5.0-amd64-netinst.iso ────────────────────╮
│  queued (#2)                                             │
╰──────────────────────────────────────────────────────────╯
```

## Install

```bash
cd ~/code/daniellionel01/bully
go build -o /usr/local/bin/bully .
```

Then just run `bully` from anywhere.

## Usage

```
bully          # start the TUI
```

1. **Paste** a magnet link into the input box
2. Press **Enter**
3. Walk away
4. Files land in `~/Downloads/bully/`

Keys: `enter` add, `q` / `ctrl+c` quit.

## How it works

```
┌──────────────────────────────────────┐
│           TUI (Bubble Tea)           │
│   input box  │  progress cards       │
├──────────────────────────────────────┤
│      Torrent Engine (anacrolix)      │
│   DHT · PEX · uTP · encryption       │
│   file-based resume storage          │
├──────────────────────────────────────┤
│          Speed Booster               │
│   samples speed ⟶ detects stalls     │
│   injects fresh trackers ⟶ DHT       │
│   keeps downloads healthy            │
├──────────────────────────────────────┤
│       Queue (JSON on disk)           │
│   persists across restarts           │
│   resumes interrupted downloads      │
└──────────────────────────────────────┘
```

## Files

```
~/.bully/
├── queue.json    # download queue (persisted)
└── bully.log     # debug logs

~/Downloads/bully/
├── .torrent.db   # piece completion state (SQLite, for resume)
├── Movie Name/   # each torrent gets its own folder
│   └── movie.mkv
└── ...
```

## Build

```bash
go build -ldflags="-s -w" -o bully .   # ~25MB stripped binary
```

## Testing

### Run all tests

```bash
go test ./...
```

### Run with verbose output (see each test)

```bash
go test ./... -v
```

### Run without cache

```bash
go test ./... -count=1
```

### Run a single package

```bash
go test ./internal/queue/ -v
go test ./internal/vpn/ -v
go test ./internal/engine/ -v
```

### Run a single test by name

```bash
go test -run TestPersistence ./internal/queue/ -v
```

### Coverage

```bash
go test ./... -cover
```

### Test structure

| Package | Tests | Type | What it verifies |
|---|---|---|---|
| `internal/queue` | 5 | Unit | Add, order, status transitions, persistence across reload, remove |
| `internal/vpn` | 3 | Unit | Tunnel interface name matching (`utun`, `wg`, `tun`, etc.), detector defaults, recheck |
| `internal/engine` | 2 | Integration | **Local seeder + downloader** — creates a 2MB random file, seeds it on loopback, downloads through the engine, verifies hash match. No internet needed. Also tests status flow and completion detection. |
| `internal/tui` | — | — | UI tests (visual, needs Bubble Tea test helpers) |

All engine tests run entirely on localhost — no network, no VPN, no DHT. A local seeder and downloader peer directly via `AddClientPeer`.
