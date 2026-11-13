package webdav

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/t0ggl/andvari/internal/store"
)

func TestWebDAVHandler(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-webdav-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := store.NewStore(tmpDir+"/config", tmpDir+"/data")
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	_, token, err := s.EnsureUserProvisioned("bob")
	if err != nil {
		t.Fatalf("EnsureUserProvisioned failed: %v", err)
	}

	server := NewServer(s)

	// Test 1: No auth -> 401
	req := httptest.NewRequest("PROPFIND", "/webdav/", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("expected WWW-Authenticate header")
	}

	// Test 2: Invalid auth -> 401
	req = httptest.NewRequest("PROPFIND", "/webdav/", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("bob:bad-token")))
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on bad password, got %d", rec.Code)
	}

	// Test 3: Valid auth -> 207 Multi-Status for PROPFIND
	req = httptest.NewRequest("PROPFIND", "/webdav/", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("bob:"+token)))
	req.Header.Set("Depth", "0")
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusMultiStatus && rec.Code != http.StatusOK {
		t.Fatalf("expected 207 MultiStatus or 200 OK, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestWebDAVQuotaEnforcement(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-webdav-quota-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set 500 byte quota
	s, err := store.NewStore(tmpDir+"/config", tmpDir+"/data", 500)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	_, token, err := s.EnsureUserProvisioned("quotauser")
	if err != nil {
		t.Fatalf("EnsureUserProvisioned failed: %v", err)
	}

	server := NewServer(s)
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte("quotauser:"+token))

	// 1. Upload 300 byte file -> within 500 byte quota -> 201 Created
	data300 := make([]byte, 300)
	req := httptest.NewRequest("PUT", "/webdav/file1.bin", strings.NewReader(string(data300)))
	req.Header.Set("Authorization", authHeader)
	req.ContentLength = 300
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated && rec.Code != http.StatusNoContent {
		t.Fatalf("expected 201/204, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Upload another 300 byte file -> 300 + 300 = 600 > 500 -> 507 Insufficient Storage
	req = httptest.NewRequest("PUT", "/webdav/file2.bin", strings.NewReader(string(data300)))
	req.Header.Set("Authorization", authHeader)
	req.ContentLength = 300
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusInsufficientStorage {
		t.Fatalf("expected 507 Insufficient Storage, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Overwrite file1.bin with 400 bytes -> (300 existing replaced by 400 = net +100 -> total 400 <= 500) -> 204 or 201
	data400 := make([]byte, 400)
	req = httptest.NewRequest("PUT", "/webdav/file1.bin", strings.NewReader(string(data400)))
	req.Header.Set("Authorization", authHeader)
	req.ContentLength = 400
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated && rec.Code != http.StatusNoContent {
		t.Fatalf("expected 201/204 on overwrite within quota, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Usage in cache should accurately reflect 400 bytes via delta tracking
	used, err := s.GetUserStorageUsage("quotauser")
	if err != nil || used != 400 {
		t.Fatalf("expected used 400 bytes, got %d (err: %v)", used, err)
	}

	// 5. Delete file1.bin via WebDAV DELETE -> usage drops to 0
	req = httptest.NewRequest("DELETE", "/webdav/file1.bin", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
		t.Fatalf("expected 204/200 on DELETE, got %d: %s", rec.Code, rec.Body.String())
	}

	used, err = s.GetUserStorageUsage("quotauser")
	if err != nil || used != 0 {
		t.Fatalf("expected used 0 bytes after delete, got %d (err: %v)", used, err)
	}

	// 6. User with unlimited quota (quota_bytes = 0) can upload freely
	_, unlimToken, err := s.EnsureUserProvisioned("unlimiteduser")
	if err != nil {
		t.Fatalf("EnsureUserProvisioned failed: %v", err)
	}
	zero := int64(0)
	if err := s.SetUserQuota("unlimiteduser", &zero); err != nil {
		t.Fatalf("SetUserQuota failed: %v", err)
	}
	unlimAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("unlimiteduser:"+unlimToken))

	bigData := make([]byte, 10000)
	req = httptest.NewRequest("PUT", "/webdav/bigfile.bin", strings.NewReader(string(bigData)))
	req.Header.Set("Authorization", unlimAuth)
	req.ContentLength = 10000
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated && rec.Code != http.StatusNoContent {
		t.Fatalf("expected 201/204 for unlimited user, got %d: %s", rec.Code, rec.Body.String())
	}
}
