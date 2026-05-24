package static

import (
	"bytes"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var (
	htmlRoot string
	prefix   string
)

func Init(root, urlPrefix string) {
	htmlRoot = root
	prefix = urlPrefix
	slog.Debug("static files root", "path", htmlRoot, "prefix", prefix)
}

var spaRoutes = map[string]string{
	"/or-gallery":   "or-gallery.html",
	"/img-editor":   "img-editor.html",
	"/img-sequence": "img-sequence.html",
	"/":             "index.html",
}

func Handler() http.Handler {
	fs := http.FileServer(http.Dir(htmlRoot))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path

		if spaFile, ok := spaRoutes[p]; ok {
			filePath := filepath.Join(htmlRoot, spaFile)
			data, err := os.ReadFile(filePath)
			if err != nil {
				if p == "/" {
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					w.Write(injectBase([]byte(indexFallback)))
					return
				}
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", mimeByExt(spaFile))
			w.Write(injectBase(data))
			return
		}

		if strings.HasSuffix(p, ".html") {
			filePath := filepath.Join(htmlRoot, filepath.Clean(strings.TrimPrefix(p, "/")))
			data, err := os.ReadFile(filePath)
			if err == nil {
				w.Header().Set("Content-Type", mimeByExt(filepath.Ext(filePath)))
				w.Write(injectBase(data))
				return
			}
		}

		filePath := path.Join("/", strings.TrimPrefix(p, "/"))
		r.URL.Path = filePath
		fs.ServeHTTP(w, r)
	})
}

func injectBase(data []byte) []byte {
	if prefix == "" || prefix == "/" {
		return data
	}
	tag := []byte("<base href=\"" + prefix + "/\">\n")
	if bytes.Contains(data, tag) {
		return data
	}
	idx := bytes.Index(data, []byte("<head>"))
	if idx == -1 {
		idx = bytes.Index(data, []byte("<HEAD>"))
	}
	if idx == -1 {
		return data
	}
	ins := idx + len("<head>")
	result := make([]byte, 0, len(data)+len(tag))
	result = append(result, data[:ins]...)
	result = append(result, tag...)
	result = append(result, data[ins:]...)
	return result
}

func mimeByExt(name string) string {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

var indexFallback = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>imgproxy_plus</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0f1117;color:#e8eaf0;font:14px Inter,Noto Sans SC,-apple-system,sans-serif;min-height:100vh;display:flex;align-items:center;justify-content:center}
.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:20px;padding:40px;max-width:900px;width:100%}
.card{background:#1a1d27;border:1px solid #2e3347;border-radius:12px;padding:28px;transition:transform .2s,box-shadow .2s,border-color .2s;cursor:pointer;text-decoration:none;color:inherit}
.card:hover{transform:translateY(-4px);box-shadow:0 12px 40px rgba(108,138,255,.15);border-color:#6c8aff}
.card .icon{font-size:36px;margin-bottom:12px}
.card h2{font-size:16px;margin-bottom:8px;color:#6c8aff}
.card p{font-size:13px;color:#8b90a0;line-height:1.6}
</style>
</head>
<body>
<div class="cards">
<a class="card" href="./or-gallery"><div class="icon">🖼️</div><h2>Gallery 画廊</h2><p>浏览图片和漫画收藏，支持目录和ZIP/CBZ直接阅读，内置漫画阅读器。</p></a>
<a class="card" href="./img-editor"><div class="icon">✂️</div><h2>图片编辑器</h2><p>裁切、缩放、旋转、翻转图片，PDF处理、网格分割等工具。</p></a>
<a class="card" href="./img-sequence"><div class="icon">📑</div><h2>图片序列编辑器</h2><p>多张图片拖拽排序，导出为PDF、ZIP、CBZ格式。</p></a>
</div>
</body>
</html>`
