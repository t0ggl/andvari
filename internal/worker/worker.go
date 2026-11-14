package worker

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/t0ggl/andvari/internal/explorer"
	"golang.org/x/net/webdav"
)

func init() {
	if len(os.Args) > 1 && os.Args[1] == "__worker" {
		RunWorkerCLI(os.Args[2:])
		os.Exit(0)
	}
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

type deltaResponseWriter struct {
	http.ResponseWriter
	beforeWrite func()
	written     bool
}

func (d *deltaResponseWriter) WriteHeader(status int) {
	if !d.written {
		d.written = true
		if d.beforeWrite != nil {
			d.beforeWrite()
		}
	}
	d.ResponseWriter.WriteHeader(status)
}

func (d *deltaResponseWriter) Write(b []byte) (int, error) {
	if !d.written {
		d.written = true
		if d.beforeWrite != nil {
			d.beforeWrite()
		}
	}
	return d.ResponseWriter.Write(b)
}

func getFileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err == nil && !fi.IsDir() {
		return fi.Size()
	}
	return 0
}

func RunWorkerCLI(args []string) {
	fs := flag.NewFlagSet("worker", flag.ExitOnError)
	user := fs.String("user", "", "Tenant username")
	userDir := fs.String("dir", "", "Tenant storage directory")
	socketPath := fs.String("socket", "", "Unix domain socket path")
	idleStr := fs.String("idle", "5m", "Worker idle timeout")
	_ = fs.Parse(args)

	if *user == "" || *userDir == "" || *socketPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: __worker --user <name> --dir <path> --socket <sock>\n")
		os.Exit(1)
	}

	idleTimeout, err := time.ParseDuration(*idleStr)
	if err != nil {
		idleTimeout = 5 * time.Minute
	}

	// Ensure user directory exists
	if err := os.MkdirAll(*userDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create user dir %s: %v\n", *userDir, err)
		os.Exit(1)
	}

	// Ensure socket directory exists and remove any stale socket
	_ = os.MkdirAll(filepath.Dir(*socketPath), 0700)
	_ = os.Remove(*socketPath)

	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to listen on socket %s: %v\n", *socketPath, err)
		os.Exit(1)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)

	_ = os.Chmod(*socketPath, 0600)

	// Apply Landlock to jail this process strictly to userDir
	if err := ApplyLandlock(*userDir); err != nil {
		log.Printf("[worker:%s] Landlock error: %v", *user, err)
	}

	lockSystem := webdav.NewMemLS()
	davHandler := &webdav.Handler{
		Prefix:     "/webdav",
		FileSystem: webdav.Dir(*userDir),
		LockSystem: lockSystem,
		Logger: func(req *http.Request, err error) {
			if err != nil && !strings.Contains(err.Error(), "file does not exist") {
				log.Printf("[worker:%s webdav] %s %s: %v", *user, req.Method, req.URL.Path, err)
			}
		},
	}

	var mu sync.Mutex
	lastActive := time.Now()

	mux := http.NewServeMux()

	// WebDAV Handler with Quota Tracking
	mux.HandleFunc("/webdav/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastActive = time.Now()
		mu.Unlock()

		quotaLimit, _ := strconv.ParseInt(r.Header.Get("X-Andvari-Quota-Limit"), 10, 64)
		usedBytes, _ := strconv.ParseInt(r.Header.Get("X-Andvari-Quota-Used"), 10, 64)

		if r.Method == "PROPFIND" {
			var bodyBytes []byte
			if r.Body != nil {
				bodyBytes, _ = io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}
			wantAvailable, wantUsed := ParsePropfindRequest(bodyBytes)
			if wantAvailable || wantUsed {
				rec := NewResponseBuffer()
				davHandler.ServeHTTP(rec, r)
				if rec.StatusCode == 0 {
					rec.StatusCode = http.StatusOK
				}
				if rec.StatusCode == http.StatusMultiStatus {
					modXML, err := InjectQuotaProperties(rec.Body.Bytes(), quotaLimit, usedBytes, wantAvailable, wantUsed)
					if err == nil {
						rec.Body.Reset()
						rec.Body.Write(modXML)
						rec.HeaderMap.Set("Content-Length", strconv.Itoa(len(modXML)))
					}
				}
				for k, vv := range rec.HeaderMap {
					for _, v := range vv {
						w.Header().Add(k, v)
					}
				}
				w.WriteHeader(rec.StatusCode)
				_, _ = w.Write(rec.Body.Bytes())
				return
			}
		}

		targetRel := strings.TrimPrefix(r.URL.Path, "/webdav")
		targetPath := filepath.Join(*userDir, filepath.Clean("/"+targetRel))

		var existingSize int64
		var deletedSize int64
		var copyNetBytes int64

		switch r.Method {
		case http.MethodPut:
			existingSize = getFileSize(targetPath)
			if quotaLimit > 0 {
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
			deletedSize = explorer.GetPathSize(targetPath)

		case "COPY":
			if quotaLimit > 0 {
				srcSize := explorer.GetPathSize(targetPath)
				var destExistingSize int64
				destHeader := r.Header.Get("Destination")
				if destHeader != "" {
					if idx := strings.Index(destHeader, "/webdav"); idx != -1 {
						destRel := strings.TrimPrefix(destHeader[idx:], "/webdav")
						destFull := filepath.Join(*userDir, filepath.Clean("/"+destRel))
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

		dw := &deltaResponseWriter{
			ResponseWriter: w,
			beforeWrite: func() {
				switch r.Method {
				case http.MethodPut:
					newSize := getFileSize(targetPath)
					delta := newSize - existingSize
					if delta != 0 {
						w.Header().Set("X-Andvari-Usage-Delta", strconv.FormatInt(delta, 10))
					}
				case http.MethodDelete:
					if deletedSize > 0 {
						w.Header().Set("X-Andvari-Usage-Delta", strconv.FormatInt(-deletedSize, 10))
					}
				case "COPY":
					if copyNetBytes > 0 {
						w.Header().Set("X-Andvari-Usage-Delta", strconv.FormatInt(copyNetBytes, 10))
					}
				}
			},
		}

		davHandler.ServeHTTP(dw, r)
	})

	// Web UI File Explorer: List & Delete
	mux.HandleFunc("/_worker/files", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastActive = time.Now()
		mu.Unlock()

		reqPath := r.URL.Query().Get("path")

		switch r.Method {
		case http.MethodGet:
			res, err := explorer.ListFiles(*userDir, reqPath)
			if err != nil {
				if errors.Is(err, explorer.ErrAccessDenied) {
					http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
					return
				}
				if errors.Is(err, explorer.ErrPathNotFound) {
					http.Error(w, `{"error":"path not found"}`, http.StatusNotFound)
					return
				}
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(res)

		case http.MethodDelete:
			targetPath := filepath.Join(*userDir, filepath.Clean("/"+reqPath))
			deletedSize := explorer.GetPathSize(targetPath)

			if err := explorer.DeletePath(*userDir, reqPath); err != nil {
				if errors.Is(err, explorer.ErrCannotDeleteRoot) || errors.Is(err, explorer.ErrAccessDenied) {
					http.Error(w, `{"error":"cannot delete specified path"}`, http.StatusForbidden)
					return
				}
				if errors.Is(err, explorer.ErrPathNotFound) {
					http.Error(w, `{"error":"path not found"}`, http.StatusNotFound)
					return
				}
				http.Error(w, `{"error":"failed to delete path"}`, http.StatusInternalServerError)
				return
			}

			w.Header().Set("X-Andvari-Usage-Delta", strconv.FormatInt(-deletedSize, 10))
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})

		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})

	// Web UI File Download
	mux.HandleFunc("/_worker/files/download", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastActive = time.Now()
		mu.Unlock()

		if r.URL.Query().Get("format") == "zip" || r.URL.Query().Get("archive") == "zip" || len(r.URL.Query()["paths"]) > 0 {
			paths := r.URL.Query()["paths"]
			if len(paths) == 0 {
				if single := r.URL.Query().Get("path"); single != "" {
					paths = []string{single}
				}
			}
			if err := explorer.DownloadArchive(w, r, *userDir, paths); err != nil {
				if errors.Is(err, explorer.ErrAccessDenied) {
					http.Error(w, "Access denied", http.StatusForbidden)
					return
				}
				if errors.Is(err, explorer.ErrPathNotFound) {
					http.Error(w, "File not found", http.StatusNotFound)
					return
				}
				http.Error(w, "Failed to download archive", http.StatusInternalServerError)
			}
			return
		}

		reqPath := r.URL.Query().Get("path")
		if err := explorer.DownloadFile(w, r, *userDir, reqPath); err != nil {
			if errors.Is(err, explorer.ErrAccessDenied) {
				http.Error(w, "Access denied", http.StatusForbidden)
				return
			}
			if errors.Is(err, explorer.ErrPathNotFound) {
				http.Error(w, "File not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Failed to download file", http.StatusInternalServerError)
			return
		}
	})

	// Web UI File Folder Creation
	mux.HandleFunc("/_worker/files/folder", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastActive = time.Now()
		mu.Unlock()

		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		reqPath := r.URL.Query().Get("path")
		if reqPath == "" && r.Body != nil {
			var body struct {
				Path string `json:"path"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			reqPath = body.Path
		}

		if err := explorer.CreateFolder(*userDir, reqPath); err != nil {
			if errors.Is(err, explorer.ErrAccessDenied) || errors.Is(err, explorer.ErrInvalidPath) {
				http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
				return
			}
			if errors.Is(err, os.ErrExist) {
				http.Error(w, `{"error":"folder already exists"}`, http.StatusConflict)
				return
			}
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	// Web UI File Rename / Move
	mux.HandleFunc("/_worker/files/rename", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastActive = time.Now()
		mu.Unlock()

		if r.Method != http.MethodPost && r.Method != http.MethodPatch {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		oldPath := r.URL.Query().Get("old_path")
		newPath := r.URL.Query().Get("new_path")
		if (oldPath == "" || newPath == "") && r.Body != nil {
			var body struct {
				OldPath string `json:"old_path"`
				NewPath string `json:"new_path"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if oldPath == "" {
				oldPath = body.OldPath
			}
			if newPath == "" {
				newPath = body.NewPath
			}
		}

		delta, err := explorer.RenamePath(*userDir, oldPath, newPath)
		if err != nil {
			if errors.Is(err, explorer.ErrAccessDenied) || errors.Is(err, explorer.ErrInvalidPath) {
				http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
				return
			}
			if errors.Is(err, explorer.ErrPathNotFound) {
				http.Error(w, `{"error":"path not found"}`, http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		if delta != 0 {
			w.Header().Set("X-Andvari-Usage-Delta", strconv.FormatInt(delta, 10))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "delta": delta})
	})

	// Web UI File Archive (ZIP) Download
	mux.HandleFunc("/_worker/files/archive", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastActive = time.Now()
		mu.Unlock()

		paths := r.URL.Query()["paths"]
		if len(paths) == 0 {
			if single := r.URL.Query().Get("path"); single != "" {
				paths = []string{single}
			}
		}

		if err := explorer.DownloadArchive(w, r, *userDir, paths); err != nil {
			if errors.Is(err, explorer.ErrAccessDenied) {
				http.Error(w, "Access denied", http.StatusForbidden)
				return
			}
			if errors.Is(err, explorer.ErrPathNotFound) {
				http.Error(w, "File not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("Failed to create archive: %v", err), http.StatusInternalServerError)
			return
		}
	})

	// Web UI File Upload
	mux.HandleFunc("/_worker/files/upload", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastActive = time.Now()
		mu.Unlock()

		if r.Method != http.MethodPost && r.Method != http.MethodPut {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		quotaLimit, _ := strconv.ParseInt(r.Header.Get("X-Andvari-Quota-Limit"), 10, 64)
		usedBytes, _ := strconv.ParseInt(r.Header.Get("X-Andvari-Quota-Used"), 10, 64)
		basePath := r.URL.Query().Get("path")

		res, err := explorer.SaveUpload(*userDir, basePath, r, quotaLimit, usedBytes)
		if err != nil {
			if errors.Is(err, explorer.ErrAccessDenied) || errors.Is(err, explorer.ErrInvalidPath) {
				http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
				return
			}
			if errors.Is(err, explorer.ErrQuotaExceeded) {
				http.Error(w, `{"error":"quota exceeded: insufficient storage"}`, http.StatusInsufficientStorage)
				return
			}
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		if res.Delta != 0 {
			w.Header().Set("X-Andvari-Usage-Delta", strconv.FormatInt(res.Delta, 10))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})

	// Negative Security Test Endpoint: Attempts direct filesystem read from inside the worker
	mux.HandleFunc("/_worker/security/test-read", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastActive = time.Now()
		mu.Unlock()

		target := r.URL.Query().Get("path")
		data, err := os.ReadFile(target)
		if err != nil {
			http.Error(w, fmt.Sprintf("read error: %v", err), http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(data)
	})

	// Ping Endpoint
	mux.HandleFunc("/_worker/ping", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastActive = time.Now()
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Idle timeout reaper goroutine
	stopIdle := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				idle := time.Since(lastActive) > idleTimeout
				mu.Unlock()
				if idle {
					_ = listener.Close()
					os.Exit(0)
				}
			case <-stopIdle:
				return
			}
		}
	}()

	// Signal readiness to supervisor
	fmt.Println("READY")
	_ = os.Stdout.Sync()

	server := &http.Server{Handler: mux}
	_ = server.Serve(listener)
	close(stopIdle)
}
