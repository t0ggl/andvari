package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/t0ggl/andvari/internal/auth"
	"github.com/t0ggl/andvari/internal/config"
	"github.com/t0ggl/andvari/internal/store"
	"github.com/t0ggl/andvari/internal/warden"
	"github.com/t0ggl/andvari/internal/worker"
)

type API struct {
	wardenClient *warden.Client
	oidcClient   *auth.OIDCClient
	startTime    time.Time
}

func NewAPI(wc *warden.Client, oidc *auth.OIDCClient) *API {
	return &API{
		wardenClient: wc,
		oidcClient:   oidc,
		startTime:    time.Now(),
	}
}

type HealthResponse struct {
	Status        string `json:"status"`
	Version       string `json:"version,omitempty"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	Error         string `json:"error,omitempty"`
}

// HandleHealth handles unauthenticated container health checks (/healthz, /api/health).
func (a *API) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	uptime := int64(time.Since(a.startTime).Seconds())
	if uptime < 0 {
		uptime = 0
	}

	w.Header().Set("Content-Type", "application/json")
	if err := a.wardenClient.CheckHealth(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(HealthResponse{
			Status:        "unhealthy",
			Version:       config.Version,
			UptimeSeconds: uptime,
			Error:         err.Error(),
		})
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(HealthResponse{
		Status:        "healthy",
		Version:       config.Version,
		UptimeSeconds: uptime,
	})
}

type TokenResponse struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
}

type UserInfoResponse struct {
	Username            string          `json:"username"`
	EffectiveUsername   string          `json:"effective_username"`
	IsAdmin             bool            `json:"is_admin"`
	IsImpersonating     bool            `json:"is_impersonating"`
	FirstLoginCompleted bool            `json:"first_login_completed"`
	Tokens              []TokenResponse `json:"tokens"`
	FlashToken          string          `json:"flash_token,omitempty"`
	QuotaBytes          *int64          `json:"quota_bytes"`
	UsedBytes           int64           `json:"used_bytes"`
}

func (a *API) HandleMe(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	me, err := a.wardenClient.GetUserInfo(cookie.Value)
	if err != nil {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(me)
}

func (a *API) HandleGenerateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	var req struct {
		Label string `json:"label"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	res, err := a.wardenClient.CreateToken(cookie.Value, req.Label)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnauthorized)
		return
	}
	if res.Error != "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": res.Error,
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"token":      res.Token,
		"token_info": res.TokenInfo,
	})
}

