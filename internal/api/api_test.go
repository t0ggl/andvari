package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/t0ggl/andvari/internal/auth"
	"github.com/t0ggl/andvari/internal/explorer"
	"github.com/t0ggl/andvari/internal/store"
	"github.com/t0ggl/andvari/internal/warden"
	"github.com/t0ggl/andvari/internal/worker"
)

type ExplorerResult = explorer.ExplorerResult
type UploadResult = explorer.UploadResult

func setupTestAPI(t *testing.T) (*API, *warden.Server, *store.Store, *auth.SessionManager, string) {
	tmpDir, err := os.MkdirTemp("", "andvari-api-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	configDir := filepath.Join(tmpDir, "config")
	storageDir := filepath.Join(tmpDir, "data")
	workerDir := filepath.Join(tmpDir, "workers")

	s, err := store.NewStore(configDir, storageDir)
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
	ws, err := warden.NewServer(s, sm, oidc, wm, sockPath)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	client := warden.NewClient(ws.SocketPath())
	return NewAPI(client, oidc), ws, s, sm, tmpDir
}

func addSessionCookie(t *testing.T, req *http.Request, sm *auth.SessionManager, session *auth.SessionData) {
	val, err := sm.EncodeCookieValue(session)
	if err != nil {
		t.Fatalf("failed to encode cookie: %v", err)
	}
	req.AddCookie(&http.Cookie{
		Name:  auth.SessionCookieName,
		Value: val,
	})
}

func TestAPITokenLifecycleAndLimits(t *testing.T) {
	api, ws, s, sm, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)
	defer ws.Close()

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
	api, ws, s, sm, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)
	defer ws.Close()

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
	api, ws, s, sm, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)
	defer ws.Close()

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

	// 4. Create folder
	req = httptest.NewRequest("POST", "/api/files/folder?path=documents/subfolder", nil)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleCreateFolder(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on folder creation, got %d", rec.Code)
	}
	if fi, err := os.Stat(filepath.Join(userDir, "documents", "subfolder")); err != nil || !fi.IsDir() {
		t.Fatalf("expected subfolder to exist")
	}

	// 5. Rename path
	req = httptest.NewRequest("POST", "/api/files/rename?old_path=documents/notes.txt&new_path=documents/subfolder/renamed.txt", nil)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleRenamePath(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on rename, got %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(userDir, "documents", "subfolder", "renamed.txt")); err != nil {
		t.Fatalf("expected renamed.txt to exist")
	}

	// 6. Archive download
	req = httptest.NewRequest("GET", "/api/files/archive?paths=documents", nil)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleArchiveDownload(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on archive download, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/zip" {
		t.Errorf("expected application/zip, got %s", rec.Header().Get("Content-Type"))
	}

	// 7. Delete file
	req = httptest.NewRequest("DELETE", "/api/files?path=documents/subfolder/renamed.txt", nil)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleFiles(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on delete, got %d", rec.Code)
	}

	// 8. Verify file is gone
	if _, err := os.Stat(filepath.Join(userDir, "documents", "subfolder", "renamed.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected file to be deleted")
	}
}

