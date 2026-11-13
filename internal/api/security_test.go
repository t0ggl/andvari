package api

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/t0ggl/andvari/internal/auth"
	"github.com/t0ggl/andvari/internal/store"
	"github.com/t0ggl/andvari/internal/webdav"
	"github.com/t0ggl/andvari/internal/worker"
)

// TestPathTraversalSecurity verifies that all forms of directory traversal attacks
// across API file listing, downloading, and deletion are safely rejected with 403 Forbidden.
func TestPathTraversalSecurity(t *testing.T) {
	api, s, sm, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)

	_, _, err := s.EnsureUserProvisioned("victim")
	if err != nil {
		t.Fatalf("failed to provision user: %v", err)
	}

	session := &auth.SessionData{
		Username: "victim",
		Groups:   []string{"users"},
	}

	// Place a secret file outside the user directory (in config directory)
	secretPath := filepath.Join(tmpDir, "config", "secret_server.key")
	if err := os.WriteFile(secretPath, []byte("SUPER_SECRET_KEY"), 0600); err != nil {
		t.Fatalf("failed to create secret file: %v", err)
	}

	traversalPayloads := []string{
		"../config/secret_server.key",
		"../../config/secret_server.key",
		"../../../etc/passwd",
		"....//....//....//etc/passwd",
		"..%2f..%2fconfig%2fsecret_server.key",
		"/etc/passwd",
		"/config/secret_server.key",
		"subdir/../../../../config/secret_server.key",
		"..\\..\\config\\secret_server.key",
		"foo/..\\..\\config\\secret_server.key",
		"secret.txt%00../../etc/passwd",
	}

	for _, payload := range traversalPayloads {
		t.Run("Download Traversal: "+payload, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/files/download?path="+payload, nil)
			addSessionCookie(t, req, sm, session)
			rec := httptest.NewRecorder()
			api.HandleFileDownload(rec, req)

			if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
				t.Fatalf("expected 403 Forbidden or 404 for traversal payload %q, got HTTP %d (body: %s)",
					payload, rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "SUPER_SECRET_KEY") {
				t.Fatalf("CRITICAL SECURITY LEAK: payload %q leaked secret content!", payload)
			}
		})

		t.Run("List Traversal: "+payload, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/files?path="+payload, nil)
			addSessionCookie(t, req, sm, session)
			rec := httptest.NewRecorder()
			api.HandleFiles(rec, req)

			if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
				t.Fatalf("expected 403 Forbidden or 404 for list traversal payload %q, got HTTP %d",
					payload, rec.Code)
			}
		})

		t.Run("Delete Traversal: "+payload, func(t *testing.T) {
			req := httptest.NewRequest("DELETE", "/api/files?path="+payload, nil)
			addSessionCookie(t, req, sm, session)
			rec := httptest.NewRecorder()
			api.HandleFiles(rec, req)

			if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
				t.Fatalf("expected 403 Forbidden or 404 for delete traversal payload %q, got HTTP %d",
					payload, rec.Code)
			}

			// Ensure secret file is still intact
			if _, err := os.Stat(secretPath); err != nil {
				t.Fatalf("CRITICAL SECURITY FLAW: secret file was deleted by payload %q!", payload)
			}
		})
	}
}

// TestSymlinkEscapeSecurity ensures that creating a symlink inside the user's storage
// directory pointing outside (e.g. to /etc or the application config directory) cannot
// be abused to read or list external files.
func TestSymlinkEscapeSecurity(t *testing.T) {
	api, s, sm, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)

	_, _, _ = s.EnsureUserProvisioned("attacker")
	userDir, err := s.UserStoragePath("attacker")
	if err != nil {
		t.Fatalf("UserStoragePath failed: %v", err)
	}

	session := &auth.SessionData{
		Username: "attacker",
		Groups:   []string{"users"},
	}

	// Create a confidential file outside user storage
	configDir := filepath.Join(tmpDir, "config")
	sensitiveFile := filepath.Join(configDir, "master.json")
	if err := os.WriteFile(sensitiveFile, []byte(`{"master_secret":"classified"}`), 0600); err != nil {
		t.Fatalf("failed to write sensitive file: %v", err)
	}

	// Create a malicious symlink inside userDir pointing to configDir
	evilSymlink := filepath.Join(userDir, "symlink_escape")
	if err := os.Symlink(configDir, evilSymlink); err != nil {
		t.Skip("Symlinks not supported on this filesystem: skipping symlink escape test")
	}

	// 1. Try listing the symlinked directory
	req := httptest.NewRequest("GET", "/api/files?path=symlink_escape", nil)
	addSessionCookie(t, req, sm, session)
	rec := httptest.NewRecorder()
	api.HandleFiles(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when listing symlink pointing outside userDir, got %d", rec.Code)
	}

	// 2. Try downloading a file through the symlink
	req = httptest.NewRequest("GET", "/api/files/download?path=symlink_escape/master.json", nil)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleFileDownload(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when downloading via symlink escape, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "classified") {
		t.Fatalf("CRITICAL SECURITY LEAK: sensitive file leaked via symlink!")
	}
}

