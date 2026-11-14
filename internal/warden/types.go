package warden



type WebDAVAuthRequest struct {
	Username string `json:"username"`
	Token    string `json:"token"`
}

type WebDAVAuthResponse struct {
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username"`
	UserDir       string `json:"user_dir,omitempty"`
	SocketPath    string `json:"socket_path,omitempty"`
	QuotaLimit    int64  `json:"quota_limit,omitempty"`
	QuotaUsed     int64  `json:"quota_used,omitempty"`
	Error         string `json:"error,omitempty"`
}

type SessionAuthRequest struct {
	SessionCookie string `json:"session_cookie"`
}

type SessionAuthResponse struct {
	Authenticated   bool   `json:"authenticated"`
	Username        string `json:"username"`
	EffectiveUser   string `json:"effective_user"`
	IsAdmin         bool   `json:"is_admin"`
	IsImpersonating bool   `json:"is_impersonating"`
	SocketPath      string `json:"socket_path,omitempty"`
	QuotaLimit      int64  `json:"quota_limit,omitempty"`
	QuotaUsed       int64  `json:"quota_used,omitempty"`
	Error           string `json:"error,omitempty"`
}

type CreateTokenRequest struct {
	SessionCookie string `json:"session_cookie"`
	Label         string `json:"label"`
}

type TokenResponse struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
}

type CreateTokenResponse struct {
	Token     string         `json:"token,omitempty"`
	TokenInfo *TokenResponse `json:"token_info,omitempty"`
	Error     string         `json:"error,omitempty"`
}

type RevokeTokenRequest struct {
	SessionCookie string `json:"session_cookie"`
	TokenID       string `json:"token_id"`
}

type RenameTokenRequest struct {
	SessionCookie string `json:"session_cookie"`
	TokenID       string `json:"token_id"`
	Label         string `json:"label"`
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
	Error               string          `json:"error,omitempty"`
}

type OnboardingCompleteRequest struct {
	SessionCookie string `json:"session_cookie"`
}

type AdminListUsersResponse struct {
	Users []string `json:"users,omitempty"`
	Error string   `json:"error,omitempty"`
}

type AdminImpersonateRequest struct {
	SessionCookie string `json:"session_cookie"`
	TargetUser    string `json:"target_user,omitempty"`
}

type AdminImpersonateResponse struct {
	SessionCookie string `json:"session_cookie,omitempty"`
	Impersonating string `json:"impersonating,omitempty"`
	Success       bool   `json:"success"`
	Error         string `json:"error,omitempty"`
}

type AdminSetQuotaRequest struct {
	SessionCookie string `json:"session_cookie"`
	TargetUser    string `json:"target_user"`
	QuotaBytes    *int64 `json:"quota_bytes"`
}

type CreateSessionRequest struct {
	Username   string   `json:"username"`
	Groups     []string `json:"groups"`
	FlashToken string   `json:"flash_token,omitempty"`
}

type CreateSessionResponse struct {
	SessionCookie string `json:"session_cookie,omitempty"`
	Error         string `json:"error,omitempty"`
}

type UsageDeltaRequest struct {
	Username string `json:"username"`
	Delta    int64  `json:"delta"`
}

type GenericResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}
