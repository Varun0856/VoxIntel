package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type WhisperClient struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func NewWhisperClient(apiKey, baseURL, model string) *WhisperClient {
	return &WhisperClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

type transcriptionResponse struct {
	Text string `json:"text"`
}

func (c *WhisperClient) Transcribe(ctx context.Context, audioPath string) (string, error) {
	const maxAttemps = 3
	var lastErr error

	for attempt := 1; attempt <= maxAttemps; attempt++ {
		text, err := c.doRequest(ctx, audioPath)
		if err == nil {
			return text, nil
		}
		lastErr = err
		if !isRetryable(err) {
			return "", err
		}
		if attempt < maxAttemps {
			backoff := time.Duration(attempt) * 2 * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}
	return "", fmt.Errorf("transcription failed after %d attempts: %w", maxAttemps, lastErr)
}

func (c *WhisperClient) doRequest(ctx context.Context, audioPath string) (string, error) {
	file, err := os.Open(audioPath)
	if err != nil {
		return "", fmt.Errorf("opening audio file: %w", err)
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return "", fmt.Errorf("creating form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("copying audio into request: %w", err)
	}

	if err := writer.WriteField("model", c.model); err != nil {
		return "", fmt.Errorf("writing model field: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("closing multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/audio/transcriptions", &body)
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", &transientError{err: fmt.Errorf("request failed: %w", err)}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		return "", &transientError{err: fmt.Errorf("api returned %d: %s", resp.StatusCode, respBody)}
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("api returned %d: %s", resp.StatusCode, respBody)
	}

	var result transcriptionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	return result.Text, nil
}

type transientError struct{ err error }

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	var te *transientError
	return errors.As(err, &te)
}
