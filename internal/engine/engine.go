package engine

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	analog "github.com/anacrolix/log"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
)

// Status represents the current state of a download.
type Status string

const (
	StatusQueued      Status = "queued"
	StatusDownloading Status = "downloading"
	StatusSeeding     Status = "seeding"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
)

// TorrentInfo holds metadata and progress for a single torrent.
type TorrentInfo struct {
	MagnetURI      string
	Name           string
	Status         Status
	Progress       float64 // 0.0 to 1.0
	BytesCompleted int64
	BytesTotal     int64
	SpeedDown      int64 // bytes per second (computed from deltas)
	SpeedUp        int64 // bytes per second
	Peers          int
	Seeds          int
	ETA            time.Duration
	AddedAt        time.Time
	CompletedAt    *time.Time
	Error          string

	// internal
	handle              *torrent.Torrent
	lastBytesCompleted  int64
	lastSpeedSample     time.Time
}

// Client wraps the anacrolix/torrent client with speed boosting logic.
type Client struct {
	cfg         *torrent.ClientConfig
	client      *torrent.Client
	downloadDir string

	mu       sync.RWMutex
	torrents map[string]*TorrentInfo // keyed by magnet URI

	// Speed booster state (per-torrent in TorrentInfo)
}

// NewClient creates a new torrent engine client.
func NewClient(downloadDir string) (*Client, error) {
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return nil, fmt.Errorf("create download dir: %w", err)
	}

	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = downloadDir
	cfg.ListenPort = 0 // random port, avoids conflicts when multiple clients or tests
	cfg.Seed = false // don't seed after download completes
	cfg.NoDefaultPortForwarding = true
	cfg.DisablePEX = false
	// Suppress noisy library logs (EOF warnings, peer errors) from the TUI
	cfg.Slogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	// Use native file storage so files land directly in ~/Downloads/bully/<TorrentName>/
	// with proper resume support via SQLite piece completion DB
	cfg.DefaultStorage = storage.NewFile(downloadDir)

	// Also suppress the anacrolix/log default handler which writes to stderr
	analog.DefaultHandler = analog.DiscardHandler

	client, err := torrent.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create torrent client: %w", err)
	}

	c := &Client{
		cfg:         cfg,
		client:      client,
		downloadDir: downloadDir,
		torrents:    make(map[string]*TorrentInfo),
	}

	log.Printf("engine started, download dir: %s", downloadDir)
	return c, nil
}

// AddFromURL downloads a .torrent file from an HTTP URL and starts downloading.
func (c *Client) AddFromURL(url string) (*TorrentInfo, error) {
	c.mu.Lock()
	if existing, ok := c.torrents[url]; ok {
		c.mu.Unlock()
		return existing, nil
	}
	c.mu.Unlock()

	log.Printf("downloading .torrent from: %s", url)

	// Download the .torrent file
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download .torrent: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download .torrent: HTTP %d", resp.StatusCode)
	}

	// Save to a temp file and load it
	tmpFile, err := os.CreateTemp("", "bully-*.torrent")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("save .torrent: %w", err)
	}
	tmpFile.Close()

	t, err := c.client.AddTorrentFromFile(tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("add torrent from file: %w", err)
	}

	info := &TorrentInfo{
		MagnetURI: url,
		Status:    StatusDownloading,
		AddedAt:   time.Now(),
		handle:    t,
	}

	c.mu.Lock()
	c.torrents[url] = info
	c.mu.Unlock()

	go c.trackTorrent(info)

	return info, nil
}

// AddMagnet adds a magnet link and begins downloading.
func (c *Client) AddMagnet(magnetURI string) (*TorrentInfo, error) {
	c.mu.Lock()
	if existing, ok := c.torrents[magnetURI]; ok {
		c.mu.Unlock()
		return existing, nil
	}
	c.mu.Unlock()

	t, err := c.client.AddMagnet(magnetURI)
	if err != nil {
		return nil, fmt.Errorf("add magnet: %w", err)
	}

	info := &TorrentInfo{
		MagnetURI: magnetURI,
		Status:    StatusDownloading,
		AddedAt:   time.Now(),
		handle:    t,
	}

	c.mu.Lock()
	c.torrents[magnetURI] = info
	c.mu.Unlock()

	// Start metadata fetch and download tracking in background
	go c.trackTorrent(info)

	return info, nil
}

// trackTorrent waits for metadata then tracks download progress.
func (c *Client) trackTorrent(info *TorrentInfo) {
	t := info.handle

	log.Printf("waiting for metadata: %s", info.MagnetURI)

	// Wait for info (metadata)
	select {
	case <-t.GotInfo():
		info.Name = t.Info().Name
		log.Printf("got metadata: %s", info.Name)
	case <-time.After(10 * time.Minute):
		c.mu.Lock()
		info.Status = StatusFailed
		info.Error = "timed out waiting for torrent metadata"
		c.mu.Unlock()
		log.Printf("metadata timeout: %s", info.MagnetURI)
		return
	}

	// Start downloading all pieces
	t.DownloadAll()

	// Poll for progress
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.updateProgress(info)
			// Exit if torrent is complete
			if info.Status == StatusCompleted {
				return
			}
		case <-t.Closed():
			c.updateProgress(info)
			log.Printf("torrent closed: %s", info.Name)
			return
		}
	}
}

