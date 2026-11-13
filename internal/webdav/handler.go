package webdav

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/t0ggl/andvari/internal/store"
	"github.com/t0ggl/andvari/internal/worker"
	"golang.org/x/net/webdav"
)

type Server struct {
	store         *store.Store
	lockSystem    webdav.LockSystem
	workerManager *worker.Manager
}

type quotaLimitReader struct {
	rc        io.ReadCloser
	remaining int64
	exceeded  bool
}

func (q *quotaLimitReader) Read(p []byte) (int, error) {
	if q.exceeded {
		return 0, errors.New("quota exceeded: insufficient storage")
	}
	if q.remaining <= 0 {
		q.exceeded = true
		return 0, errors.New("quota exceeded: insufficient storage")
	}
	if int64(len(p)) > q.remaining {
		p = p[:q.remaining]
	}
	n, err := q.rc.Read(p)
	q.remaining -= int64(n)
	return n, err
}

func (q *quotaLimitReader) Close() error {
	return q.rc.Close()
}

func NewServer(s *store.Store, wm ...*worker.Manager) *Server {
	srv := &Server{
		store:      s,
		lockSystem: webdav.NewMemLS(),
	}
	if len(wm) > 0 {
		srv.workerManager = wm[0]
	}
	return srv
}

func getFileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err == nil && !fi.IsDir() {
		return fi.Size()
	}
	return 0
}

func getPathSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if !fi.IsDir() {
		return fi.Size()
	}
	var total int64
	_ = filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
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

	usedBytes, quotaLimit, err := s.store.GetEffectiveQuota(username)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if s.workerManager != nil {
		delta, err := s.workerManager.ServeWebDAV(username, userDir, w, r, quotaLimit, usedBytes)
		if err != nil {
			log.Printf("[webdav] worker error for %s: %v", username, err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if delta != 0 {
			s.store.AddUsageDelta(username, delta)
		}
		return
	}

	targetRel := strings.TrimPrefix(r.URL.Path, "/webdav")
	targetPath := filepath.Join(userDir, filepath.Clean("/"+targetRel))

	var existingSize int64
	var deletedSize int64
	var copyNetBytes int64

	switch r.Method {
	case http.MethodPut:
		existingSize = getFileSize(targetPath)
		if quotaLimit > 0 {
			usedBytes, err := s.store.GetUserStorageUsage(username)
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			if r.ContentLength > 0 {
				netBytes := r.ContentLength - existingSize
				if netBytes > 0 && usedBytes+netBytes > quotaLimit {
					http.Error(w, "Insufficient Storage: User quota exceeded", http.StatusInsufficientStorage)
					return
				}
			}

			availableBytes := quotaLimit - usedBytes + existingSize
			if availableBytes < 0 {
				availableBytes = 0
			}
			r.Body = &quotaLimitReader{
				rc:        r.Body,
				remaining: availableBytes,
			}
		}

	case http.MethodDelete:
		deletedSize = getPathSize(targetPath)

	case "COPY":
		if quotaLimit > 0 {
			usedBytes, err := s.store.GetUserStorageUsage(username)
			if err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			srcSize := getPathSize(targetPath)
			var destExistingSize int64
			destHeader := r.Header.Get("Destination")
			if destHeader != "" {
				if idx := strings.Index(destHeader, "/webdav"); idx != -1 {
					destRel := strings.TrimPrefix(destHeader[idx:], "/webdav")
					destFull := filepath.Join(userDir, filepath.Clean("/"+destRel))
					destExistingSize = getFileSize(destFull)
				}
			}

			copyNetBytes = srcSize - destExistingSize
			if copyNetBytes > 0 && usedBytes+copyNetBytes > quotaLimit {
				http.Error(w, "Insufficient Storage: User quota exceeded", http.StatusInsufficientStorage)
				return
			}
		}
	}

	defer func() {
		switch r.Method {
		case http.MethodPut:
			newSize := getFileSize(targetPath)
			delta := newSize - existingSize
			if delta != 0 {
				s.store.AddUsageDelta(username, delta)
			}
		case http.MethodDelete:
			if deletedSize > 0 {
				s.store.AddUsageDelta(username, -deletedSize)
			}
		case "COPY":
			if copyNetBytes > 0 {
				s.store.AddUsageDelta(username, copyNetBytes)
			}
		}
	}()

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
