package webdav

import (
	"log"
	"net/http"
	"strings"

	"github.com/t0ggl/andvari/internal/store"
	"golang.org/x/net/webdav"
)

type Server struct {
	store      *store.Store
	lockSystem webdav.LockSystem
}

func NewServer(s *store.Store) *Server {
	return &Server{
		store:      s,
		lockSystem: webdav.NewMemLS(),
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	username, password, ok := r.BasicAuth()
	if !ok || username == "" || password == "" {
		w.Header().Set("WWW-Authenticate", `Basic realm="Andvari WebDAV"`)
		http.Error(w, "Unauthorized: WebDAV requires Basic authentication with your generated token", http.StatusUnauthorized)
		return
	}

	valid, _, err := s.store.AuthenticateToken(username, password)
	if err != nil || !valid {
		w.Header().Set("WWW-Authenticate", `Basic realm="Andvari WebDAV"`)
		http.Error(w, "Unauthorized: Invalid username or token", http.StatusUnauthorized)
		return
	}

	userDir, err := s.store.UserStoragePath(username)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	handler := &webdav.Handler{
		Prefix:     "/webdav",
		FileSystem: webdav.Dir(userDir),
		LockSystem: s.lockSystem,
		Logger: func(req *http.Request, err error) {
			if err != nil && !strings.Contains(err.Error(), "file does not exist") {
				log.Printf("[webdav] %s %s (user: %s): %v", req.Method, req.URL.Path, username, err)
			}
		},
	}

	handler.ServeHTTP(w, r)
}