// updateProgress reads current stats from the torrent handle.
func (c *Client) updateProgress(info *TorrentInfo) {
	t := info.handle
	if t == nil {
		return
	}

	stats := t.Stats()
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Progress
	info.BytesCompleted = t.BytesCompleted()
	info.BytesTotal = t.Length() // total size of the torrent data
	if info.BytesTotal > 0 {
		info.Progress = float64(info.BytesCompleted) / float64(info.BytesTotal)
		if info.Progress > 1.0 {
			info.Progress = 1.0
		}
	}

	// Speed calculation (download)
	deltaBytes := info.BytesCompleted - info.lastBytesCompleted
	deltaTime := now.Sub(info.lastSpeedSample)
	if deltaTime > 0 {
		info.SpeedDown = int64(float64(deltaBytes) / deltaTime.Seconds())
	}
	info.lastBytesCompleted = info.BytesCompleted
	info.lastSpeedSample = now

	// Upload speed from ConnStats (bytes written to peers)
	info.SpeedUp = stats.ConnStats.BytesWrittenData.Int64()

	// Peers and seeds
	info.Peers = stats.ActivePeers
	info.Seeds = stats.ConnectedSeeders

	// ETA
	if info.SpeedDown > 0 && info.BytesTotal > 0 && info.BytesCompleted < info.BytesTotal {
		remaining := info.BytesTotal - info.BytesCompleted
		info.ETA = time.Duration(float64(remaining)/float64(info.SpeedDown)) * time.Second
	} else {
		info.ETA = 0
	}

	// Check completion
	if info.Progress >= 1.0 && info.Status == StatusDownloading {
		info.Status = StatusCompleted
		now := time.Now()
		info.CompletedAt = &now
		log.Printf("download complete: %s", info.Name)
	}
}

// checkStall detects download stalls and triggers recovery.
// Called periodically to inject additional trackers if speed is low.
func (c *Client) CheckStalls() {
	c.mu.RLock()
	infos := make([]*TorrentInfo, 0, len(c.torrents))
	for _, info := range c.torrents {
		infos = append(infos, info)
	}
	c.mu.RUnlock()

	for _, info := range infos {
		if info.Status != StatusDownloading || info.Progress >= 0.99 {
			continue
		}

		// If we have some peers but speed is < 10 KB/s for a while, boost
		if info.Peers > 0 && info.SpeedDown < 10*1024 && info.Progress > 0.01 {
			log.Printf("[booster] slow speed for %q (%d KB/s, %d peers), adding trackers",
				info.Name, info.SpeedDown/1024, info.Peers)
			info.handle.AddTrackers([][]string{
				{"udp://tracker.openbittorrent.com:80/announce"},
				{"udp://tracker.opentrackr.org:1337/announce"},
				{"udp://9.rarbg.to:2710/announce"},
				{"udp://tracker.torrent.eu.org:451/announce"},
				{"udp://explodie.org:6969/announce"},
				{"udp://tracker.coppersurfer.tk:6969/announce"},
				{"udp://open.stealth.si:80/announce"},
				{"udp://tracker.cyberia.is:6969/announce"},
			})
		}

		// If we have 0 peers and progress is low, try DHT bootstrap
		if info.Peers == 0 && info.Progress < 0.9 {
			log.Printf("[booster] no peers for %q, adding DHT bootstrap nodes", info.Name)
			info.handle.AddTrackers([][]string{
				{"udp://tracker.openbittorrent.com:80/announce"},
				{"udp://tracker.opentrackr.org:1337/announce"},
			})
		}
	}
}

// GetAllTorrents returns a snapshot of all torrents.
func (c *Client) GetAllTorrents() []*TorrentInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*TorrentInfo, 0, len(c.torrents))
	for _, t := range c.torrents {
		result = append(result, t)
	}
	return result
}

// GetTorrent returns info for a specific magnet URI.
func (c *Client) GetTorrent(magnetURI string) *TorrentInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.torrents[magnetURI]
}

// Close shuts down the torrent client safely.
func (c *Client) Close() {
	// Collect all handles first, then drop them outside the lock
	// to avoid deadlocks (Drop triggers Closed events that try to re-acquire).
	c.mu.Lock()
	handles := make([]*torrent.Torrent, 0, len(c.torrents))
	for _, t := range c.torrents {
		if t.handle != nil {
			handles = append(handles, t.handle)
		}
	}
	c.mu.Unlock()

	for _, h := range handles {
		h.Drop()
	}

	// Now safe to close the client
	errs := c.client.Close()
	if len(errs) > 0 {
		for _, err := range errs {
			log.Printf("close error: %v", err)
		}
	}
	log.Println("engine shut down")
}
