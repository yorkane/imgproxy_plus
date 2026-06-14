package api

import (
	"net/http"

	"imgproxy_plus/internal/archive"
	"imgproxy_plus/internal/config"
)

func HandleMigrateCovers(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST allowed")
			return
		}

		result, err := archive.MigrateCovers(cfg)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}
