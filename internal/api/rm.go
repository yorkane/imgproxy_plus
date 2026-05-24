package api

import (
	"net/http"
	"os"

	"imgproxy_plus/internal/config"
)

func HandleRm(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only DELETE allowed")
			return
		}
		if cfg.FileAPIDisable {
			writeError(w, http.StatusForbidden, "forbidden", "file API disabled")
			return
		}

		reqPath, fullPath, err := extractPath(r, "/api/rm", cfg)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}

		if reqPath == "/" || fullPath == cfg.DataRoot {
			writeError(w, http.StatusForbidden, "forbidden", "cannot delete data root")
			return
		}

		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "not_found", "path not found")
			return
		}

		if err := os.RemoveAll(fullPath); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "failed to delete: "+err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
