package router

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"imgproxy_plus/internal/api"
	"imgproxy_plus/internal/auth"
	"imgproxy_plus/internal/config"
	"imgproxy_plus/internal/img"
	"imgproxy_plus/internal/proxy"
	"imgproxy_plus/internal/static"
	"imgproxy_plus/internal/webdav"
	"imgproxy_plus/internal/zipfs"
)

type Dispatcher struct {
	cfg             *config.Config
	imgproxyClient  *proxy.ImgproxyClient
	imgHandler      *img.Handler
	apiImgHandler   *api.ImgHandler
}

func New(cfg *config.Config) *Dispatcher {
	return &Dispatcher{
		cfg:            cfg,
		imgproxyClient: proxy.NewImgproxyClient(cfg.ImgproxyURL, cfg.ImgproxyKey, cfg.ImgproxySalt),
		imgHandler:     img.NewHandler(cfg),
		apiImgHandler:  api.NewImgHandler(cfg),
	}
}

func (d *Dispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origPath := r.URL.Path
	path := stripPrefix(d.cfg.URLPrefix, r.URL.Path)
	if path != origPath {
		newURL := *r.URL
		newURL.Path = path
		r2 := *r
		r2.URL = &newURL
		r = &r2
	}

	slog.Debug("request", "method", r.Method, "path", path)

	switch {
	case strings.HasPrefix(path, "/api/ls"):
		withAuth(d.cfg, api.HandleLs(d.cfg)).ServeHTTP(w, r)
	case strings.HasPrefix(path, "/api/rm"):
		withAuth(d.cfg, api.HandleRm(d.cfg)).ServeHTTP(w, r)
	case path == "/api/move":
		withAuth(d.cfg, api.HandleMove(d.cfg)).ServeHTTP(w, r)
	case strings.HasPrefix(path, "/api/mkdir"):
		withAuth(d.cfg, api.HandleMkdir(d.cfg)).ServeHTTP(w, r)
	case strings.HasPrefix(path, "/api/upload"):
		withAuth(d.cfg, api.HandleUpload(d.cfg)).ServeHTTP(w, r)
	case path == "/api/img":
		withAuth(d.cfg, d.apiImgHandler).ServeHTTP(w, r)
	case path == "/api/batch-img":
		withAuth(d.cfg, api.HandleBatchImg(d.cfg)).ServeHTTP(w, r)
	case path == "/api/gallerize":
		withAuth(d.cfg, api.HandleGallerize(d.cfg)).ServeHTTP(w, r)
	case strings.HasPrefix(path, "/zip/"):
		withAuth(d.cfg, zipfs.Handler(d.cfg)).ServeHTTP(w, r)
	case strings.HasPrefix(path, "/img/"):
		withAuth(d.cfg, d.imgHandler).ServeHTTP(w, r)
	case path == "/or-gallery" || path == "/img-editor" || path == "/img-sequence" || path == "/":
		static.Handler().ServeHTTP(w, r)
	case strings.HasPrefix(path, "/libs/"):
		static.Handler().ServeHTTP(w, r)
	case strings.HasPrefix(path, "/js/"):
		static.Handler().ServeHTTP(w, r)
	case strings.HasPrefix(path, "/css/"):
		static.Handler().ServeHTTP(w, r)
	case path == "/health":
		d.imgproxyClient.ProxyTo(w, r)
	default:
		d.smartRoute(w, r)
	}
}

func stripPrefix(prefix, path string) string {
	if prefix == "" {
		return path
	}
	p := strings.TrimSuffix(prefix, "/")
	if strings.HasPrefix(path, p+"/") {
		return path[len(p):]
	}
	if path == p {
		return "/"
	}
	return path
}

func (d *Dispatcher) smartRoute(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if r.Method == "OPTIONS" {
		webdav.Handler(d.cfg).ServeHTTP(w, r)
		return
	}

	if webdavMethod(r.Method) || path == "/" {
		webdav.Handler(d.cfg).ServeHTTP(w, r)
		return
	}

	dataPath := filepath.Join(d.cfg.DataRoot, filepath.Clean(path))
	dataRoot := filepath.Clean(d.cfg.DataRoot)
	if strings.HasPrefix(filepath.Clean(dataPath), dataRoot) {
		info, err := os.Stat(dataPath)
		if err == nil {
			if info.IsDir() {
				webdav.Handler(d.cfg).ServeHTTP(w, r)
				return
			}
			slog.Debug("serving raw file", "path", path, "file", dataPath)
			http.ServeFile(w, r, dataPath)
			return
		}
	}

	static.Handler().ServeHTTP(w, r)
}

func webdavMethod(method string) bool {
	switch method {
	case "PROPFIND", "MKCOL", "COPY", "MOVE", "LOCK", "UNLOCK":
		return true
	}
	return false
}

func withAuth(cfg *config.Config, next http.Handler) http.Handler {
	return auth.Middleware(cfg, next)
}
