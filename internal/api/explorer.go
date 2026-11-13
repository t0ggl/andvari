package api

import (
	"net/http"

	"github.com/t0ggl/andvari/internal/explorer"
)

var (
	ErrAccessDenied     = explorer.ErrAccessDenied
	ErrPathNotFound     = explorer.ErrPathNotFound
	ErrCannotDeleteRoot = explorer.ErrCannotDeleteRoot
)

type FileItem = explorer.FileItem
type ExplorerResult = explorer.ExplorerResult

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
