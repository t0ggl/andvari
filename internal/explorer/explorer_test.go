package explorer

import (
	"archive/zip"
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateFolder(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-explorer-mkdir-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	userDir := filepath.Join(tmpDir, "users", "alice")
	if err := os.MkdirAll(userDir, 0700); err != nil {
		t.Fatalf("failed to create userDir: %v", err)
	}

	// 1. Successful folder creation
	if err := CreateFolder(userDir, "documents"); err != nil {
		t.Fatalf("expected folder creation to succeed, got: %v", err)
	}
	fi, err := os.Stat(filepath.Join(userDir, "documents"))
	if err != nil || !fi.IsDir() {
		t.Fatalf("expected 'documents' to exist as a directory")
	}

	// 2. Nested folder creation
	if err := CreateFolder(userDir, "documents/projects/2026"); err != nil {
		t.Fatalf("expected nested folder creation to succeed, got: %v", err)
	}
	fi, err = os.Stat(filepath.Join(userDir, "documents", "projects", "2026"))
	if err != nil || !fi.IsDir() {
		t.Fatalf("expected nested directory to exist")
	}

	// 3. Existing folder returns os.ErrExist
	if err := CreateFolder(userDir, "documents"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected os.ErrExist for duplicate folder, got: %v", err)
	}

	// 4. Existing file returns os.ErrExist
	filePath := filepath.Join(userDir, "file.txt")
	_ = os.WriteFile(filePath, []byte("hello"), 0600)
	if err := CreateFolder(userDir, "file.txt"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected os.ErrExist when path conflicts with file, got: %v", err)
	}

	// 5. Rejection of path traversal
	if err := CreateFolder(userDir, "../outside"); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("expected ErrAccessDenied for path traversal, got: %v", err)
	}
	if err := CreateFolder(userDir, "/absolute/path"); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("expected ErrAccessDenied for absolute path, got: %v", err)
	}
	if err := CreateFolder(userDir, "foo/\x00bar"); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("expected ErrAccessDenied for null byte, got: %v", err)
	}

	// 6. Rejection of root or empty path
	if err := CreateFolder(userDir, ""); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("expected ErrInvalidPath for empty path, got: %v", err)
	}
	if err := CreateFolder(userDir, "."); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("expected ErrInvalidPath for dot path, got: %v", err)
	}
}

