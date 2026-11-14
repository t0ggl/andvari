package worker

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/t0ggl/andvari/internal/explorer"
)

func TestWorkerWebDAVAndQuota(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-worker-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	userDir := filepath.Join(tmpDir, "users", "alice")
	workerDir := filepath.Join(tmpDir, "workers")
	_ = os.MkdirAll(userDir, 0700)

	mgr := NewManager(workerDir, 1*time.Minute)
	defer mgr.Close()

	// 1. PUT a file via ServeWebDAV
	fileContent := "Hello, WebDAV Worker!"
	putReq := httptest.NewRequest("PUT", "/webdav/hello.txt", strings.NewReader(fileContent))
	putReq.Header.Set("Content-Length", "21")
	putRec := httptest.NewRecorder()

	delta, err := mgr.ServeWebDAV("alice", userDir, putRec, putReq, 1024*1024, 0)
	if err != nil {
		t.Fatalf("ServeWebDAV PUT failed: %v", err)
	}
	if putRec.Code != http.StatusCreated && putRec.Code != http.StatusNoContent {
		t.Fatalf("ServeWebDAV PUT unexpected status: %d", putRec.Code)
	}
	if delta != 21 {
		t.Errorf("expected delta=21, got %d", delta)
	}

	// Verify file was written to disk
	diskContent, err := os.ReadFile(filepath.Join(userDir, "hello.txt"))
	if err != nil || string(diskContent) != fileContent {
		t.Fatalf("file content mismatch on disk: %s (err: %v)", string(diskContent), err)
	}

	// 2. GET the file via ServeWebDAV
	getReq := httptest.NewRequest("GET", "/webdav/hello.txt", nil)
	getRec := httptest.NewRecorder()
	_, err = mgr.ServeWebDAV("alice", userDir, getRec, getReq, 1024*1024, 21)
	if err != nil {
		t.Fatalf("ServeWebDAV GET failed: %v", err)
	}
	if getRec.Code != http.StatusOK {
		t.Fatalf("ServeWebDAV GET status: %d", getRec.Code)
	}
	if getRec.Body.String() != fileContent {
		t.Errorf("expected body %q, got %q", fileContent, getRec.Body.String())
	}

	// 3. Quota enforcement: attempt PUT exceeding quota
	bigContent := bytes.Repeat([]byte("A"), 2000)
	overQuotaReq := httptest.NewRequest("PUT", "/webdav/big.txt", bytes.NewReader(bigContent))
	overQuotaReq.Header.Set("Content-Length", "2000")
	overQuotaRec := httptest.NewRecorder()

	// Quota limit is 100 bytes, already used 21 bytes
	_, err = mgr.ServeWebDAV("alice", userDir, overQuotaRec, overQuotaReq, 100, 21)
	if err != nil {
		t.Fatalf("ServeWebDAV quota PUT failed: %v", err)
	}
	if overQuotaRec.Code != http.StatusInsufficientStorage {
		t.Fatalf("expected 507 Insufficient Storage, got %d", overQuotaRec.Code)
	}

	// 4. DELETE the file via ServeWebDAV
	delReq := httptest.NewRequest("DELETE", "/webdav/hello.txt", nil)
	delRec := httptest.NewRecorder()
	delDelta, err := mgr.ServeWebDAV("alice", userDir, delRec, delReq, 1024*1024, 21)
	if err != nil {
		t.Fatalf("ServeWebDAV DELETE failed: %v", err)
	}
	if delRec.Code != http.StatusOK && delRec.Code != http.StatusNoContent {
		t.Fatalf("ServeWebDAV DELETE status: %d", delRec.Code)
	}
	if delDelta != -21 {
		t.Errorf("expected delta=-21, got %d", delDelta)
	}
}

