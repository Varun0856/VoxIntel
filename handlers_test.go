package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeTranscriber and fakeAnalyzer satisfy the Transcriber/Analyzer
// interfaces without any real network calls — this is exactly why
// handlers.go depends on interfaces instead of *WhisperClient/*GroqAnalyzer
// directly: it makes this kind of test possible at all.
type fakeTranscriber struct {
	text string
	err  error
}

func (f *fakeTranscriber) Transcribe(ctx context.Context, audioPath string) (string, error) {
	return f.text, f.err
}

type fakeAnalyzer struct {
	answer string
	err    error
}

func (f *fakeAnalyzer) Analyze(ctx context.Context, transcript, prompt string) (string, error) {
	return f.answer, f.err
}

func newTestApp(t *testing.T) *App {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	return NewApp(store, &fakeTranscriber{text: "fake transcript"}, &fakeAnalyzer{answer: `{"takeaways":[]}`})
}

func TestCreateSessionHandler(t *testing.T) {
	app := newTestApp(t)
	mux := http.NewServeMux()
	app.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/sessions", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var got Session
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Status != StatusRecorded {
		t.Errorf("expected status %q, got %q", StatusRecorded, got.Status)
	}
}

func TestGetSessionNotFound(t *testing.T) {
	app := newTestApp(t)
	mux := http.NewServeMux()
	app.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/does-not-exist", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestAnalyzeRejectsEmptyPrompt(t *testing.T) {
	app := newTestApp(t)
	sess, _ := app.store.Create()
	app.store.Update(sess.ID, func(s *Session) {
		s.Status = StatusTranscribed
		s.Transcript = "some transcript"
	})

	mux := http.NewServeMux()
	app.registerRoutes(mux)

	body, _ := json.Marshal(map[string]string{"prompt": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sess.ID+"/analyze", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for empty prompt, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestAnalyzeRejectsNotYetTranscribed(t *testing.T) {
	app := newTestApp(t)
	sess, _ := app.store.Create() // status is StatusRecorded, not transcribed

	mux := http.NewServeMux()
	app.registerRoutes(mux)

	body, _ := json.Marshal(map[string]string{"prompt": "summarize"})
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sess.ID+"/analyze", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}
}

func TestHealthCheck(t *testing.T) {
	app := newTestApp(t)
	mux := http.NewServeMux()
	app.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}
