package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"imgproxy_plus/internal/config"
)

type moveRequest struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Overwrite bool   `json:"overwrite"`
}

func HandleMove(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST allowed")
			return
		}
		if cfg.FileAPIDisable {
			writeError(w, http.StatusForbidden, "forbidden", "file API disabled")
			return
		}

		var req moveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON")
			return
		}
		if req.From == "" || req.To == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "from and to are required")
			return
		}

		fromFull, err := sanitizePath(cfg, req.From)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		toFull, err := sanitizePath(cfg, req.To)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}

		if _, err := os.Stat(fromFull); os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "not_found", "source not found")
			return
		}

		if !req.Overwrite {
			if _, err := os.Stat(toFull); err == nil {
				writeError(w, http.StatusConflict, "conflict", "destination exists")
				return
			}
		}

		if err := os.MkdirAll(filepath.Dir(toFull), 0755); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "failed to create parent dir")
			return
		}
		if err := os.Rename(fromFull, toFull); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "move failed: "+err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
