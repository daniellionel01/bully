package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bully/internal/engine"
	"bully/internal/queue"
	"bully/internal/tui"
	"bully/internal/vpn"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Setup logging to file so it doesn't interfere with TUI
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting home dir: %v\n", err)
		os.Exit(1)
	}
	logDir := filepath.Join(home, ".bully")
	os.MkdirAll(logDir, 0755)
	logFile, err := os.OpenFile(filepath.Join(logDir, "bully.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		log.SetOutput(logFile)
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	// Initialize download directory
	downloadDir := filepath.Join(home, "Downloads", "bully")
	log.Printf("download dir: %s", downloadDir)

	// Initialize queue
	q, err := queue.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error initializing queue: %v\n", err)
		os.Exit(1)
	}

	// Initialize torrent engine
	client, err := engine.NewClient(downloadDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error initializing engine: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// Start periodic stall checking (every 15s)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			client.CheckStalls()
		}
	}()

	// Resume any in-progress downloads from last session
	resumeDownloads(client, q)

	// Initialize VPN detector
	vpnDetector := vpn.NewDetector()
	log.Printf("vpn status: %s", vpnDetector.Status())

	// Start the TUI
	model := tui.NewModel(client, q, vpnDetector)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error running TUI: %v\n", err)
		os.Exit(1)
	}

	log.Println("bully shutting down. see you next time 👋")
}

// resumeDownloads restarts downloads that were in progress last session.
func resumeDownloads(client *engine.Client, q *queue.Queue) {
	for _, item := range q.All() {
		if item.Status == queue.StatusDownloading {
			log.Printf("resuming: %s", item.MagnetURI)
			go func(uri string) {
				var err error
				if strings.HasPrefix(uri, "magnet:") {
					_, err = client.AddMagnet(uri)
				} else {
					_, err = client.AddFromURL(uri)
				}
				if err != nil {
					log.Printf("failed to resume %s: %v", uri, err)
					q.Update(uri, queue.StatusFailed, 0, "", err.Error())
				}
			}(item.MagnetURI)
		}
		if item.Status == queue.StatusQueued {
			log.Printf("requeued: %s", item.MagnetURI)
		}
	}
}
