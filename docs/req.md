# imgproxy_plus — 业务需求与交互需求文档

> **定位**：基于 imgproxy 官方镜像改造的纯 Go 项目，在保留 imgproxy 全部图片处理能力的前提下，扩展为面向个人 NAS / 家庭服务器 / 小团队的一站式文件与图片服务。缓存策略完全使用 imgproxy 原生能力（Cache-Control / ETag / Last-Modified），不引入任何 nginx 或其他反向代理缓存。本文档描述语言无关的业务逻辑与交互规范，指导系统实现。

---

## 1. 产品定位

**imgproxy_plus** 基于 `ghcr.io/imgproxy/imgproxy:latest` 官方镜像改造，在 imgproxy 原生图片处理能力之上叠加文件服务层，形成单一容器的**一站式文件与图片服务**：

- **高性能图片处理** — imgproxy 原生能力：缩放/裁切/格式转换/水印/滤镜/智能裁切/自动格式，基于 libvips 极速处理
- **纯 Go 实现** — 无 nginx / OpenResty / Lua 依赖，缓存完全由 imgproxy 原生 Cache-Control / ETag / Last-Modified 控制
- **文件存储与共享** — WebDAV + HTTP 双协议访问
- **压缩包透明浏览** — ZIP/CBZ 虚拟文件系统（无需解压即可浏览内部）
- **图片/漫画画廊** — 内置 SPA 画廊阅读器
- **文件管理 API** — JSON RESTful API 覆盖增删改查
- **认证与安全** — imgproxy 原生 URL 签名 + Bearer Token + HTTP Basic Auth + IP 白名单
- **监控集成** — imgproxy 原生 Prometheus 指标

---

## 2. 系统架构

### 2.1 架构理念

**imgproxy 为核心，业务逻辑旁路注入**：

```
                        ┌─────────────────────────────────────────┐
                        │       imgproxy_plus 容器 (纯 Go)         │
                        │                                         │
  HTTP 请求 ──────────▶ │  ┌─────────────────────────────────┐   │
                        │  │       Go HTTP 请求分发器          │   │
                        │  │  (URL 路径前缀区分功能模块)      │   │
                        │  └──────────┬──────────────────────┘   │
                        │             │                           │
                        │    ┌────────┴────────┐                  │
                        │    ▼                 ▼                  │
                        │ ┌────────────┐  ┌──────────────────┐   │
                        │ │ imgproxy   │  │  Go 扩展业务层     │   │
                        │ │ 原生路由    │  │  (文件/API/ZipFS) │   │
                        │ │            │  │                    │   │
                        │ │ /signature │  │ /api/* — 文件管理  │   │
                        │ │ /opts/     │  │ /zip/* — ZIP 浏览  │   │
                        │ │ /source    │  │ /     — WebDAV     │   │
                        │ └─────┬──────┘  └────────┬─────────┘   │
                        │       │                   │              │
                        │       ▼                   ▼              │
                        │ ┌─────────────────────────────────────┐ │
                        │ │         libvips 图片处理引擎       │ │
                        │ └─────────────────────────────────────┘ │
                        │       │                   │              │
                        │       ▼                   ▼              │
                        │ ┌──────────┐   ┌───────────────┐        │
                        │ │本地文件系统│   │ RAM Disk      │        │
                        │ │(数据根目录)│   │ (tmpfs 临时)  │        │
                        │ └──────────┘   └───────────────┘        │
                        │                                         │
                        │  缓存: 完全由 imgproxy 原生 HTTP 头控制   │
                        │  (Cache-Control / ETag / Last-Modified) │
                        └─────────────────────────────────────────┘
```

### 2.2 核心架构原则

| 原则 | 说明 |
|------|------|
| **纯 Go 实现** | 项目为纯 Go 代码，不依赖 nginx / OpenResty / Lua 等外部组件 |
| **imgproxy 官方镜像为基座** | `FROM ghcr.io/imgproxy/imgproxy:latest`，保留全部原生能力和配置，不 fork 不重写 |
| **URL 前缀路由分离** | imgproxy 原生路径（签名URL）与扩展业务路径（`/api/`、`/zip/`、`/` WebDAV）通过前缀区分，互不干扰 |
| **imgproxy 进程内调用** | 扩展业务层需要图片处理时，通过 localhost HTTP 调用 imgproxy 原生 API，享受同一进程的零网络延迟 |
| **共享文件系统** | imgproxy 和扩展业务层共享同一文件系统视图，通过 `IMGPROXY_LOCAL_FILESYSTEM_ROOT` 统一访问 |
| **RAM Disk 桥接** | POST API 临时文件写入 tmpfs 内存文件系统，imgproxy 通过 `local://` URL 读取，零磁盘 I/O |
| **缓存完全由 imgproxy 原生管理** | 图片缓存由 imgproxy 原生的 `Cache-Control` / `ETag` / `Last-Modified` 头控制，不引入任何额外的缓存层（无 nginx proxy_cache 等） |

### 2.3 请求路由矩阵

