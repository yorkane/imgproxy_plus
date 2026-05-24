package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"net"
	"net/http"
	"strings"

	"imgproxy_plus/internal/config"
)

func Middleware(cfg *config.Config, next http.Handler) http.Handler {
	if cfg.AuthUser == "" && len(cfg.AuthIPWhitelist) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if checkIPWhitelist(cfg, r) {
			next.ServeHTTP(w, r)
			return
		}
		if cfg.AuthUser != "" && checkBasicAuth(cfg, r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="imgproxy_plus"`)
		http.Error(w, `{"error":"unauthorized","message":"authentication required"}`, http.StatusUnauthorized)
	})
}

func checkIPWhitelist(cfg *config.Config, r *http.Request) bool {
	if len(cfg.AuthIPWhitelist) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, netw := range cfg.AuthIPWhitelist {
		if netw.Contains(ip) {
			return true
		}
	}
	return false
}

func checkBasicAuth(cfg *config.Config, r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if auth == "" || !strings.HasPrefix(auth, "Basic ") {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(auth[6:])
	if err != nil {
		return false
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return false
	}
	userOk := subtle.ConstantTimeCompare([]byte(parts[0]), []byte(cfg.AuthUser)) == 1
	passOk := subtle.ConstantTimeCompare([]byte(parts[1]), []byte(cfg.AuthPass)) == 1
	return userOk && passOk
}
