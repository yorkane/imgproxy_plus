package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"imgproxy_plus/internal/config"
	"imgproxy_plus/internal/proxy"
)

type GallerizeRequest struct {
	Path          string `json:"path"`
	Type          string `json:"type"`
	ExtraFilePath string `json:"extra_file_path"`
	W             int    `json:"w"`
	H             int    `json:"h"`
	Fit           string `json:"fit"`
	Q             int    `json:"q"`
}

type GallerizeResponse struct {
	OK      bool                   `json:"ok"`
	Skipped bool                   `json:"skipped,omitempty"`
	Path    string                 `json:"path"`
	Type    string                 `json:"type"`
	Steps   map[string]interface{} `json:"steps"`
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
		if req.Path == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "path required")
			return
		}
		if req.Type != "v1" && req.Type != "v2" {
			writeError(w, http.StatusForbidden, "forbidden", "unsupported gallerize type")
			return
		}

		fullPath, err := sanitizePath(cfg, req.Path)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}

		if req.W <= 0 {
			req.W = 2560
		}
		if req.H <= 0 {
			req.H = 2560
		}
		if req.Fit == "" {
			req.Fit = "contain"
		}
		if req.Q <= 0 {
			req.Q = 90
		}

		client := proxy.NewImgproxyClient(cfg.ImgproxyURL, cfg.ImgproxyKey, cfg.ImgproxySalt)

		info, err := os.Stat(fullPath)
		if err != nil || !info.IsDir() {
			writeError(w, http.StatusNotFound, "not_found", "path not found or not a directory")
			return
		}

		if req.Type == "v1" {
			resp, err := gallerizeV1(fullPath, req, cfg, client)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal", err.Error())
				return
			}
			writeJSON(w, http.StatusOK, resp)
			return
		}

		writeError(w, http.StatusForbidden, "forbidden", "type v2 not yet implemented")
	}
}

func gallerizeV1(rootPath string, req GallerizeRequest, cfg *config.Config, client *proxy.ImgproxyClient) (*GallerizeResponse, error) {
	resp := &GallerizeResponse{
		OK:   true,
		Path: req.Path,
		Type: req.Type,
	}

	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, fmt.Errorf("readdir: %w", err)
	}

	var images []os.DirEntry
	var nonImages []os.DirEntry
	var subDirs []os.DirEntry

	for _, e := range entries {
		if e.IsDir() {
			subDirs = append(subDirs, e)
		} else if isImageExt(e.Name()) {
			images = append(images, e)
		} else {
			nonImages = append(nonImages, e)
		}
	}

	sortEntriesByName(images)
	sortEntriesByName(subDirs)

	allProcessed := true
	for _, img := range images {
		if !strings.HasSuffix(strings.ToLower(img.Name()), ".jiff") {
			allProcessed = false
			break
		}
	}
	hasCover := false
	for _, img := range images {
		if img.Name() == "##cover.jiff" {
			hasCover = true
			break
		}
	}
	if allProcessed && hasCover && len(subDirs) == 0 {
		resp.Skipped = true
		return resp, nil
	}

	steps := map[string]interface{}{}

	if req.ExtraFilePath != "" && len(nonImages) > 0 {
		extraFull := filepath.Join(rootPath, req.ExtraFilePath)
		os.MkdirAll(extraFull, 0755)
		count := 0
		for _, ni := range nonImages {
			os.Rename(filepath.Join(rootPath, ni.Name()), filepath.Join(extraFull, ni.Name()))
			count++
		}
		steps["move_non_images"] = map[string]int{"count": count}
	}

	if len(subDirs) > 0 {
		flattenRoot(rootPath, subDirs)
		subDirs = nil
		entries, _ = os.ReadDir(rootPath)
		for _, e := range entries {
			if e.IsDir() {
				subDirs = append(subDirs, e)
			}
		}
		steps["flatten"] = map[string]interface{}{"processed": true}
	}

	coverSteps := map[string]interface{}{}
	for _, sd := range subDirs {
		subPath := filepath.Join(rootPath, sd.Name())
		subEntries, _ := os.ReadDir(subPath)
		var subImgs []os.DirEntry
		for _, se := range subEntries {
			if !se.IsDir() && isImageExt(se.Name()) {
				subImgs = append(subImgs, se)
			}
		}
		sortEntriesByName(subImgs)
		if len(subImgs) == 0 {
			continue
		}

		sourceImg := subImgs[0]
		srcPath := filepath.Join(subPath, sourceImg.Name())
		coverPath := filepath.Join(subPath, "##cover.jiff")

		if err := generateCover(client, srcPath, coverPath); err != nil {
			slog.Warn("cover generation failed", "path", srcPath, "error", err)
			continue
		}
		coverSteps[sd.Name()] = map[string]interface{}{
			"generated": true,
			"details": map[string]interface{}{
				"source": sourceImg.Name(),
				"cover":  "##cover.jiff",
				"width":  360,
				"height": 504,
			},
		}

		converted := 0
		skipped := 0
		convErrors := map[string]string{}
		for _, img := range subImgs {
			if img.Name() == "##cover.jiff" {
				continue
			}
			src := filepath.Join(subPath, img.Name())
			if strings.HasSuffix(strings.ToLower(img.Name()), ".jiff") {
				skipped++
				continue
			}
			baseName := strings.TrimSuffix(img.Name(), filepath.Ext(img.Name()))
			dst := filepath.Join(subPath, baseName+".jiff")
			if err := convertImage(client, src, dst, req.W, req.H, req.Fit, req.Q); err != nil {
				convErrors[img.Name()] = err.Error()
				continue
			}
			converted++
			os.Remove(src)
		}
		coverSteps[sd.Name()] = map[string]interface{}{
			"generated": true,
			"details": map[string]interface{}{
				"source": sourceImg.Name(),
				"cover":  "##cover.jiff",
				"width":  360,
				"height": 504,
			},
			"convert": map[string]interface{}{
				"processed": converted,
				"skipped":   skipped,
				"errors":    convErrors,
			},
		}
	}
	steps["covers"] = coverSteps

	resp.Steps = steps

	entries2, _ := os.ReadDir(rootPath)
	var allImgs2 []os.DirEntry
	for _, e := range entries2 {
		if !e.IsDir() && isImageExt(e.Name()) {
			allImgs2 = append(allImgs2, e)
		}
	}
	if len(subDirs) == 0 && len(allImgs2) > 0 {
		converted := 0
		skipped := 0
		for _, img := range allImgs2 {
			src := filepath.Join(rootPath, img.Name())
			if strings.HasSuffix(strings.ToLower(img.Name()), ".jiff") {
				skipped++
				continue
			}
			baseName := strings.TrimSuffix(img.Name(), filepath.Ext(img.Name()))
			dst := filepath.Join(rootPath, baseName+".jiff")
			if err := convertImage(client, src, dst, req.W, req.H, req.Fit, req.Q); err != nil {
				slog.Warn("convert failed", "src", src, "error", err)
				continue
			}
			converted++
			os.Remove(src)
		}
		steps["convert"] = map[string]interface{}{
			"processed": converted,
			"skipped":   skipped,
		}
	}

	return resp, nil
}