| URL 路径 | 方法 | 路由目标 | 功能 |
|----------|------|---------|------|
| `/%signature/%opts/%source` | GET | **imgproxy 原生** | imgproxy 图片处理（签名/非签名） |
| `/health` | GET | **imgproxy 原生** | imgproxy 健康检查 |
| `/metrics` | GET | **imgproxy 原生** | Prometheus 指标（若启用） |
| `/img/<path>` | GET | **扩展层** → imgproxy | 简化图片处理入口（应用层参数 → imgproxy URL） |
| `/zip/<path>` | GET | **扩展层** | ZIP 虚拟文件系统 — HTTP 浏览 |
| `/api/ls/<path>` | GET | **扩展层** | 目录列表 JSON API（含 ZIP 内部浏览） |
| `/api/rm/<path>` | DELETE | **扩展层** | 删除文件/目录 |
| `/api/move` | POST | **扩展层** | 移动/改名 |
| `/api/mkdir/<path>` | POST | **扩展层** | 新建目录 |
| `/api/upload/<path>` | POST | **扩展层** | 上传文件 |
| `/api/img` | POST | **扩展层** → imgproxy | 实时图片处理（POST 二进制 → RAM Disk → imgproxy） |
| `/api/batch-img` | POST | **扩展层** → imgproxy | 批量图片处理（本地/远程双模式） |
| `/api/gallerize` | POST | **扩展层** → imgproxy | Gallery 目录整理（封面生成 + 批量转换） |
| `/` | WebDAV 全方法 | **扩展层** | 文件服务（WebDAV + 静态文件 + 目录浏览） |
| `/or-gallery` | GET | **扩展层** 静态 | Gallery 画廊浏览器 / 漫画阅读器 (SPA) |
| `/img-editor` | GET | **扩展层** 静态 | 图片编辑器 (SPA) |

---

## 3. imgproxy 原生能力（零改动直接继承）

> 以下为 imgproxy 官方镜像自带能力，imgproxy_plus 完整保留，无需额外开发。

### 3.1 图片处理核心

**底层引擎**：libvips — 极低内存占用，极高处理速度。

**支持格式**：
- 输入：JPEG / PNG / GIF / WebP / AVIF / JPEG XL / TIFF / ICO / SVG / HEIC 等
- 输出：JPEG / PNG / GIF / WebP / AVIF / JPEG XL / ICO

**处理选项**（URL 段式参数）：

| 类别 | 选项 | 缩写 | 说明 |
|------|------|------|------|
| 尺寸与缩放 | resize / width / height / dpr / zoom / enlarge / extend | rs / w / h / dpr / z / el / ex |
| 裁切与定位 | crop / gravity / trim / padding | c / g / t / pd |
| 旋转与翻转 | auto_rotate / rotate / flip | ar / rot / fl |
| 滤镜与特效 | blur / sharpen / pixelate | bl / sh / pix |
| 水印 | watermark | wm |
| 元数据与输出 | quality / format / max_bytes / strip_metadata / cachebuster | q / f / mb / sm / cb |
| 跳过/原始 | skip_processing / raw | skp / raw |
| 预设 | preset | pr |

**智能裁切重力**：`g:sm`（智能）、`g:ce`（居中）、`g:no:0.2:0.3`（坐标偏移）等。

### 3.2 URL 结构

```
http://host/%签名/%处理选项组/%源图片URL@%扩展名
```

**签名模式**：
- `IMGPROXY_KEY` + `IMGPROXY_SALT` → HMAC-SHA256 签名，防滥用
- 签名位置可用 `insecure` 或 `_` 占位（测试模式）

**源 URL 编码**：
- Base64（推荐）：`/aHR0cDovL2V4YW1w/bGUuY29tL2ltYWdl.jpg`
- 纯文本：`/plain/http://example.com/image.jpg@png`（需百分号编码特殊字符）

**完整示例**：
```
/AfrOrF3gWeDA6VOlDG4TzxMv39O7MXnF4CXpKUwGqRM/rs:fill:300:400/g:sm/aHR0cDovL2V4YW1w/bGUuY29tL2ltYWdl/anBn.png
```

### 3.3 图像源

| 源类型 | URL 格式 | 配置 |
|--------|---------|------|
| HTTP/HTTPS | 直接 URL（默认） | — |
| 本地文件 | `local:///path/to/file` | `IMGPROXY_LOCAL_FILESYSTEM_ROOT=/` |
| Amazon S3 | `s3://bucket/key` | `IMGPROXY_USE_S3=true` + 区域/端点 |
| Google Cloud | `gs://bucket/key` | `IMGPROXY_USE_GCS=true` + 密钥 |
| Azure Blob | `abs://container/key` | `IMGPROXY_USE_ABS=true` + 账号/密钥 |
| OpenStack Swift | `swift://container/key` | `IMGPROXY_USE_SWIFT=true` |

> **关键**：`local://` 使用**三斜杠**格式 `local:///path`，否则首段被解析为 hostname 导致 404。

### 3.4 安全能力

| 能力 | 配置 | 说明 |
|------|------|------|
| URL 签名 | `IMGPROXY_KEY` + `IMGPROXY_SALT` | HMAC-SHA256 签名，防滥用 |
| Bearer Token | `IMGPROXY_SECRET` | `Authorization: Bearer <token>` 认证 |
| 源白名单 | `IMGPROXY_ALLOWED_SOURCES` | 支持通配符 `*`，限制图片来源 |
| 尺寸限制 | `IMGPROXY_MAX_SRC_RESOLUTION` / `MAX_SRC_FILE_SIZE` | 防图片炸弹 |
| 结果限制 | `IMGPROXY_MAX_RESULT_DIMENSION` | 输出最大边长 |
| SVG 净化 | `IMGPROXY_SANITIZE_SVG=true` | 清除 SVG 脚本，防 XSS |
| 重定向限制 | `IMGPROXY_MAX_REDIRECTS` | 防重定向循环 |
| 回环禁止 | `IMGPROXY_ALLOW_LOOPBACK_SOURCE_ADDRESSES=false` | 禁止回环地址 |

