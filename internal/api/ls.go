package api

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"imgproxy_plus/internal/config"
	"imgproxy_plus/internal/ziputil"
)

type FileItem struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Size  int64  `json:"size"`
	Mtime string `json:"mtime"`
	Ctime string `json:"ctime,omitempty"`
}

type LsResponse struct {
	Path     string     `json:"path"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
	Total    int        `json:"total"`
	Sort     string     `json:"sort"`
	Order    string     `json:"order"`
	Items    []FileItem `json:"items"`
}

type sortConfig struct {
	sort  string
	order string
}

func HandleLs(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET allowed")
			return
		}

		reqPath := strings.TrimPrefix(r.URL.Path, "/api/ls")
		if reqPath == "" {
			reqPath = "/"
		}

		if strings.Contains(reqPath, "..") {
			writeError(w, http.StatusBadRequest, "bad_request", "path traversal detected")
			return
		}

		sc := sortConfig{
			sort:  r.URL.Query().Get("sort"),
			order: r.URL.Query().Get("order"),
		}
		if sc.sort == "" {
			sc.sort = "name"
		}
		if sc.order == "" {
			sc.order = "asc"
		}

		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		if pageSize < 1 || pageSize > cfg.APIPageSizeMax {
			pageSize = cfg.APIPageSizeMax
		}

		fullPath := filepath.Join(cfg.DataRoot, filepath.Clean(reqPath))
		if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(cfg.DataRoot)) {
			writeError(w, http.StatusForbidden, "forbidden", "path escapes data root")
			return
		}

		var isZipPath bool
		var zipFile, zipInner string
		dirPath := fullPath

		for seg := fullPath; seg != cfg.DataRoot && seg != "/"; seg = filepath.Dir(seg) {
			ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(seg), "."))
			if cfg.IsZipExt(ext) {
				isZipPath = true
				zipFile = seg
				zipInner = strings.TrimPrefix(fullPath, zipFile)
				zipInner = strings.TrimPrefix(zipInner, "/")
				zipInner = strings.TrimPrefix(zipInner, string(filepath.Separator))
				break
			}
		}

		if isZipPath {
			if _, err := os.Stat(zipFile); os.IsNotExist(err) {
				writeError(w, http.StatusNotFound, "not_found", "zip file not found")
				return
			}
			items, err := listZipDir(zipFile, zipInner, cfg)
			if err != nil {
				writeError(w, http.StatusNotFound, "not_found", err.Error())
				return
			}
			sorted := sortItems(items, sc)
			total := len(sorted)
			start := (page - 1) * pageSize
			if start > total {
				start = total
			}
			end := start + pageSize
			if end > total {
				end = total
			}
			writeJSON(w, http.StatusOK, LsResponse{
				Path: reqPath, Page: page, PageSize: pageSize,
				Total: total, Sort: sc.sort, Order: sc.order,
				Items: sorted[start:end],
			})
			return
		}

		info, err := os.Stat(dirPath)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "path not found")
			return
		}
		if !info.IsDir() {
			writeError(w, http.StatusNotFound, "not_found", "path is not a directory")
			return
		}

		entries, err := os.ReadDir(dirPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "failed to read directory")
			return
		}

		var items []FileItem
		for _, entry := range entries {
			ei, err := entry.Info()
			if err != nil {
				continue
			}
			ftype := "file"
			if ei.IsDir() {
				ftype = "dir"
			} else if cfg.IsZipExt(strings.TrimPrefix(filepath.Ext(ei.Name()), ".")) && cfg.ZipfsTransparent {
				ftype = "zip"
			}
			items = append(items, FileItem{
				Name:  ei.Name(),
				Type:  ftype,
				Size:  ei.Size(),
				Mtime: ei.ModTime().UTC().Format(time.RFC3339),
			})
		}

		sorted := sortItems(items, sc)
		total := len(sorted)
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}

		writeJSON(w, http.StatusOK, LsResponse{
			Path: reqPath, Page: page, PageSize: pageSize,
			Total: total, Sort: sc.sort, Order: sc.order,
			Items: sorted[start:end],
		})
	}
}

func listZipDir(zipPath, innerPath string, cfg *config.Config) ([]FileItem, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open zip: %w", err)
	}
	defer r.Close()

	seen := map[string]bool{}
	var items []FileItem
	innerPath = strings.Trim(innerPath, "/")
	if innerPath != "" {
		innerPath += "/"
	}

	for _, f := range r.File {
		name := ziputil.DecodeName(f)
		if !strings.HasPrefix(name, innerPath) {
			continue
		}
		rest := strings.TrimPrefix(name, innerPath)
		if rest == "" {
			continue
		}
		slash := strings.Index(rest, "/")
		var itemName string
		isDir := false
		if slash >= 0 {
			itemName = rest[:slash]
			isDir = true
		} else {
			itemName = rest
		}

		if seen[itemName] {
			continue
		}
		seen[itemName] = true

		ftype := "file"
		if isDir {
			ftype = "dir"
		}

		modTime := f.Modified
		if modTime.IsZero() {
			modTime = time.Time{}
		}

		size := int64(f.UncompressedSize64)
		if isDir {
			size = 0
		}

		items = append(items, FileItem{
			Name:  itemName,
			Type:  ftype,
			Size:  size,
			Mtime: modTime.UTC().Format(time.RFC3339),
		})
	}

	return items, nil
}

type typePriority struct {
	name string
	pri  int
}

func typePri(ftype string) int {
	switch ftype {
	case "dir":
		return 0
	case "zip":
		return 1
	case "file":
		return 2
	}
	return 3
}

func sortItems(items []FileItem, sc sortConfig) []FileItem {
	sorted := make([]FileItem, len(items))
	copy(sorted, items)

	sort.SliceStable(sorted, func(i, j int) bool {
		// __cover.jfif always first
		if sorted[i].Name == "__cover.jfif" && sorted[j].Name != "__cover.jfif" {
			return true
		}
		if sorted[i].Name != "__cover.jfif" && sorted[j].Name == "__cover.jfif" {
			return false
		}

		var cmp int
		switch sc.sort {
		case "size":
			cmp = int(sorted[i].Size - sorted[j].Size)
		case "mtime":
			cmp = strings.Compare(sorted[i].Mtime, sorted[j].Mtime)
		case "type":
			pi, pj := typePri(sorted[i].Type), typePri(sorted[j].Type)
			if pi != pj {
				cmp = pi - pj
			}
		default:
			cmp = naturalCmp(sorted[i].Name, sorted[j].Name)
		}
		if cmp == 0 && sc.sort != "name" {
			cmp = naturalCmp(sorted[i].Name, sorted[j].Name)
		}
		if sc.order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})

	return sorted
}

func naturalCmp(a, b string) int {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	re := func(r rune) bool { return r < '0' || r > '9' }
	fa := strings.FieldsFunc(la, re)
	fb := strings.FieldsFunc(lb, re)
	for i := 0; i < len(fa) && i < len(fb); i++ {
		na, _ := strconv.Atoi(fa[i])
		nb, _ := strconv.Atoi(fb[i])
		if na != nb {
			return na - nb
		}
		cmp := strings.Compare(fa[i], fb[i])
		if cmp != 0 {
			return cmp
		}
	}
	if len(fa) != len(fb) {
		return len(fa) - len(fb)
	}
	return strings.Compare(la, lb)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code, "message": msg})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func extractPath(r *http.Request, prefix string, cfg *config.Config) (string, string, error) {
	reqPath := strings.TrimPrefix(r.URL.Path, prefix)
	if reqPath == "" || reqPath == "/" {
		return "", "", fmt.Errorf("path empty")
	}
	if strings.Contains(reqPath, "..") {
		return "", "", fmt.Errorf("path traversal")
	}
	fullPath := filepath.Join(cfg.DataRoot, filepath.Clean(reqPath))
	if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(cfg.DataRoot)) {
		return "", "", fmt.Errorf("path escapes data root")
	}
	return reqPath, fullPath, nil
}

func sanitizePath(cfg *config.Config, reqPath string) (string, error) {
	if strings.Contains(reqPath, "..") {
		return "", fmt.Errorf("path traversal")
	}
	full := filepath.Join(cfg.DataRoot, filepath.Clean(reqPath))
	if !strings.HasPrefix(filepath.Clean(full), filepath.Clean(cfg.DataRoot)) {
		return "", fmt.Errorf("path escapes data root")
	}
	return full, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sortedDirEntries(path string) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		ii, _ := entries[i].Info()
		ji, _ := entries[j].Info()
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		ni := ""
		nj := ""
		if ii != nil {
			ni = ii.Name()
		}
		if ji != nil {
			nj = ji.Name()
		}
		return naturalCmp(ni, nj) < 0
	})
	return entries, nil
}