func TestRenamePath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-explorer-rename-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	userDir := filepath.Join(tmpDir, "users", "alice")
	if err := os.MkdirAll(filepath.Join(userDir, "docs"), 0700); err != nil {
		t.Fatalf("failed to create userDir: %v", err)
	}

	filePath := filepath.Join(userDir, "docs", "note.txt")
	_ = os.WriteFile(filePath, []byte("my note contents"), 0600)

	// 1. Rename file in place
	delta, err := RenamePath(userDir, "docs/note.txt", "docs/renamed.txt")
	if err != nil {
		t.Fatalf("RenamePath in place failed: %v", err)
	}
	if delta != 0 {
		t.Errorf("expected delta=0 for new destination, got %d", delta)
	}
	if _, err := os.Stat(filepath.Join(userDir, "docs", "renamed.txt")); err != nil {
		t.Fatalf("expected docs/renamed.txt to exist")
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected original note.txt to no longer exist")
	}

	// 2. Move file to new subdirectory (auto-creating parents)
	delta, err = RenamePath(userDir, "docs/renamed.txt", "archive/2026/note.txt")
	if err != nil {
		t.Fatalf("RenamePath to new directory failed: %v", err)
	}
	if delta != 0 {
		t.Errorf("expected delta=0, got %d", delta)
	}
	if _, err := os.Stat(filepath.Join(userDir, "archive", "2026", "note.txt")); err != nil {
		t.Fatalf("expected archive/2026/note.txt to exist")
	}

	// 3. Rename a folder
	if err := CreateFolder(userDir, "folder_a"); err != nil {
		t.Fatalf("CreateFolder failed: %v", err)
	}
	delta, err = RenamePath(userDir, "folder_a", "folder_b")
	if err != nil {
		t.Fatalf("Rename folder failed: %v", err)
	}
	if delta != 0 {
		t.Errorf("expected delta=0, got %d", delta)
	}
	if _, err := os.Stat(filepath.Join(userDir, "folder_b")); err != nil {
		t.Fatalf("expected folder_b to exist")
	}

	// 4. Overwrite existing file and check quota delta
	destPath := filepath.Join(userDir, "existing.txt")
	_ = os.WriteFile(destPath, []byte("1234567890"), 0600) // 10 bytes
	srcPath := filepath.Join(userDir, "src.txt")
	_ = os.WriteFile(srcPath, []byte("new replacement"), 0600)

	delta, err = RenamePath(userDir, "src.txt", "existing.txt")
	if err != nil {
		t.Fatalf("overwriting rename failed: %v", err)
	}
	if delta != -10 {
		t.Errorf("expected delta=-10 for replaced destination file, got %d", delta)
	}
	content, _ := os.ReadFile(destPath)
	if string(content) != "new replacement" {
		t.Errorf("expected destination to contain new content, got %s", string(content))
	}

	// 5. Cannot move directory into itself
	if err := os.MkdirAll(filepath.Join(userDir, "parent", "sub"), 0700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	_, err = RenamePath(userDir, "parent", "parent/sub/nested")
	if err == nil {
		t.Fatalf("expected error moving directory into itself")
	}

	// 6. Non-existent source returns ErrPathNotFound
	_, err = RenamePath(userDir, "non_existent.txt", "somewhere.txt")
	if !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("expected ErrPathNotFound, got: %v", err)
	}

	// 7. Path traversal rejection
	_, err = RenamePath(userDir, "docs", "../outside")
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("expected ErrAccessDenied for destination traversal, got: %v", err)
	}
	_, err = RenamePath(userDir, "../outside", "docs/new")
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("expected ErrAccessDenied for source traversal, got: %v", err)
	}
}

