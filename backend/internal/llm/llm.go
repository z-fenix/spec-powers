// Package llm provides the AI/LLM access layer: a minimal OpenAI-compatible
// chat-completions client plus the Client interface the classic splitter
// mocks in tests.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client generates one completion for a system + user prompt pair.
type Client interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// OpenAIClient talks to any OpenAI-compatible /chat/completions endpoint.
type OpenAIClient struct {
	BaseURL string // e.g. https://api.openai.com/v1
	APIKey  string
	Model   string
	HTTP    *http.Client
}

func NewOpenAIClient(baseURL, apiKey, model string) *OpenAIClient {
	return &OpenAIClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		HTTP:    &http.Client{Timeout: 5 * time.Minute},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *OpenAIClient) Complete(ctx context.Context, system, user string) (string, error) {
	if c.APIKey == "" {
		return "", fmt.Errorf("llm: API key is required")
	}
	if c.Model == "" {
		return "", fmt.Errorf("llm: model is required")
	}
	body, err := json.Marshal(chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		return "", fmt.Errorf("llm: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("llm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: call failed: %w", err)
	}
	defer resp.Body.Close()
	var parsed chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("llm: decode response (status %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm: endpoint returned status %d", resp.StatusCode)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("llm: response has no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}
