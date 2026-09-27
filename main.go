package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

//go:embed static
var staticFiles embed.FS

func main() {
	loadConfigFile(".env")
	port := getEnv("PORT", "8080")
	dataDir := getEnv("DATA_DIR", "data")

	store, err := NewStore(dataDir)
	if err != nil {
		log.Fatalf("failed to initialize store: %v", err)
	}

	transcribeAPIKey := os.Getenv("TRANSCRIBE_API_KEY")
	if transcribeAPIKey == "" {
		log.Fatal("TRANSCRIBE_API_KEY is not set")
	}
	transcribeBaseURL := getEnv("TRANSCRIBE_BASE_URL", "https://api.groq.com/openai/v1")
	transcribeModel := getEnv("TRANSCRIBE_MODEL", "whisper-large-v3")

	transcriber := NewWhisperClient(transcribeAPIKey, transcribeBaseURL, transcribeModel)

	analyzeAPIKey := os.Getenv("ANALYZE_API_KEY")
	if analyzeAPIKey == "" {
		log.Fatal("ANALYZE_API_KEY is not set")
	}
	analyzeBaseURL := getEnv("ANALYZE_BASE_URL", "https://api.groq.com/openai/v1")
	analyzeModel := getEnv("ANALYZE_MODEL", "llama-3.3-70b-versatile")

	analyzer := NewGroqAnalyzer(analyzeAPIKey, analyzeBaseURL, analyzeModel)

	app := NewApp(store, transcriber, analyzer)
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("failed to load embedded static files: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	app.registerRoutes(mux)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      basicAuth(mux),
		ReadTimeout:  15 * time.Minute,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("VoxIntel listening on :%s (data dir: %s)", port, dataDir)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("shutting down gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
	log.Println("shutdown complete")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadConfigFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}