func TestFileUploadAndQuotas(t *testing.T) {
	api, ws, s, sm, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)
	defer ws.Close()

	s.SetDefaultQuota(500)
	_, _, _ = s.EnsureUserProvisioned("uploader")
	userDir, _ := s.UserStoragePath("uploader")

	session := &auth.SessionData{
		Username: "uploader",
		Groups:   []string{"users"},
	}

	// 1. Upload recursive directory structure with multipart
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	p1, _ := writer.CreateFormFile("files", "project/assets/logo.png")
	_, _ = p1.Write([]byte("LOGO_IMAGE_BYTES"))
	p2, _ := writer.CreateFormFile("files", "project/readme.txt")
	_, _ = p2.Write([]byte("Read this first."))
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/api/files/upload?path=", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	addSessionCookie(t, req, sm, session)
	rec := httptest.NewRecorder()
	api.HandleFileUpload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on multipart upload, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var upRes UploadResult
	if err := json.Unmarshal(rec.Body.Bytes(), &upRes); err != nil {
		t.Fatalf("failed to decode upload response: %v", err)
	}
	if len(upRes.Uploaded) != 2 {
		t.Fatalf("expected 2 uploaded files, got %d", len(upRes.Uploaded))
	}

	// Verify files exist on disk
	f1 := filepath.Join(userDir, "project", "assets", "logo.png")
	if data, err := os.ReadFile(f1); err != nil || string(data) != "LOGO_IMAGE_BYTES" {
		t.Fatalf("file 1 content mismatch or not found: %v", err)
	}

	f2 := filepath.Join(userDir, "project", "readme.txt")
	if data, err := os.ReadFile(f2); err != nil || string(data) != "Read this first." {
		t.Fatalf("file 2 content mismatch or not found: %v", err)
	}

	// Verify quota updated
	used, limit, _ := s.GetEffectiveQuota("uploader")
	expectedUsed := int64(len("LOGO_IMAGE_BYTES") + len("Read this first."))
	if used != expectedUsed {
		t.Fatalf("expected used=%d, got %d (limit=%d)", expectedUsed, used, limit)
	}

	// 2. Upload empty directory
	emptyDirReq := httptest.NewRequest("POST", "/api/files/upload?path=project/empty_folder&is_dir=true", nil)
	addSessionCookie(t, emptyDirReq, sm, session)
	emptyRec := httptest.NewRecorder()
	api.HandleFileUpload(emptyRec, emptyDirReq)
	if emptyRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on empty directory creation, got %d", emptyRec.Code)
	}
	if stat, err := os.Stat(filepath.Join(userDir, "project", "empty_folder")); err != nil || !stat.IsDir() {
		t.Fatalf("expected directory project/empty_folder to exist: %v", err)
	}

	// 3. Quota exceeded test
	bigPayload := make([]byte, 1000)
	bigBody := &bytes.Buffer{}
	bigWriter := multipart.NewWriter(bigBody)
	bp, _ := bigWriter.CreateFormFile("files", "overflow.bin")
	_, _ = bp.Write(bigPayload)
	_ = bigWriter.Close()

	quotaReq := httptest.NewRequest("POST", "/api/files/upload?path=", bigBody)
	quotaReq.Header.Set("Content-Type", bigWriter.FormDataContentType())
	addSessionCookie(t, quotaReq, sm, session)
	quotaRec := httptest.NewRecorder()
	api.HandleFileUpload(quotaRec, quotaReq)

	if quotaRec.Code != http.StatusInsufficientStorage {
		t.Fatalf("expected 507 Insufficient Storage on quota exceed, got %d (body: %s)", quotaRec.Code, quotaRec.Body.String())
	}

	// Verify overflow file was NOT left on disk
	if _, err := os.Stat(filepath.Join(userDir, "overflow.bin")); !os.IsNotExist(err) {
		t.Fatalf("expected overflow.bin to not exist on disk")
	}
}

func TestAPIQuotas(t *testing.T) {
	api, ws, s, sm, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)
	defer ws.Close()

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

func TestAPITokenRename(t *testing.T) {
	api, ws, s, sm, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)
	defer ws.Close()

	user, _, err := s.EnsureUserProvisioned("emily")
	if err != nil {
		t.Fatalf("failed to provision emily: %v", err)
	}
	tokenID := user.Tokens[0].ID

	session := &auth.SessionData{
		Username: "emily",
		Groups:   []string{"users"},
	}

	// 1. Rename with PATCH
	patchBody := bytes.NewBufferString(fmt.Sprintf(`{"id":%q,"label":"Home Desktop"}`, tokenID))
	req := httptest.NewRequest("PATCH", "/api/tokens", patchBody)
	addSessionCookie(t, req, sm, session)
	rec := httptest.NewRecorder()
	api.HandleRenameToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on rename PATCH, got %d: %s", rec.Code, rec.Body.String())
	}

	var renameResp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&renameResp); err != nil {
		t.Fatalf("failed to decode rename response: %v", err)
	}
	tokInfo := renameResp["token_info"].(map[string]interface{})
	if tokInfo["label"] != "Home Desktop" || tokInfo["id"] != tokenID {
		t.Fatalf("unexpected token_info: %+v", tokInfo)
	}

	// 2. Rename with PUT and query param ID
	putBody := bytes.NewBufferString(`{"label":"Work Laptop"}`)
	req = httptest.NewRequest("PUT", "/api/tokens?id="+tokenID, putBody)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleRenameToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on rename PUT, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Verify via HandleMe
	req = httptest.NewRequest("GET", "/api/me", nil)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleMe(rec, req)

	var meResp UserInfoResponse
	_ = json.NewDecoder(rec.Body).Decode(&meResp)
	if len(meResp.Tokens) != 1 || meResp.Tokens[0].Label != "Work Laptop" {
		t.Fatalf("expected updated token label in /api/me to be 'Work Laptop', got %q", meResp.Tokens[0].Label)
	}

	// 4. Unauthenticated rename -> 401
	body := bytes.NewBufferString(fmt.Sprintf(`{"id":%q,"label":"Sneaky"}`, tokenID))
	req = httptest.NewRequest("PATCH", "/api/tokens", body)
	rec = httptest.NewRecorder()
	api.HandleRenameToken(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated rename, got %d", rec.Code)
	}

	// 5. Missing token ID -> 400
	body = bytes.NewBufferString(`{"label":"No ID"}`)
	req = httptest.NewRequest("PATCH", "/api/tokens", body)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleRenameToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing token ID, got %d", rec.Code)
	}

	// 6. Empty label -> 400
	body = bytes.NewBufferString(fmt.Sprintf(`{"id":%q,"label":"   "}`, tokenID))
	req = httptest.NewRequest("PATCH", "/api/tokens", body)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleRenameToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty label, got %d", rec.Code)
	}

	// 7. Non-existent token ID -> 404
	body = bytes.NewBufferString(`{"id":"non-existent-uuid","label":"Valid Label"}`)
	req = httptest.NewRequest("PATCH", "/api/tokens", body)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleRenameToken(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent token, got %d", rec.Code)
	}

	// 8. Method not allowed (POST or GET) -> 405
	req = httptest.NewRequest("GET", "/api/tokens", nil)
	addSessionCookie(t, req, sm, session)
	rec = httptest.NewRecorder()
	api.HandleRenameToken(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET on HandleRenameToken, got %d", rec.Code)
	}
}

