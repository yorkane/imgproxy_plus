package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"imgproxy_plus/internal/config"
	"imgproxy_plus/internal/proxy"
)

type ImgHandler struct {
	cfg    *config.Config
	client *proxy.ImgproxyClient
}

func NewImgHandler(cfg *config.Config) *ImgHandler {
	return &ImgHandler{
		cfg:    cfg,
		client: proxy.NewImgproxyClient(cfg.ImgproxyURL, cfg.ImgproxyKey, cfg.ImgproxySalt),
	}
}

func genUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b)
}

func (h *ImgHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST allowed")
		return
	}

	if r.ContentLength == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "empty body")
		return
	}

	id := genUUID()
	tmpDir := filepath.Join(h.cfg.RamdiskPath, ".imgapi-tmp")
	os.MkdirAll(tmpDir, 0755)
	tmpPath := filepath.Join(tmpDir, id)
	defer os.Remove(tmpPath)

	f, err := os.Create(tmpPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to create temp file")
		return
	}
	_, err = io.Copy(f, r.Body)
	f.Close()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "failed to write temp file")
		return
	}

	isAnimated := detectAnimated(tmpPath)
	sourceURL := "local:///" + tmpPath

	query := r.URL.Query()
	if isAnimated && imgHasParams(query) {
		slog.Debug("animated image, forwarding directly")
		w.Header().Set("X-Imgproxy", "passthrough-animated")
		http.ServeFile(w, r, tmpPath)
		return
	}
	if isAnimated {
		w.Header().Set("X-Imgproxy", "passthrough")
		http.ServeFile(w, r, tmpPath)
		return
	}

	opts := makeImgOptions(query)
	u := h.client.BuildProcessURL(opts.resize, opts.gravity, opts.quality, opts.format, sourceURL)

	if opts.resize == "" && opts.format == "" && opts.quality == "" {
		w.Header().Set("X-Imgproxy", "passthrough")
		http.ServeFile(w, r, tmpPath)
		return
	}

	if err := h.client.Fetch(u, w); err != nil {
		writeError(w, http.StatusBadGateway, "bad_gateway", "imgproxy call failed")
		return
	}
}

type imgOptions struct {
	resize  string
	gravity string
	quality string
	format  string
}

func makeImgOptions(query map[string][]string) imgOptions {
	var o imgOptions
	w := param(query, "w")
	h := param(query, "h")
	fit := param(query, "fit")
	crop := param(query, "crop")

	if crop != "" {
		o.resize = fmt.Sprintf("crop:%s", crop)
	} else if w != "" || h != "" {
		wV, hV := w, h
		if wV == "" {
			wV = "0"
		}
		if hV == "" {
			hV = "0"
		}
		rt := "fit"
		switch fit {
		case "cover":
			rt = "fill"
		case "fill":
			rt = "force"
		}
		o.resize = fmt.Sprintf("rs:%s:%s:%s", rt, wV, hV)
	}
	if g := param(query, "gravity"); g != "" {
		o.gravity = fmt.Sprintf("g:%s", g)
	}
	if q := param(query, "q"); q != "" {
		o.quality = fmt.Sprintf("q:%s", q)
	}
	if f := param(query, "fmt"); f != "" {
		o.format = fmt.Sprintf("format:%s", f)
	}
	return o
}

func param(query map[string][]string, key string) string {
	v, ok := query[key]
	if !ok || len(v) == 0 {
		return ""
	}
	return v[0]
}

func imgHasParams(query map[string][]string) bool {
	return param(query, "w") != "" || param(query, "h") != "" ||
		param(query, "fit") != "" || param(query, "crop") != "" ||
		param(query, "q") != "" || param(query, "fmt") != ""
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

	if string(buf[:4]) == "\x89PNG" {
		return false
	}

	if string(buf[:4]) == "RIFF" && n >= 12 && string(buf[8:12]) == "WEBP" {
		if n >= 17 && string(buf[12:16]) == "VP8X" {
			if buf[16]&0x02 != 0 {
				return true
			}
		}
		return false
	}

	if string(buf[:4]) == "GIF8" {
		return true
	}

	return false
}

func DetectAnimated(path string) bool {
	return detectAnimated(path)
}

type BatchImgResult struct {
	Src     string `json:"src"`
	Dst     string `json:"dst"`
	SizeIn  int64  `json:"size_in"`
	SizeOut int64  `json:"size_out"`
	Ms      int64  `json:"ms"`
}

type BatchImgResponse struct {
	OK      bool             `json:"ok"`
	Total   int              `json:"total"`
	Done    int              `json:"done"`
	Skipped int              `json:"skipped"`
	Errors  []string         `json:"errors"`
	Results []BatchImgResult `json:"results"`
}

var imgExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".jfif": true, ".jiff": true,
	".png": true, ".webp": true, ".gif": true, ".bmp": true,
	".tiff": true, ".tif": true, ".avif": true, ".svg": true,
	".ico": true, ".heic": true, ".heif": true,
}

func isImageExt(name string) bool {
	return imgExts[strings.ToLower(filepath.Ext(name))]
}

type batchImgReq struct {
	Path        string `json:"path"`
	W           int    `json:"w"`
	H           int    `json:"h"`
	Fit         string `json:"fit"`
	Fmt         string `json:"fmt"`
	Q           int    `json:"q"`
	OutSuffix   string `json:"out_suffix"`
	OutDir      string `json:"out_dir"`
	Overwrite   *bool  `json:"overwrite"`
	Recursive   bool   `json:"recursive"`
	Mode        string `json:"mode"`
	RemoteURL   string `json:"remote_url"`
	Concurrency int    `json:"concurrency"`
}

func HandleBatchImg(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST allowed")
			return
		}

		var req batchImgReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON")
			return
		}
		if req.Path == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "path required")
			return
		}

		fullPath, err := sanitizePath(cfg, req.Path)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}

		if req.Mode == "" {
			req.Mode = "local"
		}
		overwrite := true
		if req.Overwrite != nil {
			overwrite = *req.Overwrite
		}

		client := proxy.NewImgproxyClient(cfg.ImgproxyURL, cfg.ImgproxyKey, cfg.ImgproxySalt)
		images := scanImgFiles(fullPath, req.Recursive, nil)
		resp := BatchImgResponse{OK: true, Total: len(images)}

		for _, imgPath := range images {
			start := time.Now()

			outPath := buildOutPath(imgPath, req.OutSuffix, req.OutDir)
			if outPath == "" {
				resp.Skipped++
				continue
			}
			if !overwrite && fileExists(outPath) {
				resp.Skipped++
				continue
			}

			srcInfo, _ := os.Stat(imgPath)
			var srcSize int64
			if srcInfo != nil {
				srcSize = srcInfo.Size()
			}

			sourceURL := "local:///" + imgPath
			var o imgOptions

			if req.W > 0 || req.H > 0 {
				rt := "fit"
				if req.Fit == "cover" {
					rt = "fill"
				}
				o.resize = fmt.Sprintf("rs:%s:%d:%d", rt, req.W, req.H)
			}
			if req.Q > 0 {
				o.quality = fmt.Sprintf("q:%d", req.Q)
			}
			if req.Fmt != "" {
				o.format = fmt.Sprintf("format:%s", req.Fmt)
			}

			u := client.BuildProcessURL(o.resize, o.gravity, o.quality, o.format, sourceURL)

			resp2, err := http.Get(u)
			if err != nil {
				resp.Errors = append(resp.Errors, fmt.Sprintf("%s: %v", imgPath, err))
				continue
			}

			outFile, err := os.Create(outPath)
			if err != nil {
				resp2.Body.Close()
				resp.Errors = append(resp.Errors, fmt.Sprintf("%s: %v", imgPath, err))
				continue
			}
			written, _ := io.Copy(outFile, resp2.Body)
			outFile.Close()
			resp2.Body.Close()

			elapsed := time.Since(start).Milliseconds()
			resp.Results = append(resp.Results, BatchImgResult{
				Src: imgPath, Dst: outPath,
				SizeIn: srcSize, SizeOut: written, Ms: elapsed,
			})
			resp.Done++
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func scanImgFiles(dirPath string, recursive bool, ignoreExts map[string]bool) []string {
	var result []string
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return result
	}
	for _, entry := range entries {
		full := filepath.Join(dirPath, entry.Name())
		if entry.IsDir() {
			if recursive {
				result = append(result, scanImgFiles(full, true, ignoreExts)...)
			}
			continue
		}
		if ignoreExts != nil && ignoreExts[strings.ToLower(filepath.Ext(entry.Name()))] {
			continue
		}
		if isImageExt(entry.Name()) {
			result = append(result, full)
		}
	}
	return result
}

func buildOutPath(srcPath, suffix, outDir string) string {
	dir := filepath.Dir(srcPath)
	ext := filepath.Ext(srcPath)
	base := strings.TrimSuffix(filepath.Base(srcPath), ext)
	if outDir != "" {
		if suffix != "" {
			return filepath.Join(outDir, base+suffix+ext)
		}
		return filepath.Join(outDir, filepath.Base(srcPath))
	}
	if suffix != "" {
		return filepath.Join(dir, base+suffix+ext)
	}
	return ""
}