// TestRootDirectoryDeletionProtection ensures a user cannot delete their own root directory,
// protecting against storage destruction.
func TestRootDirectoryDeletionProtection(t *testing.T) {
	api, s, sm, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)

	_, _, _ = s.EnsureUserProvisioned("user1")
	userDir, _ := s.UserStoragePath("user1")

	session := &auth.SessionData{
		Username: "user1",
		Groups:   []string{"users"},
	}

	rootDeletePayloads := []string{"", "/", ".", "./", "../user1"}

	for _, p := range rootDeletePayloads {
		req := httptest.NewRequest("DELETE", "/api/files?path="+p, nil)
		addSessionCookie(t, req, sm, session)
		rec := httptest.NewRecorder()
		api.HandleFiles(rec, req)

		if rec.Code != http.StatusForbidden && rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 403 Forbidden or 400 Bad Request when attempting to delete root with path %q, got HTTP %d", p, rec.Code)
		}

		// Verify user directory still exists
		if stat, err := os.Stat(userDir); err != nil || !stat.IsDir() {
			t.Fatalf("user storage directory was deleted by payload %q!", p)
		}
	}
}

// TestCrossUserIsolation verifies that User A cannot read or access User B's files.
func TestCrossUserIsolation(t *testing.T) {
	api, s, sm, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)

	_, _, _ = s.EnsureUserProvisioned("alice")
	_, _, _ = s.EnsureUserProvisioned("bob")

	bobDir, _ := s.UserStoragePath("bob")
	bobFile := filepath.Join(bobDir, "bob_private.txt")
	if err := os.WriteFile(bobFile, []byte("BOB_PRIVATE_DATA"), 0644); err != nil {
		t.Fatalf("failed to create bob private file: %v", err)
	}

	aliceSession := &auth.SessionData{
		Username: "alice",
		Groups:   []string{"users"},
	}

	// Alice attempts to access Bob's private file
	req := httptest.NewRequest("GET", "/api/files/download?path=../bob/bob_private.txt", nil)
	addSessionCookie(t, req, sm, sessionToCookie(aliceSession))
	rec := httptest.NewRecorder()
	api.HandleFileDownload(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when Alice accesses Bob's file, got HTTP %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "BOB_PRIVATE_DATA") {
		t.Fatalf("CRITICAL SECURITY LEAK: Alice was able to read Bob's private file!")
	}
}

func sessionToCookie(s *auth.SessionData) *auth.SessionData {
	return s
}

// TestUsernamePathTraversalValidation checks that malicious usernames are rejected immediately
// by the storage layer to prevent writing config or data files outside intended directories.
func TestUsernamePathTraversalValidation(t *testing.T) {
	_, s, _, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)

	badUsernames := []string{
		"../admin",
		"../../etc",
		"..",
		".",
		"/root",
		"user/sub",
		"user\\sub",
		"user\x00evil",
		"....//....//evil",
	}

	for _, username := range badUsernames {
		t.Run("Username: "+username, func(t *testing.T) {
			_, err := s.UserStoragePath(username)
			if !errors.Is(err, store.ErrInvalidUsername) {
				t.Fatalf("expected ErrInvalidUsername for UserStoragePath(%q), got: %v", username, err)
			}

			_, _, err = s.EnsureUserProvisioned(username)
			if !errors.Is(err, store.ErrInvalidUsername) {
				t.Fatalf("expected ErrInvalidUsername for EnsureUserProvisioned(%q), got: %v", username, err)
			}

			_, err = s.GetUser(username)
			if !errors.Is(err, store.ErrInvalidUsername) {
				t.Fatalf("expected ErrInvalidUsername for GetUser(%q), got: %v", username, err)
			}
		})
	}
}