func TestWorkerFileExplorerOps(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-worker-explorer-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	userDir := filepath.Join(tmpDir, "users", "bob")
	workerDir := filepath.Join(tmpDir, "workers")
	_ = os.MkdirAll(filepath.Join(userDir, "docs"), 0700)

	docPath := filepath.Join(userDir, "docs", "doc1.txt")
	_ = os.WriteFile(docPath, []byte("document one contents"), 0600)

	mgr := NewManager(workerDir, 1*time.Minute)
	defer mgr.Close()

	// 1. List files
	listReq := httptest.NewRequest("GET", "/api/files?path=docs", nil)
	listRec := httptest.NewRecorder()
	if err := mgr.ServeFilesList("bob", userDir, listRec, listReq); err != nil {
		t.Fatalf("ServeFilesList failed: %v", err)
	}
	if listRec.Code != http.StatusOK {
		t.Fatalf("ServeFilesList status: %d", listRec.Code)
	}

	var res explorer.ExplorerResult
	if err := json.Unmarshal(listRec.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal ExplorerResult failed: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].Name != "doc1.txt" {
		t.Fatalf("unexpected items: %+v", res.Items)
	}

	// 2. Download file
	downReq := httptest.NewRequest("GET", "/api/files/download?path=docs/doc1.txt", nil)
	downRec := httptest.NewRecorder()
	if err := mgr.ServeFileDownload("bob", userDir, downRec, downReq); err != nil {
		t.Fatalf("ServeFileDownload failed: %v", err)
	}
	if downRec.Code != http.StatusOK {
		t.Fatalf("ServeFileDownload status: %d", downRec.Code)
	}
	if downRec.Body.String() != "document one contents" {
		t.Errorf("download content mismatch: %q", downRec.Body.String())
	}

	// 3. Delete file
	delReq := httptest.NewRequest("DELETE", "/api/files?path=docs/doc1.txt", nil)
	delRec := httptest.NewRecorder()
	delta, err := mgr.ServeFilesDelete("bob", userDir, delRec, delReq)
	if err != nil {
		t.Fatalf("ServeFilesDelete failed: %v", err)
	}
	if delRec.Code != http.StatusOK {
		t.Fatalf("ServeFilesDelete status: %d", delRec.Code)
	}
	if delta != -21 {
		t.Errorf("expected delta=-21, got %d", delta)
	}

	// Verify file is gone
	if _, err := os.Stat(docPath); !os.IsNotExist(err) {
		t.Errorf("expected doc1.txt to be deleted")
	}

	// 4. Upload file with recursive subdirectories
	uploadBody := &bytes.Buffer{}
	writer := multipart.NewWriter(uploadBody)
	part, err := writer.CreateFormFile("files", "nested/sub/hello.txt")
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	_, _ = part.Write([]byte("greetings from landlock worker!"))
	_ = writer.Close()

	upReq := httptest.NewRequest("POST", "/api/files/upload?path=docs", uploadBody)
	upReq.Header.Set("Content-Type", writer.FormDataContentType())
	upRec := httptest.NewRecorder()

	upDelta, err := mgr.ServeFilesUpload("bob", userDir, upRec, upReq, 10000, 0)
	if err != nil {
		t.Fatalf("ServeFilesUpload failed: %v", err)
	}
	if upRec.Code != http.StatusOK {
		t.Fatalf("ServeFilesUpload status: %d (body: %s)", upRec.Code, upRec.Body.String())
	}
	expectedLen := int64(len("greetings from landlock worker!"))
	if upDelta != expectedLen {
		t.Errorf("expected upDelta=%d, got %d", expectedLen, upDelta)
	}

	// Verify file created in userDir
	uploadedFilePath := filepath.Join(userDir, "docs", "nested", "sub", "hello.txt")
	content, err := os.ReadFile(uploadedFilePath)
	if err != nil {
		t.Fatalf("failed to read uploaded file: %v", err)
	}
	if string(content) != "greetings from landlock worker!" {
		t.Errorf("uploaded content mismatch: %q", string(content))
	}
}

func TestWorkerProcessRestartOnKill(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-worker-restart-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	userDir := filepath.Join(tmpDir, "users", "charlie")
	workerDir := filepath.Join(tmpDir, "workers")
	_ = os.MkdirAll(userDir, 0700)

	mgr := NewManager(workerDir, 1*time.Minute)
	defer mgr.Close()

	// Start worker by making a request
	w1, err := mgr.GetWorker("charlie", userDir)
	if err != nil {
		t.Fatalf("GetWorker failed: %v", err)
	}
	pid1 := w1.Cmd.Process.Pid

	// Kill the worker process
	if err := w1.Cmd.Process.Kill(); err != nil {
		t.Fatalf("failed to kill worker: %v", err)
	}
	// Wait a moment for process to terminate
	_, _ = w1.Cmd.Process.Wait()

	// Next request should seamlessly restart worker
	w2, err := mgr.GetWorker("charlie", userDir)
	if err != nil {
		t.Fatalf("GetWorker after kill failed: %v", err)
	}
	pid2 := w2.Cmd.Process.Pid

	if pid1 == pid2 {
		t.Errorf("expected new worker process with different pid")
	}

	// Verify new worker is functional
	resp, err := w2.Client.Get("http://unix/_worker/ping")
	if err != nil {
		t.Fatalf("ping failed on restarted worker: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"ok"`) {
		t.Errorf("unexpected ping response: %s", string(body))
	}
}