### 3.5 缓存控制

imgproxy OSS 版不提供内置响应缓存，通过 HTTP 头指导客户端缓存行为：
- `Cache-Control: max-age=<TTL>`（`IMGPROXY_TTL`，默认 1 年）
- `ETag`（`IMGPROXY_USE_ETAG=true`）
- `Last-Modified`（`IMGPROXY_USE_LAST_MODIFIED=true`）
- `Cache-Control` 透传（`IMGPROXY_CACHE_CONTROL_PASSTHROUGH=true`）

**本项目的缓存策略**：完全依赖 imgproxy 原生的缓存控制头，不在应用层引入任何额外缓存机制（不使用 nginx proxy_cache 等）。客户端/浏览器根据 `Cache-Control` / `ETag` / `Last-Modified` 头自行决定缓存行为。

### 3.6 监控

- **Prometheus**：`IMGPROXY_PROMETHEUS_BIND=:9090` → `/metrics`
- **Datadog**：`IMGPROXY_DATADOG_ENABLE=true`
- **错误报告**：Bugsnag / Honeybadger / Sentry / Airbrake

### 3.7 预设系统

```bash
IMGPROXY_PRESETS=default=resizing_type:fill/enlarge:1,thumbnail=rs:fill:300:400/q:80
IMGPROXY_ONLY_PRESETS=true  # 仅允许预设，禁止自定义选项
```

### 3.8 自动格式

| 变量 | 说明 |
|------|------|
| `IMGPROXY_AUTO_WEBP` | 根据 Accept 头自动返回 WebP |
| `IMGPROXY_AUTO_AVIF` | 根据 Accept 头自动返回 AVIF |
| `IMGPROXY_AUTO_JXL` | 根据 Accept 头自动返回 JPEG XL |
| `IMGPROXY_ENFORCE_WEBP` | 强制 WebP |
| `IMGPROXY_PREFERRED_FORMATS` | 首选格式列表 |

### 3.9 水印

| 变量 | 说明 |
|------|------|
| `IMGPROXY_WATERMARK_DATA` | Base64 编码水印数据 |
| `IMGPROXY_WATERMARK_PATH` | 本地水印文件路径 |
| `IMGPROXY_WATERMARK_URL` | 水印图片 URL |
| `IMGPROXY_WATERMARK_OPACITY` | 基础不透明度 |

---

## 4. 扩展功能需求

> 以下为 imgproxy_plus 在 imgproxy 原生能力之上新增的业务功能。

### 4.1 WebDAV 文件服务

**需求**：提供完整的 WebDAV 协议支持，兼容 macOS Finder、Cyberduck、davfs2 等客户端。

**功能要求**：
- 支持 WebDAV 完整方法集：PROPFIND / MKCOL / COPY / MOVE / LOCK / UNLOCK / PUT / DELETE
- Microsoft 客户端兼容性修复（特殊请求头/行为处理）
- 数据根目录通过配置变量指定（默认 `/data`）
- CORS 头支持所有 WebDAV 方法
- 提供美观的 HTML 目录浏览（当客户端为浏览器时）

**与 imgproxy 的协同**：
- WebDAV 根目录即 `IMGPROXY_LOCAL_FILESYSTEM_ROOT`，WebDAV 上传的文件立即可通过 imgproxy `local://` URL 处理
- 无需双容器共享卷，单容器内直接访问同一文件系统

### 4.2 简化图片处理入口 `/img/<path>`

**需求**：在 imgproxy 原生 URL 语法之上，提供对人类更友好的简化图片处理入口。

**设计**：
```
GET /img/<path>?w=300&h=400&fit=cover&fmt=webp&q=80
```

**参数映射**：

| 应用层参数 | imgproxy 原生参数 | 映射规则 |
|-----------|------------------|---------|
| `w=300` | `rs:*:300:*` 或 `w:300` | 宽度 |
| `h=400` | `rs:*:*:400` 或 `h:400` | 高度 |
| `fit=contain` | `resizing_type:fit` | contain→fit |
| `fit=cover` | `resizing_type:fill` | cover→fill |
| `fit=fill` | `resizing_type:fill` + 强制尺寸 | 不保持比例 |
| `fit=scale` | `resizing_type:fit` + 仅 w | 等比缩放 |
| `fmt=webp` | `format:webp` | 输出格式 |
| `q=80` | `quality:80` | 输出质量 |
| `crop=x,y,w,h` | `crop:w:h:x:y` | 裁切参数格式转换 |

**实现方式**：
1. 扩展层解析应用层参数
2. 构建等效的 imgproxy 签名 URL（若启用签名）
3. 内部 HTTP 重定向或直接调用 imgproxy 处理
4. 源文件通过 `local:///data/path` 传递给 imgproxy

