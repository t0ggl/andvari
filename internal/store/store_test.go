package store

import (
	"fmt"
	"os"
	"path/filepath"
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

	// 5. Add tokens up to MaxTokensPerUser (20)
	for i := 3; i <= MaxTokensPerUser; i++ {
		_, _, err := s.AddToken("alice", fmt.Sprintf("Token %d", i))
		if err != nil {
			t.Fatalf("AddToken failed for token %d: %v", i, err)
		}
	}

	// 6. Try adding 21st token -> should fail
	_, _, err = s.AddToken("alice", fmt.Sprintf("Token %d", MaxTokensPerUser+1))
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

func TestStoreQuotas(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-quota-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configDir := tmpDir + "/config"
	storageDir := tmpDir + "/data"

	// 1000 bytes default quota
	s, err := NewStore(configDir, storageDir, 1000)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	_, _, err = s.EnsureUserProvisioned("quotauser")
	if err != nil {
		t.Fatalf("EnsureUserProvisioned failed: %v", err)
	}

	// Initial usage should be 0, quota should be 1000
	used, quota, err := s.GetEffectiveQuota("quotauser")
	if err != nil {
		t.Fatalf("GetEffectiveQuota failed: %v", err)
	}
	if used != 0 || quota != 1000 {
		t.Fatalf("expected 0 used and 1000 quota, got used=%d quota=%d", used, quota)
	}

	// Write a 250 byte file to user storage
	userDir, _ := s.UserStoragePath("quotauser")
	testFile := userDir + "/sample.txt"
	if err := os.WriteFile(testFile, make([]byte, 250), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Invalidate cache and check
	s.InvalidateUsage("quotauser")
	used, quota, err = s.GetEffectiveQuota("quotauser")
	if err != nil {
		t.Fatalf("GetEffectiveQuota failed: %v", err)
	}
	if used != 250 || quota != 1000 {
		t.Fatalf("expected 250 used and 1000 quota, got used=%d quota=%d", used, quota)
	}

	// Set custom user quota to 5000 bytes
	var customQuota int64 = 5000
	if err := s.SetUserQuota("quotauser", &customQuota); err != nil {
		t.Fatalf("SetUserQuota failed: %v", err)
	}

	used, quota, err = s.GetEffectiveQuota("quotauser")
	if err != nil {
		t.Fatalf("GetEffectiveQuota failed: %v", err)
	}
	if used != 250 || quota != 5000 {
		t.Fatalf("expected 250 used and 5000 quota, got used=%d quota=%d", used, quota)
	}

	// Reset user quota back to default (nil)
	if err := s.SetUserQuota("quotauser", nil); err != nil {
		t.Fatalf("SetUserQuota nil failed: %v", err)
	}
	_, quota, err = s.GetEffectiveQuota("quotauser")
	if err != nil {
		t.Fatalf("GetEffectiveQuota failed: %v", err)
	}
	if quota != 1000 {
		t.Fatalf("expected quota reset to 1000, got %d", quota)
	}
}

func TestStoreRenameToken(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-rename-test-*")
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

	user, _, err := s.EnsureUserProvisioned("bob")
	if err != nil {
		t.Fatalf("EnsureUserProvisioned failed: %v", err)
	}

	tokenID := user.Tokens[0].ID

	// 1. Rename existing token
	updated, err := s.RenameToken("bob", tokenID, "Work Laptop")
	if err != nil {
		t.Fatalf("RenameToken failed: %v", err)
	}
	if updated.Label != "Work Laptop" {
		t.Fatalf("expected label 'Work Laptop', got %q", updated.Label)
	}
	if updated.ID != tokenID {
		t.Fatalf("expected token ID to remain unchanged, got %q", updated.ID)
	}

	// 2. Verify GetUser returns updated token label
	freshUser, err := s.GetUser("bob")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if len(freshUser.Tokens) != 1 || freshUser.Tokens[0].Label != "Work Laptop" {
		t.Fatalf("expected token label in store to be 'Work Laptop', got %q", freshUser.Tokens[0].Label)
	}

	// 3. Rename with empty or whitespace label fails
	if _, err := s.RenameToken("bob", tokenID, ""); err != ErrEmptyTokenLabel {
		t.Fatalf("expected ErrEmptyTokenLabel for empty label, got %v", err)
	}
	if _, err := s.RenameToken("bob", tokenID, "   "); err != ErrEmptyTokenLabel {
		t.Fatalf("expected ErrEmptyTokenLabel for whitespace label, got %v", err)
	}

	// 4. Rename with non-existent tokenID fails
	if _, err := s.RenameToken("bob", "non-existent-id", "New Name"); err != ErrTokenNotFound {
		t.Fatalf("expected ErrTokenNotFound, got %v", err)
	}

	// 5. Rename for non-existent user fails
	if _, err := s.RenameToken("nonexistent", tokenID, "New Name"); err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

func TestStoreCheckHealth(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-health-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configDir := filepath.Join(tmpDir, "config")
	storageDir := filepath.Join(tmpDir, "data")

	s, err := NewStore(configDir, storageDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// 1. Healthy store
	if err := s.CheckHealth(); err != nil {
		t.Fatalf("expected healthy check, got: %v", err)
	}

	if s.ConfigDir() != configDir {
		t.Fatalf("expected ConfigDir %q, got %q", configDir, s.ConfigDir())
	}
	if s.StorageDir() != storageDir {
		t.Fatalf("expected StorageDir %q, got %q", storageDir, s.StorageDir())
	}

	// 2. Unwritable config directory
	if err := os.Chmod(configDir, 0555); err != nil {
		t.Fatalf("failed to chmod configDir: %v", err)
	}
	if err := s.CheckHealth(); err == nil {
		_ = os.Chmod(configDir, 0755)
		t.Fatalf("expected error for unwritable configDir, got nil")
	}
	_ = os.Chmod(configDir, 0755)

	// Healthy again
	if err := s.CheckHealth(); err != nil {
		t.Fatalf("expected healthy check after restoring configDir, got: %v", err)
	}

	// 3. Unwritable storage directory
	if err := os.Chmod(storageDir, 0555); err != nil {
		t.Fatalf("failed to chmod storageDir: %v", err)
	}
	if err := s.CheckHealth(); err == nil {
		_ = os.Chmod(storageDir, 0755)
		t.Fatalf("expected error for unwritable storageDir, got nil")
	}
	_ = os.Chmod(storageDir, 0755)

	// Healthy again
	if err := s.CheckHealth(); err != nil {
		t.Fatalf("expected healthy check after restoring storageDir, got: %v", err)
	}
}
