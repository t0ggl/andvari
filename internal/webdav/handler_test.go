package webdav

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
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
