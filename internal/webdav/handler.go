package webdav

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"imgproxy_plus/internal/config"
)

func Handler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		method := r.Method
		reqPath := r.URL.Path

		corsHeaders(w)

		if strings.Contains(reqPath, "..") {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		fullPath := filepath.Join(cfg.DataRoot, filepath.Clean(reqPath))
		if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(cfg.DataRoot)) {
			if reqPath != "/" && !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(cfg.DataRoot)) {
				fullPath = filepath.Join(cfg.DataRoot, strings.TrimPrefix(reqPath, "/"))
				if !strings.HasPrefix(filepath.Clean(fullPath), filepath.Clean(cfg.DataRoot)) {
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
			}
		}

		switch method {
		case "GET", "HEAD":
			info, err := os.Stat(fullPath)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			if info.IsDir() {
				serveDirListing(w, r, fullPath, reqPath, cfg)
			} else {
				if method == "HEAD" {
					w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
					return
				}
				http.ServeFile(w, r, fullPath)
			}

		case "PUT":
			if strings.HasSuffix(reqPath, "/") {
				os.MkdirAll(fullPath, 0755)
				w.WriteHeader(http.StatusCreated)
				return
			}
			os.MkdirAll(filepath.Dir(fullPath), 0755)
			f, err := os.Create(fullPath)
			if err != nil {
				http.Error(w, "create failed", http.StatusInternalServerError)
				return
			}
			defer f.Close()
			if r.Body != nil {
				ioCopy(f, r.Body)
			}
			w.WriteHeader(http.StatusCreated)

		case "DELETE":
			err := os.RemoveAll(fullPath)
			if err != nil {
				http.Error(w, "delete failed", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		case "MKCOL":
			os.MkdirAll(fullPath, 0755)
			w.WriteHeader(http.StatusCreated)

		case "COPY", "MOVE":
			dest := r.Header.Get("Destination")
			if dest == "" {
				http.Error(w, "Destination header required", http.StatusBadRequest)
				return
			}
			dest = stripHost(dest)
			destFull := filepath.Join(cfg.DataRoot, filepath.Clean(dest))

			overwrite := r.Header.Get("Overwrite") != "F"

			if method == "COPY" {
				if err := copyPath(fullPath, destFull); err != nil {
					if os.IsExist(err) && !overwrite {
						http.Error(w, "Precondition Failed", http.StatusPreconditionFailed)
						return
					}
					http.Error(w, "copy failed", http.StatusInternalServerError)
					return
				}
			} else {
				if err := os.Rename(fullPath, destFull); err != nil {
					http.Error(w, "move failed", http.StatusInternalServerError)
					return
				}
			}
			w.WriteHeader(http.StatusCreated)

		case "PROPFIND":
			handlePropfind(w, r, fullPath, reqPath)

		case "OPTIONS":
			w.Header().Set("Allow", "GET,HEAD,PUT,DELETE,MKCOL,COPY,MOVE,PROPFIND,OPTIONS,LOCK,UNLOCK")
			w.Header().Set("DAV", "1,2")
			w.WriteHeader(http.StatusOK)

		case "LOCK":
			handleLock(w, r, reqPath)

		case "UNLOCK":
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	}
}

func corsHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET,HEAD,PUT,DELETE,MKCOL,COPY,MOVE,PROPFIND,OPTIONS,LOCK,UNLOCK,POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Depth,Destination,Overwrite,Authorization")
	w.Header().Set("Access-Control-Expose-Headers", "DAV")
}

func stripHost(url string) string {
	if idx := strings.Index(url, "://"); idx >= 0 {
		url = url[idx+3:]
	}
	if idx := strings.Index(url, "/"); idx >= 0 {
		url = url[idx:]
	}
	return filepath.Clean(url)
}