**缓存策略**：
- `/img/` 入口的缓存完全依赖 imgproxy 返回的 `Cache-Control` / `ETag` / `Last-Modified` 头
- 不引入任何额外缓存层（无 nginx proxy_cache 等）
- 客户端/浏览器根据 imgproxy 的缓存头自行管理缓存

**动态图保护**：自动检测动态 WebP/GIF（文件头扫描），检测到后直接返回原始字节，避免动画帧丢失。响应头标记 `X-Imgproxy: passthrough-animated`。

### 4.3 实时图片处理 API `POST /api/img`

**需求**：将图片二进制 POST 到此接口，服务端完成处理后返回结果，专为高吞吐实时场景设计。

**端点**：`POST /api/img`

**请求**：
- Content-Type: 任意图片 MIME 类型
- Body: 原始图片二进制字节（无需 multipart）
- 查询参数同 4.2 节处理参数

**RAM Disk 桥接流程**：
1. 扩展层将请求体写入 RAM Disk 临时目录（tmpfs）
2. 构建 imgproxy `local:///mnt/ramdisk/.imgapi-tmp/<uuid>` URL
3. 内部调用 imgproxy 处理
4. 返回处理结果
5. 立即删除临时文件
6. **零磁盘 I/O**，纯内存操作

**响应**：
- 成功：处理后的图片二进制 + 对应 MIME 类型
- 响应头 `X-Imgproxy`：`processed`（已处理）或 `passthrough`（无参数直通）或 `passthrough-animated`（动态图保护）
- 响应头 `Cache-Control` / `ETag` / `Last-Modified`：透传 imgproxy 原生缓存头

**错误码**：

| HTTP 状态 | error 代码 | 触发条件 |
|-----------|-----------|----------|
| `400` | `bad_request` | 请求 body 为空 |
| `405` | `method_not_allowed` | 非 POST 方法 |
| `415` | `unsupported_media_type` | 图片格式无法解码 |
| `502` | `bad_gateway` | imgproxy 处理失败 |
| `500` | `internal` | 图片编码失败 |

### 4.4 ZipFS — ZIP 虚拟文件系统

**需求**：将 ZIP/CBZ 等压缩包当作只读目录访问，无需解压。

**两种访问模式**：
1. **HTTP 直接访问**：`/zip/<path>.zip/<inner_path>` — 永远可用
2. **WebDAV 透明接管**：WebDAV 客户端访问含 ZIP 路径时，自动返回 ZIP 内部结构 — 受 `ZIPFS_TRANSPARENT` 开关控制

**与 imgproxy 的协同**：
- ZIP 内部图片可通过 `/img/` 入口处理：先由 ZipFS 提取，再调用 imgproxy
- 或构建 imgproxy 源 URL 指向 WebDAV 提供的 ZIP 内部文件

**关键配置**：
- `ZIP_EXTS`：支持的扩展名列表（默认 `zip,cbz`），大小写不敏感
- `ZIPFS_TRANSPARENT`：WebDAV 透明接管开关（默认 `true`）

**交互约束**：
- ZIP 内部文件的 mtime/ctime 信息可能不可用（取决于 ZIP 库能力），此时返回空字符串

### 4.5 目录 JSON API

**需求**：提供程序化目录浏览接口，支持分页和排序，同时支持普通目录和 ZIP 内部结构。

**端点**：`GET /api/ls/<path>`

**关键特性**：
- 分页：`page` + `page_size`（最大 `API_PAGE_SIZE_MAX`，默认 200）
- 排序：`sort`(name/size/mtime/ctime/type) + `order`(asc/desc)
- ZIP 透明浏览：路径中包含 ZIP 扩展名时自动切换到 ZIP 内部浏览
- 缓存：无应用层缓存，目录列表实时反映文件系统状态

**`<path>` 寻址模式**：

| 模式 | 示例 | 说明 |
|------|------|------|
| 普通目录 | `/api/ls/archives` | 列出文件系统目录的直接子项 |
| ZIP 内部路径 | `/api/ls/archives/book.cbz` | 列出 ZIP 文件根目录的内容 |
| ZIP 子目录 | `/api/ls/archives/book.cbz/chapter1` | 列出 ZIP 内部子目录的内容 |

**成功响应 `200 OK`**：

```json
{
  "path":      "/archives",
  "page":      1,
  "page_size": 50,
  "total":     3,
  "sort":      "name",
  "order":     "asc",
  "items": [
    {
      "name":  "test_assets.cbz",
      "type":  "zip",
      "size":  3032,
      "mtime": "2026-03-14T12:01:10Z",
      "ctime": "2026-03-14T12:01:10Z"
    },
    {
      "name":  "subdir",
      "type":  "dir",
      "size":  128,
      "mtime": "2026-03-14T11:00:00Z",
      "ctime": "2026-03-14T11:00:00Z"
    },
    {
      "name":  "readme.txt",
      "type":  "file",
      "size":  512,
      "mtime": "2026-03-01T08:00:00Z",
      "ctime": "2026-03-01T08:00:00Z"
    }
  ]
}
```

**`type` 字段值**：

| 值 | 含义 |
|----|------|
| `dir` | 普通文件系统目录或 ZIP 内部虚拟子目录 |
| `file` | 普通文件或 ZIP 内部普通文件 |
| `zip` | ZIP 兼容压缩包（且 `ZIPFS_TRANSPARENT=true`）。关闭后返回 `"file"` |

