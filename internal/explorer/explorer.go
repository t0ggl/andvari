package explorer

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	ErrAccessDenied     = errors.New("access denied: path escapes user directory")
	ErrPathNotFound     = errors.New("path not found")
	ErrCannotDeleteRoot = errors.New("cannot delete root directory")
	ErrQuotaExceeded    = errors.New("quota exceeded: insufficient storage")
	ErrInvalidPath      = errors.New("invalid path")
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

type UploadedFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type UploadResult struct {
	Success  bool           `json:"success"`
	Uploaded []UploadedFile `json:"uploaded"`
	Delta    int64          `json:"delta"`
}

// ResolveSafePath ensures requested path is safely confined to userDir.
func ResolveSafePath(userDir, requestedPath string) (string, error) {
	if strings.Contains(requestedPath, "\x00") {
		return "", ErrAccessDenied
	}

	// Normalize backslashes to forward slashes for cross-platform safety
	normalized := strings.ReplaceAll(requestedPath, "\\", "/")

	// Trim any leading slashes so filepath.Join doesn't ignore userDir
	cleanReq := filepath.Clean(normalized)
	cleanReq = strings.TrimPrefix(cleanReq, "/")

	fullPath := filepath.Clean(filepath.Join(userDir, cleanReq))

	rel, err := filepath.Rel(userDir, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return "", ErrAccessDenied
	}

	// If the file or directory exists on disk, resolve symlinks to ensure
	// symlinks cannot escape the canonical user directory.
	if realUserDir, err := filepath.EvalSymlinks(userDir); err == nil {
		if realTarget, err := filepath.EvalSymlinks(fullPath); err == nil {
			realRel, err := filepath.Rel(realUserDir, realTarget)
			if err != nil || strings.HasPrefix(realRel, "..") || realRel == ".." {
				return "", ErrAccessDenied
			}
		}
	}

	return fullPath, nil
}

func ListFiles(userDir, requestedPath string) (*ExplorerResult, error) {
	targetPath, err := ResolveSafePath(userDir, requestedPath)
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
	targetPath, err := ResolveSafePath(userDir, requestedPath)
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

	targetPath, err := ResolveSafePath(userDir, requestedPath)
	if err != nil {
		return err
	}

	// Double check we are not deleting the user root
	if targetPath == userDir {
		return ErrCannotDeleteRoot
	}

	if realUserDir, err := filepath.EvalSymlinks(userDir); err == nil {
		if realTarget, err := filepath.EvalSymlinks(targetPath); err == nil && realTarget == realUserDir {
			return ErrCannotDeleteRoot
		}
	}

	if _, err := os.Stat(targetPath); err != nil {
		if os.IsNotExist(err) {
			return ErrPathNotFound
		}
		return err
	}

	return os.RemoveAll(targetPath)
}

// GetPathSize calculates the size of a specific file or directory in bytes.
func GetPathSize(path string) int64 {
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

func getPathFileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err == nil && !fi.IsDir() {
		return fi.Size()
	}
	return 0
}

type quotaLimitReader struct {
	rc        io.Reader
	remaining int64
	exceeded  bool
}

func (q *quotaLimitReader) Read(p []byte) (int, error) {
	if q.exceeded {
		return 0, ErrQuotaExceeded
	}
	if q.remaining <= 0 {
		q.exceeded = true
		return 0, ErrQuotaExceeded
	}
	if int64(len(p)) > q.remaining {
		p = p[:q.remaining]
	}
	n, err := q.rc.Read(p)
	q.remaining -= int64(n)
	return n, err
}

func saveSingleStream(userDir, relTarget string, stream io.Reader, quotaLimit, usedBytes int64) (*UploadedFile, int64, error) {
	if unescaped, err := url.PathUnescape(relTarget); err == nil {
		relTarget = unescaped
	}
	if unescaped, err := url.QueryUnescape(relTarget); err == nil {
		relTarget = unescaped
	}

	if strings.Contains(relTarget, "\x00") {
		return nil, 0, ErrAccessDenied
	}
	normalized := strings.ReplaceAll(relTarget, "\\", "/")
	if strings.HasPrefix(normalized, "/") || filepath.IsAbs(relTarget) {
		return nil, 0, ErrAccessDenied
	}
	cleanRel := filepath.Clean(normalized)
	if cleanRel == "." || cleanRel == "" {
		return nil, 0, ErrInvalidPath
	}

	targetPath, err := ResolveSafePath(userDir, cleanRel)
	if err != nil {
		return nil, 0, err
	}
	if targetPath == userDir {
		return nil, 0, ErrInvalidPath
	}

	// Verify target is not an existing directory
	if stat, err := os.Stat(targetPath); err == nil && stat.IsDir() {
		return nil, 0, fmt.Errorf("destination %q is an existing directory", cleanRel)
	}

	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return nil, 0, err
	}

	var existingSize int64
	if stat, err := os.Stat(targetPath); err == nil && !stat.IsDir() {
		existingSize = stat.Size()
	}

	var reader io.Reader = stream
	if quotaLimit > 0 {
		available := quotaLimit - usedBytes + existingSize
		if available < 0 {
			available = 0
		}
		reader = &quotaLimitReader{
			rc:        stream,
			remaining: available,
		}
	}

	tmpFile := filepath.Join(parentDir, fmt.Sprintf(".upload-%d.tmp", time.Now().UnixNano()))
	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, 0, err
	}

	written, copyErr := io.Copy(f, reader)
	closeErr := f.Close()

	if copyErr != nil {
		_ = os.Remove(tmpFile)
		if errors.Is(copyErr, ErrQuotaExceeded) || strings.Contains(copyErr.Error(), "quota exceeded") {
			return nil, 0, ErrQuotaExceeded
		}
		return nil, 0, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpFile)
		return nil, 0, closeErr
	}

	if err := os.Rename(tmpFile, targetPath); err != nil {
		_ = os.Remove(tmpFile)
		return nil, 0, err
	}

	delta := written - existingSize
	item := &UploadedFile{
		Name: filepath.Base(targetPath),
		Path: cleanRel,
		Size: written,
	}
	return item, delta, nil
}

