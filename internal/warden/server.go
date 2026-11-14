package warden

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/t0ggl/andvari/internal/auth"
	"github.com/t0ggl/andvari/internal/store"
	"github.com/t0ggl/andvari/internal/worker"
)

type Server struct {
	store          *store.Store
	sessionManager *auth.SessionManager
	oidcClient     *auth.OIDCClient
	workerManager  *worker.Manager
	socketPath     string
	listener       net.Listener
	server         *http.Server
	mu             sync.Mutex
	closed         bool
}

func NewServer(
	s *store.Store,
	sm *auth.SessionManager,
	oidc *auth.OIDCClient,
	wm *worker.Manager,
	socketPath string,
) (*Server, error) {
	if socketPath == "" {
		return nil, errors.New("socket path cannot be empty")
	}

	if len(socketPath) > 100 {
		base := filepath.Base(socketPath)
		socketPath = filepath.Join("/tmp", fmt.Sprintf("andv-%d-%s", os.Getpid(), base))
	}

	_ = os.MkdirAll(filepath.Dir(socketPath), 0700)
	_ = os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on warden socket %s: %w", socketPath, err)
	}
	_ = os.Chmod(socketPath, 0600)

	srv := &Server{
		store:          s,
		sessionManager: sm,
		oidcClient:     oidc,
		workerManager:  wm,
		socketPath:     socketPath,
		listener:       listener,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/warden/auth/webdav", srv.handleWebDAVAuth)
	mux.HandleFunc("/warden/auth/session", srv.handleSessionAuth)
	mux.HandleFunc("/warden/tokens/create", srv.handleCreateToken)
	mux.HandleFunc("/warden/tokens/revoke", srv.handleRevokeToken)
	mux.HandleFunc("/warden/tokens/rename", srv.handleRenameToken)
	mux.HandleFunc("/warden/me", srv.handleMe)
	mux.HandleFunc("/warden/onboarding/complete", srv.handleCompleteOnboarding)
	mux.HandleFunc("/warden/admin/users", srv.handleAdminListUsers)
	mux.HandleFunc("/warden/admin/impersonate", srv.handleAdminImpersonate)
	mux.HandleFunc("/warden/admin/quota", srv.handleAdminSetQuota)
	mux.HandleFunc("/warden/session/login", srv.handleSessionLogin)
	mux.HandleFunc("/warden/usage/delta", srv.handleUsageDelta)
	mux.HandleFunc("/warden/health", srv.handleHealth)

	srv.server = &http.Server{Handler: mux}

	go func() {
		_ = srv.server.Serve(listener)
	}()

	return srv, nil
}

func (s *Server) SocketPath() string {
	return s.socketPath
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	var err error
	if s.server != nil {
		err = s.server.Close()
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	_ = os.Remove(s.socketPath)
	return err
}

func (s *Server) authenticateSession(cookie string) (*auth.SessionData, string, bool, error) {
	if s.sessionManager == nil {
		return nil, "", false, errors.New("session manager not configured")
	}
	session, err := s.sessionManager.DecodeCookieValue(cookie)
	if err != nil || session == nil || session.Username == "" {
		return nil, "", false, errors.New("unauthenticated")
	}

	isAdmin := false
	if s.oidcClient != nil {
		isAdmin = s.oidcClient.IsAdmin(session.Groups)
	}

	effectiveUser := session.Username
	isImpersonating := false
	if isAdmin && session.ImpersonatedUser != "" {
		effectiveUser = session.ImpersonatedUser
		isImpersonating = true
	}

	return session, effectiveUser, isImpersonating, nil
}

func (s *Server) handleWebDAVAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req WebDAVAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Token == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(WebDAVAuthResponse{Authenticated: false, Error: "missing username or token"})
		return
	}

	valid, _, err := s.store.AuthenticateToken(req.Username, req.Token)
	if err != nil || !valid {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(WebDAVAuthResponse{Authenticated: false, Error: "invalid credentials"})
		return
	}

	userDir, err := s.store.UserStoragePath(req.Username)
	if err != nil {
		http.Error(w, `{"error":"failed to determine user storage path"}`, http.StatusInternalServerError)
		return
	}

	usedBytes, quotaLimit, err := s.store.GetEffectiveQuota(req.Username)
	if err != nil {
		http.Error(w, `{"error":"failed to determine user quota"}`, http.StatusInternalServerError)
		return
	}

	var sockPath string
	if s.workerManager != nil {
		workerInst, err := s.workerManager.GetWorker(req.Username, userDir)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to get worker: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		sockPath = workerInst.SocketPath
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(WebDAVAuthResponse{
		Authenticated: true,
		Username:      req.Username,
		UserDir:       userDir,
		SocketPath:    sockPath,
		QuotaLimit:    quotaLimit,
		QuotaUsed:     usedBytes,
	})
}