func flattenRoot(rootPath string, subDirs []os.DirEntry) {
	for _, sd := range subDirs {
		subPath := filepath.Join(rootPath, sd.Name())
		subEntries, err := os.ReadDir(subPath)
		if err != nil {
			continue
		}

		if len(subDirs) == 1 {
			for _, se := range subEntries {
				os.Rename(filepath.Join(subPath, se.Name()), filepath.Join(rootPath, se.Name()))
			}
			os.Remove(subPath)
			return
		}

		for _, se := range subEntries {
			if se.IsDir() {
				deepEntries, _ := os.ReadDir(filepath.Join(subPath, se.Name()))
				for _, de := range deepEntries {
					os.Rename(filepath.Join(subPath, se.Name(), de.Name()), filepath.Join(subPath, de.Name()))
				}
				os.Remove(filepath.Join(subPath, se.Name()))
			}
		}
	}
}

func generateCover(client *proxy.ImgproxyClient, srcPath, coverPath string) error {
	u := client.BuildProcessURL(
		"rs:fill:360:504",
		"g:sm",
		"q:80",
		"format:webp",
		"local:///"+srcPath,
	)
	return downloadToFile(u, coverPath)
}

func convertImage(client *proxy.ImgproxyClient, srcPath, dstPath string, w, h int, fit string, q int) error {
	rt := "fit"
	if fit == "cover" {
		rt = "fill"
	}
	resizeOpt := fmt.Sprintf("rs:%s:%d:%d", rt, w, h)
	qualityOpt := fmt.Sprintf("q:%d", q)
	u := client.BuildProcessURL(resizeOpt, "", qualityOpt, "format:webp", "local:///"+srcPath)
	return downloadToFile(u, dstPath)
}

func downloadToFile(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	os.MkdirAll(filepath.Dir(path), 0755)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	io.Copy(f, resp.Body)
	return nil
}

func sortEntriesByName(entries []os.DirEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return naturalCmp(entries[i].Name(), entries[j].Name()) < 0
	})
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
