package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"imgproxy_plus/internal/config"
)

func HandleUpload(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST allowed")
			return
		}
		if cfg.FileAPIDisable {
			writeError(w, http.StatusForbidden, "forbidden", "file API disabled")
			return
		}

		reqPath := strings.TrimPrefix(r.URL.Path, "/api/upload")
		if reqPath == "" || strings.HasSuffix(reqPath, "/") {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid upload path")
			return
		}

		if strings.Contains(reqPath, "..") {
			writeError(w, http.StatusBadRequest, "bad_request", "path traversal detected")
			return
		}

		fullPath := filepath.Join(cfg.DataRoot, filepath.Clean(reqPath))
		if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(cfg.DataRoot)) {
			writeError(w, http.StatusForbidden, "forbidden", "path escapes data root")
			return
		}

		if strings.HasSuffix(reqPath, "/") {
			writeError(w, http.StatusBadRequest, "bad_request", "path is a directory")
			return
		}

		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "mkdir parent failed")
			return
		}

		f, err := os.Create(fullPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "create file failed")
			return
		}
		defer f.Close()

		written, err := io.Copy(f, r.Body)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "write failed")
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "size": written})
	}
}
