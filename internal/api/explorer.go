package api

import (
	"net/http"

	"github.com/t0ggl/andvari/internal/explorer"
)

var (
	ErrAccessDenied     = explorer.ErrAccessDenied
	ErrPathNotFound     = explorer.ErrPathNotFound
	ErrCannotDeleteRoot = explorer.ErrCannotDeleteRoot
	ErrQuotaExceeded    = explorer.ErrQuotaExceeded
	ErrInvalidPath      = explorer.ErrInvalidPath
)

type FileItem = explorer.FileItem
type ExplorerResult = explorer.ExplorerResult
type UploadedFile = explorer.UploadedFile
type UploadResult = explorer.UploadResult

func ResolveSafePath(userDir, requestedPath string) (string, error) {
	return explorer.ResolveSafePath(userDir, requestedPath)
}

func ListFiles(userDir, requestedPath string) (*ExplorerResult, error) {
	return explorer.ListFiles(userDir, requestedPath)
}

func DownloadFile(w http.ResponseWriter, r *http.Request, userDir, requestedPath string) error {
	return explorer.DownloadFile(w, r, userDir, requestedPath)
}

func DeletePath(userDir, requestedPath string) error {
	return explorer.DeletePath(userDir, requestedPath)
}

func GetPathSize(path string) int64 {
	return explorer.GetPathSize(path)
}

func SaveUpload(userDir, basePath string, r *http.Request, quotaLimit, usedBytes int64) (*UploadResult, error) {
	return explorer.SaveUpload(userDir, basePath, r, quotaLimit, usedBytes)
}

func CreateFolder(userDir, requestedPath string) error {
	return explorer.CreateFolder(userDir, requestedPath)
}

func RenamePath(userDir, oldPath, newPath string) (int64, error) {
	return explorer.RenamePath(userDir, oldPath, newPath)
}

func DownloadArchive(w http.ResponseWriter, r *http.Request, userDir string, requestedPaths []string) error {
	return explorer.DownloadArchive(w, r, userDir, requestedPaths)
}