func (a *API) HandleRevokeToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	tokenID := r.URL.Query().Get("id")
	if tokenID == "" {
		var req struct {
			TokenID string `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		tokenID = req.TokenID
	}
	if tokenID == "" {
		http.Error(w, `{"error":"missing token id"}`, http.StatusBadRequest)
		return
	}
	if err := a.wardenClient.RevokeToken(cookie.Value, tokenID); err != nil {
		if errors.Is(err, store.ErrTokenNotFound) || strings.Contains(err.Error(), "not found") {
			http.Error(w, `{"error":"token not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (a *API) HandleRenameToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPut {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	var req struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	tokenID := req.ID
	if tokenID == "" {
		tokenID = r.URL.Query().Get("id")
	}
	if tokenID == "" {
		http.Error(w, `{"error":"missing token id"}`, http.StatusBadRequest)
		return
	}
	newLabel := strings.TrimSpace(req.Label)
	if newLabel == "" {
		http.Error(w, `{"error":"token label cannot be empty"}`, http.StatusBadRequest)
		return
	}
	info, err := a.wardenClient.RenameToken(cookie.Value, tokenID, newLabel)
	if err != nil {
		if errors.Is(err, store.ErrTokenNotFound) || strings.Contains(err.Error(), "not found") {
			http.Error(w, `{"error":"token not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"token_info": info,
	})
}

func (a *API) HandleCompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	if err := a.wardenClient.CompleteOnboarding(cookie.Value); err != nil {
		http.Error(w, `{"error":"failed to update preference"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (a *API) HandleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	users, err := a.wardenClient.AdminListUsers(cookie.Value)
	if err != nil {
		if strings.Contains(err.Error(), "forbidden") {
			http.Error(w, `{"error":"forbidden: admin access required"}`, http.StatusForbidden)
			return
		}
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"users": users,
	})
}

func (a *API) HandleAdminImpersonate(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req struct {
			TargetUser string `json:"target_user"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TargetUser == "" {
			http.Error(w, `{"error":"invalid target_user"}`, http.StatusBadRequest)
			return
		}
		res, err := a.wardenClient.AdminImpersonate(cookie.Value, req.TargetUser)
		if err != nil {
			if strings.Contains(err.Error(), "forbidden") {
				http.Error(w, `{"error":"forbidden: admin access required"}`, http.StatusForbidden)
				return
			}
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, `{"error":"target user not found"}`, http.StatusNotFound)
				return
			}
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		isHTTPS := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
		// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- Secure is set dynamically from request TLS / X-Forwarded-Proto for LAN HTTP or HTTPS proxy deployments
		http.SetCookie(w, &http.Cookie{
			Name:     auth.SessionCookieName,
			Value:    res.SessionCookie,
			Path:     "/",
			HttpOnly: true,
			Secure:   isHTTPS,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400 * 7,
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"impersonating": res.Impersonating,
		})
	case http.MethodDelete:
		res, err := a.wardenClient.AdminStopImpersonate(cookie.Value)
		if err != nil {
			if strings.Contains(err.Error(), "forbidden") {
				http.Error(w, `{"error":"forbidden: admin access required"}`, http.StatusForbidden)
				return
			}
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		isHTTPS := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
		// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- Secure is set dynamically from request TLS / X-Forwarded-Proto for LAN HTTP or HTTPS proxy deployments
		http.SetCookie(w, &http.Cookie{
			Name:     auth.SessionCookieName,
			Value:    res.SessionCookie,
			Path:     "/",
			HttpOnly: true,
			Secure:   isHTTPS,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400 * 7,
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (a *API) HandleAdminSetQuota(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	var req struct {
		TargetUser string `json:"target_user"`
		QuotaBytes *int64 `json:"quota_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TargetUser == "" {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}
	if err := a.wardenClient.AdminSetQuota(cookie.Value, req.TargetUser, req.QuotaBytes); err != nil {
		if strings.Contains(err.Error(), "forbidden") {
			http.Error(w, `{"error":"forbidden: admin access required"}`, http.StatusForbidden)
			return
		}
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (a *API) HandleFiles(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	authRes, err := a.wardenClient.AuthenticateSession(cookie.Value)
	if err != nil || authRes == nil || !authRes.Authenticated {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet, http.MethodDelete:
		delta, err := worker.ProxyFilesListOrDelete(authRes.SocketPath, authRes.EffectiveUser, w, r)
		if err != nil {
			log.Printf("[api] worker ProxyFilesListOrDelete error for %s: %v", authRes.EffectiveUser, err)
			http.Error(w, `{"error":"worker error"}`, http.StatusInternalServerError)
			return
		}
		if delta != 0 {
			_ = a.wardenClient.AddUsageDelta(authRes.EffectiveUser, delta)
		}
	case http.MethodPost:
		if r.URL.Query().Get("action") == "mkdir" {
			if err := worker.ProxyCreateFolder(authRes.SocketPath, authRes.EffectiveUser, w, r); err != nil {
				log.Printf("[api] worker ProxyCreateFolder error for %s: %v", authRes.EffectiveUser, err)
				http.Error(w, `{"error":"worker error"}`, http.StatusInternalServerError)
			}
			return
		}
		if r.URL.Query().Get("action") == "rename" {
			delta, err := worker.ProxyRenamePath(authRes.SocketPath, authRes.EffectiveUser, w, r)
			if err != nil {
				log.Printf("[api] worker ProxyRenamePath error for %s: %v", authRes.EffectiveUser, err)
				http.Error(w, `{"error":"worker error"}`, http.StatusInternalServerError)
				return
			}
			if delta != 0 {
				_ = a.wardenClient.AddUsageDelta(authRes.EffectiveUser, delta)
			}
			return
		}
		delta, err := worker.ProxyFileUpload(authRes.SocketPath, authRes.EffectiveUser, w, r, authRes.QuotaLimit, authRes.QuotaUsed)
		if err != nil {
			log.Printf("[api] worker ProxyFileUpload error for %s: %v", authRes.EffectiveUser, err)
			http.Error(w, `{"error":"worker error"}`, http.StatusInternalServerError)
			return
		}
		if delta != 0 {
			_ = a.wardenClient.AddUsageDelta(authRes.EffectiveUser, delta)
		}
	case http.MethodPatch:
		delta, err := worker.ProxyRenamePath(authRes.SocketPath, authRes.EffectiveUser, w, r)
		if err != nil {
			log.Printf("[api] worker ProxyRenamePath error for %s: %v", authRes.EffectiveUser, err)
			http.Error(w, `{"error":"worker error"}`, http.StatusInternalServerError)
			return
		}
		if delta != 0 {
			_ = a.wardenClient.AddUsageDelta(authRes.EffectiveUser, delta)
		}
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (a *API) HandleFileUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	authRes, err := a.wardenClient.AuthenticateSession(cookie.Value)
	if err != nil || authRes == nil || !authRes.Authenticated {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	delta, err := worker.ProxyFileUpload(authRes.SocketPath, authRes.EffectiveUser, w, r, authRes.QuotaLimit, authRes.QuotaUsed)
	if err != nil {
		log.Printf("[api] worker ProxyFileUpload error for %s: %v", authRes.EffectiveUser, err)
		http.Error(w, `{"error":"worker error"}`, http.StatusInternalServerError)
		return
	}
	if delta != 0 {
		_ = a.wardenClient.AddUsageDelta(authRes.EffectiveUser, delta)
	}
}

func (a *API) HandleFileDownload(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}
	authRes, err := a.wardenClient.AuthenticateSession(cookie.Value)
	if err != nil || authRes == nil || !authRes.Authenticated {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	if err := worker.ProxyFileDownload(authRes.SocketPath, authRes.EffectiveUser, w, r); err != nil {
		log.Printf("[api] worker ProxyFileDownload error for %s: %v", authRes.EffectiveUser, err)
		http.Error(w, "failed to download file", http.StatusInternalServerError)
	}
}

func (a *API) HandleCreateFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	authRes, err := a.wardenClient.AuthenticateSession(cookie.Value)
	if err != nil || authRes == nil || !authRes.Authenticated {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	if err := worker.ProxyCreateFolder(authRes.SocketPath, authRes.EffectiveUser, w, r); err != nil {
		log.Printf("[api] worker ProxyCreateFolder error for %s: %v", authRes.EffectiveUser, err)
		http.Error(w, `{"error":"worker error"}`, http.StatusInternalServerError)
	}
}

func (a *API) HandleRenamePath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPatch {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	authRes, err := a.wardenClient.AuthenticateSession(cookie.Value)
	if err != nil || authRes == nil || !authRes.Authenticated {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	delta, err := worker.ProxyRenamePath(authRes.SocketPath, authRes.EffectiveUser, w, r)
	if err != nil {
		log.Printf("[api] worker ProxyRenamePath error for %s: %v", authRes.EffectiveUser, err)
		http.Error(w, `{"error":"worker error"}`, http.StatusInternalServerError)
		return
	}
	if delta != 0 {
		_ = a.wardenClient.AddUsageDelta(authRes.EffectiveUser, delta)
	}
}

func (a *API) HandleArchiveDownload(w http.ResponseWriter, r *http.Request) {
	a.HandleFileDownload(w, r)
}

// OIDC Handlers
func (a *API) HandleLogin(w http.ResponseWriter, r *http.Request) {
	state := auth.GenerateState()
	isHTTPS := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- Secure is set dynamically from request TLS / X-Forwarded-Proto for LAN HTTP or HTTPS proxy deployments
	http.SetCookie(w, &http.Cookie{
		Name:     "andvari_oauth_state",
		Value:    state,
		Path:     "/auth/callback",
		HttpOnly: true,
		Secure:   isHTTPS,
		MaxAge:   300,
		SameSite: http.SameSiteLaxMode,
	})

	authURL := a.oidcClient.AuthCodeURL(state)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

func (a *API) HandleCallback(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("andvari_oauth_state")
	if err != nil || cookie.Value == "" {
		http.Error(w, "missing state cookie", http.StatusBadRequest)
		return
	}

	state := r.URL.Query().Get("state")
	if state != cookie.Value {
		http.Error(w, "invalid state parameter", http.StatusBadRequest)
		return
	}

	isHTTPS := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

	// Clear state cookie
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- Secure is set dynamically from request TLS / X-Forwarded-Proto for LAN HTTP or HTTPS proxy deployments
	http.SetCookie(w, &http.Cookie{
		Name:     "andvari_oauth_state",
		Value:    "",
		Path:     "/auth/callback",
		HttpOnly: true,
		Secure:   isHTTPS,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code parameter", http.StatusBadRequest)
		return
	}

	claims, err := a.oidcClient.ExchangeAndVerify(r.Context(), code)
	if err != nil {
		http.Error(w, "OIDC verification failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	sessionCookie, err := a.wardenClient.CreateSession(claims.Username, claims.Groups, "")
	if err != nil {
		http.Error(w, "failed to provision session: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- Secure is set dynamically from request TLS / X-Forwarded-Proto for LAN HTTP or HTTPS proxy deployments
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    sessionCookie,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 7,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *API) HandleLogout(w http.ResponseWriter, r *http.Request) {
	isHTTPS := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- Secure is set dynamically from request TLS / X-Forwarded-Proto for LAN HTTP or HTTPS proxy deployments
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
