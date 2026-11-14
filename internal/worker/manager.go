package worker

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type WorkerInstance struct {
	Username   string
	UserDir    string
	SocketPath string
	Cmd        *exec.Cmd
	Transport  *http.Transport
	Client     *http.Client
	LastActive time.Time
}

type Manager struct {
	socketDir   string
	idleTimeout time.Duration
	executable  string
	mu          sync.Mutex
	workers     map[string]*WorkerInstance
	closed      bool
	stopReaper  chan struct{}
}

func isProcessAlive(p *os.Process) bool {
	if p == nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func NewManager(socketDir string, idleTimeout time.Duration) *Manager {
	// Linux Unix domain sockets are limited to 108 characters.
	// If socketDir is empty or long, use a short temporary directory in /tmp.
	if socketDir == "" || len(socketDir) > 70 {
		sub, err := os.MkdirTemp("/tmp", "andv-*")
		if err == nil {
			socketDir = sub
		} else {
			socketDir = "/tmp/andvari-workers"
			_ = os.MkdirAll(socketDir, 0700)
		}
	} else {
		_ = os.MkdirAll(socketDir, 0700)
	}

	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}

	m := &Manager{
		socketDir:   socketDir,
		idleTimeout: idleTimeout,
		executable:  exe,
		workers:     make(map[string]*WorkerInstance),
		stopReaper:  make(chan struct{}),
	}

	go m.reaperLoop()
	return m
}

func (m *Manager) reaperLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.mu.Lock()
			now := time.Now()
			for user, w := range m.workers {
				if now.Sub(w.LastActive) > m.idleTimeout {
					log.Printf("[worker-manager] worker for %s idle for %v, stopping", user, now.Sub(w.LastActive))
					if w.Cmd != nil && w.Cmd.Process != nil {
						_ = w.Cmd.Process.Kill()
					}
					_ = os.Remove(w.SocketPath)
					delete(m.workers, user)
				}
			}
			m.mu.Unlock()
		case <-m.stopReaper:
			return
		}
	}
}

func (m *Manager) socketPathForUser(username string) string {
	sum := sha256.Sum256([]byte(username))
	shortHash := fmt.Sprintf("%x", sum)[:16]
	return filepath.Join(m.socketDir, fmt.Sprintf("%s.sock", shortHash))
}

func (m *Manager) GetWorker(username, userDir string) (*WorkerInstance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, fmt.Errorf("worker manager is closed")
	}

	if w, ok := m.workers[username]; ok {
		if w.Cmd != nil && w.Cmd.Process != nil && isProcessAlive(w.Cmd.Process) {
			w.LastActive = time.Now()
			return w, nil
		}
		// Stale or dead worker
		if w.Cmd != nil && w.Cmd.Process != nil {
			_ = w.Cmd.Process.Kill()
			_, _ = w.Cmd.Process.Wait()
		}
		_ = os.Remove(w.SocketPath)
		delete(m.workers, username)
	}

	sockPath := m.socketPathForUser(username)
	_ = os.Remove(sockPath)

	cmd := exec.Command(
		m.executable,
		"__worker",
		"--user", username,
		"--dir", userDir,
		"--socket", sockPath,
		"--idle", m.idleTimeout.String(),
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open stdout pipe: %w", err)
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start worker for %s: %w", username, err)
	}

	readyCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() && strings.TrimSpace(scanner.Text()) == "READY" {
			readyCh <- nil
		} else {
			readyCh <- fmt.Errorf("worker failed to report READY: %s", stderrBuf.String())
		}
	}()

	select {
	case err := <-readyCh:
		if err != nil {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return nil, err
		}
	case <-time.After(5 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return nil, fmt.Errorf("timed out waiting for worker %s: %s", username, stderrBuf.String())
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sockPath)
		},
		DisableKeepAlives: false,
		MaxIdleConns:      10,
		IdleConnTimeout:   30 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   0,
	}

	w := &WorkerInstance{
		Username:   username,
		UserDir:    userDir,
		SocketPath: sockPath,
		Cmd:        cmd,
		Transport:  transport,
		Client:     client,
		LastActive: time.Now(),
	}

	m.workers[username] = w
	return w, nil
}

func (m *Manager) ServeWebDAV(username, userDir string, w http.ResponseWriter, r *http.Request, quotaLimit, usedBytes int64) (int64, error) {
	worker, err := m.GetWorker(username, userDir)
	if err != nil {
		return 0, fmt.Errorf("failed to get worker for %s: %w", username, err)
	}

	var delta int64
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "unix"
			req.Header.Set("X-Andvari-User", username)
			req.Header.Set("X-Andvari-Quota-Limit", strconv.FormatInt(quotaLimit, 10))
			req.Header.Set("X-Andvari-Quota-Used", strconv.FormatInt(usedBytes, 10))
		},
		Transport: worker.Transport,
		ModifyResponse: func(resp *http.Response) error {
			if deltaStr := resp.Header.Get("X-Andvari-Usage-Delta"); deltaStr != "" {
				if d, err := strconv.ParseInt(deltaStr, 10, 64); err == nil {
					delta = d
				}
				resp.Header.Del("X-Andvari-Usage-Delta")
			}
			return nil
		},
	}

	proxy.ServeHTTP(w, r)
	return delta, nil
}

