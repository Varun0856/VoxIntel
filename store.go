package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Store struct {
	mu      sync.RWMutex
	dataDir string
	cache   map[string]*Session
}

func NewStore(dataDir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "sessions"), 0o755); err != nil {
		return nil, fmt.Errorf("creating sessions dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "audio"), 0o755); err != nil {
		return nil, fmt.Errorf("creating audio dir: %w", err)
	}

	s := &Store{dataDir: dataDir, cache: make(map[string]*Session)}
	if err := s.loadAll(); err != nil {
		return nil, fmt.Errorf("loading existing sessions: %w", err)
	}
	return s, nil
}

func (s *Store) loadAll() error {
	entries, err := os.ReadDir(filepath.Join(s.dataDir, "sessions"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(s.dataDir, "sessions", entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		s.cache[sess.ID] = &sess
	}
	return nil
}

func (s *Store) sessionFilePath(id string) string {
	return filepath.Join(s.dataDir, "sessions", id+".json")
}

func (s *Store) AudioFilePath(id, ext string) string {
	return filepath.Join(s.dataDir, "audio", id+ext)
}

func (s *Store) persist(sess *Session) error {
	data, err := json.MarshalIndent(sess, "", " ")
	if err != nil {
		return fmt.Errorf("marshaling session: %s: %w", sess.ID, err)
	}
	path := s.sessionFilePath(sess.ID)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming into place: %w", err)
	}
	return nil
}

func (s *Store) Create() (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess := &Session{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		CreatedAt: time.Now(),
		Status:    StatusRecorded,
	}
	s.cache[sess.ID] = sess
	if err := s.persist(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) Get(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.cache[id]
	return sess, ok
}

func (s *Store) List() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Session, 0, len(s.cache))
	for _, sess := range s.cache {
		out = append(out, sess)
	}
	return out
}

func (s *Store) Update(id string, fn func(*Session)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.cache[id]
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	fn(sess)
	return s.persist(sess)
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.cache[id]
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}

	if sess.AudioPath != "" {
		_ = os.Remove(sess.AudioPath)
	}

	if err := os.Remove(s.sessionFilePath(id)); err != nil {
		return fmt.Errorf("removing session file: %w", err)
	}
	delete(s.cache, id)
	return nil
}