// TestSessionTamperingAndForging checks that forged, tampered, or expired session cookies
// fail authentication cleanly without panicking.
func TestSessionTamperingAndForging(t *testing.T) {
	api, _, _, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)

	// 1. Missing cookie -> 401
	req := httptest.NewRequest("GET", "/api/me", nil)
	rec := httptest.NewRecorder()
	api.HandleMe(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing session cookie, got %d", rec.Code)
	}

	// 2. Tampered cookie content (corrupted signature)
	req = httptest.NewRequest("GET", "/api/me", nil)
	req.AddCookie(&http.Cookie{
		Name:  "andvari_session",
		Value: "tampered_payload_value_here|invalid_signature",
	})
	rec = httptest.NewRecorder()
	api.HandleMe(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for tampered session cookie, got %d", rec.Code)
	}

	// 3. Forged cookie created with different secret key
	foreignSM := auth.NewSessionManager([]byte("different-secret-key-attacker-32!"))
	foreignRec := httptest.NewRecorder()
	foreignReq := httptest.NewRequest("GET", "/", nil)
	_ = foreignSM.SaveSession(foreignRec, foreignReq, &auth.SessionData{
		Username: "admin",
		Groups:   []string{"admin"},
	})
	forgedCookie := foreignRec.Result().Cookies()[0]

	req = httptest.NewRequest("GET", "/api/me", nil)
	req.AddCookie(forgedCookie)
	rec = httptest.NewRecorder()
	api.HandleMe(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for forged session cookie signed with foreign key, got %d", rec.Code)
	}
}

// TestOAuthCSRFProtection verifies that OIDC callback strictly validates state parameters
// to prevent CSRF authentication hijacking.
func TestOAuthCSRFProtection(t *testing.T) {
	api, _, _, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)

	// 1. Missing state cookie -> 400
	req := httptest.NewRequest("GET", "/auth/callback?state=xyz&code=abc", nil)
	rec := httptest.NewRecorder()
	api.HandleCallback(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing state cookie, got %d", rec.Code)
	}

	// 2. Mismatched state parameter -> 400
	req = httptest.NewRequest("GET", "/auth/callback?state=attacker_state&code=abc", nil)
	req.AddCookie(&http.Cookie{
		Name:  "andvari_oauth_state",
		Value: "legitimate_state",
	})
	rec = httptest.NewRecorder()
	api.HandleCallback(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for mismatched state, got %d", rec.Code)
	}

	// 3. Missing code -> 400
	req = httptest.NewRequest("GET", "/auth/callback?state=legitimate_state", nil)
	req.AddCookie(&http.Cookie{
		Name:  "andvari_oauth_state",
		Value: "legitimate_state",
	})
	rec = httptest.NewRecorder()
	api.HandleCallback(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing code, got %d", rec.Code)
	}
}

// TestWebDAVSecurity verifies that WebDAV enforces authentication and isolates storage.
func TestWebDAVSecurity(t *testing.T) {
	_, s, _, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)

	_, tokenAlice, _ := s.EnsureUserProvisioned("alice")
	_, _, _ = s.EnsureUserProvisioned("bob")

	// Store a secret file in bob's directory
	bobDir, _ := s.UserStoragePath("bob")
	_ = os.WriteFile(filepath.Join(bobDir, "bob_private.txt"), []byte("BOB_SECRET"), 0644)

	server := webdav.NewServer(s)

	// 1. WebDAV unauthenticated request -> 401
	req := httptest.NewRequest("GET", "/webdav/bob_private.txt", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated WebDAV request, got %d", rec.Code)
	}

	// 2. WebDAV request with invalid token -> 401
	req = httptest.NewRequest("GET", "/webdav/bob_private.txt", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("alice:invalid-token")))
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid WebDAV token, got %d", rec.Code)
	}

	// 3. WebDAV path traversal attempt by Alice to read Bob's file
	req = httptest.NewRequest("GET", "/webdav/../bob/bob_private.txt", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("alice:"+tokenAlice)))
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	// golang.org/x/net/webdav confines Dir(userDir) so traversal yields 404 or 400, never 200 with Bob's content
	if strings.Contains(rec.Body.String(), "BOB_SECRET") {
		t.Fatalf("CRITICAL SECURITY LEAK: Alice accessed Bob's file via WebDAV path traversal!")
	}
}