func copyPath(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if srcInfo.IsDir() {
		os.MkdirAll(dst, srcInfo.Mode())
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyPath(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	os.MkdirAll(filepath.Dir(dst), 0755)
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, srcInfo.Mode())
}

func handlePropfind(w http.ResponseWriter, r *http.Request, fullPath, reqPath string) {
	depth := r.Header.Get("Depth")
	if depth == "" {
		depth = "infinity"
	}

	type propfindEntry struct {
		Path string
		Info os.FileInfo
	}

	var entries []propfindEntry

	info, err := os.Stat(fullPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	entries = append(entries, propfindEntry{Path: reqPath, Info: info})

	if info.IsDir() && (depth == "1" || depth == "infinity") {
		dirEntries, _ := os.ReadDir(fullPath)
		for _, de := range dirEntries {
			di, err := de.Info()
			if err != nil {
				continue
			}
			childPath := reqPath
			if !strings.HasSuffix(childPath, "/") {
				childPath += "/"
			}
			childPath += de.Name()
			if di.IsDir() {
				childPath += "/"
			}
			entries = append(entries, propfindEntry{Path: childPath, Info: di})
		}
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("DAV", "1,2")
	w.WriteHeader(http.StatusMultiStatus)

	fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?>
<D:multistatus xmlns:D="DAV:">`)
	for _, e := range entries {
		href := html.EscapeString(e.Path)
		if href == "" {
			href = "/"
		}
		isDir := e.Info.IsDir()
		resType := ""
		if isDir {
			resType = "<D:collection/>"
		}
		modTime := e.Info.ModTime().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
		size := ""
		if !isDir {
			size = fmt.Sprintf("<D:getcontentlength>%d</D:getcontentlength>", e.Info.Size())
		}
		fmt.Fprintf(w, `<D:response>
<D:href>%s</D:href>
<D:propstat>
<D:prop>
%s
<D:getlastmodified>%s</D:getlastmodified>
%s
<D:displayname>%s</D:displayname>
</D:prop>
<D:status>HTTP/1.1 200 OK</D:status>
</D:propstat>
</D:response>`, href, resType, modTime, size, html.EscapeString(filepath.Base(e.Path)))
	}
	fmt.Fprint(w, "</D:multistatus>")
}

func handleLock(w http.ResponseWriter, r *http.Request, reqPath string) {
	token := "opaquelocktoken:" + randomToken()
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?>
<D:prop xmlns:D="DAV:">
<D:lockdiscovery>
<D:activelock>
<D:locktype><D:write/></D:locktype>
<D:lockscope><D:exclusive/></D:lockscope>
<D:depth>infinity</D:depth>
<D:timeout>Second-3600</D:timeout>
<D:locktoken><D:href>%s</D:href></D:locktoken>
</D:activelock>
</D:lockdiscovery>
</D:prop>`, token)
}

func randomToken() string {
	b := make([]byte, 16)
	f, _ := os.Open("/dev/urandom")
	if f != nil {
		f.Read(b)
		f.Close()
	}
	for i := range b {
		b[i] = b[i]%16 + 'a'
	}
	return string(b)
}

func serveDirListing(w http.ResponseWriter, r *http.Request, fullPath, reqPath string, cfg *config.Config) {
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		http.Error(w, "readdir failed", http.StatusInternalServerError)
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var prefix string
	if cfg.URLPrefix != "" && cfg.URLPrefix != "/" {
		prefix = cfg.URLPrefix
	}
	fmt.Fprint(w, `<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Index of `+html.EscapeString(reqPath)+`</title>
<script src="`+prefix+`/libs/__or_preview.js"></script>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0f1117;color:#e8eaf0;font:14px Inter,Noto Sans SC,-apple-system,sans-serif;padding:20px;max-width:900px;margin:auto}
h1{font-size:18px;margin-bottom:16px;color:#6c8aff}
a{color:#e8eaf0;text-decoration:none}
a:hover{color:#6c8aff}
table{width:100%;border-collapse:collapse}
td{padding:8px 12px;border-bottom:1px solid #2e3347}
tr:hover{background:#1a1d27}
.size,.time{color:#8b90a0;font-size:13px;text-align:right;white-space:nowrap}
.name{white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:400px}
.name a{display:flex;align-items:center;gap:6px}
.icon{width:20px;text-align:center;flex-shrink:0}
</style></head><body>
<h1>📂 Index of `+html.EscapeString(reqPath)+`</h1>
<table>
<tr><th class="name">Name</th><th class="size">Size</th><th class="time">Modified</th></tr>`)

	if reqPath != "/" {
		parent := filepath.Dir(strings.TrimRight(reqPath, "/"))
		if parent == "." {
			parent = "/"
		}
		fmt.Fprintf(w, `<tr><td class="name"><a href="%s"><span class="icon">📁</span>../</a></td><td class="size">-</td><td class="time">-</td></tr>`, parent)
	}

	for _, e := range entries {
		name := e.Name()
		ei, _ := e.Info()
		size := "-"
		modTime := "-"
		icon := "📄"
		href := reqPath
		if !strings.HasSuffix(href, "/") {
			href += "/"
		}
		href += name
		if e.IsDir() {
			icon = "📁"
			href += "/"
		} else {
			if ei != nil {
				size = fmtSize(ei.Size())
				modTime = ei.ModTime().Format("2006-01-02 15:04")
			}
		}
		fmt.Fprintf(w, `<tr><td class="name"><a href="%s"><span class="icon">%s</span>%s</a></td><td class="size">%s</td><td class="time">%s</td></tr>`,
			href, icon, html.EscapeString(name), size, modTime)
	}

	fmt.Fprint(w, `</table></body></html>`)
}

func fmtSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	if size < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(size)/1024/1024)
	}
	return fmt.Sprintf("%.1f GB", float64(size)/1024/1024/1024)
}

func ioCopy(dst *os.File, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				return written, ew
			}
		}
		if er != nil {
			if er != io.EOF {
				return written, er
			}
			break
		}
	}
	return written, nil
}
