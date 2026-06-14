package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"imgproxy_plus/internal/archive"
	"imgproxy_plus/internal/config"
)

type GallerizeRequest struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type GallerizeResponse struct {
	OK    bool           `json:"ok"`
	Error string         `json:"error,omitempty"`
	Path  string         `json:"path"`
	Type  string         `json:"type"`
	CBZ   []string       `json:"cbz,omitempty"`
	Stats *archive.Stats `json:"stats,omitempty"`
}

func HandleGallerize(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST allowed")
			return
		}

		var req GallerizeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON")
			return
		}

		if req.Type != "v2" {
			writeError(w, http.StatusForbidden, "forbidden", "only type v2 supported")
			return
		}

		if req.Path == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "path required")
			return
		}
		if strings.Contains(req.Path, "..") {
			writeError(w, http.StatusBadRequest, "bad_request", "path traversal")
			return
		}

		scanDir := filepath.Clean(cfg.GalleryScanDir)
		fullPath := filepath.Join(scanDir, filepath.Clean(req.Path))
		if !strings.HasPrefix(filepath.Clean(fullPath), scanDir) {
			writeError(w, http.StatusForbidden, "forbidden", "path escapes scan dir")
			return
		}

		info, err := os.Stat(fullPath)
		if err != nil || !info.IsDir() {
			writeError(w, http.StatusNotFound, "not_found", "directory not found")
			return
		}

		slog.Info("manual gallerize triggered", "path", req.Path)

		result, err := archive.ProcessOne(fullPath, cfg)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}

		resp := GallerizeResponse{
			OK:    true,
			Path:  req.Path,
			Type:  req.Type,
			CBZ:   result.CBZ,
			Stats: &result.Stats,
		}

		for _, cbz := range result.CBZ {
			slog.Info("gallerize generated cbz", "file", cbz)
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
