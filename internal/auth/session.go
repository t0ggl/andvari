package auth

import (
	"crypto/sha256"
	"net/http"
	"time"

	"github.com/gorilla/securecookie"
)

const sessionCookieName = "andvari_session"

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

func (sm *SessionManager) GetSession(r *http.Request) (*SessionData, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, err
	}

	var data SessionData
	if err := sm.sc.Decode(sessionCookieName, cookie.Value, &data); err != nil {
		return nil, err
	}

	return &data, nil
}

func (sm *SessionManager) SaveSession(w http.ResponseWriter, r *http.Request, data *SessionData) error {
	if data.CreatedAt == 0 {
		data.CreatedAt = time.Now().Unix()
	}

	encoded, err := sm.sc.Encode(sessionCookieName, data)
	if err != nil {
		return err
	}

	isHTTPS := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
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
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}
