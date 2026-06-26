# 规则
始终用简体中文回答和思考
本项目是Docker 项目，始终在Docker 中进行测试，有必要可以将代码挂载进入容器，加快调试速度

# AGENTS.md

> 完整业务文档见 `docs/req.md`。
每次完成一个功能，都要更新本文档

## 构建

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o imgproxy_plus .
```

## 架构

- **双进程单容器**：`entrypoint.sh` 先启动 imgproxy（bg, `:8081`, libvips），再 `exec` Go 二进制（fg, `:8080`）。
- **Go 层不做图片处理** — 它是 HTTP 分发器，图片请求通过 `localhost:8081` 转发给 imgproxy。
- **无数据库、无配置文件** — 100% env var 配置。`.env` 在 `.gitignore` 中，模板是 `.env.example`。
- **只有 1 个外部依赖**：`golang.org/x/text v0.14.0`，用于 ZIP 内 GBK/Shift-JIS 编码检测。其余全是 stdlib。
- **无测试套件**。不要尝试 `go test`。

## 包职责

| 包 | 职责 |
|----|------|
| `internal/config/` | env var → `Config` 结构体 |
| `internal/router/` | **核心分发器** — 所有路由在此匹配 |
| `internal/proxy/` | imgproxy HTTP 客户端（HMAC-SHA256 签名 / insecure 透传） |
| `internal/auth/` | Basic Auth + IP 白名单中间件 |
| `internal/img/` | `/img/` 入口 — 参数映射 + base64 源编码 + 签名后调用 imgproxy |
| `internal/api/` | 文件管理 REST API（ls/rm/move/mkdir/upload/img/batch-img/gallerize） |
| `internal/webdav/` | WebDAV（PROPFIND/MKCOL/COPY/MOVE/LOCK/UNLOCK/PUT/DELETE）+ HTML 目录浏览 |
| `internal/zipfs/` | ZIP/CBZ 透明浏览 — 不解压直接访问内部文件 |
| `internal/ziputil/` | 非 UTF-8 ZIP 文件名解码：GBK/Shift-JIS，汉字/假名/全角字符占比 ≥ 15% 判定 |
| `internal/static/` | 静态文件服务 + HTML `<base>` 标签注入 |
| `internal/archive/` | **Gallery Archive 引擎** — 定时扫描 → 解压 → 分组 → 转换 → CBZ 打包 |
| `internal/archive/unpack/` | 多格式解压：zip/tar/xz/gz/rar/7z/cbz/cbr/pdf |

## 路由匹配顺序（关键）

在 `dispatcher.go:ServeHTTP` 中按以下顺序匹配：

1. `/api/*` — 文件管理 API
2. `/zip/*` — ZIP 内部文件
3. `/img/*` — 图片处理入口
4. `/or-gallery`, `/img-editor`, `/img-sequence`, `/` — SPA 页面
5. `/libs/`, `/js/`, `/css/` — **绕过认证**的静态资源
6. `/health` — 代理到 imgproxy 健康检查
7. 其余全部 → `smartRoute`：WebDAV 方法 → 文件系统路径 → 静态文件兜底

## URL 前缀 (`PLUS_URL_PREFIX`)

- 前缀在进入路由前被 `stripPrefix` 剥离，**不要在路由代码中手动加前缀**。
- `static` 包自动向 HTML 的 `<head>` 注入 `<base href="/prefix/">`。
- 仅当 `PLUS_URL_PREFIX` 非空且非 `/` 时生效。

## imgproxy 调用

- **本地文件访问**：源路径是 `local:///<data_root>/<path>`（三斜杠），对应 `IMGPROXY_LOCAL_FILESYSTEM_ROOT=/`。
- **签名**：`HMAC-SHA256(salt + path_segment, key)` → URL-safe Base64。KEY/SALT 须是 hex 编码。
- **无签名**：若 `IMGPROXY_KEY` 或 `IMGPROXY_SALT` 为空，使用 `/insecure` 前缀跳过签名。
- **动态图保护**：检测到 GIF/动态 WebP 且有处理参数时，直传原图并标记 `X-Imgproxy: passthrough-animated`。

## 其他要点

- **JSON 错误格式**：`{"error":"<code>","message":"<desc>"}`。错误码：`bad_request` / `forbidden` / `not_found` / `conflict` / `internal` / `bad_gateway` 等。
- **html/ 路径查找**：先尝试 `exeDir/html`，再 `exeDir/../html`，最后 `./html`。Docker 镜像中 html 在 `/usr/local/bin/html/`。
- **`PLUS_LOCAL_PATH`** 是 docker-compose 的 volume mount 变量，不是 Go 程序的配置项。
- **RAM Disk**：`/mnt/ramdisk` 用于 `/api/img` 临时文件处理。

## CBZ → MP4 批量转换工具

`/data/gallery/videos/cbz2mp4.py` — 一键脚本：CBZ 解压 → 动画 WebP 转 H.265 MP4 (hevc_nvenc) → 按角色合并 → 1080p。

```bash
# 全流程 (最常用)
python3 cbz2mp4.py all /data/gallery/videos

# 仅转换 (不合并)
python3 cbz2mp4.py convert /data/gallery/videos

# 仅合并 (已有独立 MP4)
python3 cbz2mp4.py merge /data/gallery/videos/output/<name>

# 环境变量
WORKERS=12 CQ=18 OUT_WIDTH=1920 OUT_HEIGHT=1080 python3 cbz2mp4.py all /data/gallery/videos
```

**依赖**: `python3`, `Pillow`, `ffmpeg` (hevc_nvenc), `nvidia GPU`

**流程**:
1. 扫描 `*.cbz`，解压出所有 `.webp`
2. PIL 逐帧读取动画 WebP，rawvideo pipe → ffmpeg hevc_nvenc (p7, cq=21)
3. 按角色名（文件名 `_N.mp4` 前的部分）分组，concat demuxer 合并
4. `scale→pad` 缩放至 1920×1080（保持比例，黑边填充）


## meta-api 元数据查询和处理
 https://n8n.c.gatepro.cn/webhook/search_gallery_by_id_or_filename
接口中,可以通过post json 数据来查询 gallery的meta 信息:
 { "file_name": "08 [AI Generated]", 
    "gallery_id":  113 
}

### archived 目录文件查询方式
/onas/16t4/archived 下包含了类似 `4011955_2a90e31578-Toph cosplay.cbz` 文件或者目录
其中 `4011955` 是gallery_id  `Toph cosplay` 是 file_name
可以通过这些信息 去 meta-api 查询信息.
当活得到了 元数据类似:
{
  "match": [
    {
      "url": "https://e-hentai.org/g/4002166/28219bd6d3/",
      "title": "キャロン08 [AI Generated]",
      "gallery_id": "4002166",
      "uploader": "francocute",
      "pages": 83,
      "tags": "parody:cream lemon,character:caron,female:ponytail,other:ai generated",
      "category": "Misc",
      "thumbnail": "https://ehgt.org/w/02/443/86222-red95tne.webp",
      "posted_date": "2026-06-21 03:48",
      "rating": 0
    }
  ]
}

### 画廊库结构:
扫描 /onas/16t4/archived/ 下的cbz 文件, 获得cbz 元数据后,将cbz文件移动到如下地址
/onas/16t4/ehen/{category}/{uploader}/{gallery_id}-{file_name}.cbz
最终更新 noco21 数据的 eh_gallery-260620 表的`reader_url` 字段为 `/ehen/{category}/{uploader}/{gallery_id}-{file_name}.cbz`

## ehen 自动路由 (Go 集成)

> **实现：** `internal/archive/ehen.go`
> 原 Python 脚本 `scripts/move_cbz_to_ehen.py` 的逻辑已移植到 Go，直接在 archive 引擎中运行。

### 工作流

当 `EHEN_ENABLED=true` 时，每个 `PackCBZ` 生成的 CBZ 文件会自动：

1. **解析文件名** — 提取 `{gallery_id}_{token}-{file_name}`
2. **解析元数据**（级联查询）:
   - DB `eh_page-260604` 精确 URL 匹配
   - DB `eh_page-260604` gid 模糊匹配
   - DB `eh_page-260604` title 模糊匹配
   - n8n webhook 同步查询（带 `gallery_id` + `gallery_token`）
3. **移动到** `/data/ehen/{category}/{uploader}/{gallery_id}-{file_name}.cbz`
4. **更新 DB** `eh_gallery-260620` 的 `reader_url` 和 `cover_url`
5. 文件权限设为 `0666` (rw-rw-rw-)

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `EHEN_ENABLED` | `true` | 是否开启 ehen 自动路由 |
| `EHEN_DIR` | `/data/ehen` | ehen 目标根目录 |
| `EHEN_META_WEBHOOK` | `https://n8n.c.gatepro.cn/webhook/search_gallery_by_id_or_filename` | 元数据查询 webhook |
| `PGHOST/PGPORT/PGUSER/PGPASSWORD/PGDATABASE` | — | noco21 数据库连接 |

### docker-compose 挂载映射

`PLUS_LOCAL_PATH=/onas/16t4/` → 容器内 `/data/`:
- `/data/archived/` → `/onas/16t4/archived/` (归档临时目录)
- `/data/ehen/` → `/onas/16t4/ehen/` (分类库目录)

### 依赖

- `github.com/lib/pq` v1.12.3 — 纯 Go PostgreSQL 驱动，支持 `CGO_ENABLED=0` 编译

