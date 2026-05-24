# imgproxy_plus — 业务需求与实现文档

> **定位**：基于 imgproxy 官方镜像改造的纯 Go 项目，在保留 imgproxy 全部图片处理能力的前提下，扩展为面向个人 NAS / 家庭服务器 / 小团队的一站式文件与图片服务。缓存策略完全使用 imgproxy 原生能力（Cache-Control / ETag / Last-Modified），不引入任何 nginx 或其他反向代理缓存。

> **状态**：✅ 已实现，运行在生产环境中。此文档描述最终实现版本的设计与行为。

---

## 1. 产品定位

**imgproxy_plus** 基于 `ghcr.io/imgproxy/imgproxy:latest` 官方镜像改造，在 imgproxy 原生图片处理能力之上叠加文件服务层，形成单一容器的**一站式文件与图片服务**：

- **高性能图片处理** — imgproxy 原生能力：缩放/裁切/格式转换/水印/滤镜/智能裁切/自动格式，基于 libvips 极速处理
- **纯 Go 实现** — 无 nginx / OpenResty / Lua 依赖，std lib 为主 + `golang.org/x/text`（ZIP 编码检测）
- **文件存储与共享** — WebDAV + HTTP 双协议访问，支持 HTML 目录浏览
- **原始文件直出** — 任何数据目录下的文件可通过 HTTP GET 直接访问
- **压缩包透明浏览** — ZIP/CBZ 虚拟文件系统（无需解压即可浏览内部），GBK / Shift-JIS 编码自动检测
- **图片/漫画画廊** — 内置 SPA 画廊阅读器，支持目录和 ZIP 直接阅读
- **文件管理 API** — JSON RESTful API 覆盖增删改查
- **URL 前缀支持** — 可作为二级目录部署在其他网站之下（`PLUS_URL_PREFIX`）
- **认证与安全** — imgproxy 原生 URL 签名 + Basic Auth + IP 白名单

---

## 2. 系统架构

### 2.1 最终实现架构

```
                       ┌─────────────────────────────────────────────────┐
                       │            imgproxy_plus 容器                     │
                       │                                                  │
                       │  entrypoint.sh 启动                              │
                       │    ├── imgproxy (后台, 端口:8081)                 │
                       │    └── imgproxy_plus (前台, 端口:8080)            │
                       │                                                  │
 HTTP 请求 ──────────▶ │  ┌──────────────────────────────────────────┐   │
                       │  │       Go HTTP 请求分发器 (stdlib net/http) │   │
                       │  │                                            │   │
                       │  │  1. 剥离 URL 前缀 (PLUS_URL_PREFIX)       │   │
                       │  │  2. URL 路径前缀区分功能模块               │   │
                       │  │  3. 认证中间件 (Basic Auth + IP白名单)     │   │
                       │  └──────────┬───────────────────────────────┘   │
                       │             │                                    │
                       │    ┌────────┴────────┐                           │
                       │    ▼                 ▼                            │
                       │ ┌────────────┐  ┌──────────────────────────┐    │
                       │ │ imgproxy   │  │  Go 扩展业务层             │    │
                       │ │ localhost  │  │  (纯 stdlib, 零依赖)      │    │
                       │ │ :8081      │  │                            │    │
                       │ │            │  │ /img/* — 图片处理入口       │    │
                       │ │ /signature │  │ /zip/* — ZIP 虚拟文件系统  │    │
                       │ │ /opts/     │  │ /api/ls — 目录列表         │    │
                       │ │ /encsrc    │  │ /api/rm,move,mkdir,upload  │    │
                       │ │            │  │ /api/img — 实时图片处理     │    │
                       │ │ libvips    │  │ /api/batch-img — 批量处理   │    │
                       │ │ 引擎       │  │ /api/gallerize — 画廊整理   │    │
                       │ └─────┬──────┘  │ /          — WebDAV+文件    │    │
                       │       │         │ /or-gallery — 画廊SPA       │    │
                       │       │         │ /img-editor — 编辑器SPA     │    │
                       │       │         │ /img-sequence — 序列SPA    │    │
                       │       ▼         └────────┬─────────────────┘    │
                       │ ┌────────────────────────┴───────────────────┐  │
                       │ │         共享文件系统 /data                   │  │
                       │ │  (IMGPROXY_LOCAL_FILESYSTEM_ROOT=/)         │  │
                       │ └─────────────────────────────────────────────┘  │
                       │                                                  │
                       │  缓存: 完全由 imgproxy 原生 HTTP 头控制            │
                       │  (Cache-Control / ETag / Last-Modified)          │
                       └──────────────────────────────────────────────────┘
```

