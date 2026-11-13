package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"

	"github.com/t0ggl/andvari/internal/auth"
	"github.com/t0ggl/andvari/internal/store"
)

type API struct {
	store          *store.Store
	sessionManager *auth.SessionManager
	oidcClient     *auth.OIDCClient
}

func NewAPI(s *store.Store, sm *auth.SessionManager, oidc *auth.OIDCClient) *API {
	return &API{
		store:          s,
		sessionManager: sm,
		oidcClient:     oidc,
	}
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

func (a *API) getEffectiveUser(session *auth.SessionData) (effectiveUser string, isImpersonating bool) {
	isAdmin := a.oidcClient.IsAdmin(session.Groups)
	if isAdmin && session.ImpersonatedUser != "" {
		return session.ImpersonatedUser, true
	}
	return session.Username, false
}

func (a *API) HandleMe(w http.ResponseWriter, r *http.Request) {
	session, err := a.sessionManager.GetSession(r)
	if err != nil || session == nil || session.Username == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	isAdmin := a.oidcClient.IsAdmin(session.Groups)
	effectiveUser, isImpersonating := a.getEffectiveUser(session)

	userMeta, err := a.store.GetUser(effectiveUser)
	if err != nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}

	tokens := make([]TokenResponse, 0, len(userMeta.Tokens))
	for _, tok := range userMeta.Tokens {
		var lastUsed *string
		if tok.LastUsedAt != nil {
			s := tok.LastUsedAt.Format("2006-01-02 15:04:05 MST")
			lastUsed = &s
		}
		tokens = append(tokens, TokenResponse{
			ID:         tok.ID,
			Label:      tok.Label,
			CreatedAt:  tok.CreatedAt.Format("2006-01-02 15:04:05 MST"),
			LastUsedAt: lastUsed,
		})
	}

	usedBytes, quotaBytes, _ := a.store.GetEffectiveQuota(effectiveUser)
	var quotaPtr *int64
	if quotaBytes > 0 {
		quotaPtr = &quotaBytes
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

	// Consume flash token so it's only shown once
	if session.FlashToken != "" {
		session.FlashToken = ""
		_ = a.sessionManager.SaveSession(w, r, session)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *API) HandleGenerateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	session, err := a.sessionManager.GetSession(r)
	if err != nil || session == nil || session.Username == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	effectiveUser, _ := a.getEffectiveUser(session)

	var req struct {
		Label string `json:"label"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	rawToken, tokenInfo, err := a.store.AddToken(effectiveUser, req.Label)
	if err != nil {
		if errors.Is(err, store.ErrMaxTokensReached) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "Maximum of 2 active tokens allowed. Please revoke one before generating a new token.",
			})
			return
		}
		http.Error(w, `{"error":"failed to generate token"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"token": rawToken,
		"token_info": TokenResponse{
			ID:        tokenInfo.ID,
			Label:     tokenInfo.Label,
			CreatedAt: tokenInfo.CreatedAt.Format("2006-01-02 15:04:05 MST"),
		},
	})
}

func (a *API) HandleRevokeToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	session, err := a.sessionManager.GetSession(r)
	if err != nil || session == nil || session.Username == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	effectiveUser, _ := a.getEffectiveUser(session)

	tokenID := r.URL.Query().Get("id")
	if tokenID == "" {
		http.Error(w, `{"error":"missing token id"}`, http.StatusBadRequest)
		return
	}

	if err := a.store.RevokeToken(effectiveUser, tokenID); err != nil {
		if errors.Is(err, store.ErrTokenNotFound) {
			http.Error(w, `{"error":"token not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to revoke token"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (a *API) HandleCompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	session, err := a.sessionManager.GetSession(r)
	if err != nil || session == nil || session.Username == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	effectiveUser, _ := a.getEffectiveUser(session)
	if err := a.store.SetFirstLoginCompleted(effectiveUser); err != nil {
		http.Error(w, `{"error":"failed to update preference"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (a *API) HandleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	session, err := a.sessionManager.GetSession(r)
	if err != nil || session == nil || session.Username == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	if !a.oidcClient.IsAdmin(session.Groups) {
		http.Error(w, `{"error":"forbidden: admin access required"}`, http.StatusForbidden)
		return
	}

	users, err := a.store.ListUsers()
	if err != nil {
		http.Error(w, `{"error":"failed to list users"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"users": users,
	})
}

func (a *API) HandleAdminImpersonate(w http.ResponseWriter, r *http.Request) {
	session, err := a.sessionManager.GetSession(r)
	if err != nil || session == nil || session.Username == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	if !a.oidcClient.IsAdmin(session.Groups) {
		http.Error(w, `{"error":"forbidden: admin access required"}`, http.StatusForbidden)
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

		// Ensure user exists
		_, err := a.store.GetUser(req.TargetUser)
		if err != nil {
			http.Error(w, `{"error":"target user not found"}`, http.StatusNotFound)
			return
		}

		session.ImpersonatedUser = req.TargetUser
		if err := a.sessionManager.SaveSession(w, r, session); err != nil {
			http.Error(w, `{"error":"failed to update session"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"impersonating": req.TargetUser,
		})

	case http.MethodDelete:
		session.ImpersonatedUser = ""
		if err := a.sessionManager.SaveSession(w, r, session); err != nil {
			http.Error(w, `{"error":"failed to update session"}`, http.StatusInternalServerError)
			return
		}

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

	session, err := a.sessionManager.GetSession(r)
	if err != nil || session == nil || session.Username == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	if !a.oidcClient.IsAdmin(session.Groups) {
		http.Error(w, `{"error":"forbidden: admin access required"}`, http.StatusForbidden)
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

	if err := a.store.SetUserQuota(req.TargetUser, req.QuotaBytes); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
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
	session, err := a.sessionManager.GetSession(r)
	if err != nil || session == nil || session.Username == "" {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}

	effectiveUser, _ := a.getEffectiveUser(session)
	userDir, err := a.store.UserStoragePath(effectiveUser)
	if err != nil {
		http.Error(w, `{"error":"invalid user"}`, http.StatusBadRequest)
		return
	}

	reqPath := r.URL.Query().Get("path")

	switch r.Method {
	case http.MethodGet:
		result, err := ListFiles(userDir, reqPath)
		if err != nil {
			if errors.Is(err, ErrAccessDenied) {
				http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
				return
			}
			if errors.Is(err, ErrPathNotFound) {
				http.Error(w, `{"error":"path not found"}`, http.StatusNotFound)
				return
			}
			http.Error(w, `{"error":"failed to list files"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)

	case http.MethodDelete:
		targetPath := filepath.Join(userDir, filepath.Clean("/"+reqPath))
		deletedSize := GetPathSize(targetPath)
		if err := DeletePath(userDir, reqPath); err != nil {
			if errors.Is(err, ErrCannotDeleteRoot) || errors.Is(err, ErrAccessDenied) {
				http.Error(w, `{"error":"cannot delete specified path"}`, http.StatusForbidden)
				return
			}
			if errors.Is(err, ErrPathNotFound) {
				http.Error(w, `{"error":"path not found"}`, http.StatusNotFound)
				return
			}
			http.Error(w, `{"error":"failed to delete path"}`, http.StatusInternalServerError)
			return
		}

		if deletedSize > 0 {
			a.store.AddUsageDelta(effectiveUser, -deletedSize)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (a *API) HandleFileDownload(w http.ResponseWriter, r *http.Request) {
	session, err := a.sessionManager.GetSession(r)
	if err != nil || session == nil || session.Username == "" {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	effectiveUser, _ := a.getEffectiveUser(session)
	userDir, err := a.store.UserStoragePath(effectiveUser)
	if err != nil {
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}

	reqPath := r.URL.Query().Get("path")
	if err := DownloadFile(w, r, userDir, reqPath); err != nil {
		if errors.Is(err, ErrPathNotFound) {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrAccessDenied) {
			http.Error(w, "access denied", http.StatusForbidden)
			return
		}
		http.Error(w, "failed to download file", http.StatusInternalServerError)
		return
	}
}

// OIDC Handlers
func (a *API) HandleLogin(w http.ResponseWriter, r *http.Request) {
	state := auth.GenerateState()

	http.SetCookie(w, &http.Cookie{
		Name:     "andvari_oauth_state",
		Value:    state,
		Path:     "/auth/callback",
		HttpOnly: true,
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

	// Clear state cookie
	http.SetCookie(w, &http.Cookie{
		Name:   "andvari_oauth_state",
		Value:  "",
		Path:   "/auth/callback",
		MaxAge: -1,
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

	// Ensure user directory & initial token are provisioned
	userMeta, initialToken, err := a.store.EnsureUserProvisioned(claims.Username)
	if err != nil {
		http.Error(w, "failed to provision user", http.StatusInternalServerError)
		return
	}

	session := &auth.SessionData{
		Username:   userMeta.Username,
		Groups:     claims.Groups,
		FlashToken: initialToken,
	}

	if err := a.sessionManager.SaveSession(w, r, session); err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *API) HandleLogout(w http.ResponseWriter, r *http.Request) {
	a.sessionManager.ClearSession(w)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
