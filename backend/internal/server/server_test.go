package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"specpowers/backend/internal/config"
)

// testDSN skips the test when no real Postgres is configured.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("SP_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("SP_TEST_PG_DSN not set; skipping Postgres integration test")
	}
	return dsn
}

func testConfig(t *testing.T, dsn string) config.Config {
	return config.Config{
		Addr:          ":0",
		DatabaseURL:   dsn,
		JWTSecret:     "server-test-secret",
		Env:           "test",
		AttachmentDir: t.TempDir(),
	}
}

// TestBuildWiresHealthAndAuth checks that the extracted server builder
// produces a fully routed handler: health answers without auth, and the auth
// endpoints are mounted.
func TestBuildWiresHealthAndAuth(t *testing.T) {
	s, err := Build(context.Background(), testConfig(t, testDSN(t)), Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	defer s.Close()

	w := httptest.NewRecorder()
	s.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("health: got %d", w.Code)
	}

	email := fmt.Sprintf("srv-%d@example.com", time.Now().UnixNano())
	body, _ := json.Marshal(map[string]string{"email": email, "password": "pw123456", "display_name": "Server Test"})
	w = httptest.NewRecorder()
	s.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		bytes.NewReader(body)))
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("register: got %d body %s", w.Code, w.Body.String())
	}
}

func jsonBody(v any) io.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}