### 2.2 核心架构原则

| 原则 | 说明 |
|------|------|
| **纯 Go 实现** | Go stdlib 为主，仅添加 `golang.org/x/text` 用于 ZIP 内 GBK/Shift-JIS 编码检测 |
| **imgproxy 官方镜像为基座** | `FROM ghcr.io/imgproxy/imgproxy:latest`，保留全部原生能力 |
| **双进程 + 端口隔离** | imgproxy 在 `:8081` 内部运行，imgproxy_plus 扩展层在 `:8080` 对外服务 |
| **URL 前缀路由分离** | URL 前缀区分 imgproxy 原生和扩展业务路径；支持 `PLUS_URL_PREFIX` 二级目录部署 |
| **imgproxy 进程内调用** | 扩展层通过 `http://localhost:8081` 调用 imgproxy，零网络延迟 |
| **共享文件系统** | `IMGPROXY_LOCAL_FILESYSTEM_ROOT=/`，imgproxy 通过 `local://` 访问同一文件系统 |
| **原始文件直出** | GET 请求若匹配数据根目录下的真实文件，直接 `http.ServeFile` 返回 |
| **缓存完全由 imgproxy 管理** | 不引入任何额外缓存层，客户端根据 `Cache-Control` / `ETag` / `Last-Modified` 头自行管理 |

### 2.3 服务端口架构

| 进程 | 端口 | 环境变量 | 说明 |
|------|------|----------|------|
| **imgproxy_plus** | 8080 | `PLUS_HTTP_PORT` | 主服务端口，对外暴露 |
| **imgproxy** | 8081 | `IMGPROXY_BIND` | 内部端口，仅 localhost 调用 |

### 2.4 请求路由矩阵

| URL 路径 | 方法 | 路由目标 | 功能 |
|----------|------|---------|------|
| `/%signature/%opts/%encsrc` | GET | **imgproxy 原生** (透传) | imgproxy 图片处理 |
| `/img/<path>` | GET | **扩展层** → imgproxy | 简化图片处理（参数映射 + base64编码 + 签名） |
| `/zip/<path>` | GET | **扩展层** | ZIP 虚拟文件系统 — 直接访问内部文件 |
| `/api/ls/<path>` | GET | **扩展层** | 目录列表 JSON（含 ZIP/CBZ 内部浏览） |
| `/api/rm/<path>` | DELETE | **扩展层** | 删除文件/目录（递归） |
| `/api/move` | POST | **扩展层** | 移动/改名（JSON: `{from, to, overwrite}`） |
| `/api/mkdir/<path>` | POST | **扩展层** | 创建目录（`mkdir -p` 语义） |
| `/api/upload/<path>` | POST | **扩展层** | 上传文件（raw body） |
| `/api/img` | POST | **扩展层** → imgproxy | 实时图片处理（RAM Disk 桥接） |
| `/api/batch-img` | POST | **扩展层** → imgproxy | 批量图片处理（本地/远程双模式） |
| `/api/gallerize` | POST | **扩展层** → imgproxy | Gallery 目录整理 |
| `/` | 全方法 | **扩展层** | 导航页（GET）+ WebDAV（PROPFIND/MKCOL等） |
| `/<data_path>` | GET | **扩展层** | 原始文件直出（文件）/ HTML 目录浏览（目录） |
| `/or-gallery` | GET | **扩展层** 静态 | Gallery 画廊 SPA |
| `/img-editor` | GET | **扩展层** 静态 | 图片编辑器 SPA |
| `/img-sequence` | GET | **扩展层** 静态 | 图片序列编辑器 SPA |
| `/health` | GET | **imgproxy 原生** (代理) | imgproxy 健康检查 |
| `/metrics` | GET | **imgproxy 原生** (代理) | Prometheus 指标 |

---

## 3. imgproxy 原生能力（零改动直接继承）

### 3.1 图片处理引擎

**底层引擎**：libvips — 极低内存占用，极高处理速度。

**支持格式**：
- 输入：JPEG / PNG / GIF / WebP / AVIF / JPEG XL / TIFF / ICO / SVG / HEIC 等
- 输出：JPEG / PNG / GIF / WebP / AVIF / JPEG XL / ICO

**本地文件访问**：`local:///path/to/file`（三斜杠格式），配合 `IMGPROXY_LOCAL_FILESYSTEM_ROOT=/`

