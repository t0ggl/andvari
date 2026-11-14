package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/t0ggl/andvari/internal/api"
	"github.com/t0ggl/andvari/internal/auth"
	"github.com/t0ggl/andvari/internal/config"
	"github.com/t0ggl/andvari/internal/store"
	"github.com/t0ggl/andvari/internal/warden"
	"github.com/t0ggl/andvari/internal/webdav"
	"github.com/t0ggl/andvari/internal/worker"
	"github.com/t0ggl/andvari/web"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "__web" {
		runWebCLI(os.Args[2:])
		return
	}

	cfg := config.Load()

	log.Printf("Starting Andvari Supervisor (Warden)...")
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

	// Start Warden Unix domain socket server
	sockPath := filepath.Join(cfg.WorkerDir, "warden.sock")
	wardenServer, err := warden.NewServer(userStore, sessionManager, oidcClient, workerManager, sockPath)
	if err != nil {
		log.Fatalf("Failed to initialize Warden server: %v", err)
	}
	defer wardenServer.Close()

	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}

	// Spawn sandboxed Web Server subprocess
	webCmd := exec.Command(exe, "__web", "-warden-sock", wardenServer.SocketPath())
	webCmd.Stdout = os.Stdout
	webCmd.Stderr = os.Stderr
	webCmd.Env = os.Environ()

	if err := webCmd.Start(); err != nil {
		log.Fatalf("Failed to start sandboxed web server: %v", err)
	}

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	cmdErrChan := make(chan error, 1)
	go func() {
		cmdErrChan <- webCmd.Wait()
	}()

	select {
	case sig := <-stopChan:
		log.Printf("Received signal %v, gracefully shutting down Warden and sandboxed web server...", sig)
		if webCmd.Process != nil {
			_ = webCmd.Process.Signal(syscall.SIGTERM)
		}
	case err := <-cmdErrChan:
		log.Printf("Sandboxed web server exited: %v", err)
	}

	select {
	case <-cmdErrChan:
	case <-time.After(5 * time.Second):
		if webCmd.Process != nil {
			_ = webCmd.Process.Kill()
		}
	}

	log.Printf("Andvari exited cleanly.")
}

func runWebCLI(args []string) {
	fs := flag.NewFlagSet("__web", flag.ExitOnError)
	wardenSock := fs.String("warden-sock", "", "Path to warden Unix domain socket")
	_ = fs.Parse(args)

	if *wardenSock == "" {
		fmt.Fprintf(os.Stderr, "Error: -warden-sock is required for __web\n")
		os.Exit(1)
	}

	cfg := config.Load()

	// Apply Web Server Landlock sandbox: deny /data and /config, allow only worker socket dir and TLS certs
	if err := warden.ApplyWebLandlock(cfg.WorkerDir); err != nil {
		log.Printf("[landlock:web] warning: failed to apply Landlock sandbox: %v", err)
	}

	wardenClient := warden.NewClient(*wardenSock)

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

	webdavServer := webdav.NewServer(wardenClient)
	apiServer := api.NewAPI(wardenClient, oidcClient)

	mux := http.NewServeMux()

	// Health check endpoints (unauthenticated)
	mux.HandleFunc("/healthz", apiServer.HandleHealth)
	mux.HandleFunc("/api/health", apiServer.HandleHealth)

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
	mux.HandleFunc("/api/files/upload", apiServer.HandleFileUpload)
	mux.HandleFunc("/api/files/folder", apiServer.HandleCreateFolder)
	mux.HandleFunc("/api/files/rename", apiServer.HandleRenamePath)
	mux.HandleFunc("/api/files/archive", apiServer.HandleArchiveDownload)

	// Auth routes
	mux.HandleFunc("/auth/login", apiServer.HandleLogin)
	mux.HandleFunc("/auth/callback", apiServer.HandleCallback)
	mux.HandleFunc("/auth/logout", apiServer.HandleLogout)

	// Embedded Web UI SPA
	mux.Handle("/", web.Handler())

	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  1 * time.Hour,
		WriteTimeout: 1 * time.Hour,
	}

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[web] sandboxed server listening on %s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[web] HTTP server failure: %v", err)
		}
	}()

	<-stopChan
	log.Printf("[web] shutting down sandboxed server gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[web] forced shutdown error: %v", err)
	}
}
