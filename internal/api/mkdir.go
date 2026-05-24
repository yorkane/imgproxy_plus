package api

import (
	"net/http"
	"os"

	"imgproxy_plus/internal/config"
)

func HandleMkdir(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST allowed")
			return
		}
		if cfg.FileAPIDisable {
			writeError(w, http.StatusForbidden, "forbidden", "file API disabled")
			return
		}

		_, fullPath, err := extractPath(r, "/api/mkdir", cfg)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}

		if info, err := os.Stat(fullPath); err == nil {
			if info.IsDir() {
				writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
				return
			}
			writeError(w, http.StatusConflict, "conflict", "path exists but is not a directory")
			return
		}

		if err := os.MkdirAll(fullPath, 0755); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "mkdir failed: "+err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