### 3.2 处理选项速览

| 类别 | 选项 | 缩写 | 说明 |
|------|------|------|------|
| 尺寸 | resize / width / height / dpr | rs / w / h / dpr | 缩放与尺寸 |
| 裁切 | crop / gravity / trim | c / g / t | 裁切与位置 |
| 旋转 | rotate / flip | rot / fl | 旋转翻转 |
| 滤镜 | blur / sharpen | bl / sh | 模糊锐化 |
| 质量 | quality / format | q / f | 输出质量与格式 |
| 控制 | skip_processing / raw | skp / raw | 跳过处理/原始模式 |
| 预设 | preset | pr | 预定义处理配置 |

### 3.3 URL 签名

```
/%signature/%processing_options/%encoded_source_url
```

- **签名**：`HMAC-SHA256(salt + path, key)` → URL-safe Base64
- **源 URL**：`local:///data/file.jpg` → URL-safe Base64 编码
- 可在 `/health` 或 `/metrics` 路径绕过签名

**本项目签名生成**（Go 实现）：
```go
mac := hmac.New(sha256.New, key)
mac.Write(salt)
mac.Write([]byte("/rs:fill:200:200/bG9jYWw6Ly8vL2RhdGEvMS5qcGVn"))
sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
```

---

## 4. 扩展功能实现

### 4.1 WebDAV 文件服务 + 原始文件直出

**实现点**：
- WebDAV 完整方法集：PROPFIND / MKCOL / COPY / MOVE / LOCK / UNLOCK / PUT / DELETE
- 浏览器访问目录时返回 HTML 浏览页面（含文件排序）
- `GET <data_path>` 若路径指向真实文件：直接 `http.ServeFile` 返回
- 若路径指向真实目录：由 WebDAV handler 生成 HTML 目录列表
- 路径不对应任何文件/目录时：回退到静态文件服务

**画廊阅读器的配合**：画廊 reader 用 `item.url`（数据路径）作为 img src → 浏览器发起 GET → 服务器直出文件

### 4.2 简化图片处理入口 `/img/<path>`

**URL 格式**：
```
GET /img/<data_path>?w=300&h=400&fit=cover&fmt=webp&q=80
```

**内部映射流程**：
1. 源文件路径 → `local:///<data_root>/<path>`
2. 源 URL → URL-safe Base64 编码
3. 查询参数 → imgproxy 处理选项（`rs:fill:300:400/q:80/format:webp`）
4. 构建路径：`/%sig/%opts/%encoded_source`
5. 签名：`HMAC-SHA256(salt + "/rs:fill:300:400/q:80/format:webp/base64source", key)`
6. fetch imgproxy `http://localhost:8081/%sig/%opts/%encoded_source`

**参数映射**：

| 应用层参数 | imgproxy 原生 | 说明 |
|-----------|--------------|------|
| `w=300` | `rs:fit:300:0` | 仅宽度 |
| `w=300&h=400` | `rs:fit:300:400` | 宽度+高度 |
| `fit=contain` | `resizing_type:fit` | 包含模式 |
| `fit=cover` | `resizing_type:fill` | 填充模式 |
| `fit=fill` | `resizing_type:force` | 强制尺寸 |
| `fmt=webp` | `format:webp` | 输出格式 |
| `q=80` | `quality:80` | 输出质量 |
| `crop=x,y,w,h` | `crop:w:h:x:y` | 裁切 |

**动态图保护**：检测 GIF/动态 WebP 文件头，若有处理参数则直传原图（避免帧丢失），标记 `X-Imgproxy: passthrough-animated`。

### 4.3 原始文件直出

**实现**（`smartRoute` 中）：
```go
dataPath := filepath.Join(cfg.DataRoot, r.URL.Path)
if info, err := os.Stat(dataPath); err == nil {
    if info.IsDir() {
        // HTML 目录列表
    }
    // 原始文件
    http.ServeFile(w, r, dataPath)
}
```

**意义**：画廊 reader 的 `img.src = "/completed/book/ch1/01.jpg"` 直接由服务器返回文件，无需经过 `/img/` 处理层。

### 4.4 实时图片处理 `POST /api/img`

**流程**：
1. 请求 body → 写入 RAM Disk `/mnt/ramdisk/.imgapi-tmp/<uuid>`
2. 构建 `local:///mnt/ramdisk/.imgapi-tmp/<uuid>` → Base64 编码
3. 调用 imgproxy `http://localhost:8081/...`
4. 返回结果 → 删除临时文件 → 零磁盘 I/O