func TestDownloadArchive(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-explorer-zip-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	userDir := filepath.Join(tmpDir, "users", "alice")
	_ = os.MkdirAll(filepath.Join(userDir, "docs", "sub"), 0700)
	_ = os.WriteFile(filepath.Join(userDir, "docs", "file1.txt"), []byte("file one contents"), 0600)
	_ = os.WriteFile(filepath.Join(userDir, "docs", "sub", "file2.txt"), []byte("file two contents"), 0600)
	_ = os.WriteFile(filepath.Join(userDir, "root.txt"), []byte("root file"), 0600)

	// 1. Download folder as ZIP
	req := httptest.NewRequest("GET", "/api/files/download?path=docs", nil)
	rec := httptest.NewRecorder()

	err = DownloadArchive(rec, req, userDir, []string{"docs"})
	if err != nil {
		t.Fatalf("DownloadArchive for folder failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/zip" {
		t.Errorf("expected application/zip Content-Type, got %s", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "docs.zip") {
		t.Errorf("expected Content-Disposition with docs.zip, got %s", rec.Header().Get("Content-Disposition"))
	}

	// Verify zip contents
	zipReader, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("failed to parse zip: %v", err)
	}

	foundEntries := make(map[string]bool)
	for _, f := range zipReader.File {
		foundEntries[f.Name] = true
	}

	if !foundEntries["docs/file1.txt"] {
		t.Errorf("missing docs/file1.txt in zip entries: %+v", foundEntries)
	}
	if !foundEntries["docs/sub/file2.txt"] {
		t.Errorf("missing docs/sub/file2.txt in zip entries: %+v", foundEntries)
	}

	// 2. Download multiple items as ZIP
	reqMulti := httptest.NewRequest("GET", "/api/files/archive?paths=docs/file1.txt&paths=root.txt", nil)
	recMulti := httptest.NewRecorder()

	err = DownloadArchive(recMulti, reqMulti, userDir, []string{"docs/file1.txt", "root.txt"})
	if err != nil {
		t.Fatalf("DownloadArchive for multi items failed: %v", err)
	}
	if !strings.Contains(recMulti.Header().Get("Content-Disposition"), "archive.zip") {
		t.Errorf("expected archive.zip in Content-Disposition, got %s", recMulti.Header().Get("Content-Disposition"))
	}

	zipReaderMulti, err := zip.NewReader(bytes.NewReader(recMulti.Body.Bytes()), int64(recMulti.Body.Len()))
	if err != nil {
		t.Fatalf("failed to parse multi zip: %v", err)
	}

	foundMulti := make(map[string]bool)
	for _, f := range zipReaderMulti.File {
		foundMulti[f.Name] = true
	}
	if !foundMulti["file1.txt"] {
		t.Errorf("missing file1.txt in multi zip entries: %+v", foundMulti)
	}
	if !foundMulti["root.txt"] {
		t.Errorf("missing root.txt in multi zip entries: %+v", foundMulti)
	}

	// 3. DownloadFile on directory automatically invokes DownloadArchive
	reqDir := httptest.NewRequest("GET", "/api/files/download?path=docs", nil)
	recDir := httptest.NewRecorder()
	err = DownloadFile(recDir, reqDir, userDir, "docs")
	if err != nil {
		t.Fatalf("DownloadFile on directory failed: %v", err)
	}
	if recDir.Header().Get("Content-Type") != "application/zip" {
		t.Errorf("expected DownloadFile on dir to return application/zip, got %s", recDir.Header().Get("Content-Type"))
	}

	// 4. Traversal rejection
	recTrap := httptest.NewRecorder()
	err = DownloadArchive(recTrap, req, userDir, []string{"../../etc/passwd"})
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("expected ErrAccessDenied for path traversal, got: %v", err)
	}

	// 5. Header injection sanitization
	reqInject := httptest.NewRequest("GET", "/api/files/archive?paths=root.txt&filename=evil%0d%0aSet-Cookie:bad=true%22.zip", nil)
	recInject := httptest.NewRecorder()
	err = DownloadArchive(recInject, reqInject, userDir, []string{"root.txt"})
	if err != nil {
		t.Fatalf("DownloadArchive failed: %v", err)
	}
	disposition := recInject.Header().Get("Content-Disposition")
	if strings.Contains(disposition, "\r") || strings.Contains(disposition, "\n") || strings.Count(disposition, `"`) > 2 {
		t.Errorf("header injection in Content-Disposition: %q", disposition)
	}
}

func TestAncestorSymlinkEscapeSecurity(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-symlink-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outsideDir := filepath.Join(tmpDir, "outside")
	_ = os.MkdirAll(outsideDir, 0700)
	userDir := filepath.Join(tmpDir, "user")
	_ = os.MkdirAll(userDir, 0700)

	// Create symlink pointing outside
	symlinkPath := filepath.Join(userDir, "symlink_dir")
	if err := os.Symlink(outsideDir, symlinkPath); err != nil {
		t.Skip("symlinks not supported on filesystem")
	}

	// 1. CreateFolder through symlink to nonexistent subfolder should be rejected
	err = CreateFolder(userDir, "symlink_dir/new_subfolder")
	if !errors.Is(err, ErrAccessDenied) {
		t.Errorf("expected ErrAccessDenied for CreateFolder through symlink, got %v", err)
	}

	// 2. RenamePath to nonexistent path inside symlink should be rejected
	_ = os.WriteFile(filepath.Join(userDir, "file.txt"), []byte("data"), 0600)
	_, err = RenamePath(userDir, "file.txt", "symlink_dir/moved.txt")
	if !errors.Is(err, ErrAccessDenied) {
		t.Errorf("expected ErrAccessDenied for RenamePath destination through symlink, got %v", err)
	}
}
