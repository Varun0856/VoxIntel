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

	app := NewApp(store, nil, nil)
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
