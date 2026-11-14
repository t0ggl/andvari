package webdav

import (
	"log"
	"net/http"

	"github.com/t0ggl/andvari/internal/warden"
	"github.com/t0ggl/andvari/internal/worker"
)

type Server struct {
	wardenClient *warden.Client
}

func NewServer(wc *warden.Client) *Server {
	return &Server{wardenClient: wc}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	username, password, ok := r.BasicAuth()
	if !ok || username == "" || password == "" {
		w.Header().Set("WWW-Authenticate", `Basic realm="Andvari WebDAV"`)
		http.Error(w, "Unauthorized: WebDAV requires Basic authentication with your generated token", http.StatusUnauthorized)
		return
	}

	authRes, err := s.wardenClient.AuthenticateWebDAV(username, password)
	if err != nil || authRes == nil || !authRes.Authenticated {
		w.Header().Set("WWW-Authenticate", `Basic realm="Andvari WebDAV"`)
		http.Error(w, "Unauthorized: Invalid username or token", http.StatusUnauthorized)
		return
	}

	delta, err := worker.ProxyWebDAV(authRes.SocketPath, username, w, r, authRes.QuotaLimit, authRes.QuotaUsed)
	if err != nil {
		log.Printf("[webdav] proxy error for %s: %v", username, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if delta != 0 {
		_ = s.wardenClient.AddUsageDelta(username, delta)
	}
}