func (s *Server) handleSessionAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req SessionAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionCookie == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(SessionAuthResponse{Authenticated: false, Error: "unauthenticated"})
		return
	}

	session, effectiveUser, isImpersonating, err := s.authenticateSession(req.SessionCookie)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(SessionAuthResponse{Authenticated: false, Error: "unauthenticated"})
		return
	}

	userDir, err := s.store.UserStoragePath(effectiveUser)
	if err != nil {
		http.Error(w, `{"error":"failed to get user storage path"}`, http.StatusInternalServerError)
		return
	}

	usedBytes, quotaLimit, _ := s.store.GetEffectiveQuota(effectiveUser)

	var sockPath string
	if s.workerManager != nil {
		workerInst, err := s.workerManager.GetWorker(effectiveUser, userDir)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to get worker: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		sockPath = workerInst.SocketPath
	}

	isAdmin := false
	if s.oidcClient != nil {
		isAdmin = s.oidcClient.IsAdmin(session.Groups)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(SessionAuthResponse{
		Authenticated:   true,
		Username:        session.Username,
		EffectiveUser:   effectiveUser,
		IsAdmin:         isAdmin,
		IsImpersonating: isImpersonating,
		SocketPath:      sockPath,
		QuotaLimit:      quotaLimit,
		QuotaUsed:       usedBytes,
	})
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req CreateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	_, effectiveUser, _, err := s.authenticateSession(req.SessionCookie)
	if err != nil {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	rawToken, tokenInfo, err := s.store.AddToken(effectiveUser, req.Label)
	if err != nil {
		if errors.Is(err, store.ErrMaxTokensReached) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(CreateTokenResponse{
				Error: fmt.Sprintf("Maximum of %d active tokens allowed. Please revoke one before generating a new token.", store.MaxTokensPerUser),
			})
			return
		}
		http.Error(w, `{"error":"failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(CreateTokenResponse{
		Token: rawToken,
		TokenInfo: &TokenResponse{
			ID:        tokenInfo.ID,
			Label:     tokenInfo.Label,
			CreatedAt: tokenInfo.CreatedAt.Format("2006-01-02 15:04:05 MST"),
		},
	})
}

func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req RevokeTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TokenID == "" {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	_, effectiveUser, _, err := s.authenticateSession(req.SessionCookie)
	if err != nil {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	if err := s.store.RevokeToken(effectiveUser, req.TokenID); err != nil {
		if errors.Is(err, store.ErrTokenNotFound) {
			http.Error(w, `{"error":"token not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to revoke token"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(GenericResponse{Success: true})
}

func (s *Server) handleRenameToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPatch {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req RenameTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TokenID == "" {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	_, effectiveUser, _, err := s.authenticateSession(req.SessionCookie)
	if err != nil {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	updatedToken, err := s.store.RenameToken(effectiveUser, req.TokenID, req.Label)
	if err != nil {
		if errors.Is(err, store.ErrTokenNotFound) {
			http.Error(w, `{"error":"token not found"}`, http.StatusNotFound)
			return
		}
		if errors.Is(err, store.ErrEmptyTokenLabel) {
			http.Error(w, `{"error":"token label cannot be empty"}`, http.StatusBadRequest)
			return
		}
		http.Error(w, `{"error":"failed to rename token"}`, http.StatusInternalServerError)
		return
	}

	var lastUsed *string
	if updatedToken.LastUsedAt != nil {
		formatted := updatedToken.LastUsedAt.Format("2006-01-02 15:04:05 MST")
		lastUsed = &formatted
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"token_info": TokenResponse{
			ID:         updatedToken.ID,
			Label:      updatedToken.Label,
			CreatedAt:  updatedToken.CreatedAt.Format("2006-01-02 15:04:05 MST"),
			LastUsedAt: lastUsed,
		},
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req SessionAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionCookie == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	session, effectiveUser, isImpersonating, err := s.authenticateSession(req.SessionCookie)
	if err != nil {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	userMeta, err := s.store.GetUser(effectiveUser)
	if err != nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}

	tokens := make([]TokenResponse, 0, len(userMeta.Tokens))
	for _, tok := range userMeta.Tokens {
		var lastUsed *string
		if tok.LastUsedAt != nil {
			formatted := tok.LastUsedAt.Format("2006-01-02 15:04:05 MST")
			lastUsed = &formatted
		}
		tokens = append(tokens, TokenResponse{
			ID:         tok.ID,
			Label:      tok.Label,
			CreatedAt:  tok.CreatedAt.Format("2006-01-02 15:04:05 MST"),
			LastUsedAt: lastUsed,
		})
	}

	usedBytes, quotaBytes, _ := s.store.GetEffectiveQuota(effectiveUser)
	var quotaPtr *int64
	if quotaBytes > 0 {
		quotaPtr = &quotaBytes
	}

	isAdmin := false
	if s.oidcClient != nil {
		isAdmin = s.oidcClient.IsAdmin(session.Groups)
	}

	resp := UserInfoResponse{
		Username:            session.Username,
		EffectiveUsername:   effectiveUser,
		IsAdmin:             isAdmin,
		IsImpersonating:     isImpersonating,
		FirstLoginCompleted: userMeta.Preferences.FirstLoginCompleted,
		Tokens:              tokens,
		FlashToken:          session.FlashToken,
		QuotaBytes:          quotaPtr,
		UsedBytes:           usedBytes,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleCompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req OnboardingCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionCookie == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	_, effectiveUser, _, err := s.authenticateSession(req.SessionCookie)
	if err != nil {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	if err := s.store.SetFirstLoginCompleted(effectiveUser); err != nil {
		http.Error(w, `{"error":"failed to update preference"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(GenericResponse{Success: true})
}

func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req SessionAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionCookie == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	session, _, _, err := s.authenticateSession(req.SessionCookie)
	if err != nil {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	if s.oidcClient == nil || !s.oidcClient.IsAdmin(session.Groups) {
		http.Error(w, `{"error":"forbidden: admin access required"}`, http.StatusForbidden)
		return
	}

	users, err := s.store.ListUsers()
	if err != nil {
		http.Error(w, `{"error":"failed to list users"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(AdminListUsersResponse{Users: users})
}

func (s *Server) handleAdminImpersonate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req AdminImpersonateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionCookie == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	session, _, _, err := s.authenticateSession(req.SessionCookie)
	if err != nil {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	if s.oidcClient == nil || !s.oidcClient.IsAdmin(session.Groups) {
		http.Error(w, `{"error":"forbidden: admin access required"}`, http.StatusForbidden)
		return
	}

	if req.TargetUser != "" {
		// Ensure target user exists
		_, err := s.store.GetUser(req.TargetUser)
		if err != nil {
			http.Error(w, `{"error":"target user not found"}`, http.StatusNotFound)
			return
		}
		session.ImpersonatedUser = req.TargetUser
	} else {
		session.ImpersonatedUser = ""
	}

	newCookie, err := s.sessionManager.EncodeCookieValue(session)
	if err != nil {
		http.Error(w, `{"error":"failed to encode session"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(AdminImpersonateResponse{
		SessionCookie: newCookie,
		Impersonating: session.ImpersonatedUser,
		Success:       true,
	})
}

func (s *Server) handleAdminSetQuota(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req AdminSetQuotaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionCookie == "" || req.TargetUser == "" {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	session, _, _, err := s.authenticateSession(req.SessionCookie)
	if err != nil {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	if s.oidcClient == nil || !s.oidcClient.IsAdmin(session.Groups) {
		http.Error(w, `{"error":"forbidden: admin access required"}`, http.StatusForbidden)
		return
	}

	if err := s.store.SetUserQuota(req.TargetUser, req.QuotaBytes); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(GenericResponse{Success: true})
}

func (s *Server) handleSessionLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	userMeta, initialToken, err := s.store.EnsureUserProvisioned(req.Username)
	if err != nil {
		http.Error(w, `{"error":"failed to provision user"}`, http.StatusInternalServerError)
		return
	}

	flash := req.FlashToken
	if flash == "" {
		flash = initialToken
	}

	session := &auth.SessionData{
		Username:   userMeta.Username,
		Groups:     req.Groups,
		FlashToken: flash,
	}

	encoded, err := s.sessionManager.EncodeCookieValue(session)
	if err != nil {
		http.Error(w, `{"error":"failed to create session"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(CreateSessionResponse{
		SessionCookie: encoded,
	})
}

func (s *Server) handleUsageDelta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req UsageDeltaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	if req.Delta != 0 {
		s.store.AddUsageDelta(req.Username, req.Delta)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(GenericResponse{Success: true})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, `{"error":"store not initialized"}`, http.StatusServiceUnavailable)
		return
	}

	if err := s.store.CheckHealth(); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}
