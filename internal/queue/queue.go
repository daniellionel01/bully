package queue

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Status represents the state of a queue item.
type Status string

const (
	StatusQueued     Status = "queued"
	StatusDownloading Status = "downloading"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
)

// Item represents a download in the queue.
type Item struct {
	MagnetURI    string    `json:"magnet_uri"`
	Name         string    `json:"name"`
	Status       Status    `json:"status"`
	Progress     float64   `json:"progress"`
	AddedAt      time.Time `json:"added_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	Error        string    `json:"error,omitempty"`
}

// Queue manages a persistent download queue.
type Queue struct {
	mu       sync.RWMutex
	items    []*Item
	filePath string
}

// New creates or loads a queue from disk.
func New() (*Queue, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	bullyDir := filepath.Join(home, ".bully")
	if err := os.MkdirAll(bullyDir, 0755); err != nil {
		return nil, fmt.Errorf("create bully dir: %w", err)
	}

	q := &Queue{
		filePath: filepath.Join(bullyDir, "queue.json"),
	}

	if err := q.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load queue: %w", err)
	}

	return q, nil
}

// load reads the queue from disk.
func (q *Queue) load() error {
	data, err := os.ReadFile(q.filePath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &q.items)
}

// save writes the queue to disk.
func (q *Queue) save() error {
	data, err := json.MarshalIndent(q.items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(q.filePath, data, 0644)
}

// Add adds a magnet URI to the queue.
func (q *Queue) Add(magnetURI string) (*Item, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check for duplicates
	for _, item := range q.items {
		if item.MagnetURI == magnetURI {
			return item, nil
		}
	}

	item := &Item{
		MagnetURI: magnetURI,
		Status:    StatusQueued,
		AddedAt:   time.Now(),
	}

	q.items = append(q.items, item)

	if err := q.save(); err != nil {
		return nil, fmt.Errorf("save queue: %w", err)
	}

	return item, nil
}

// Next returns the next queued item, or nil if none.
func (q *Queue) Next() *Item {
	q.mu.RLock()
	defer q.mu.RUnlock()

	for _, item := range q.items {
		if item.Status == StatusQueued {
			return item
		}
	}
	return nil
}

// HasActive returns true if any item is currently downloading.
func (q *Queue) HasActive() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	for _, item := range q.items {
		if item.Status == StatusDownloading {
			return true
		}
	}
	return false
}

// Update updates an item's status and progress.
func (q *Queue) Update(magnetURI string, status Status, progress float64, name string, errMsg string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, item := range q.items {
		if item.MagnetURI == magnetURI {
			item.Status = status
			item.Progress = progress
			if name != "" {
				item.Name = name
			}
			if errMsg != "" {
				item.Error = errMsg
			}
			if status == StatusCompleted {
				now := time.Now()
				item.CompletedAt = &now
			}
			break
		}
	}

	// Best-effort save, don't crash on IO error
	_ = q.save()
}

// All returns all items in the queue.
func (q *Queue) All() []*Item {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]*Item, len(q.items))
	copy(result, q.items)
	return result
}

// Remove removes an item from the queue.
func (q *Queue) Remove(magnetURI string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, item := range q.items {
		if item.MagnetURI == magnetURI {
			q.items = append(q.items[:i], q.items[i+1:]...)
			return q.save()
		}
	}
	return nil
}

// Count returns the number of items in the queue.
func (q *Queue) Count() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.items)
}
