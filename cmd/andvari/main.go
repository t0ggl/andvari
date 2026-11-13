package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/t0ggl/andvari/internal/api"
	"github.com/t0ggl/andvari/internal/auth"
	"github.com/t0ggl/andvari/internal/config"
	"github.com/t0ggl/andvari/internal/store"
	"github.com/t0ggl/andvari/internal/webdav"
	"github.com/t0ggl/andvari/internal/worker"
	"github.com/t0ggl/andvari/web"
)

func main() {
	cfg := config.Load()

	log.Printf("Starting Andvari WebDAV server...")
	log.Printf("Config path: %s", cfg.ConfigPath)
	log.Printf("Storage path: %s", cfg.StoragePath)
	log.Printf("Listen address: %s", cfg.ListenAddr)
	if cfg.DefaultQuotaBytes > 0 {
		log.Printf("Default user quota: %d bytes", cfg.DefaultQuotaBytes)
	} else {
		log.Printf("Default user quota: unlimited")
	}

	userStore, err := store.NewStore(cfg.ConfigPath, cfg.StoragePath, cfg.DefaultQuotaBytes)
	if err != nil {
		log.Fatalf("Failed to initialize user store: %v", err)
	}

	sessionManager := auth.NewSessionManager(cfg.SessionSecret)

	ctx := context.Background()
	oidcClient, err := auth.NewOIDCClient(
		ctx,
		cfg.OIDCIssuerURL,
		cfg.OIDCClientID,
		cfg.OIDCClientSecret,
		cfg.OIDCRedirectURL,
		cfg.OIDCScopes,
		cfg.OIDCAdminGroup,
		cfg.OIDCUsernameClaim,
		cfg.DevMode,
	)
	if err != nil {
		log.Fatalf("Failed to initialize OIDC client: %v", err)
	}

	workerManager := worker.NewManager(cfg.WorkerDir, 5*time.Minute)
	defer workerManager.Close()

	webdavServer := webdav.NewServer(userStore, workerManager)
	apiServer := api.NewAPI(userStore, sessionManager, oidcClient, workerManager)

	mux := http.NewServeMux()

	// WebDAV endpoint
	mux.Handle("/webdav/", webdavServer)

	// API routes
	mux.HandleFunc("/api/me", apiServer.HandleMe)
	mux.HandleFunc("/api/tokens", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			apiServer.HandleGenerateToken(w, r)
		case http.MethodDelete:
			apiServer.HandleRevokeToken(w, r)
		case http.MethodPatch, http.MethodPut:
			apiServer.HandleRenameToken(w, r)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/onboarding/complete", apiServer.HandleCompleteOnboarding)
	mux.HandleFunc("/api/admin/users", apiServer.HandleAdminListUsers)
	mux.HandleFunc("/api/admin/impersonate", apiServer.HandleAdminImpersonate)
	mux.HandleFunc("/api/admin/quota", apiServer.HandleAdminSetQuota)
	mux.HandleFunc("/api/files", apiServer.HandleFiles)
	mux.HandleFunc("/api/files/download", apiServer.HandleFileDownload)

	// Auth routes
	mux.HandleFunc("/auth/login", apiServer.HandleLogin)
	mux.HandleFunc("/auth/callback", apiServer.HandleCallback)
	mux.HandleFunc("/auth/logout", apiServer.HandleLogout)

	// Embedded Web UI SPA
	mux.Handle("/", web.Handler())

	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  1 * time.Hour, // Long timeouts for large WebDAV file uploads
		WriteTimeout: 1 * time.Hour,
	}

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Server listening on %s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server failure: %v", err)
		}
	}()

	<-stopChan
	log.Printf("Shutting down server gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced shutdown error: %v", err)
	}

	log.Printf("Andvari exited cleanly.")
}
