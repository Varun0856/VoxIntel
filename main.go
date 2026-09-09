package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
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
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("static")))
	app.registerRoutes(mux)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Minute,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("VoxIntel listening on :%s (data dir: %s)", port, dataDir)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
