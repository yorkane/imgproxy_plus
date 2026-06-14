package api

import (
	"net/http"

	"imgproxy_plus/internal/archive"
	"imgproxy_plus/internal/config"
)

func HandleArchiveStatus(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET allowed")
			return
		}

		data, err := archive.GetStatusJSON()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}
}
