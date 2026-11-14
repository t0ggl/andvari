package webdav

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/t0ggl/andvari/internal/store"
	"github.com/t0ggl/andvari/internal/worker"
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

func TestWebDAVPropfindQuota(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-webdav-propfind-quota-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set 500 byte quota limit
	s, err := store.NewStore(tmpDir+"/config", tmpDir+"/data", 500)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	workerDir := tmpDir + "/workers"
	wm := worker.NewManager(workerDir, 1*time.Minute)
	defer wm.Close()

	// Test both standalone WebDAV server (wm == nil) and worker-backed WebDAV server
	servers := []struct {
		name   string
		server *Server
	}{
		{name: "standalone", server: NewServer(s)},
		{name: "worker", server: NewServer(s, wm)},
	}

	for _, tc := range servers {
		t.Run(tc.name, func(t *testing.T) {
			propUser := tc.name + "_propuser"
			_, token, err := s.EnsureUserProvisioned(propUser)
			if err != nil {
				t.Fatalf("EnsureUserProvisioned failed: %v", err)
			}
			authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(propUser+":"+token))

			// 1. Upload 200 bytes
			data200 := make([]byte, 200)
			putReq := httptest.NewRequest("PUT", "/webdav/data.bin", strings.NewReader(string(data200)))
			putReq.Header.Set("Authorization", authHeader)
			putReq.ContentLength = 200
			putRec := httptest.NewRecorder()
			tc.server.ServeHTTP(putRec, putReq)
			if putRec.Code != http.StatusCreated && putRec.Code != http.StatusNoContent {
				t.Fatalf("PUT data failed: code %d, body %s", putRec.Code, putRec.Body.String())
			}

			// 2. PROPFIND with empty body (RFC 4918 allprop)
			propReq := httptest.NewRequest("PROPFIND", "/webdav/", nil)
			propReq.Header.Set("Authorization", authHeader)
			propReq.Header.Set("Depth", "0")
			propRec := httptest.NewRecorder()
			tc.server.ServeHTTP(propRec, propReq)

			if propRec.Code != http.StatusMultiStatus {
				t.Fatalf("expected 207 MultiStatus, got %d (body: %s)", propRec.Code, propRec.Body.String())
			}
			body := propRec.Body.String()
			if !strings.Contains(body, "<D:quota-used-bytes>200</D:quota-used-bytes>") {
				t.Errorf("expected <D:quota-used-bytes>200</D:quota-used-bytes>, got: %s", body)
			}
			if !strings.Contains(body, "<D:quota-available-bytes>300</D:quota-available-bytes>") {
				t.Errorf("expected <D:quota-available-bytes>300</D:quota-available-bytes>, got: %s", body)
			}
			if !strings.Contains(body, "<D:status>HTTP/1.1 200 OK</D:status>") {
				t.Errorf("expected 200 OK status in propstat, got: %s", body)
			}

			// 3. PROPFIND requesting only quota properties
			specificBody := `<?xml version="1.0" encoding="utf-8" ?>
<D:propfind xmlns:D="DAV:">
  <D:prop>
    <D:quota-available-bytes/>
    <D:quota-used-bytes/>
  </D:prop>
</D:propfind>`
			specReq := httptest.NewRequest("PROPFIND", "/webdav/", strings.NewReader(specificBody))
			specReq.Header.Set("Authorization", authHeader)
			specReq.Header.Set("Depth", "0")
			specRec := httptest.NewRecorder()
			tc.server.ServeHTTP(specRec, specReq)

			if specRec.Code != http.StatusMultiStatus {
				t.Fatalf("expected 207 MultiStatus, got %d (body: %s)", specRec.Code, specRec.Body.String())
			}
			specBody := specRec.Body.String()
			if !strings.Contains(specBody, "<D:quota-used-bytes>200</D:quota-used-bytes>") {
				t.Errorf("expected <D:quota-used-bytes>200</D:quota-used-bytes>, got: %s", specBody)
			}
			if !strings.Contains(specBody, "<D:quota-available-bytes>300</D:quota-available-bytes>") {
				t.Errorf("expected <D:quota-available-bytes>300</D:quota-available-bytes>, got: %s", specBody)
			}
			if strings.Contains(specBody, "404 Not Found") {
				t.Errorf("did not expect 404 Not Found in propstat, got: %s", specBody)
			}

			// 4. Unlimited quota user
			unlimUser := tc.name + "_unlimuser"
			_, unlimToken, err := s.EnsureUserProvisioned(unlimUser)
			if err != nil {
				t.Fatalf("EnsureUserProvisioned failed: %v", err)
			}
			zero := int64(0)
			if err := s.SetUserQuota(unlimUser, &zero); err != nil {
				t.Fatalf("SetUserQuota failed: %v", err)
			}
			unlimAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(unlimUser+":"+unlimToken))

			// Upload 150 bytes for unlimited user
			data150 := make([]byte, 150)
			uPutReq := httptest.NewRequest("PUT", "/webdav/unlim.bin", strings.NewReader(string(data150)))
			uPutReq.Header.Set("Authorization", unlimAuth)
			uPutReq.ContentLength = 150
			uPutRec := httptest.NewRecorder()
			tc.server.ServeHTTP(uPutRec, uPutReq)
			if uPutRec.Code != http.StatusCreated && uPutRec.Code != http.StatusNoContent {
				t.Fatalf("unlimited PUT failed: code %d, body %s", uPutRec.Code, uPutRec.Body.String())
			}

			// PROPFIND on unlimited user
			uPropReq := httptest.NewRequest("PROPFIND", "/webdav/", nil)
			uPropReq.Header.Set("Authorization", unlimAuth)
			uPropReq.Header.Set("Depth", "0")
			uPropRec := httptest.NewRecorder()
			tc.server.ServeHTTP(uPropRec, uPropReq)

			if uPropRec.Code != http.StatusMultiStatus {
				t.Fatalf("expected 207 MultiStatus for unlimited user, got %d", uPropRec.Code)
			}
			uBody := uPropRec.Body.String()
			expectedAvail := strconv.FormatInt(worker.UnlimitedQuotaCeiling-150, 10)
			if !strings.Contains(uBody, "<D:quota-available-bytes>"+expectedAvail+"</D:quota-available-bytes>") {
				t.Errorf("expected <D:quota-available-bytes>%s</D:quota-available-bytes>, got: %s", expectedAvail, uBody)
			}
			if !strings.Contains(uBody, "<D:quota-used-bytes>150</D:quota-used-bytes>") {
				t.Errorf("expected <D:quota-used-bytes>150</D:quota-used-bytes>, got: %s", uBody)
			}
		})
	}
}

