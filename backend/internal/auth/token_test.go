package auth

import (
	"testing"
	"time"
)

func newTestTokens() *TokenService {
	return NewTokenService("test-secret", 15*time.Minute)
}

func TestIssueAndVerifyRoundTrip(t *testing.T) {
	ts := newTestTokens()
	token, err := ts.Issue("user-123")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	userID, err := ts.Verify(token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if userID != "user-123" {
		t.Errorf("userID = %q, want user-123", userID)
	}
}

func TestVerifyRejectsExpired(t *testing.T) {
	ts := NewTokenService("test-secret", -time.Minute)
	token, err := ts.Issue("user-123")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := ts.Verify(token); err == nil {
		t.Error("expired token accepted")
	}
}

func TestVerifyRejectsTamperedPayload(t *testing.T) {
	ts := newTestTokens()
	token, err := ts.Issue("user-123")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// Flip a character in the payload segment.
	b := []byte(token)
	for i := 1; i < len(b); i++ {
		if b[i] == '.' {
			b[i-1]++
			break
		}
	}
	if _, err := ts.Verify(string(b)); err == nil {
		t.Error("tampered token accepted")
	}
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	other := NewTokenService("other-secret", 15*time.Minute)
	token, err := other.Issue("user-123")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := newTestTokens().Verify(token); err == nil {
		t.Error("token signed with wrong secret accepted")
	}
}

func TestIssueRejectsEmptyUserID(t *testing.T) {
	if _, err := newTestTokens().Issue(""); err == nil {
		t.Error("empty userID accepted")
	}
}
