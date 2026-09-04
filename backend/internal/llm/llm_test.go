package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type capturedRequest struct {
	auth     string
	path     string
	model    string
	messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
}

func newTestServer(t *testing.T, status int, body string, captured *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if captured != nil {
			captured.auth = r.Header.Get("Authorization")
			captured.path = r.URL.Path
			raw, _ := io.ReadAll(r.Body)
			var parsed struct {
				Model    string `json:"model"`
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}
			_ = json.Unmarshal(raw, &parsed)
			captured.model = parsed.Model
			captured.messages = parsed.Messages
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestOpenAIClientComplete(t *testing.T) {
	captured := &capturedRequest{}
	srv := newTestServer(t, http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"hello output"}}]}`, captured)
	defer srv.Close()

	c := &OpenAIClient{BaseURL: srv.URL, APIKey: "sk-test", Model: "m1"}
	got, err := c.Complete(context.Background(), "be a splitter", "split this")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != "hello output" {
		t.Errorf("Complete = %q, want %q", got, "hello output")
	}
	if captured.auth != "Bearer sk-test" {
		t.Errorf("Authorization = %q, want Bearer sk-test", captured.auth)
	}
	if captured.path != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", captured.path)
	}
	if captured.model != "m1" {
		t.Errorf("model = %q, want m1", captured.model)
	}
	if len(captured.messages) != 2 ||
		captured.messages[0].Role != "system" || captured.messages[0].Content != "be a splitter" ||
		captured.messages[1].Role != "user" || captured.messages[1].Content != "split this" {
		t.Errorf("messages = %+v, want [system, user]", captured.messages)
	}
}

func TestOpenAIClientHTTPError(t *testing.T) {
	srv := newTestServer(t, http.StatusUnauthorized, `{"error":{"message":"bad key"}}`, nil)
	defer srv.Close()

	c := &OpenAIClient{BaseURL: srv.URL, APIKey: "sk-test", Model: "m1"}
	_, err := c.Complete(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("expected error on 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401, got %v", err)
	}
}

func TestOpenAIClientEmptyChoices(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, `{"choices":[]}`, nil)
	defer srv.Close()

	c := &OpenAIClient{BaseURL: srv.URL, APIKey: "sk-test", Model: "m1"}
	_, err := c.Complete(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("expected error on empty choices")
	}
}

func TestOpenAIClientMissingAPIKey(t *testing.T) {
	c := &OpenAIClient{BaseURL: "http://unused", APIKey: "", Model: "m1"}
	if _, err := c.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("expected error when API key is missing")
	}
}

func TestOpenAIClientIntegration(t *testing.T) {
	key := os.Getenv("SP_TEST_LLM_API_KEY")
	if key == "" {
		t.Skip("SP_TEST_LLM_API_KEY not set; skipping LLM integration test")
	}
	base := os.Getenv("SP_TEST_LLM_BASE_URL")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	model := os.Getenv("SP_TEST_LLM_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}
	c := &OpenAIClient{BaseURL: base, APIKey: key, Model: model}
	out, err := c.Complete(context.Background(), "Reply with exactly one word.", "Say: pong")
	if err != nil {
		t.Fatalf("integration Complete: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Error("integration output should not be empty")
	}
}
