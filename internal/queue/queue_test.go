package queue

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddAndRetrieve(t *testing.T) {
	// Override home for test
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, ".bully"), 0755)

	q, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// Add a item
	item, err := q.Add("magnet:?xt=urn:btih:TESTHASH123")
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != StatusQueued {
		t.Errorf("expected queued, got %s", item.Status)
	}
	if item.MagnetURI != "magnet:?xt=urn:btih:TESTHASH123" {
		t.Errorf("magnet URI mismatch")
	}

	// Add duplicate — should return existing
	item2, err := q.Add("magnet:?xt=urn:btih:TESTHASH123")
	if err != nil {
		t.Fatal(err)
	}
	if item2 != item {
		t.Error("expected same item for duplicate add")
	}

	// Count is still 1
	if q.Count() != 1 {
		t.Errorf("expected 1 item, got %d", q.Count())
	}
}

func TestQueueOrder(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, ".bully"), 0755)

	q, _ := New()

	q.Add("magnet:A")
	q.Add("magnet:B")
	q.Add("magnet:C")

	// First queued item should be A
	next := q.Next()
	if next.MagnetURI != "magnet:A" {
		t.Errorf("expected A, got %s", next.MagnetURI)
	}

	// Mark A as downloading
	q.Update("magnet:A", StatusDownloading, 0.5, "File A", "")
	if q.HasActive() != true {
		t.Error("expected HasActive to be true")
	}

	// Next should be B (A is downloading, not queued)
	next = q.Next()
	if next.MagnetURI != "magnet:B" {
		t.Errorf("expected B, got %s", next.MagnetURI)
	}
}

func TestStatusTransitions(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, ".bully"), 0755)

	q, _ := New()
	item, _ := q.Add("magnet:TEST")
	_ = item

	// queued → downloading
	q.Update("magnet:TEST", StatusDownloading, 0, "test.iso", "")
	all := q.All()
	if all[0].Status != StatusDownloading || all[0].Name != "test.iso" {
		t.Errorf("update failed: %+v", all[0])
	}

	// downloading → completed
	q.Update("magnet:TEST", StatusCompleted, 1.0, "test.iso", "")
	all = q.All()
	if all[0].Status != StatusCompleted || all[0].CompletedAt == nil {
		t.Error("expected completed with timestamp")
	}

	// downloading → failed
	q.Add("magnet:FAIL")
	q.Update("magnet:FAIL", StatusFailed, 0.1, "", "connection lost")
	all = q.All()
	if all[1].Status != StatusFailed || all[1].Error != "connection lost" {
		t.Error("expected failed with error message")
	}
}

func TestPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, ".bully"), 0755)

	// Create and populate queue
	q1, _ := New()
	q1.Add("magnet:PERSIST1")
	q1.Add("magnet:PERSIST2")
	q1.Update("magnet:PERSIST1", StatusDownloading, 0.75, "file1.mkv", "")

	// Re-load from disk
	q2, _ := New()
	all := q2.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 items after reload, got %d", len(all))
	}
	if all[0].Progress != 0.75 || all[0].Name != "file1.mkv" {
		t.Errorf("progress/name not persisted: %+v", all[0])
	}
}

func TestRemove(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, ".bully"), 0755)

	q, _ := New()
	q.Add("magnet:A")
	q.Add("magnet:B")

	q.Remove("magnet:A")
	if q.Count() != 1 {
		t.Errorf("expected 1 after remove, got %d", q.Count())
	}
	if q.All()[0].MagnetURI != "magnet:B" {
		t.Error("wrong item removed")
	}
}
