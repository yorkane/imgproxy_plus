package zipfs

import (
	"archive/zip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"imgproxy_plus/internal/config"
	"imgproxy_plus/internal/ziputil"
)

func Handler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		reqPath := strings.TrimPrefix(r.URL.Path, "/zip")
		if reqPath == "" || reqPath == "/" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if strings.Contains(reqPath, "..") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		fullPath := filepath.Join(cfg.DataRoot, filepath.Clean(reqPath))
		if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(cfg.DataRoot)) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		zipFile, innerPath := splitZipPath(fullPath, cfg)
		if zipFile == "" {
			http.Error(w, "not a zip file", http.StatusNotFound)
			return
		}

		if _, err := os.Stat(zipFile); os.IsNotExist(err) {
			http.Error(w, "zip not found", http.StatusNotFound)
			return
		}

		serveZipFile(w, r, zipFile, innerPath)
	}
}

func splitZipPath(fullPath string, cfg *config.Config) (zipFile, innerPath string) {
	path := fullPath
	for {
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
		if cfg.IsZipExt(ext) {
			zipFile = path
			innerPath = strings.TrimPrefix(fullPath, path)
			innerPath = strings.TrimPrefix(innerPath, "/")
			innerPath = strings.TrimPrefix(innerPath, string(filepath.Separator))
			return
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", ""
		}
		path = parent
	}
}

func serveZipFile(w http.ResponseWriter, r *http.Request, zipPath, innerPath string) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		http.Error(w, "cannot open zip", http.StatusInternalServerError)
		return
	}
	defer zr.Close()

	innerPath = strings.Trim(innerPath, "/")

	for _, f := range zr.File {
		name := strings.Trim(ziputil.DecodeName(f), "/")
		if strings.EqualFold(name, innerPath) || name == innerPath {
			if f.FileInfo().IsDir() {
				http.Error(w, "is directory", http.StatusNotFound)
				return
			}

			rc, err := f.Open()
			if err != nil {
				http.Error(w, "cannot read zip entry", http.StatusInternalServerError)
				return
			}
			defer rc.Close()

			mime := mimeByExt(f.Name)
			w.Header().Set("Content-Type", mime)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", f.UncompressedSize64))
			w.Header().Set("Accept-Ranges", "bytes")

			if r.Method == http.MethodHead {
				return
			}

			if _, err := io.Copy(w, rc); err != nil {
				slog.Warn("zip serve copy error", "error", err)
			}
			return
		}
	}

	http.Error(w, "not found in zip", http.StatusNotFound)
}

func mimeByExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".jfif", ".jiff":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".avif":
		return "image/avif"
	case ".bmp":
		return "image/bmp"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain"
	case ".html", ".htm":
		return "text/html"
	case ".xml":
		return "application/xml"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}
