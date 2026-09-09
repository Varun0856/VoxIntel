package main

import "time"

type Status string

const (
	StatusRecorded     Status = "recorded"
	StatusTranscribing Status = "transcribing"
	StatusTranscribed  Status = "transcribed"
	StatusFailed       Status = "failed"
)

type AnalysisResult struct {
	Prompt    string    `json:"prompt"`
	Answer    string    `json:"answer"`
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	ID         string           `json:"id"`
	CreatedAt  time.Time        `json:"created_at"`
	Status     Status           `json:"status"`
	AudioPath  string           `json:"audio_path,omitempty"`
	Transcript string           `json:"transcript,omitempty"`
	Error      string           `json:"error,omitempty"`
	Analyses   []AnalysisResult `json:"analyses,omitempty"`
}