func (m *Manager) ServeFilesList(username, userDir string, w http.ResponseWriter, r *http.Request) error {
	worker, err := m.GetWorker(username, userDir)
	if err != nil {
		return err
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "unix"
			req.URL.Path = "/_worker/files"
			req.Header.Set("X-Andvari-User", username)
		},
		Transport: worker.Transport,
	}

	proxy.ServeHTTP(w, r)
	return nil
}

func (m *Manager) ServeFilesDelete(username, userDir string, w http.ResponseWriter, r *http.Request) (int64, error) {
	worker, err := m.GetWorker(username, userDir)
	if err != nil {
		return 0, err
	}

	var delta int64
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "unix"
			req.URL.Path = "/_worker/files"
			req.Header.Set("X-Andvari-User", username)
		},
		Transport: worker.Transport,
		ModifyResponse: func(resp *http.Response) error {
			if deltaStr := resp.Header.Get("X-Andvari-Usage-Delta"); deltaStr != "" {
				if d, err := strconv.ParseInt(deltaStr, 10, 64); err == nil {
					delta = d
				}
				resp.Header.Del("X-Andvari-Usage-Delta")
			}
			return nil
		},
	}

	proxy.ServeHTTP(w, r)
	return delta, nil
}

func (m *Manager) ServeFileDownload(username, userDir string, w http.ResponseWriter, r *http.Request) error {
	worker, err := m.GetWorker(username, userDir)
	if err != nil {
		return err
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "unix"
			req.URL.Path = "/_worker/files/download"
			req.Header.Set("X-Andvari-User", username)
		},
		Transport: worker.Transport,
	}

	proxy.ServeHTTP(w, r)
	return nil
}

func (m *Manager) ServeArchiveDownload(username, userDir string, w http.ResponseWriter, r *http.Request) error {
	worker, err := m.GetWorker(username, userDir)
	if err != nil {
		return err
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "unix"
			req.URL.Path = "/_worker/files/archive"
			req.Header.Set("X-Andvari-User", username)
		},
		Transport: worker.Transport,
	}

	proxy.ServeHTTP(w, r)
	return nil
}

func (m *Manager) ServeFilesCreateFolder(username, userDir string, w http.ResponseWriter, r *http.Request) error {
	worker, err := m.GetWorker(username, userDir)
	if err != nil {
		return err
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "unix"
			req.URL.Path = "/_worker/files/folder"
			req.Header.Set("X-Andvari-User", username)
		},
		Transport: worker.Transport,
	}

	proxy.ServeHTTP(w, r)
	return nil
}

func (m *Manager) ServeFilesRename(username, userDir string, w http.ResponseWriter, r *http.Request) (int64, error) {
	worker, err := m.GetWorker(username, userDir)
	if err != nil {
		return 0, err
	}

	var delta int64
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "unix"
			req.URL.Path = "/_worker/files/rename"
			req.Header.Set("X-Andvari-User", username)
		},
		Transport: worker.Transport,
		ModifyResponse: func(resp *http.Response) error {
			if deltaStr := resp.Header.Get("X-Andvari-Usage-Delta"); deltaStr != "" {
				if d, err := strconv.ParseInt(deltaStr, 10, 64); err == nil {
					delta = d
				}
				resp.Header.Del("X-Andvari-Usage-Delta")
			}
			return nil
		},
	}

	proxy.ServeHTTP(w, r)
	return delta, nil
}

func (m *Manager) ServeFilesUpload(username, userDir string, w http.ResponseWriter, r *http.Request, quotaLimit, usedBytes int64) (int64, error) {
	worker, err := m.GetWorker(username, userDir)
	if err != nil {
		return 0, err
	}

	var delta int64
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "unix"
			req.URL.Path = "/_worker/files/upload"
			req.Header.Set("X-Andvari-User", username)
			req.Header.Set("X-Andvari-Quota-Limit", strconv.FormatInt(quotaLimit, 10))
			req.Header.Set("X-Andvari-Quota-Used", strconv.FormatInt(usedBytes, 10))
		},
		Transport: worker.Transport,
		ModifyResponse: func(resp *http.Response) error {
			if deltaStr := resp.Header.Get("X-Andvari-Usage-Delta"); deltaStr != "" {
				if d, err := strconv.ParseInt(deltaStr, 10, 64); err == nil {
					delta = d
				}
				resp.Header.Del("X-Andvari-Usage-Delta")
			}
			return nil
		},
	}

	proxy.ServeHTTP(w, r)
	return delta, nil
}

// SecurityTestRead attempts to read an arbitrary file from within the worker for security verification.
func (m *Manager) SecurityTestRead(username, userDir, targetPath string) ([]byte, int, error) {
	worker, err := m.GetWorker(username, userDir)
	if err != nil {
		return nil, 0, err
	}

	u := fmt.Sprintf("http://unix/_worker/security/test-read?path=%s", url.QueryEscape(targetPath))
	resp, err := worker.Client.Get(u)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}
	m.closed = true
	close(m.stopReaper)

	for _, w := range m.workers {
		if w.Cmd != nil && w.Cmd.Process != nil {
			_ = w.Cmd.Process.Kill()
		}
		_ = os.Remove(w.SocketPath)
	}
	m.workers = make(map[string]*WorkerInstance)
	_ = os.RemoveAll(m.socketDir)
	return nil
}
