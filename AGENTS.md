# AGENTS.md

Instructions for AI coding agents working in this repository.

## Project overview

**bully** is a zero-config BitTorrent downloader for macOS with a terminal UI (TUI). Built in Go using [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [anacrolix/torrent](https://github.com/anacrolix/torrent).

## Build & run

```bash
# Build (stripped binary, ~25MB)
go build -ldflags="-s -w" -o bully .

# Install to PATH
go build -o /usr/local/bin/bully .

# Run
./bully

# Run tests
go test ./...
```

## Architecture

```
main.go              # Entry point: wiring, logging, resume logic
internal/
  engine/            # Torrent client wrapper (anacrolix/torrent)
  queue/             # Download queue persisted as JSON (~/.bully/queue.json)
  tui/               # Bubble Tea terminal UI (input box + progress cards)
  vpn/               # VPN status detection
```

### Key flows

1. **Add download**: User pastes magnet link → queue item created → engine starts downloading → progress fed back to TUI via Bubble Tea messages
2. **Resume**: On startup, all `StatusDownloading` queue items are re-added to the engine. Partial downloads live in `~/Downloads/bully/.resume/`.
3. **Speed booster**: A background goroutine ticks every 15s and calls `client.CheckStalls()` to detect slow/stuck downloads and re-announce trackers.
4. **State**: Queue persists to `~/.bully/queue.json`. Logs write to `~/.bully/bully.log`.

## Coding conventions

- **Go standard layout**: `main.go` at root, packages under `internal/`.
- **Error handling**: Log to file, surface user-facing errors via TUI. Don't `panic` in library code.
- **Concurrency**: Use goroutines + channels for async work. Bubble Tea's `tea.Cmd` for TUI-bound async.
- **No external config files**: Everything is zero-config. Use sensible defaults and `~/.bully/` for persisted state.
- **macOS target**: The VPN detection and default paths assume macOS.

## Dependencies

- [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) — TUI framework
- [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) — terminal styling
- [anacrolix/torrent](https://github.com/anacrolix/torrent) — BitTorrent protocol implementation
- Standard library only for everything else

## Before committing

- Run `go vet ./...` and `go test ./...`
- Ensure the binary builds cleanly: `go build -o /dev/null .`
- Keep the README in sync with any CLI or behavioural changes
