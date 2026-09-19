package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type GroqAnalyzer struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func NewGroqAnalyzer(apiKey, baseURL, model string) *GroqAnalyzer {
	return &GroqAnalyzer{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

const systemPrompt = `Create a "Top Takeaways" section for this session that will be displayed immediately after the session ends.

The purpose is NOT to summarize the session comprehensively. It is to capture the handful of ideas that an attendee should remember, photograph, and take back with them.

Identify 5-7 of the strongest takeaways from the session, prioritizing:
- Provocative or counterintuitive ideas
- Specific insights that challenge conventional thinking
- Practical lessons attendees can apply
- Important observations, shifts, or implications discussed
- Memorable points that make someone think, "That's worth remembering."
- Any strong frameworks, principles, or distinctions introduced during the session

Write each takeaway as a strong, standalone statement, followed by a short explanation if needed.

Avoid:
- Generic statements such as "Collaboration is important"
- Session-summary language such as "The speaker discussed..."
- Repeating the agenda or presentation structure
- Corporate jargon and marketing language
- Long paragraphs
- Claims or insights that were not actually discussed in the session

Use only the content provided from the session. Do not invent insights.

Return your response strictly as JSON with no prose outside the JSON, in exactly this shape:
{"takeaways":[{"statement":"...","explanation":"..."}]}
Include 5 to 7 items. "explanation" may be an empty string if the statement stands on its own.`

func (a *GroqAnalyzer) Analyze(ctx context.Context, transcript, prompt string) (string, error) {
	const maxAttempts = 3
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		answer, err := a.doRequest(ctx, transcript, prompt)
		if err == nil {
			return answer, nil
		}
		lastErr = err

		if !isRetryable(err) {
			return "", err
		}
		if attempt < maxAttempts {
			backoff := time.Duration(attempt) * 2 * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}
	return "", fmt.Errorf("analysis failed after %d attempts: %w", maxAttempts, lastErr)
}

func (a *GroqAnalyzer) doRequest(ctx context.Context, transcript, prompt string) (string, error) {
	userContent := fmt.Sprintf("Transcript:\n%s\n\nQuestion: %s", transcript, prompt)

	reqBody := chatRequest{
		Model: a.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		},
		ResponseFormat: &responseFormat{Type: "json_object"},
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.client.Do(req)
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

	var result chatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("api returned no choices")
	}
	return result.Choices[0].Message.Content, nil
}
