package auth

import (
	"testing"
)

func TestArgon2idHashAndCompare(t *testing.T) {
	token, err := GenerateSecureToken("andv")
	if err != nil {
		t.Fatalf("GenerateSecureToken failed: %v", err)
	}

	hash, err := HashPassword(token)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	match, err := ComparePasswordAndHash(token, hash)
	if err != nil {
		t.Fatalf("ComparePasswordAndHash returned error: %v", err)
	}
	if !match {
		t.Fatalf("expected password to match hash")
	}

	wrongMatch, err := ComparePasswordAndHash("wrong-password", hash)
	if err != nil {
		t.Fatalf("ComparePasswordAndHash returned error on wrong password: %v", err)
	}
	if wrongMatch {
		t.Fatalf("expected wrong password to not match")
	}
}