### 4.5 ZipFS — ZIP 虚拟文件系统

**两种访问模式**：
1. **`/zip/<archive>/<internal>`** — HTTP 直接访问 ZIP 内部文件
2. **`/api/ls/<archive>/<internal>`** — ZIP 目录 JSON API（分页/排序）

**编码处理**：
- `archive/zip` 读取 ZIP 文件名
- `ft.ValidString` 检测是否 UTF-8
- 自动尝试 GBK / Shift-JIS 解码（优先 GBK）
- 使用 `golang.org/x/text/encoding` 做解码转换
- 通过 Han/假名/全角字符占比 ≥ 15% 判断解码正确性

**关键配置**：`ZIP_EXTS=zip,cbz`，`ZIPFS_TRANSPARENT=true`

### 4.6 URL 前缀支持 (`PLUS_URL_PREFIX`)

**设计目标**：服务可作为二级目录部署在其他网站下。

**实现机制**：
1. **配置**：`PLUS_URL_PREFIX=/imgproxy`
2. **前端适配**：HTML 页面动态注入 `<base href="/imgproxy/">` 标签
3. **后端适配**：启动时注册前缀映射，请求进入时 strip 前缀再分发
4. **相对路径**：前端 JS/CSS 引用改用相对路径（`libs/` 代替 `/libs/`）

**访问方式**：
```
http://host/imgproxy/               → 导航页
http://host/imgproxy/or-gallery     → 画廊
http://host/imgproxy/img/1.jpg?w=200 → 图片处理
http://host/imgproxy/api/ls/        → 目录列表
```

**`<base>` 注入机制**：
- 静态文件服务器在返回 `.html` 文件时，检测 `<head>` 标签并注入 `<base href="<prefix>/">`
- WebDAV HTML 目录列表同理
- 仅当 `PLUS_URL_PREFIX` 非空且不等于 `/` 时生效

### 4.7 其他 API

与原始设计一致，已全部实现：
- `GET /api/ls/<path>` — 目录 JSON API（分页、排序、ZIP 内浏览）
- `DELETE /api/rm/<path>` — 删除
- `POST /api/move` — 移动/改名
- `POST /api/mkdir/<path>` — 创建目录
- `POST /api/upload/<path>` — 上传文件
- `POST /api/batch-img` — 批量图片处理
- `POST /api/gallerize` — Gallery 目录整理

---

## 5. 前端 SPA

| 页面 | URL | 功能 |
|------|-----|------|
| **导航首页** | `/` | 功能入口导航卡片 |
| **Gallery 画廊** | `/or-gallery` | 封面卡片浏览 + 内置漫画阅读器，支持目录和 ZIP/CBZ |
| **图片编辑器** | `/img-editor` | 裁切/缩放/旋转/翻转 PDF 处理等 |
| **图片序列编辑器** | `/img-sequence` | 多图拖拽排序，导出 PDF/ZIP/CBZ |

**内置第三方库**（`html/libs/`）：
- JSZip 3.10.1 — ZIP 处理
- PDF.js 4.0 — PDF 渲染
- jsPDF 2.5.1 — PDF 生成

---

## 6. 配置系统

### 6.1 双层配置体系

| 层 | 变量前缀 | 说明 |
|----|---------|------|
| imgproxy 原生 | `IMGPROXY_*` | 所有 imgproxy 官方配置全部保留 |
| 扩展层 | `PLUS_*` / `AUTH_*` / `ZIP_*` | imgproxy_plus 扩展配置 |

### 6.2 扩展层配置参考

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PLUS_DATA_ROOT` | `/data` | 数据根目录 |
| `PLUS_RAMDISK_PATH` | `/mnt/ramdisk` | RAM Disk 路径 |
| `PLUS_HTTP_PORT` | `8080` | 扩展层 HTTP 端口 |
| `PLUS_URL_PREFIX` | `` | URL 前缀（二级目录部署用） |
| `PLUS_LOG_LEVEL` | `warn` | 日志级别 (debug/info/warn/error) |
| `AUTH_USER` | `` | Basic Auth 凭证 `user:pass` |
| `AUTH_IP_WHITELIST` | `` | 免认证 IP（逗号分隔，支持 CIDR） |
| `ZIP_EXTS` | `zip,cbz` | ZIP 兼容扩展名 |
| `ZIPFS_TRANSPARENT` | `true` | ZIP 透明浏览 |
| `API_PAGE_SIZE_MAX` | `200` | API 最大分页 |
| `FILEAPI_DISABLE` | `false` | 禁用文件管理 API |

### 6.3 imgproxy 核心配置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_BIND` | `:8081` | imgproxy 监听地址（内部端口） |
| `IMGPROXY_LOCAL_FILESYSTEM_ROOT` | `/` | 本地文件系统根 |
| `IMGPROXY_KEY` | — | URL 签名密钥（hex 编码，与 SALT 同时设置才签名） |
| `IMGPROXY_SALT` | — | URL 签名盐值（hex 编码） |
| `IMGPROXY_QUALITY` | `80` | 默认输出质量 |
| `IMGPROXY_AUTO_WEBP` | `false` | 自动 WebP |
| `IMGPROXY_LOG_LEVEL` | `warn` | 日志级别 |
| `IMGPROXY_MALLOC` | — | malloc 实现（推荐 `jemalloc`） |