**排序规则**：

| `sort` 值 | 排序依据 | 相同时次要排序 |
|-----------|----------|---------------|
| `name`（默认）| 文件名（大小写不敏感） | — |
| `size` | 文件大小（字节） | 名称升序 |
| `mtime` | 最后修改时间 | 名称升序 |
| `ctime` | 元数据变更时间 | 名称升序 |
| `type` | 类型优先级：`dir` < `zip` < `file` | 名称升序 |

**错误响应**：

| HTTP 状态 | error 代码 | 触发条件 |
|-----------|-----------|----------|
| `400` | `bad_request` | 路径包含 `..`（路径穿越检测） |
| `404` | `not_found` | 路径不存在，或路径是普通文件（非目录且非 ZIP） |
| `500` | `internal` | 无法打开目录或 ZIP 文件 |

### 4.6 文件管理 API

**需求**：提供文件增删改操作的 RESTful API。

| 端点 | 方法 | 功能 |
|------|------|------|
| `/api/rm/<path>` | DELETE | 删除（递归，等同 `rm -rf`） |
| `/api/move` | POST | 移动/改名（JSON body: `{from, to, overwrite}`) |
| `/api/mkdir/<path>` | POST | 创建目录（`mkdir -p` 语义，幂等） |
| `/api/upload/<path>` | POST | 上传文件（raw body，自动创建父目录，已存在则覆盖） |

**`DELETE /api/rm/<path>`**：
- 成功响应：`{"ok": true}`
- 目录递归删除

**`POST /api/move`**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `from` | string | ✅ | 源路径 |
| `to` | string | ✅ | 目标路径 |
| `overwrite` | bool | — | `true` 时允许覆盖已存在的目标（默认 `false`） |

- 目标路径父目录若不存在，自动创建

**`POST /api/mkdir/<path>`**：
- 创建目录，自动创建所有缺失的父目录
- 目标路径已是目录时，幂等返回 200

**`POST /api/upload/<path>`**：
- 请求体为原始文件字节（非 multipart）
- 父目录不存在时自动创建
- 已存在则覆盖
- 成功响应：`{"ok": true, "size": 1024}`

**文件管理 API 错误码**：

| HTTP 状态 | error 代码 | 触发条件 |
|-----------|-----------|----------|
| `400` | `bad_request` | 路径包含 `..`；请求体为空；JSON 字段缺失；上传路径以 `/` 结尾 |
| `403` | `forbidden` | 尝试删除根目录本身；路径逃出根目录范围；功能被禁用 |
| `404` | `not_found` | 源路径不存在 |
| `405` | `method_not_allowed` | 错误的 HTTP 方法 |
| `409` | `conflict` | 目标已存在且未设 `overwrite:true`；`mkdir` 时路径存在但是文件 |
| `500` | `internal` | 文件系统操作失败 |

**安全规则**：
- 所有 API 路径检测 `..`（返回 400）
- 所有路径操作验证最终路径在数据根目录范围内（403）
- 可通过 `FILEAPI_DISABLE=true` 一键禁用所有文件管理端点

### 4.7 批量图片处理 API

**需求**：对服务器本地文件或目录中的图片进行批量处理，支持本地模式和远程模式。

**端点**：`POST /api/batch-img`

**请求体（JSON）**：

必填字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `path` | string | 本地文件或目录路径（相对于数据根目录或绝对路径） |

图片处理参数：同 4.2 节处理参数（`w`, `h`, `fit`, `crop`, `fmt`, `q`, `ignore_exts`）

输出控制：

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `out_suffix` | string | — | 文件名后、扩展名前插入后缀（如 `-thumb` → `photo-thumb.jpg`） |
| `out_dir` | string | — | 输出写到此目录（保留文件名）；不存在时自动创建 |
| `overwrite` | bool | `true` | 输出已存在时是否覆盖；`false` 时跳过 |
| `recursive` | bool | `false` | 是否递归处理子目录 |

处理模式：

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `mode` | string | `local` | `local` — 通过本地 imgproxy 处理；`remote` — 转发给远端 `/api/img` |

远程模式参数（`mode=remote` 时生效）：

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `remote_url` | string | **必填** | 远端 `/api/img` 完整 URL |
| `concurrency` | int 1-64 | `4` | 同时向远端发送的并发请求数 |
| `connect_timeout_ms` | int | `5000` | TCP 连接超时（毫秒） |
| `send_timeout_ms` | int | `30000` | 发送数据超时（毫秒） |
| `recv_timeout_ms` | int | `60000` | 接收结果超时（毫秒） |

> `out_suffix` 和 `out_dir` 均不指定时，原地覆盖源文件。

**本地模式实现**：
1. 扫描目标路径下所有图片文件
2. 为每个图片构建 imgproxy `local://` URL
3. 调用本地 imgproxy 处理
4. 将结果写入输出路径

**成功响应 `200 OK`**：

```json
{
  "ok":      true,
  "total":   42,
  "done":    41,
  "skipped": 1,
  "errors":  [],
  "results": [
    {
      "src":      "/data/photos/a.jpg",
      "dst":      "/data/thumbs/a.jpg",
      "size_in":  2048000,
      "size_out": 184320,
      "ms":       38
    }
  ]
}
```

### 4.8 Gallery 整理 API