// TestWorkerImpersonationIsolation verifies that file explorer operations with admin
// impersonation route to the impersonated user's Landlocked worker process.
func TestWorkerImpersonationIsolation(t *testing.T) {
	api, s, sm, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)

	wm := worker.NewManager(filepath.Join(tmpDir, "workers"), 1*time.Minute)
	defer wm.Close()

	apiWithWorker := NewAPI(s, sm, api.oidcClient, wm)

	_, _, err := s.EnsureUserProvisioned("adminuser")
	if err != nil {
		t.Fatalf("failed to provision adminuser: %v", err)
	}
	_, _, err = s.EnsureUserProvisioned("bob")
	if err != nil {
		t.Fatalf("failed to provision bob: %v", err)
	}
	_, _, err = s.EnsureUserProvisioned("alice")
	if err != nil {
		t.Fatalf("failed to provision alice: %v", err)
	}

	bobDir, _ := s.UserStoragePath("bob")
	aliceDir, _ := s.UserStoragePath("alice")
	_ = os.WriteFile(filepath.Join(bobDir, "bob_file.txt"), []byte("bob file contents"), 0644)
	_ = os.WriteFile(filepath.Join(aliceDir, "alice_file.txt"), []byte("alice file contents"), 0644)

	// Admin impersonating Bob
	session := &auth.SessionData{
		Username:         "adminuser",
		Groups:           []string{"admin"},
		ImpersonatedUser: "bob",
	}

	// 1. List files while impersonating Bob -> should return Bob's files
	listReq := httptest.NewRequest("GET", "/api/files", nil)
	addSessionCookie(t, listReq, sm, session)
	listRec := httptest.NewRecorder()
	apiWithWorker.HandleFiles(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for impersonated list, got %d (body: %s)", listRec.Code, listRec.Body.String())
	}
	if !strings.Contains(listRec.Body.String(), "bob_file.txt") {
		t.Errorf("expected bob_file.txt in list response, got %s", listRec.Body.String())
	}
	if strings.Contains(listRec.Body.String(), "alice_file.txt") {
		t.Errorf("SECURITY LEAK: impersonated list leaked alice_file.txt: %s", listRec.Body.String())
	}

	// 2. Download file while impersonating Bob -> should download Bob's file
	downReq := httptest.NewRequest("GET", "/api/files/download?path=bob_file.txt", nil)
	addSessionCookie(t, downReq, sm, session)
	downRec := httptest.NewRecorder()
	apiWithWorker.HandleFileDownload(downRec, downReq)

	if downRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for download, got %d", downRec.Code)
	}
	if downRec.Body.String() != "bob file contents" {
		t.Errorf("expected 'bob file contents', got %q", downRec.Body.String())
	}

	// 3. Clear impersonation: adminuser now sees own empty folder
	session.ImpersonatedUser = ""
	adminListReq := httptest.NewRequest("GET", "/api/files", nil)
	addSessionCookie(t, adminListReq, sm, session)
	adminListRec := httptest.NewRecorder()
	apiWithWorker.HandleFiles(adminListRec, adminListReq)

	if adminListRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin own list, got %d", adminListRec.Code)
	}
	if strings.Contains(adminListRec.Body.String(), "bob_file.txt") || strings.Contains(adminListRec.Body.String(), "alice_file.txt") {
		t.Errorf("SECURITY LEAK: admin list leaked other users files: %s", adminListRec.Body.String())
	}
}

// TestWebDAVWithWorkerServer verifies WebDAV requests through the worker manager.
func TestWebDAVWithWorkerServer(t *testing.T) {
	_, s, _, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)

	wm := worker.NewManager(filepath.Join(tmpDir, "workers"), 1*time.Minute)
	defer wm.Close()

	server := webdav.NewServer(s, wm)

	_, tokenAlice, _ := s.EnsureUserProvisioned("alice")
	aliceDir, _ := s.UserStoragePath("alice")

	// 1. Upload via WebDAV worker
	putReq := httptest.NewRequest("PUT", "/webdav/report.txt", strings.NewReader("Quarterly Report"))
	putReq.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("alice:"+tokenAlice)))
	putReq.Header.Set("Content-Length", "16")
	putRec := httptest.NewRecorder()
	server.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusCreated && putRec.Code != http.StatusNoContent {
		t.Fatalf("expected 201/204 for WebDAV PUT with worker, got %d", putRec.Code)
	}

	// Verify file on disk
	content, err := os.ReadFile(filepath.Join(aliceDir, "report.txt"))
	if err != nil || string(content) != "Quarterly Report" {
		t.Fatalf("file content on disk mismatch: %q (err: %v)", string(content), err)
	}

	// 2. Read via WebDAV worker
	getReq := httptest.NewRequest("GET", "/webdav/report.txt", nil)
	getReq.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("alice:"+tokenAlice)))
	getRec := httptest.NewRecorder()
	server.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for WebDAV GET with worker, got %d", getRec.Code)
	}
	if getRec.Body.String() != "Quarterly Report" {
		t.Errorf("expected 'Quarterly Report', got %q", getRec.Body.String())
	}
}
