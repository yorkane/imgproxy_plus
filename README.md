# imgproxy_plus

**一站式文件与图片服务** — 基于 [imgproxy](https://imgproxy.net) 官方镜像，纯 Go 扩展层，单容器部署。

## 核心特性

- 🖼️ **高性能图片处理** — imgproxy + libvips，缩放/裁切/格式转换/水印一键完成
- 📂 **文件管理** — WebDAV + HTTP 双协议访问，HTML 目录浏览，原始文件直出
- 📦 **ZIP 透明浏览** — 无需解压直接浏览 ZIP/CBZ 内部（GBK/Shift-JIS 自动编码检测）
- 🗄️ **Gallery 自动归档** — 定时扫描目录，解压→分组→转换→打包 CBZ，并发转换
- 🎬 **视频浏览** — Gallery 内浏览/播放视频文件，支持 ZIP 内视频播放
- 🎨 **内置 SPA** — Gallery 漫画阅读器、图片编辑器、序列编辑器
- 🔒 **安全认证** — URL 签名 + Basic Auth + IP 白名单
- 🏷️ **URL 前缀** — 可作为二级目录部署在其他网站之下
- 🐳 **单容器** — 基于 imgproxy:latest，内存仅 ~33MB

## 快速开始

```bash
# 构建
docker build -t imgproxy_plus .

# 生成签名密钥
KEY=$(openssl rand -hex 32)
SALT=$(openssl rand -hex 32)

# 启动
docker run -d --name imgproxy_plus \
  -p 8082:8080 \
  -v /path/to/your/data:/data:ro \
  -e PLUS_DATA_ROOT=/data \
  -e IMGPROXY_BIND=:8081 \
  -e IMGPROXY_LOCAL_FILESYSTEM_ROOT=/ \
  -e IMGPROXY_KEY=$KEY \
  -e IMGPROXY_SALT=$SALT \
  imgproxy_plus
```

访问 `http://localhost:8082/` 即可使用。

## 架构

```
HTTP 请求 → imgproxy_plus (Go, :8080) ──┐
                 │                       │
    ┌────────────┼───────────────┐       │
    ▼            ▼               ▼       ▼
 /img/*    /api/ls/*    /or-gallery   imgproxy
 图片处理   文件管理     前端SPA      (libvips, :8081)
                 │
                 ▼
            /data (共享文件系统)
```

- **imgproxy_plus** — Go HTTP 分发器（主进程，8080）
- **imgproxy** — 图片处理引擎（子进程，8081，仅 localhost）
- 缓存完全由 imgproxy 原生 `Cache-Control` / `ETag` / `Last-Modified` 控制

## API 速览

| 端点 | 方法 | 说明 |
|------|------|------|
| `/img/<path>?w=200&h=200&fit=cover&fmt=webp` | GET | 图片处理 |
| `/api/ls/<path>` | GET | 目录列表（JSON，含 ZIP 内浏览） |
| `/api/rm/<path>` | DELETE | 删除文件 |
| `/api/move` | POST | 移动/改名 |
| `/api/mkdir/<path>` | POST | 创建目录 |
| `/api/upload/<path>` | POST | 上传文件 |
| `/api/img` | POST | 实时图片处理 |
| `/api/batch-img` | POST | 批量处理 |
| `/api/gallerize` | POST | Gallery 自动归档（type=v2） |
| `/api/archive-status` | GET | 查看 archive 处理进度和日志 |
| `/api/migrate-covers` | POST | 迁移旧封面到 `__cover.jfif` |
| `/zip/<archive>/<inner>` | GET | ZIP 内文件直出 |
| `/<data_path>` | GET | 原始文件直出 / HTML 目录浏览 |
| `/or-gallery` | GET | Gallery 画廊阅读器 |
| `/img-editor` | GET | 图片编辑器 |
| `/img-sequence` | GET | 图片序列编辑器 |

## 图片处理示例

```bash
# 裁剪为 200×200 缩略图
/img/photo.jpg?w=200&h=200&fit=cover

# 转换为 WebP q=80
/img/photo.jpg?w=800&h=600&fit=contain&fmt=webp&q=80

# 仅设置宽度，等比缩放
/img/photo.jpg?w=400

# 指定裁切区域
/img/photo.jpg?crop=100,100,300,200

# 直接获取原图（无处理参数时直传）
/img/photo.jpg
```

## 配置

### 扩展层

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PLUS_DATA_ROOT` | `/data` | 数据根目录 |
| `PLUS_HTTP_PORT` | `8080` | 服务端口 |
| `PLUS_URL_PREFIX` | `` | URL 前缀（二级目录部署用，如 `/imgproxy`） |
| `PLUS_LOG_LEVEL` | `warn` | 日志级别 |
| `AUTH_USER` | `` | Basic Auth `user:pass` |
| `AUTH_IP_WHITELIST` | `` | 免认证 IP |
| `ZIP_EXTS` | `zip,cbz` | ZIP 扩展名 |
| `FILEAPI_DISABLE` | `false` | 禁用文件管理 API |
| `GALLERY_AUTO_ENABLED` | `true` | 启用自动归档 |
| `GALLERY_SCAN_DIR` | `/data/aria2/data/completed` | 扫描源目录 |
| `GALLERY_ARCHIVE_DIR` | `/data/archived` | CBZ 输出目录 |
| `GALLERY_SCAN_INTERVAL` | `1800` | 扫描间隔（秒） |
| `GALLERY_ARCHIVE_FMT` | `webp` | 输出格式（webp/avif） |
| `GALLERY_ARCHIVE_W` | `2560` | 转换最大宽度 |
| `GALLERY_ARCHIVE_H` | `2560` | 转换最大高度 |
| `GALLERY_ARCHIVE_Q` | `90` | 转换质量 |
| `GALLERY_ARCHIVE_CONCURRENCY` | `0` | 并发转换数（0=CPU-2） |

### imgproxy

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_BIND` | `:8081` | 内部端口 |
| `IMGPROXY_LOCAL_FILESYSTEM_ROOT` | `/` | 本地文件根 |
| `IMGPROXY_KEY` | — | 签名密钥（hex） |
| `IMGPROXY_SALT` | — | 签名盐值（hex） |
| `IMGPROXY_QUALITY` | `80` | 默认质量 |
| `IMGPROXY_AUTO_WEBP` | `false` | 自动 WebP |
| `IMGPROXY_MALLOC` | — | jemalloc / tcmalloc |

完整配置见 [imgproxy 官方文档](https://docs.imgproxy.net)。

## 二级目录部署

设置 `PLUS_URL_PREFIX=/imgproxy`：所有路由自动挂载到 `/imgproxy/` 下，HTML 自动注入 `<base>` 标签。

```bash
docker run -e PLUS_URL_PREFIX=/imgproxy ...
# 访问: http://host/imgproxy/
# 访问: http://host/imgproxy/or-gallery
# 访问: http://host/imgproxy/img/file.jpg?w=200
```

## 技术栈

- **Go** 1.22, 纯 stdlib 为主
- `golang.org/x/text` — ZIP 内 GBK/Shift-JIS 编码检测
- **imgproxy** latest — libvips 图片处理引擎
- **前端** — 原生 JS SPA，内置 JSZip / PDF.js / jsPDF

## 项目结构

```
imgproxy_plus/
├── main.go                  # 入口，初始化配置和服务
├── Dockerfile               # 多阶段构建
├── entrypoint.sh            # 容器启动脚本（imgproxy + imgproxy_plus）
├── go.mod / go.sum
├── internal/
│   ├── api/                 # 文件管理 API (ls/rm/move/mkdir/upload/img/batch/gallerize/migrate)
│   ├── archive/              # Gallery 自动归档引擎
│   │   └── unpack/           # 多格式解压 (zip/tar/rar/7z/xz/pdf)
│   ├── auth/                # 认证中间件
│   ├── config/              # 环境变量配置
│   ├── img/                 # /img/ 图片处理 handler
│   ├── proxy/               # imgproxy HTTP 客户端（签名/URL构建/代理）
│   ├── router/              # HTTP 路由分发器
│   ├── static/              # 静态文件服务 + base 标签注入
│   ├── webdav/              # WebDAV handler + HTML 目录浏览
│   ├── zipfs/               # ZIP 虚拟文件系统
│   └── ziputil/             # ZIP 编码解码 (GBK/Shift-JIS)
├── html/                    # 前端 SPA (or-gallery/img-editor/img-sequence)
│   └── libs/                # 第三方库 (JSZip/PDF.js/jsPDF)
└── docs/
    └── req.md               # 详细设计文档
```

## License

MIT
