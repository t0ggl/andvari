package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/t0ggl/andvari/internal/api"
	"github.com/t0ggl/andvari/internal/auth"
	"github.com/t0ggl/andvari/internal/store"
	"github.com/t0ggl/andvari/internal/warden"
	"github.com/t0ggl/andvari/internal/worker"
)

func TestHealthCheckEndpoints(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-main-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configDir := filepath.Join(tmpDir, "config")
	storageDir := filepath.Join(tmpDir, "data")
	workerDir := filepath.Join(tmpDir, "workers")

	userStore, err := store.NewStore(configDir, storageDir, 0)
	if err != nil {
		t.Fatalf("failed to initialize user store: %v", err)
	}

	sessionManager := auth.NewSessionManager([]byte("test-session-secret-32-bytes-long!"))
	oidcClient, err := auth.NewOIDCClient(
		context.Background(),
		"", "", "", "", nil, "admin", "preferred_username", true,
	)
	if err != nil {
		t.Fatalf("failed to initialize OIDC client: %v", err)
	}

	workerManager := worker.NewManager(workerDir, 5*time.Minute)
	defer workerManager.Close()

	sockPath := filepath.Join(workerDir, "warden.sock")
	wardenServer, err := warden.NewServer(userStore, sessionManager, oidcClient, workerManager, sockPath)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	defer wardenServer.Close()

	wardenClient := warden.NewClient(wardenServer.SocketPath())
	apiServer := api.NewAPI(wardenClient, oidcClient)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", apiServer.HandleHealth)
	mux.HandleFunc("/api/health", apiServer.HandleHealth)

	endpoints := []string{"/healthz", "/api/health"}
	for _, ep := range endpoints {
		t.Run("Endpoint_"+ep, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, ep, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 OK for %s, got %d", ep, rec.Code)
			}

			var resp api.HealthResponse
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response for %s: %v", ep, err)
			}

			if resp.Status != "healthy" {
				t.Errorf("expected status 'healthy', got %q", resp.Status)
			}
			if resp.Version == "" {
				t.Errorf("expected non-empty version, got %q", resp.Version)
			}
			if resp.UptimeSeconds < 0 {
				t.Errorf("expected non-negative uptime, got %d", resp.UptimeSeconds)
			}
		})
	}
}