func TestAPIHealth(t *testing.T) {
	api, ws, s, _, tmpDir := setupTestAPI(t)
	defer os.RemoveAll(tmpDir)
	defer ws.Close()

	configDir := s.ConfigDir()
	storageDir := s.StorageDir()

	// 1. Success case: GET unauthenticated /healthz
	start := time.Now()
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	api.HandleHealth(rec, req)
	duration := time.Since(start)

	if duration > 100*time.Millisecond {
		t.Errorf("health check took too long: %v (expected non-blocking <100ms)", duration)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var health HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&health); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}
	if health.Status != "healthy" {
		t.Errorf("expected status 'healthy', got %q", health.Status)
	}
	if health.Version == "" {
		t.Errorf("expected non-empty version in health response")
	}
	if health.UptimeSeconds < 0 {
		t.Errorf("expected non-negative uptime, got %d", health.UptimeSeconds)
	}
	if health.Error != "" {
		t.Errorf("expected empty error on healthy, got %q", health.Error)
	}

	// 2. Simulated uptime
	api.startTime = time.Now().Add(-123 * time.Second)
	req = httptest.NewRequest("GET", "/api/health", nil)
	rec = httptest.NewRecorder()
	api.HandleHealth(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	var uptimeHealth HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&uptimeHealth); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}
	if uptimeHealth.UptimeSeconds < 123 {
		t.Errorf("expected uptime >= 123, got %d", uptimeHealth.UptimeSeconds)
	}

	// 3. HEAD /healthz should also succeed
	headReq := httptest.NewRequest("HEAD", "/healthz", nil)
	headRec := httptest.NewRecorder()
	api.HandleHealth(headRec, headReq)
	if headRec.Code != http.StatusOK {
		t.Errorf("expected 200 for HEAD /healthz, got %d", headRec.Code)
	}

	// 4. Method not allowed (POST)
	postReq := httptest.NewRequest("POST", "/healthz", nil)
	postRec := httptest.NewRecorder()
	api.HandleHealth(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 Method Not Allowed for POST, got %d", postRec.Code)
	}

	// 5. Unwritable config directory -> 503 Service Unavailable
	if err := os.Chmod(configDir, 0555); err != nil {
		t.Fatalf("failed to chmod configDir: %v", err)
	}
	rec = httptest.NewRecorder()
	api.HandleHealth(rec, req)
	_ = os.Chmod(configDir, 0755)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable for unwritable configDir, got %d", rec.Code)
	}
	var unwritableConfig HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&unwritableConfig); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if unwritableConfig.Status != "unhealthy" || unwritableConfig.Error == "" {
		t.Errorf("expected unhealthy response with error message, got: %+v", unwritableConfig)
	}

	// 6. Unwritable storage directory -> 503 Service Unavailable
	if err := os.Chmod(storageDir, 0555); err != nil {
		t.Fatalf("failed to chmod storageDir: %v", err)
	}
	rec = httptest.NewRecorder()
	api.HandleHealth(rec, req)
	_ = os.Chmod(storageDir, 0755)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable for unwritable storageDir, got %d", rec.Code)
	}
	var unwritableStorage HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&unwritableStorage); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if unwritableStorage.Status != "unhealthy" || unwritableStorage.Error == "" {
		t.Errorf("expected unhealthy response with error message, got: %+v", unwritableStorage)
	}

	// 7. Disconnected warden -> 503 Service Unavailable
	downAPI := NewAPI(warden.NewClient("/tmp/nonexistent-warden.sock"), nil)
	rec = httptest.NewRecorder()
	downAPI.HandleHealth(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for disconnected warden, got %d", rec.Code)
	}
}
