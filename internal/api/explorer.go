package api

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	ErrAccessDenied    = errors.New("access denied: path escapes user directory")
	ErrPathNotFound    = errors.New("path not found")
	ErrCannotDeleteRoot = errors.New("cannot delete root directory")
)

type FileItem struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	IsDir   bool      `json:"is_dir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

type ExplorerResult struct {
	CurrentPath string     `json:"current_path"`
	Breadcrumbs []string   `json:"breadcrumbs"`
	Items       []FileItem `json:"items"`
}

// resolveSafePath ensures requested path is safely confined to userDir.
func resolveSafePath(userDir, requestedPath string) (string, error) {
	// Trim any leading slashes so filepath.Join doesn't ignore userDir
	cleanReq := filepath.Clean(requestedPath)
	cleanReq = strings.TrimPrefix(cleanReq, "/")
	cleanReq = strings.TrimPrefix(cleanReq, "\\")

	fullPath := filepath.Clean(filepath.Join(userDir, cleanReq))

	rel, err := filepath.Rel(userDir, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return "", ErrAccessDenied
	}
	return fullPath, nil
}

func ListFiles(userDir, requestedPath string) (*ExplorerResult, error) {
	targetPath, err := resolveSafePath(userDir, requestedPath)
	if err != nil {
		return nil, err
	}

	stat, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrPathNotFound
		}
		return nil, err
	}

	if !stat.IsDir() {
		return nil, errors.New("path is not a directory")
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return nil, err
	}

	rel, _ := filepath.Rel(userDir, targetPath)
	if rel == "." {
		rel = ""
	}

	var breadcrumbs []string
	if rel != "" {
		parts := strings.Split(rel, string(filepath.Separator))
		breadcrumbs = parts
	} else {
		breadcrumbs = []string{}
	}

	var items []FileItem
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		itemRelPath := filepath.Join(rel, entry.Name())
		items = append(items, FileItem{
			Name:    entry.Name(),
			Path:    itemRelPath,
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	// Sort directories first, then alphabetical
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	return &ExplorerResult{
		CurrentPath: rel,
		Breadcrumbs: breadcrumbs,
		Items:       items,
	}, nil
}

func DownloadFile(w http.ResponseWriter, r *http.Request, userDir, requestedPath string) error {
	targetPath, err := resolveSafePath(userDir, requestedPath)
	if err != nil {
		return err
	}

	stat, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrPathNotFound
		}
		return err
	}

	if stat.IsDir() {
		return errors.New("cannot download a directory directly")
	}

	file, err := os.Open(targetPath)
	if err != nil {
		return err
	}
	defer file.Close()

	ext := filepath.Ext(stat.Name())
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, stat.Name()))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))

	_, err = io.Copy(w, file)
	return err
}

func DeletePath(userDir, requestedPath string) error {
	if requestedPath == "" || requestedPath == "/" || requestedPath == "." {
		return ErrCannotDeleteRoot
	}

	targetPath, err := resolveSafePath(userDir, requestedPath)
	if err != nil {
		return err
	}

	// Double check we are not deleting the user root
	if targetPath == userDir {
		return ErrCannotDeleteRoot
	}

	return os.RemoveAll(targetPath)
}
