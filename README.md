# ⚡ bully

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

## Why bully?

| Problem | bully's solution |
|---|---|
| WebTorrent Desktop crashes on M2 | Native Go binary — no Electron, no JS GC |
| Confusing interfaces | One input box. Paste → Enter → done |
| Starts at KB/s, jumps to MB/s on refresh | Speed booster: detects stalls, re-announces trackers & DHT automatically |
| Lost connections, restarts | State persists to `~/.bully/`. Close & reopen → resumes where you left off |

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
├── .resume/      # partial download state (for resume)
└── *.iso, *.mkv  # your completed files
```

## Build

```bash
go build -ldflags="-s -w" -o bully .   # ~25MB stripped binary
```
