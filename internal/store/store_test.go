package store

import (
	"os"
	"testing"
)

func TestStoreOperations(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configDir := tmpDir + "/config"
	storageDir := tmpDir + "/data"

	s, err := NewStore(configDir, storageDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// 1. Ensure provisioning creates initial token
	user, initialToken, err := s.EnsureUserProvisioned("alice")
	if err != nil {
		t.Fatalf("EnsureUserProvisioned failed: %v", err)
	}
	if initialToken == "" {
		t.Fatalf("expected initialToken to be non-empty")
	}
	if len(user.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(user.Tokens))
	}

	// 2. Authenticate initial token
	valid, _, err := s.AuthenticateToken("alice", initialToken)
	if err != nil || !valid {
		t.Fatalf("AuthenticateToken failed for initial token: valid=%v, err=%v", valid, err)
	}

	// 3. Second authentication should hit cache and succeed
	valid, _, err = s.AuthenticateToken("alice", initialToken)
	if err != nil || !valid {
		t.Fatalf("AuthenticateToken cache hit failed: valid=%v, err=%v", valid, err)
	}

	// 4. Add second token
	secondToken, token2Info, err := s.AddToken("alice", "Token 2")
	if err != nil {
		t.Fatalf("AddToken failed: %v", err)
	}
	if secondToken == "" || token2Info == nil {
		t.Fatalf("expected second token and info")
	}

	// 5. Try adding 3rd token -> should fail
	_, _, err = s.AddToken("alice", "Token 3")
	if err != ErrMaxTokensReached {
		t.Fatalf("expected ErrMaxTokensReached, got: %v", err)
	}

	// 6. Authenticate second token
	valid, _, err = s.AuthenticateToken("alice", secondToken)
	if err != nil || !valid {
		t.Fatalf("AuthenticateToken failed for second token: valid=%v, err=%v", valid, err)
	}

	// 7. Revoke initial token
	initialTokenID := user.Tokens[0].ID
	if err := s.RevokeToken("alice", initialTokenID); err != nil {
		t.Fatalf("RevokeToken failed: %v", err)
	}

	// 8. Authenticate initial token -> should now fail
	valid, _, err = s.AuthenticateToken("alice", initialToken)
	if err != nil || valid {
		t.Fatalf("expected initial token to be invalid after revocation, got valid=%v", valid)
	}

	// 9. Now adding a replacement token should succeed
	thirdToken, _, err := s.AddToken("alice", "Token 3")
	if err != nil {
		t.Fatalf("AddToken should succeed after revocation: %v", err)
	}
	if thirdToken == "" {
		t.Fatalf("expected valid third token")
	}
}
