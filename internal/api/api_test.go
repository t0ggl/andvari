package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/t0ggl/andvari/internal/auth"
	"github.com/t0ggl/andvari/internal/store"
)

func setupTestAPI(t *testing.T) (*API, *store.Store, *auth.SessionManager, string) {
	tmpDir, err := os.MkdirTemp("", "andvari-api-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	configDir := filepath.Join(tmpDir, "config")
	storageDir := filepath.Join(tmpDir, "data")

	s, err := store.NewStore(configDir, storageDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	sm := auth.NewSessionManager([]byte("test-session-secret-32-bytes-long!"))
	oidc, err := auth.NewOIDCClient(context.Background(), "", "", "", "", []string{}, "admin", "preferred_username", true)
	if err != nil {
		t.Fatalf("NewOIDCClient failed: %v", err)
	}

	return NewAPI(s, sm, oidc), s, sm, tmpDir
}

func addSessionCookie(t *testing.T, req *http.Request, sm *auth.SessionManager, session *auth.SessionData) {
	rec := httptest.NewRecorder()
	if err := sm.SaveSession(rec, req, session); err != nil {
		t.Fatalf("failed to save test session: %v", err)
	}
	cookie := rec.Result().Cookies()[0]
	req.AddCookie(cookie)
}

func TestAPITokenLifecycleAndLimits(t *testing.T) {
	api, s, sm, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)

	_, initialToken, err := s.EnsureUserProvisioned("charlie")
	if err != nil {
		t.Fatalf("failed to provision charlie: %v", err)
	}

	session := &auth.SessionData{
		Username:   "charlie",
		Groups:     []string{"users"},
		FlashToken: initialToken,
	}

	// 1. GET /api/me should return user info and flash token
	req := httptest.NewRequest("GET", "/api/me", nil)
	addSessionCookie(t, req, sm, session)
	rec := httptest.NewRecorder()
	api.HandleMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var meResp UserInfoResponse
	if err := json.NewDecoder(rec.Body).Decode(&meResp); err != nil {
		t.Fatalf("failed to decode me response: %v", err)
	}
	if meResp.Username != "charlie" || len(meResp.Tokens) != 1 || meResp.FlashToken != initialToken {
		t.Fatalf("unexpected me response: %+v", meResp)
	}

	// 2. POST /api/tokens to generate second token
	reqBody := bytes.NewBufferString(`{"label":"Token 2"}`)
	req = httptest.NewRequest("POST", "/api/tokens", reqBody)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleGenerateToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var genResp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&genResp); err != nil {
		t.Fatalf("failed to decode token response: %v", err)
	}
	token2Raw := genResp["token"].(string)
	if token2Raw == "" {
		t.Fatalf("expected raw token in response")
	}

	// 3. POST /api/tokens up to 20 tokens
	for i := 3; i <= store.MaxTokensPerUser; i++ {
		reqBody = bytes.NewBufferString(fmt.Sprintf(`{"label":"Token %d"}`, i))
		req = httptest.NewRequest("POST", "/api/tokens", reqBody)
		addSessionCookie(t, req, sm, session)
		rec = httptest.NewRecorder()
		api.HandleGenerateToken(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 when generating token %d, got %d: %s", i, rec.Code, rec.Body.String())
		}
	}

	// 4. POST /api/tokens for a 21st token -> must fail with 400
	reqBody = bytes.NewBufferString(fmt.Sprintf(`{"label":"Token %d"}`, store.MaxTokensPerUser+1))
	req = httptest.NewRequest("POST", "/api/tokens", reqBody)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleGenerateToken(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when generating 21st token, got %d", rec.Code)
	}

	// 5. Revoke token 2
	tokInfo := genResp["token_info"].(map[string]interface{})
	token2ID := tokInfo["id"].(string)

	req = httptest.NewRequest("DELETE", "/api/tokens?id="+token2ID, nil)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleRevokeToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on revoke, got %d", rec.Code)
	}

	// 6. User should now have 19 active tokens
	user, err := s.GetUser("charlie")
	if err != nil || len(user.Tokens) != store.MaxTokensPerUser-1 {
		t.Fatalf("expected %d tokens remaining, got %d", store.MaxTokensPerUser-1, len(user.Tokens))
	}
}

