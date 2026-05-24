package img

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"imgproxy_plus/internal/config"
	"imgproxy_plus/internal/proxy"
)

type Handler struct {
	cfg    *config.Config
	client *proxy.ImgproxyClient
}

func NewHandler(cfg *config.Config) *Handler {
	return &Handler{
		cfg:    cfg,
		client: proxy.NewImgproxyClient(cfg.ImgproxyURL, cfg.ImgproxyKey, cfg.ImgproxySalt),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method_not_allowed","message":"only GET allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	imgPath := strings.TrimPrefix(r.URL.Path, "/img/")
	if imgPath == "" {
		http.Error(w, `{"error":"bad_request","message":"path empty"}`, http.StatusBadRequest)
		return
	}
	if strings.Contains(imgPath, "..") {
		http.Error(w, `{"error":"bad_request","message":"path traversal"}`, http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(h.cfg.DataRoot, filepath.Clean(imgPath))
	if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(h.cfg.DataRoot)) {
		http.Error(w, `{"error":"forbidden","message":"path escapes data root"}`, http.StatusForbidden)
		return
	}

	if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
		firstImg := findFirstImage(fullPath)
		if firstImg == "" {
			http.Error(w, `{"error":"not_found","message":"no image found in directory"}`, http.StatusNotFound)
			return
		}
		fullPath = firstImg
	}

	sourceEncoded := encodeLocalSource(fullPath)
	query := r.URL.Query()

	isAnimated := detectAnimated(fullPath)
	if isAnimated {
		if hasProcessingParams(query) {
			slog.Debug("animated image detected, passing through", "path", imgPath)
			w.Header().Set("X-Imgproxy", "passthrough-animated")
			http.ServeFile(w, r, fullPath)
			return
		}
		w.Header().Set("X-Imgproxy", "passthrough")
		http.ServeFile(w, r, fullPath)
		return
	}

	opts := buildOptions(query)
	u := h.client.BuildProcessURL(opts.resize, opts.gravity, opts.quality, opts.format, sourceEncoded)

	if err := h.client.Fetch(u, w); err != nil {
		slog.Error("imgproxy fetch failed", "error", err)
		http.Error(w, `{"error":"bad_gateway","message":"imgproxy call failed"}`, http.StatusBadGateway)
		return
	}
}

type imgOptions struct {
	resize  string
	gravity string
	quality string
	format  string
}

func buildOptions(query url.Values) imgOptions {
	var o imgOptions

	w := query.Get("w")
	h_ := query.Get("h")
	fit := query.Get("fit")
	crop := query.Get("crop")

	if crop != "" {
		o.resize = fmt.Sprintf("crop:%s", crop)
	} else if w != "" || h_ != "" {
		wVal := w
		if wVal == "" {
			wVal = "0"
		}
		hVal := h_
		if hVal == "" {
			hVal = "0"
		}
		rt := "fit"
		switch fit {
		case "cover":
			rt = "fill"
		case "fill":
			rt = "force"
		}
		o.resize = fmt.Sprintf("rs:%s:%s:%s", rt, wVal, hVal)
	}

	if g := query.Get("gravity"); g != "" {
		o.gravity = fmt.Sprintf("g:%s", g)
	}
	if q := query.Get("q"); q != "" {
		o.quality = fmt.Sprintf("q:%s", q)
	}
	if f := query.Get("fmt"); f != "" {
		o.format = fmt.Sprintf("format:%s", f)
	}

	return o
}

func hasProcessingParams(query url.Values) bool {
	return query.Get("w") != "" || query.Get("h") != "" ||
		query.Get("fit") != "" || query.Get("crop") != "" ||
		query.Get("q") != "" || query.Get("fmt") != ""
}

func encodeLocalSource(path string) string {
	return "local:///" + path
}

func detectAnimated(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 30)
	n, _ := io.ReadFull(f, buf)
	if n < 4 {
		return false
	}

	if bytes.HasPrefix(buf, []byte("\x89PNG")) {
		return false
	}

	if bytes.HasPrefix(buf, []byte("RIFF")) && n >= 12 &&
		bytes.Equal(buf[8:12], []byte("WEBP")) {
		if n >= 17 && bytes.HasPrefix(buf[12:], []byte("VP8X")) {
			if buf[16]&0x10 != 0 {
				return true
			}
		}
		return false
	}

	if bytes.HasPrefix(buf, []byte("GIF8")) {
		return true
	}

	return false
}

var imgExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".jfif": true, ".jiff": true,
	".png": true, ".gif": true, ".webp": true, ".bmp": true,
	".avif": true, ".heic": true, ".heif": true, ".svg": true,
}

func findFirstImage(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var subDirs []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			subDirs = append(subDirs, filepath.Join(dir, name))
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext == "" {
			ext = "." + strings.ToLower(name)
		}
		if imgExts[ext] {
			return filepath.Join(dir, name)
		}
	}
	for _, sub := range subDirs {
		if found := findFirstImage(sub); found != "" {
			return found
		}
	}
	return ""
}