**需求**：自动化整理图片/漫画收藏目录结构，生成封面缩略图，批量转换图片格式。

**端点**：`POST /api/gallerize`

**请求参数**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `path` | string | ✅ | 目标目录路径（相对于数据根目录） |
| `type` | string | ✅ | 处理类型：`"v1"` 或 `"v2"`；其他值返回 403 |
| `extra_file_path` | string | — | 非图片文件的目标目录 |
| `w` | int | — | 图片转换目标宽度（默认 `2560`） |
| `h` | int | — | 图片转换目标高度（默认 `2560`） |
| `fit` | string | — | 缩放模式（默认 `contain`） |
| `q` | int | — | 输出质量 1-100（默认 `90`） |

**Type `v1` 处理步骤**：

1. **幂等跳过检测** — 若目录中所有可处理图片都已是 `.jiff` 且存在 `##cover.jiff`，直接返回 `{"ok":true,"skipped":true}`
2. **移动非图片文件** — 将根目录中非图片文件移动到 `extra_file_path`
3. **扁平化目录结构** — 保留一层子目录，二级及更深层上移
4. **单 gallery 提升** — 若仅一个子目录，将其内容提升至根目录
5. **目录名校验** — Windows 兼容性（≤38字符，无非法字符 `<>:"/\|?*`，非保留名，不以空格或 `.` 结尾），失败返回 400
6. **生成封面 `##cover.jiff`** — 取排序第一张图片，调用 imgproxy 生成 360×504 cover fit、WebP、q=80 的封面
7. **批量转换图片** — 将所有非 `.jiff` 图片调用 imgproxy 转换为 `.jiff`（WebP 内容，扩展名重命名），默认 2560×2560 contain, q=90

**`.jiff` 格式说明**：WebP 图片重命名扩展名，内容与 `.webp` 完全一致，用于通过扩展名快速区分已处理/未处理文件。

**响应示例**：

```json
{
  "ok": true,
  "path": "/comics/my-collection",
  "type": "v1",
  "steps": {
    "move_non_images": {"count": 2},
    "flatten": {"dirs": ["gallery_a", "gallery_b"]},
    "promote": {"performed": false},
    "covers": {
      "gallery_a": {"generated": true, "details": {"source":"img1.png","cover":"##cover.jiff","width":360,"height":504}},
      "gallery_b": {"generated": true, "details": {"source":"img3.jpeg","cover":"##cover.jiff","width":360,"height":504}}
    },
    "convert": {"processed": 3, "skipped": 0, "errors": {}}
  }
}
```

### 4.9 前端 SPA

**Gallery 画廊浏览器** (`/or-gallery`)：
- 封面卡片浏览 + 内置漫画阅读器
- 单/双页模式、LTR/RTL 方向、触屏翻页、全屏
- 支持目录和 ZIP/CBZ 直接阅读
- 图片通过 imgproxy 原生 URL 或 `/img/` 简化入口处理

**图片编辑器** (`/img-editor`)：独立 SPA 图片处理工具

**前端第三方库**（内置，不依赖 CDN）：
- JSZip — ZIP 文件处理
- PDF.js — PDF 渲染

### 4.10 认证与安全（扩展层）

**在 imgproxy 原生安全能力之上叠加**：

**HTTP Basic Auth**：
- `AUTH_USER=username:password` 启用
- 仅对扩展层端点（WebDAV / API / SPA）生效
- imgproxy 原生端点继续使用 `IMGPROXY_SECRET` Bearer Token 认证

**IP 白名单**：
- `AUTH_IP_WHITELIST=192.168.1.0/24,10.0.0.5` — 免认证访问

**安全通用规则**：
- 所有 API 路径检测 `..`（400）
- 所有路径操作验证最终路径在数据根目录范围内（403）
- CORS：所有 API 端点配置完整的 CORS 头
- 错误响应：统一 JSON 格式 `{"error":"<code>","message":"<desc>"}`
- HTTP 方法校验：返回 405 + `Allow` 头

---

## 5. 配置系统设计

### 5.1 双层配置

**imgproxy 原生配置**（全部保留）：

