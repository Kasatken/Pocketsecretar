package security

import (
	"testing"
	"time"
)

func TestIssueAndParseAccessToken(t *testing.T) {
	manager := NewTokenManager("access-secret", "refresh-secret", 10*time.Minute, 24*time.Hour)

	token, _, err := manager.IssueAccessToken(42)
	if err != nil {
		t.Fatalf("issue access token: %v", err)
	}

	claims, err := manager.ParseAccessToken(token)
	if err != nil {
		t.Fatalf("parse access token: %v", err)
	}
	if claims.UserID != 42 {
		t.Fatalf("expected user id 42, got %d", claims.UserID)
	}
}

func TestRefreshTokenRejectsWrongSecret(t *testing.T) {
	managerA := NewTokenManager("a1", "r1", 10*time.Minute, 24*time.Hour)
	managerB := NewTokenManager("a2", "r2", 10*time.Minute, 24*time.Hour)

	token, _, err := managerA.IssueRefreshToken(77)
	if err != nil {
		t.Fatalf("issue refresh token: %v", err)
	}
	if _, err := managerB.ParseRefreshToken(token); err == nil {
		t.Fatal("expected parse error with different secret")
	}
}
