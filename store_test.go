package main

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestStoreCreateAndGet(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	sess, err := store.Create()
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if sess.Status != StatusRecorded {
		t.Errorf("expected status %q, got %q", StatusRecorded, sess.Status)
	}

	got, ok := store.Get(sess.ID)
	if !ok {
		t.Fatal("Get returned ok=false for a session that was just created")
	}
	if got.ID != sess.ID {
		t.Errorf("got session ID %q, want %q", got.ID, sess.ID)
	}
}

func TestStoreGetMissing(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	_, ok := store.Get("does-not-exist")
	if ok {
		t.Error("expected ok=false for a missing session, got true")
	}
}

func TestStoreUpdate(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	sess, _ := store.Create()

	err = store.Update(sess.ID, func(s *Session) {
		s.Status = StatusTranscribed
		s.Transcript = "hello world"
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, _ := store.Get(sess.ID)
	if got.Status != StatusTranscribed {
		t.Errorf("expected status %q, got %q", StatusTranscribed, got.Status)
	}
	if got.Transcript != "hello world" {
		t.Errorf("expected transcript %q, got %q", "hello world", got.Transcript)
	}
}

func TestStoreUpdateMissing(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	err = store.Update("does-not-exist", func(s *Session) {})
	if err == nil {
		t.Error("expected an error updating a missing session, got nil")
	}
}

// TestStorePersistsAcrossRestart is the important one: it proves the whole
// point of writing to disk instead of keeping sessions only in memory —
// a fresh Store pointed at the same directory should see what a previous
// Store wrote, simulating a process restart.
func TestStorePersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()

	store1, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	sess, _ := store1.Create()
	store1.Update(sess.ID, func(s *Session) {
		s.Status = StatusTranscribed
		s.Transcript = "persisted transcript"
	})

	// Simulate a restart: a brand new Store instance, same directory.
	store2, err := NewStore(dir)
	if err != nil {
		t.Fatalf("second NewStore failed: %v", err)
	}

	got, ok := store2.Get(sess.ID)
	if !ok {
		t.Fatal("session did not survive a simulated restart")
	}
	if got.Transcript != "persisted transcript" {
		t.Errorf("got transcript %q, want %q", got.Transcript, "persisted transcript")
	}
}

func TestStoreDelete(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	sess, _ := store.Create()
	if err := store.Delete(sess.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, ok := store.Get(sess.ID)
	if ok {
		t.Error("expected session to be gone after Delete, but Get found it")
	}
}

// TestStoreConcurrentAccess exercises the sync.RWMutex under real concurrent
// load. Run this specifically with -race (see instructions below) — that's
// what actually catches a broken lock, not just this test passing normally.
func TestStoreConcurrentAccess(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	sess, _ := store.Create()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			store.Get(sess.ID)
		}()
		go func() {
			defer wg.Done()
			store.Update(sess.ID, func(s *Session) {
				s.Transcript = "concurrent write"
			})
		}()
	}
	wg.Wait()
}

func TestStoreAudioFilePath(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	got := store.AudioFilePath("abc123", ".webm")
	want := filepath.Join(dir, "audio", "abc123.webm")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
