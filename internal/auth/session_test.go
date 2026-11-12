package auth

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestSessionManager(t *testing.T) {
	secret := []byte("a-very-secret-32-byte-key-12345!")
	sm := NewSessionManager(secret)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)

	session := &SessionData{
		Username:         "alice",
		Groups:           []string{"admin", "users"},
		ImpersonatedUser: "bob",
		FlashToken:       "andv_test123",
	}

	if err := sm.SaveSession(rec, req, session); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	cookie := rec.Result().Cookies()[0]

	// Read session back
	readReq := httptest.NewRequest("GET", "/", nil)
	readReq.AddCookie(cookie)

	loaded, err := sm.GetSession(readReq)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if loaded.Username != "alice" || loaded.ImpersonatedUser != "bob" || loaded.FlashToken != "andv_test123" {
		t.Fatalf("session mismatch: %+v", loaded)
	}

	// Test clear session
	clearRec := httptest.NewRecorder()
	sm.ClearSession(clearRec)
	clearedCookie := clearRec.Result().Cookies()[0]
	if clearedCookie.MaxAge >= 0 {
		t.Fatalf("expected negative MaxAge on clear cookie")
	}
}

func TestOIDCClientDevMode(t *testing.T) {
	client, err := NewOIDCClient(context.Background(), "", "", "", "", []string{}, "admin", "preferred_username", true)
	if err != nil {
		t.Fatalf("NewOIDCClient failed: %v", err)
	}

	url := client.AuthCodeURL("state123")
	if url != "/auth/callback?code=dev-code&state=state123" {
		t.Fatalf("unexpected dev code URL: %s", url)
	}

	claims, err := client.ExchangeAndVerify(context.Background(), "dev-code")
	if err != nil {
		t.Fatalf("ExchangeAndVerify failed: %v", err)
	}
	if claims.Username != "devuser" {
		t.Fatalf("expected devuser, got: %s", claims.Username)
	}

	if !client.IsAdmin([]string{"admin"}) {
		t.Fatalf("expected admin group to return true")
	}
	if client.IsAdmin([]string{"regular"}) {
		t.Fatalf("expected regular group to return false")
	}
}
