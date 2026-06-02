package auth

import (
	"testing"
	"time"
)

func TestIssueAndParseJWT(t *testing.T) {
	secret := "test-secret"
	token, err := IssueJWT(secret, 42, "test@example.com", time.Hour)
	if err != nil {
		t.Fatalf("IssueJWT failed: %v", err)
	}
	claims, err := ParseJWT(secret, token)
	if err != nil {
		t.Fatalf("ParseJWT failed: %v", err)
	}
	if claims.UserID != 42 || claims.Email != "test@example.com" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
}