func TestAdminImpersonation(t *testing.T) {
	api, s, sm, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)

	_, _, _ = s.EnsureUserProvisioned("admin_user")
	_, _, _ = s.EnsureUserProvisioned("target_user")

	adminSession := &auth.SessionData{
		Username: "admin_user",
		Groups:   []string{"admin"},
	}

	nonAdminSession := &auth.SessionData{
		Username: "regular_user",
		Groups:   []string{"users"},
	}

	// 1. Non-admin listing users -> 403
	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	addSessionCookie(t, req, sm, nonAdminSession)
	rec := httptest.NewRecorder()
	api.HandleAdminListUsers(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d", rec.Code)
	}

	// 2. Admin listing users -> 200 with both users
	req = httptest.NewRequest("GET", "/api/admin/users", nil)
	addSessionCookie(t, req, sm, adminSession)
	rec = httptest.NewRecorder()
	api.HandleAdminListUsers(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin, got %d", rec.Code)
	}

	// 3. Admin impersonates target_user
	body := bytes.NewBufferString(`{"target_user":"target_user"}`)
	req = httptest.NewRequest("POST", "/api/admin/impersonate", body)
	addSessionCookie(t, req, sm, adminSession)
	rec = httptest.NewRecorder()
	api.HandleAdminImpersonate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on impersonate, got %d", rec.Code)
	}

	// 4. Update session with cookie from impersonation
	cookie := rec.Result().Cookies()[0]
	adminSession.ImpersonatedUser = "target_user"

	req = httptest.NewRequest("GET", "/api/me", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	api.HandleMe(rec, req)

	var meResp UserInfoResponse
	_ = json.NewDecoder(rec.Body).Decode(&meResp)
	if meResp.EffectiveUsername != "target_user" || !meResp.IsImpersonating {
		t.Fatalf("expected impersonation of target_user, got: %+v", meResp)
	}
}

func TestFileExplorerOperations(t *testing.T) {
	api, s, sm, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)

	_, _, _ = s.EnsureUserProvisioned("david")
	userDir, _ := s.UserStoragePath("david")

	// Create some test files
	subDir := filepath.Join(userDir, "documents")
	_ = os.MkdirAll(subDir, 0750)
	testFile := filepath.Join(subDir, "notes.txt")
	_ = os.WriteFile(testFile, []byte("Hello Andvari!"), 0644)

	session := &auth.SessionData{
		Username: "david",
		Groups:   []string{"users"},
	}

	// 1. List root directory
	req := httptest.NewRequest("GET", "/api/files", nil)
	addSessionCookie(t, req, sm, session)
	rec := httptest.NewRecorder()
	api.HandleFiles(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var res ExplorerResult
	_ = json.NewDecoder(rec.Body).Decode(&res)
	if len(res.Items) != 1 || res.Items[0].Name != "documents" || !res.Items[0].IsDir {
		t.Fatalf("unexpected items: %+v", res.Items)
	}

	// 2. Download file
	req = httptest.NewRequest("GET", "/api/files/download?path=documents/notes.txt", nil)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleFileDownload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on download, got %d", rec.Code)
	}
	if rec.Body.String() != "Hello Andvari!" {
		t.Fatalf("expected file contents, got: %s", rec.Body.String())
	}

	// 3. Traversal attack should be blocked
	req = httptest.NewRequest("GET", "/api/files/download?path=../../etc/passwd", nil)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleFileDownload(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on path traversal, got %d", rec.Code)
	}

	// 4. Delete file
	req = httptest.NewRequest("DELETE", "/api/files?path=documents/notes.txt", nil)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleFiles(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on delete, got %d", rec.Code)
	}

	// 5. Verify file is gone
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Fatalf("expected file to be deleted")
	}
}

func TestAPIQuotas(t *testing.T) {
	api, s, sm, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)

	s.SetDefaultQuota(2048)

	_, _, _ = s.EnsureUserProvisioned("admin_user")
	_, _, _ = s.EnsureUserProvisioned("regular_user")

	adminSession := &auth.SessionData{
		Username: "admin_user",
		Groups:   []string{"admin"},
	}

	userSession := &auth.SessionData{
		Username: "regular_user",
		Groups:   []string{"users"},
	}

	// 1. Regular user checks /api/me -> quota_bytes should be 2048
	req := httptest.NewRequest("GET", "/api/me", nil)
	addSessionCookie(t, req, sm, userSession)
	rec := httptest.NewRecorder()
	api.HandleMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var meResp UserInfoResponse
	_ = json.NewDecoder(rec.Body).Decode(&meResp)
	if meResp.QuotaBytes == nil || *meResp.QuotaBytes != 2048 {
		t.Fatalf("expected quota_bytes 2048, got: %v", meResp.QuotaBytes)
	}

	// 2. Non-admin calls /api/admin/quota -> 403
	body := bytes.NewBufferString(`{"target_user":"regular_user","quota_bytes":4096}`)
	req = httptest.NewRequest("POST", "/api/admin/quota", body)
	addSessionCookie(t, req, sm, userSession)
	rec = httptest.NewRecorder()
	api.HandleAdminSetQuota(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d", rec.Code)
	}

	// 3. Admin calls /api/admin/quota -> 200
	body = bytes.NewBufferString(`{"target_user":"regular_user","quota_bytes":4096}`)
	req = httptest.NewRequest("POST", "/api/admin/quota", body)
	addSessionCookie(t, req, sm, adminSession)
	rec = httptest.NewRecorder()
	api.HandleAdminSetQuota(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Regular user checks /api/me -> quota_bytes is now 4096
	req = httptest.NewRequest("GET", "/api/me", nil)
	addSessionCookie(t, req, sm, userSession)
	rec = httptest.NewRecorder()
	api.HandleMe(rec, req)

	var meResp2 UserInfoResponse
	_ = json.NewDecoder(rec.Body).Decode(&meResp2)
	if meResp2.QuotaBytes == nil || *meResp2.QuotaBytes != 4096 {
		t.Fatalf("expected quota_bytes 4096, got: %v", meResp2.QuotaBytes)
	}
}