通过 `IMGPROXY_*` 环境变量配置，详见 [imgproxy 官方文档](https://docs.imgproxy.net)。核心配置：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_BIND` | `:8080` | 监听地址 |
| `IMGPROXY_LOCAL_FILESYSTEM_ROOT` | — | 本地文件系统根（设为 `/` 以支持全路径访问） |
| `IMGPROXY_KEY` | — | URL 签名密钥（hex） |
| `IMGPROXY_SALT` | — | URL 签名盐值（hex） |
| `IMGPROXY_SECRET` | — | Bearer Token 认证 |
| `IMGPROXY_ALLOWED_SOURCES` | — | 源白名单 |
| `IMGPROXY_WORKERS` | `2×CPU` | 最大并发处理数 |
| `IMGPROXY_MAX_SRC_FILE_SIZE` | `0` | 最大源文件大小 |
| `IMGPROXY_MAX_SRC_RESOLUTION` | `50` | 最大源分辨率（Mpx） |
| `IMGPROXY_TTL` | `31536000` | Cache-Control max-age |
| `IMGPROXY_USE_ETAG` | — | 启用 ETag |
| `IMGPROXY_QUALITY` | `80` | 默认输出质量 |
| `IMGPROXY_FORMAT_QUALITY` | — | 按格式设置质量 |
| `IMGPROXY_AUTO_WEBP` | — | 自动 WebP |
| `IMGPROXY_AUTO_AVIF` | — | 自动 AVIF |
| `IMGPROXY_PRESETS` | — | 处理预设 |
| `IMGPROXY_PROMETHEUS_BIND` | — | Prometheus 端口 |
| `IMGPROXY_LOG_LEVEL` | `info` | 日志级别 |
| `IMGPROXY_MALLOC` | — | malloc 实现（推荐 `jemalloc`） |

**扩展层配置**：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PLUS_DATA_ROOT` | `/data` | 数据根目录（应与 `IMGPROXY_LOCAL_FILESYSTEM_ROOT` 对齐） |
| `PLUS_RAMDISK_PATH` | `/mnt/ramdisk` | RAM Disk 挂载路径 |
| `PLUS_HTTP_PORT` | `8080` | 扩展层 HTTP 端口（可与 imgproxy 共享或分离） |
| `PLUS_LOG_LEVEL` | `warn` | 扩展层日志级别 |
| `AUTH_USER` | — | Basic Auth 凭证（`username:password`） |
| `AUTH_IP_WHITELIST` | — | 免认证 IP（逗号分隔，支持 CIDR） |
| `ZIP_EXTS` | `zip,cbz` | ZIP 兼容扩展名列表 |
| `ZIPFS_TRANSPARENT` | `true` | ZIP 透明浏览开关 |
| `API_PAGE_SIZE_MAX` | `200` | 目录 API 最大分页数 |
| `FILEAPI_DISABLE` | `false` | 禁用文件管理 API |
| `RAMDISK_SIZE` | `512m` | RAM Disk 大小 |
| `UID` / `GID` | `1000` | 运行用户/组 ID |

### 5.2 配置交互

```
环境变量 → 容器启动脚本
    ├── imgproxy 原生: IMGPROXY_* 变量直接传递
    └── 扩展层: PLUS_* / AUTH_* / ZIP_* 等变量传递给扩展业务层
```

---

## 6. 图片处理引擎集成规范

### 6.1 进程内调用

- 扩展层通过 `http://localhost:8080`（或 imgproxy 监听的端口）调用 imgproxy
- 单容器内零网络延迟
- 支持 imgproxy 原生 URL 签名（若配置了 `IMGPROXY_KEY`/`IMGPROXY_SALT`）

### 6.2 URL 构建规范

**`/img/<path>` 入口 → imgproxy URL 映射**：

```
应用层请求:
  GET /img/photos/cat.jpg?w=300&h=400&fit=cover&fmt=webp&q=80

映射为 imgproxy URL:
  /insecure/rs:fill:300:400/q:80/format:webp/local:///data/photos/cat.jpg

  （若启用签名则替换 /insecure/ 为签名值）
```

**`/api/img` 入口 → RAM Disk 桥接**：

```
1. POST body → 写入 /mnt/ramdisk/.imgapi-tmp/<uuid>
2. imgproxy URL: /insecure/q:80/format:webp/local:///mnt/ramdisk/.imgapi-tmp/<uuid>
3. 处理完成后删除临时文件
```

### 6.3 关键约束

| 约束 | 说明 |
|------|------|
| `local://` 三斜杠 | `local:///path/to/file`，否则首段被解析为 hostname |
| imgproxy 不对 `local://` 做 base64 | 直接使用 raw URL |
| 格式通过 `format:<ext>` 指定 | 不是 `@ext` 后缀 |
| 动态图保护 | 处理前检测动态 WebP/GIF，检测到则直接返回原字节 |
| 文件系统操作直调系统 API | stat/mkdir/rename/unlink/rmdir/readdir 直接调用，不通过 shell 子进程 |
| ZIP 内部时间戳 | ZIP 内文件 mtime/ctime 可能不可用，返回空字符串 |

---

## 7. 错误处理规范

### 7.1 统一错误响应格式（扩展层）

```json
{
  "error":   "<错误代码>",
  "message": "<可读描述>"
}
```

### 7.2 通用错误码

| HTTP 状态 | error 代码 | 适用场景 |
|-----------|-----------|---------|
| `400` | `bad_request` | 路径穿越、参数缺失、请求体为空、JSON 格式错误 |
| `403` | `forbidden` | 路径超出根目录、功能被禁用、认证类型不匹配 |
| `404` | `not_found` | 路径不存在 |
| `405` | `method_not_allowed` | HTTP 方法错误（应返回 `Allow` 头） |
| `409` | `conflict` | 目标已存在、路径类型冲突 |
| `415` | `unsupported_media_type` | 图片格式无法解码 |
| `500` | `internal` | 内部错误（文件系统、编码等） |
| `502` | `bad_gateway` | imgproxy 处理失败 |

> imgproxy 原生端点的错误响应格式由 imgproxy 自身决定，不遵循上述扩展层格式。

---

## 8. API 接口总览

### 8.1 imgproxy 原生端点

| 接口 | 方法 | 路径 | 说明 |
|------|------|------|------|
| 图片处理 | GET | `/%signature/%opts/%source` | imgproxy 核心功能 |
| 健康检查 | GET | `/health` | imgproxy 健康检查 |
| Prometheus | GET | `/metrics` | 指标端点（若启用） |

### 8.2 扩展层端点

| 接口 | 方法 | 路径 | 请求格式 | 响应格式 |
|------|------|------|---------|---------|
| 简化图片处理 | GET | `/img/<path>` | Query params | Binary image |
| 目录列表 | GET | `/api/ls/<path>` | Query params | JSON |
| 删除 | DELETE | `/api/rm/<path>` | — | JSON |
| 移动/改名 | POST | `/api/move` | JSON body | JSON |
| 新建目录 | POST | `/api/mkdir/<path>` | — | JSON |
| 上传 | POST | `/api/upload/<path>` | Raw binary | JSON |
| 实时图片处理 | POST | `/api/img` | Binary + Query params | Binary image |
| 批量图片处理 | POST | `/api/batch-img` | JSON body | JSON |
| Gallery 整理 | POST | `/api/gallerize` | JSON body | JSON |
| ZIP 浏览 | GET | `/zip/<path>` | — | Binary / HTML |
| WebDAV | 全方法 | `/` | WebDAV protocol | WebDAV response |
| Gallery SPA | GET | `/or-gallery` | — | HTML |
| 图片编辑器 | GET | `/img-editor` | — | HTML |

### 8.3 跨接口一致性要求

1. **路径参数**：所有接口的 `<path>` 相对于数据根目录，行为一致
2. **路径安全**：所有接口均需检测 `..` 和路径范围
3. **CORS**：所有扩展层 API 端点支持 CORS
4. **图片处理参数**：`/img/`、`/api/img`、`/api/batch-img` 三入口参数一致
5. **ZIP 浏览**：`/api/ls/` 和 `/zip/` 端点访问同一 ZIP 文件，内容一致
6. **认证**：imgproxy 端点用 `IMGPROXY_SECRET`，扩展层端点用 `AUTH_USER` + IP 白名单
7. **错误格式**：扩展层 API 统一 JSON 错误格式；imgproxy 原生端点保持自身格式

---

## 9. Docker 部署架构

### 9.1 单容器模式（推荐）

```yaml
services:
  imgproxy_plus:
    image: imgproxy_plus:latest
    ports:
      - "5080:8080"    # HTTP（imgproxy + 扩展层共享）
      - "9090:9090"    # Prometheus（可选）
    environment:
      # imgproxy 原生配置
      - IMGPROXY_LOCAL_FILESYSTEM_ROOT=/
      - IMGPROXY_KEY=<hex_key>
      - IMGPROXY_SALT=<hex_salt>
      - IMGPROXY_SECRET=<bearer_token>
      - IMGPROXY_ALLOWED_SOURCES=local://
      - IMGPROXY_WORKERS=2
      - IMGPROXY_QUALITY=80
      - IMGPROXY_AUTO_WEBP=true
      - IMGPROXY_FORMAT_QUALITY=jpeg=75,webp=80,avif=50
      - IMGPROXY_PROMETHEUS_BIND=:9090
      - IMGPROXY_LOG_LEVEL=warn
      - IMGPROXY_MALLOC=jemalloc
      # 扩展层配置
      - PLUS_DATA_ROOT=/data
      - AUTH_USER=gallery:password
      - AUTH_IP_WHITELIST=192.168.1.0/24
      - ZIP_EXTS=zip,cbz
    volumes:
      - /nas/data:/data          # 文件数据
      - ramdisk:/mnt/ramdisk     # RAM Disk（POST API 临时文件）
    restart: unless-stopped

volumes:
  ramdisk:
    driver: local
    driver_opts:
      type: tmpfs
      device: tmpfs
      o: "size=512m"
```

### 9.2 与原架构对比

| 特性 | docker-openresty-tool (原) | imgproxy_plus (新) |
|------|--------------------------|-------------------|
| 容器数量 | 2（yot + imgproxy） | 1（imgproxy_plus） |
| 基础镜像 | OpenResty (Nginx + Lua) | imgproxy 官方（纯 Go） |
| 编程语言 | Lua | Go |
| 图片处理调用 | 跨容器 HTTP | 进程内 localhost HTTP |
| 文件系统共享 | 需共享卷 | 同一容器，天然共享 |
| 缓存机制 | nginx proxy_cache | imgproxy 原生 Cache-Control / ETag / Last-Modified 头 |
| WebDAV | ✅ | ✅ |
| ZipFS | ✅ | ✅ |
| Gallery | ✅ | ✅ |
| URL 签名 | 需自行实现 | imgproxy 原生支持 |
| 自动格式 | 需自行实现 | imgproxy 原生支持 |
| 水印 | 需自行实现 | imgproxy 原生支持 |
| Prometheus | 需自行实现 | imgproxy 原生支持 |
| RAM Disk 桥接 | 跨容器共享 tmpfs | 容器内 tmpfs |

---

## 10. 待办与演进方向

- [ ] `type: "v2"` gallerize 模式
- [ ] Gallery SPA 的读者进度记忆 / 书签功能
- [ ] 基于 WebSocket 的实时目录变更推送
- [ ] 图片 EXIF 元数据读取 API
- [ ] imgproxy Pro 特性集成（视频预览、对象检测、高级水印）
- [ ] S3 / GCS / ABS 源直接浏览 API（利用 imgproxy 原生云存储能力）
- [ ] 预设管理 UI（基于 `IMGPROXY_PRESETS`）
