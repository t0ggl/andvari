package auth

import (
	"crypto/sha256"
	"net/http"
	"time"

	"github.com/gorilla/securecookie"
)

const SessionCookieName = "andvari_session"

type SessionData struct {
	Username         string   `json:"username"`
	Groups           []string `json:"groups"`
	ImpersonatedUser string   `json:"impersonated_user,omitempty"`
	FlashToken       string   `json:"flash_token,omitempty"`
	CreatedAt        int64    `json:"created_at"`
}

type SessionManager struct {
	sc *securecookie.SecureCookie
}

func NewSessionManager(secret []byte) *SessionManager {
	// Derive 32-byte hash key and 32-byte block key from secret
	hashKey := sha256.Sum256(secret)
	blockKey := sha256.Sum256(append(secret, []byte("_andvari_block_key")...))

	sc := securecookie.New(hashKey[:], blockKey[:])
	sc.MaxAge(86400 * 7) // 7 days
	return &SessionManager{sc: sc}
}

func (sm *SessionManager) DecodeCookieValue(val string) (*SessionData, error) {
	var data SessionData
	if err := sm.sc.Decode(SessionCookieName, val, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func (sm *SessionManager) EncodeCookieValue(data *SessionData) (string, error) {
	if data.CreatedAt == 0 {
		data.CreatedAt = time.Now().Unix()
	}
	return sm.sc.Encode(SessionCookieName, data)
}

func (sm *SessionManager) GetSession(r *http.Request) (*SessionData, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, err
	}
	return sm.DecodeCookieValue(cookie.Value)
}

func (sm *SessionManager) SaveSession(w http.ResponseWriter, r *http.Request, data *SessionData) error {
	if data.CreatedAt == 0 {
		data.CreatedAt = time.Now().Unix()
	}

	encoded, err := sm.sc.Encode(SessionCookieName, data)
	if err != nil {
		return err
	}

	isHTTPS := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- Secure is set dynamically from request TLS / X-Forwarded-Proto for LAN HTTP or HTTPS proxy deployments
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    encoded,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 7,
	})

	return nil
}

func (sm *SessionManager) ClearSession(w http.ResponseWriter) {
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure -- clearing session cookie across HTTP and HTTPS deployments
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