// SaveUpload handles saving one or more uploaded files from a request into userDir.
// It supports multipart/form-data with subpaths, raw stream uploads, and directory creation (is_dir=true).
func SaveUpload(userDir, basePath string, r *http.Request, quotaLimit, usedBytes int64) (*UploadResult, error) {
	if unescaped, err := url.PathUnescape(basePath); err == nil {
		basePath = unescaped
	}
	if unescaped, err := url.QueryUnescape(basePath); err == nil {
		basePath = unescaped
	}
	if strings.Contains(basePath, "\x00") {
		return nil, ErrAccessDenied
	}
	normBase := strings.ReplaceAll(basePath, "\\", "/")
	if strings.HasPrefix(normBase, "/") || filepath.IsAbs(basePath) {
		return nil, ErrAccessDenied
	}

	// 1. Check if directory creation requested (e.g. is_dir=true or empty folder drop)
	if r.URL.Query().Get("is_dir") == "true" {
		cleanBase := filepath.Clean(normBase)
		if cleanBase == "" || cleanBase == "/" || cleanBase == "." {
			return nil, ErrInvalidPath
		}
		targetPath, err := ResolveSafePath(userDir, cleanBase)
		if err != nil {
			return nil, err
		}
		if targetPath == userDir {
			return nil, ErrInvalidPath
		}
		if err := os.MkdirAll(targetPath, 0755); err != nil {
			return nil, err
		}
		return &UploadResult{
			Success: true,
			Uploaded: []UploadedFile{
				{
					Name: filepath.Base(targetPath),
					Path: cleanBase,
					Size: 0,
				},
			},
			Delta: 0,
		}, nil
	}

	var uploaded []UploadedFile
	var totalDelta int64
	currentUsed := usedBytes

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		mr, err := r.MultipartReader()
		if err != nil {
			return nil, err
		}

		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}

			fileName := part.FileName()
			if fileName == "" {
				// Form field without file name - skip
				part.Close()
				continue
			}

			// In Go's mime/multipart, part.FileName() strips directory components with filepath.Base().
			// To support recursive folder uploads, we extract the raw filename from Content-Disposition
			// or X-Relative-Path.
			itemRel := fileName
			if cd := part.Header.Get("Content-Disposition"); cd != "" {
				if _, params, err := mime.ParseMediaType(cd); err == nil {
					if fn, ok := params["filename"]; ok && fn != "" {
						itemRel = fn
					}
				}
			}
			if relHeader := part.Header.Get("X-Relative-Path"); relHeader != "" {
				itemRel = relHeader
			}

			if unescaped, err := url.PathUnescape(itemRel); err == nil {
				itemRel = unescaped
			}
			if unescaped, err := url.QueryUnescape(itemRel); err == nil {
				itemRel = unescaped
			}
			if strings.Contains(itemRel, "\x00") {
				part.Close()
				return nil, ErrAccessDenied
			}
			normItem := strings.ReplaceAll(itemRel, "\\", "/")
			if strings.HasPrefix(normItem, "/") || filepath.IsAbs(itemRel) {
				part.Close()
				return nil, ErrAccessDenied
			}

			combinedRel := filepath.Join(normBase, normItem)
			item, delta, err := saveSingleStream(userDir, combinedRel, part, quotaLimit, currentUsed)
			part.Close()
			if err != nil {
				return nil, err
			}

			uploaded = append(uploaded, *item)
			currentUsed += delta
			totalDelta += delta
		}
	} else {
		// Raw body upload
		fileName := r.URL.Query().Get("filename")
		targetRel := basePath
		if fileName != "" {
			targetRel = filepath.Join(basePath, fileName)
		}
		if targetRel == "" || targetRel == "." {
			return nil, ErrInvalidPath
		}

		item, delta, err := saveSingleStream(userDir, targetRel, r.Body, quotaLimit, currentUsed)
		if err != nil {
			return nil, err
		}
		uploaded = append(uploaded, *item)
		totalDelta = delta
	}

	return &UploadResult{
		Success:  true,
		Uploaded: uploaded,
		Delta:    totalDelta,
	}, nil
}

