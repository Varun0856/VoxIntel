package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
)

const maxUploadSize = 250 << 20

type Transcriber interface {
	Transcribe(ctx context.Context, audioPath string) (string, error)
}

type Analyzer interface {
	Analyze(ctx context.Context, transcript, prompt string) (string, error)
}

type App struct {
	store       *Store
	transcriber Transcriber
	analyzer    Analyzer
}

func NewApp(store *Store, t Transcriber, a Analyzer) *App {
	return &App{store: store, transcriber: t, analyzer: a}
}

func (a *App) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/sessions", a.createSession)
	mux.HandleFunc("GET /api/sessions", a.listSessions)
	mux.HandleFunc("GET /api/sessions/{id}", a.getSession)
	mux.HandleFunc("POST /api/sessions/{id}/audio", a.uploadAudio)
	mux.HandleFunc("POST /api/sessions/{id}/analyze", a.analyzeSession)
	mux.HandleFunc("GET /api/sessions/{id}/export", a.exportSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", a.deleteSession)
}

func writeJson(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJson(w, status, map[string]string{"error": msg})
}

func (a *App) createSession(w http.ResponseWriter, r *http.Request) {
	sess, err := a.store.Create()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	writeJson(w, http.StatusCreated, sess)
}

func (a *App) listSessions(w http.ResponseWriter, r *http.Request) {
	writeJson(w, http.StatusOK, a.store.List())
}

func (a *App) getSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := a.store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJson(w, http.StatusOK, sess)
}

func (a *App) uploadAudio(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := a.store.Get(id); !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	ext := extFromContentType(r.Header.Get("Content-Type"))
	audioPath := a.store.AudioFilePath(id, ext)

	f, err := createFile(audioPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save audio")
		return
	}
	defer f.Close()

	if _, err := copyBody(f, r.Body); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "upload too large or failed")
		return
	}

	err = a.store.Update(id, func(s *Session) {
		s.AudioPath = audioPath
		s.Status = StatusTranscribing
	})

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update session")
		return
	}

	go a.runTranscription(context.Background(), id, audioPath)

	writeJson(w, http.StatusAccepted, map[string]string{"status": string(StatusTranscribing)})
}

func (a *App) runTranscription(ctx context.Context, id, audioPath string) {
	text, err := a.transcriber.Transcribe(ctx, audioPath)
	if err != nil {
		a.store.Update(id, func(s *Session) {
			s.Status = StatusFailed
			s.Error = err.Error()
		})
		return
	}
	a.store.Update(id, func(s *Session) {
		s.Status = StatusTranscribed
		s.Transcript = text
	})
}

func (a *App) analyzeSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := a.store.Get(id)
	if !ok {

		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if sess.Status != StatusTranscribed {
		writeError(w, http.StatusConflict, "session is not transcribed yet")
		return
	}

	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.Prompt) == "" {
		writeError(w, http.StatusBadRequest, "prompt cannot be empty")
		return
	}
	answer, err := a.analyzer.Analyze(r.Context(), sess.Transcript, body.Prompt)
	if err != nil {
		writeError(w, http.StatusBadGateway, "analysis failed: "+err.Error())
		return
	}
	result := AnalysisResult{Prompt: body.Prompt, Answer: answer}
	err = a.store.Update(id, func(s *Session) {
		s.Analyses = append(s.Analyses, result)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save analysis")
		return
	}
	writeJson(w, http.StatusOK, result)

}

func (a *App) exportSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := a.store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	var b strings.Builder
	b.WriteString("# VoxIntel Transcript - " + sess.ID + "\n\n")
	b.WriteString(sess.Transcript + "\n")
	if len(sess.Analyses) > 0 {
		b.WriteString("\n## Analyses\n\n")
		for _, res := range sess.Analyses {
			b.WriteString("**Q:** " + res.Prompt + "\n\n**A:** " + res.Answer + "\n\n")
		}
	}
	w.Header().Set("Content-Type", "text/markdown")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sess.ID+".md\"")
	w.Write([]byte(b.String()))
}

func (a *App) deleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.store.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func extFromContentType(ct string) string {
	switch {
	case strings.Contains(ct, "webm"):
		return ".webm"
	case strings.Contains(ct, "ogg"):
		return ".ogg"
	case strings.Contains(ct, "wav"):
		return ".wav"
	default:
		return ".webm"
	}
}

func createFile(path string) (*os.File, error) {
	return os.Create(path)
}
func copyBody(dst *os.File, src io.Reader) (int64, error) {
	return io.Copy(dst, src)
}

var errNotImplemented = errors.New("not implemented")
