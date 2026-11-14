package warden

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/t0ggl/andvari/internal/auth"
	"github.com/t0ggl/andvari/internal/store"
	"github.com/t0ggl/andvari/internal/worker"
)

func setupTestWarden(t *testing.T) (*Server, *Client, *store.Store, *auth.SessionManager, string) {
	tmpDir, err := os.MkdirTemp("", "andvari-warden-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	configDir := filepath.Join(tmpDir, "config")
	storageDir := filepath.Join(tmpDir, "data")
	workerDir := filepath.Join(tmpDir, "workers")

	s, err := store.NewStore(configDir, storageDir, 1000000)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	sm := auth.NewSessionManager([]byte("test-session-secret-32-bytes-long!"))
	oidc, err := auth.NewOIDCClient(context.Background(), "", "", "", "", []string{}, "admin", "preferred_username", true)
	if err != nil {
		t.Fatalf("NewOIDCClient failed: %v", err)
	}

	wm := worker.NewManager(workerDir, 5*time.Minute)

	sockPath := filepath.Join(workerDir, "warden.sock")
	srv, err := NewServer(s, sm, oidc, wm, sockPath)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	client := NewClient(srv.SocketPath())
	return srv, client, s, sm, tmpDir
}

func TestWardenWebDAVAuth(t *testing.T) {
	srv, client, s, _, tmpDir := setupTestWarden(t)
	defer os.RemoveAll(tmpDir)
	defer srv.Close()

	_, token, err := s.EnsureUserProvisioned("alice")
	if err != nil {
		t.Fatalf("failed to provision user: %v", err)
	}

	// 1. Valid authentication
	authRes, err := client.AuthenticateWebDAV("alice", token)
	if err != nil {
		t.Fatalf("AuthenticateWebDAV failed: %v", err)
	}
	if !authRes.Authenticated {
		t.Fatalf("expected authenticated=true, got false: %s", authRes.Error)
	}
	if authRes.Username != "alice" {
		t.Errorf("expected username 'alice', got %s", authRes.Username)
	}
	if authRes.SocketPath == "" {
		t.Errorf("expected non-empty socket path for worker")
	}

	// 2. Invalid token
	invalidRes, err := client.AuthenticateWebDAV("alice", "bad_token")
	if err != nil {
		t.Fatalf("AuthenticateWebDAV unexpected error: %v", err)
	}
	if invalidRes.Authenticated {
		t.Errorf("expected authenticated=false for invalid token")
	}

	// 3. Nonexistent user
	nonExistentRes, err := client.AuthenticateWebDAV("nobody", "token")
	if err != nil {
		t.Fatalf("AuthenticateWebDAV unexpected error: %v", err)
	}
	if nonExistentRes.Authenticated {
		t.Errorf("expected authenticated=false for nonexistent user")
	}
}

func TestWardenTokenLifecycle(t *testing.T) {
	srv, client, _, _, tmpDir := setupTestWarden(t)
	defer os.RemoveAll(tmpDir)
	defer srv.Close()

	cookie, err := client.CreateSession("bob", []string{"users"}, "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// 1. Get initial user info
	me, err := client.GetUserInfo(cookie)
	if err != nil {
		t.Fatalf("GetUserInfo failed: %v", err)
	}
	if me.Username != "bob" {
		t.Errorf("expected username 'bob', got %s", me.Username)
	}

	// 2. Create token
	createRes, err := client.CreateToken(cookie, "Laptop Sync")
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	if createRes.Token == "" || createRes.TokenInfo == nil {
		t.Fatalf("expected valid token and tokenInfo in response")
	}
	tokenID := createRes.TokenInfo.ID

	// 3. Rename token
	renamed, err := client.RenameToken(cookie, tokenID, "Desktop Sync")
	if err != nil {
		t.Fatalf("RenameToken failed: %v", err)
	}
	if renamed.Label != "Desktop Sync" {
		t.Errorf("expected label 'Desktop Sync', got %s", renamed.Label)
	}

	// 4. Revoke token
	if err := client.RevokeToken(cookie, tokenID); err != nil {
		t.Fatalf("RevokeToken failed: %v", err)
	}

	// 5. Complete onboarding
	if err := client.CompleteOnboarding(cookie); err != nil {
		t.Fatalf("CompleteOnboarding failed: %v", err)
	}

	meAfter, err := client.GetUserInfo(cookie)
	if err != nil {
		t.Fatalf("GetUserInfo failed: %v", err)
	}
	if !meAfter.FirstLoginCompleted {
		t.Errorf("expected FirstLoginCompleted=true")
	}
}

func TestWardenAdminPrivileges(t *testing.T) {
	srv, client, _, _, tmpDir := setupTestWarden(t)
	defer os.RemoveAll(tmpDir)
	defer srv.Close()

	userCookie, err := client.CreateSession("standard_user", []string{"users"}, "")
	if err != nil {
		t.Fatalf("CreateSession user failed: %v", err)
	}

	adminCookie, err := client.CreateSession("admin_user", []string{"admin"}, "")
	if err != nil {
		t.Fatalf("CreateSession admin failed: %v", err)
	}

	// Non-admin attempts admin ops -> forbidden
	if _, err := client.AdminListUsers(userCookie); err == nil {
		t.Errorf("expected forbidden for non-admin ListUsers")
	}
	var quota int64 = 5000
	if err := client.AdminSetQuota(userCookie, "standard_user", &quota); err == nil {
		t.Errorf("expected forbidden for non-admin SetQuota")
	}
	if _, err := client.AdminImpersonate(userCookie, "standard_user"); err == nil {
		t.Errorf("expected forbidden for non-admin Impersonate")
	}

	// Admin executes admin ops -> succeeds
	users, err := client.AdminListUsers(adminCookie)
	if err != nil {
		t.Fatalf("AdminListUsers failed: %v", err)
	}
	if len(users) < 2 {
		t.Errorf("expected at least 2 users, got %d", len(users))
	}

	if err := client.AdminSetQuota(adminCookie, "standard_user", &quota); err != nil {
		t.Fatalf("AdminSetQuota failed: %v", err)
	}

	impRes, err := client.AdminImpersonate(adminCookie, "standard_user")
	if err != nil {
		t.Fatalf("AdminImpersonate failed: %v", err)
	}
	if impRes.Impersonating != "standard_user" {
		t.Errorf("expected impersonating standard_user, got %s", impRes.Impersonating)
	}

	// Verify me endpoint under impersonation
	meImp, err := client.GetUserInfo(impRes.SessionCookie)
	if err != nil {
		t.Fatalf("GetUserInfo under impersonation failed: %v", err)
	}
	if meImp.EffectiveUsername != "standard_user" || !meImp.IsImpersonating {
		t.Errorf("expected effectiveUser standard_user with isImpersonating=true, got %s (%v)", meImp.EffectiveUsername, meImp.IsImpersonating)
	}

	// Stop impersonation
	stopRes, err := client.AdminStopImpersonate(impRes.SessionCookie)
	if err != nil {
		t.Fatalf("AdminStopImpersonate failed: %v", err)
	}
	if stopRes.Impersonating != "" {
		t.Errorf("expected cleared impersonating, got %s", stopRes.Impersonating)
	}
}