---

## 7. 错误处理规范

### 7.1 统一 JSON 错误格式

```json
{
  "error": "<错误代码>",
  "message": "<可读描述>"
}
```

### 7.2 通用错误码

| HTTP 状态 | error 代码 | 适用场景 |
|-----------|-----------|---------|
| `400` | `bad_request` | 路径穿越、参数缺失、请求体为空 |
| `403` | `forbidden` | 路径超出根目录、功能被禁用 |
| `404` | `not_found` | 路径不存在、文件不存在 |
| `405` | `method_not_allowed` | HTTP 方法错误 |
| `409` | `conflict` | 目标已存在、路径类型冲突 |
| `415` | `unsupported_media_type` | 图片格式无法解码 |
| `500` | `internal` | 内部错误（文件系统、编码等） |
| `502` | `bad_gateway` | imgproxy 处理失败 |

---

## 8. Docker 部署

### 8.1 Dockerfile 结构

```dockerfile
# 阶段 1: Go 编译
FROM golang:1.22-alpine
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o imgproxy_plus .

# 阶段 2: 镜像
FROM ghcr.io/imgproxy/imgproxy:latest
COPY --from=builder imgproxy_plus /usr/local/bin/
COPY html/ /usr/local/bin/html/
COPY entrypoint.sh /usr/local/bin/
```

### 8.2 启动流程

```bash
entrypoint.sh
  ├── 加载 malloc 库 (jemalloc/tcmalloc)
  ├── 设置 IMGPROXY_BIND=:8081
  ├── 启动 imgproxy（后台进程）
  └── exec imgproxy_plus（主进程，监听 :8080）
```

### 8.3 docker-compose 示例

```yaml
services:
  imgproxy_plus:
    image: imgproxy_plus:latest
    ports:
      - "8082:8080"
    environment:
      - PLUS_DATA_ROOT=/data
      - IMGPROXY_BIND=:8081
      - IMGPROXY_LOCAL_FILESYSTEM_ROOT=/
      - IMGPROXY_KEY=<hex_key>
      - IMGPROXY_SALT=<hex_salt>
      - IMGPROXY_QUALITY=80
      - IMGPROXY_AUTO_WEBP=true
      - IMGPROXY_MALLOC=jemalloc
      # 可选：二级目录部署
      # - PLUS_URL_PREFIX=/imgproxy
    volumes:
      - /nas/data:/data:ro
      - ramdisk:/mnt/ramdisk
    restart: unless-stopped

volumes:
  ramdisk:
    driver: local
    driver_opts:
      type: tmpfs
      device: tmpfs
      o: "size=512m"
```

---

## 9. 与原架构对比

| 特性 | 原方案 (docker-openresty-tool) | imgproxy_plus |
|------|-------------------------------|-------------|
| 容器数 | 2（OpenResty + imgproxy） | 1 |
| 基础镜像 | OpenResty (Nginx+Lua) | imgproxy 官方 |
| 业务语言 | Lua | Go (stdlib为主) |
| 图片处理调用 | 跨容器 HTTP | 进程内 localhost |
| 依赖 | nginx + lua模块 + 外部库 | golang.org/x/text |
| 缓存策略 | nginx proxy_cache | imgproxy 原生 HTTP 头 |
| URL 前缀 | 不支持 | PLUS_URL_PREFIX |
| ZIP 编码 | 不支持 | GBK/Shift-JIS 自动检测 |
| 原始文件直出 | nginx try_files | Go smartRoute |
| 二进制大小 | N/A | ~9MB (静态链接) |
| 内存占用 | ~100MB+ | ~33MB |
