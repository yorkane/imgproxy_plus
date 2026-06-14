# Gallery Archive — 自动画廊归档引擎

> **定位**：定时扫描指定源目录，自动解压、转换、打包为 CBZ 画廊文件。替代现有手动触发的 `/api/gallerize`。

---

## 1. 概述

Gallery Archive 是一个内置在 imgproxy_plus 中的自动归档引擎。它定时扫描源目录（本地/NAS 挂载），将发现的图片和压缩包自动处理为标准化 CBZ 画廊文件，供 `/or-gallery` 前端加载。

### 1.1 核心能力

- **定时自动扫描**：Go 内部 goroutine + time.Ticker，无需外部 cron
- **多格式解压**：zip / tar / xz / gz / rar / 7z / cbz / cbr / pdf
- **智能分组**：自动识别章节结构，独立或合并打包
- **图片转换**：通过 imgproxy 将静态图转为 WebP/AVIF，保留动态图
- **体积控制**：过小图片自动剔除，转换参数可配置
- **封面生成**：自动选取封面图，命名为 `__cover.jfif`（360x504, fit=fill, gravity=sm, q=80）
- **非图片分离**：视频/音频移入 misc/，压缩包解压后删除
- **图片并发转换**：单分组内 goroutine 池并发调用 imgproxy，可配并发数
- **优雅关闭**：SIGTERM 等待当前任务完成后退出，不中断转换

---

## 2. 处理流程

### 2.1 主循环

```
┌─────────────────────────────────────────┐
│  启动时若 GALLERY_AUTO_ENABLED=true      │
│  启动 goroutine: every SCAN_INTERVAL     │
│                                          │
│  loop:                                   │
│    1. 扫描 GALLERY_SCAN_DIR              │
│    2. 跳过 _failed 结尾的目录            │
│    3. 遍历所有未处理的一级子目录         │
│    4. → processOne(root) 逐个处理          │
│    5. sleep SCAN_INTERVAL                  │
└─────────────────────────────────────────┘
```

每次扫描处理**全部**未处理目录，依次串行。

### 2.2 单目录处理流程 `processOne(root)`

```
输入: GALLERY_SCAN_DIR/3844145/

1. UNPACK
   遍历 root 下所有文件:
   ├── 文件是压缩包 (zip/tar/xz/gz/rar/7z/cbz/cbr/pdf)
   │   → 解压到 root/.tmp/<archive_name>/
   │   → 将解压内容提取到 root/ 下（保留子目录结构）
   │   → 删除源压缩包
   ├── 文件是图片 → 保留
   ├── 文件是视频/音频 → 保留（后续移入 misc/）
   └── 文件是其他 → 保留（后续移入 misc/）

2. BUILD TREE
   遍历 root 下所有内容，构建分组树:

   root                           ← 始终存在
   ├── 散图 (root 下直接图片)     → group "3844145"
   ├── subdir_A/                  → 若内部图片 >= MIN_CHAPTER → 独立分组
   │   ├── subdir_A1/             → 若内部图片 >= MIN_CHAPTER → 独立分组
   │   │   └── *.jpg              → 否则合并到 subdir_A 分组
   │   └── *.jpg                  → 若 subdir_A 被合并，这些图片进入父级
   └── subdir_B/                  → 若内部图片 < MIN_CHAPTER → 合并到根分组

3. PROCESS EACH GROUP

   对每个分组 group:

   a. 收集该分组下所有图片文件
   b. 分类:
      ├── 动态 GIF         → 不转换，保留原名，直接加入 cbz
      ├── 动态 WebP        → 不转换，保留原名，直接加入 cbz
      └── 其他图片          → 需转换
    c. 需转换的图片 → **并发**通过 imgproxy 转为 .jfif (WebP/AVIF)
       - 并发数由 `GALLERY_ARCHIVE_CONCURRENCY` 控制（0 = CPU核数-2）
       - 参数: w=ARCHIVE_W, h=ARCHIVE_H, fit=ARCHIVE_FIT, q=ARCHIVE_Q, fmt=ARCHIVE_FMT
      - 转换成功后删除源文件
   d. 转换后检查: 文件大小 < ARCHIVE_MIN_KB * 1024 → 删除
      - 排除 __cover.jfif
    e. 选定封面:
       - 优先匹配文件名含 "cover" 单词 (不区分大小写)
       - 否则取文件名自然排序第一的图片
       - 封面源图必须是转换后的 .jfif 或动态图（不转换的）
       - 若封面图已是 .jfif → 直接复制为 __cover.jfif
       - 若封面图是动态图 → 用该图通过 imgproxy 生成 __cover.jfif
    f. 非图片文件处理:
       - 视频/音频 → 移到 group 对应目录下的 misc/
       - 其他文件 → 移到 group 对应目录下的 misc/
    g. 打包 CBZ:
       - ZIP store 模式（无压缩）
       - 仅含: __cover.jfif + *.jfif + 动态图原文件(保留原名)
      - 内部无子目录，所有文件在 cbz 根下
      - 命名: <group_cbz_name>.cbz（见下文命名规则）
   h. 移动 cbz 到 GALLERY_ARCHIVE_DIR/

4. CLEANUP
   处理完所有分组后:
   ├── 若 root 下无 misc/ 内容 → 删除 root 整个目录
   └── 若 root 下有 misc/ 内容 → 保留目录，但删除已转换的源文件

5. ON ERROR
   若任何步骤失败:
   - 将 root 重命名为 root_failed
   - 写入 .gallery_error 记录失败原因
   - 返回（下次扫描跳过 _failed 目录）
```

### 2.3 分组规则详解

```
规则: 目录内图片数 >= MIN_CHAPTER(5) → 独立分组
      目录内图片数 <  MIN_CHAPTER(5) → 合并到父级分组
```

**例 1: 简单散图**
```
3844145/
├── 01.jpg
├── 02.jpg
├── cover.jpg
└── readme.txt

分组: [3844145] ← 所有散图归入根分组
CBZ:  3844145.cbz
```

**例 2: 有子目录（独立 + 合并）**
```
3844145/
├── 001.jpg
├── ch01/  (10 pics)   ← >= 5 → 独立
├── ch02/  (3 pics)    ← < 5  → 合并到根
└── readme.txt

分组:
  [3844145]         → 001.jpg + ch02 的 3 张
  [3844145-ch01]    → ch01 的 10 张

CBZ:
  3844145.cbz
  3844145-ch01.cbz
```

**例 3: 深度嵌套**
```
3844145/
└── gallery-name-2/
    ├── cover.jpg
    ├── ch01/  (10 pics)   ← >= 5 → 独立
    ├── ch02/  (2 pics)    ← < 5  → 合并到 gallery-name-2
    └── info.txt

分组:
  [3844145-gallery-name-2]       → cover.jpg + ch02 的 2 张
  [3844145-gallery-name-2-ch01]  → ch01 的 10 张

CBZ:
  3844145-gallery-name-2.cbz
  3844145-gallery-name-2-ch01.cbz
```

**例 4: 压缩包内包含多子目录**
```
3844145/
└── all.zip 解压后 →
    ├── ch01/  (10 pics)
    ├── ch02/  (8 pics)
    └── ch03/  (2 pics)

分组:
  [3844145-ch01]  → ch01
  [3844145-ch02]  → ch02
  ch03 合并到 [3844145]

CBZ:
  3844145-ch01.cbz
  3844145-ch02.cbz
  3844145.cbz        ← 含 ch03
```

### 2.4 CBZ 命名规则

```
<一级目录名>[-<中间路径>][-<末级子目录名>].cbz
```

- 一级目录是 `GALLERY_SCAN_DIR` 的直接子目录名
- 中间路径是一级到分组目录之间的目录名
- 用 `-` 连接所有路径段
- 根级分组只用一级目录名

---

## 3. 配置项

| 环境变量 | 类型 | 默认值 | 说明 |
|----------|------|--------|------|
| `GALLERY_AUTO_ENABLED` | bool | `false` | 启用自动扫描 |
| `GALLERY_SCAN_DIR` | path | `/data/ssd1/aria2/completed` | 扫描源目录 |
| `GALLERY_ARCHIVE_DIR` | path | `/data/ssd1/aria2/archived` | CBZ 输出目录 |
| `GALLERY_SCAN_INTERVAL` | int (秒) | `1800` | 扫描间隔，默认 30 分钟 |
| `GALLERY_ARCHIVE_FMT` | string | `webp` | 输出格式：`webp` 或 `avif` |
| `GALLERY_ARCHIVE_W` | int | `2560` | 转换最大宽度 |
| `GALLERY_ARCHIVE_H` | int | `2560` | 转换最大高度 |
| `GALLERY_ARCHIVE_FIT` | string | `cover` | 缩放模式：`contain` / `cover` / `fill` |
| `GALLERY_ARCHIVE_Q` | int | `90` | 输出质量 (1-100) |
| `GALLERY_ARCHIVE_MIN_KB` | int | `10` | 转换后最小文件大小（KB），小于则删除 |
| `GALLERY_ARCHIVE_MIN_CHAPTER` | int | `5` | 子目录独立打包最少图片数 |
| `GALLERY_ARCHIVE_CONCURRENCY` | int | `0` (自动=CPU核数-2) | 单分组内并发转换图片数，0 自动取 `max(1, CPU-2)` |

---

## 4. 解压支持格式

| 格式 | 扩展名 | 解压方式 |
|------|--------|----------|
| ZIP | `.zip`, `.cbz` | Go `archive/zip`（原生） |
| TAR | `.tar` | Go `archive/tar`（原生） |
| GZIP | `.gz`, `.tgz` | Go `compress/gzip`(嵌套 tar) + tar |
| XZ | `.xz`, `.txz` | 外部 `xz` 命令 + tar |
| RAR | `.rar`, `.cbr` | 外部 `unrar` 命令 |
| 7z | `.7z` | 外部 `7z` 命令 |
| PDF | `.pdf` | 外部 `pdfimages` → fallback `pdftoppm` |

### 4.1 Docker 镜像依赖

需要在 `Dockerfile` 中额外安装：

```dockerfile
RUN apk add --no-cache unrar p7zip poppler-utils xz
```

### 4.2 PDF 处理策略

```
1. pdfimages -j <pdf> <outdir>/img
   → 提取内嵌图片（JPEG/PNG）
2. 若提取结果为空或无图片 → fallback:
   pdftoppm -jpeg -r 150 <pdf> <outdir>/page
   → 逐页渲染为 JPEG
```

---

## 5. 图片分类规则

### 5.1 图片扩展名（需转换的）

```
.jpg .jpeg .png .gif .webp .bmp .tiff .tif .ico .heic .heif .avif .jxl .svg .pic
```

### 5.2 动态图检测（不转换，保留原名）

使用 `detectAnimated()` 检测：
- **GIF**: 文件头魔数 `GIF8` → 若包含多帧则为动态
- **WebP**: 文件头 `VP8X` 且 bit 1 (ANIM) 置位 → 动态 WebP

检测到动态图时：
- 不转换，保留原文件名和扩展名
- 直接放入 CBZ
- 不检查文件大小阈值

### 5.3 视频/音频扩展名（移入 misc）

```
.mp4 .mkv .avi .mov .wmv .flv .webm .mpg .mpeg .m4v .3gp
.mp3 .aac .ogg .wav .flac .m4a .wma .opus .ape .aiff
```

### 5.4 最小文件过滤

转换后的 `.jfif` 文件若 < `MIN_KB` * 1024 字节 → 删除（不进入 CBZ）。
**排除** `__cover.jfif`。

---

## 6. 封面生成

### 6.1 选择规则

```
1. 在分组的所有图片中查找文件名含 "cover" 的
   (不区分大小写，如 cover.jpg, COVER.PNG, front-cover.webp)
2. 若找到多张 → 取自然排序第一的
3. 若无 cover → 取所有图片自然排序第一的
4. 所选图片必须是转换后的 .jfif 或动态图原文件
```

### 6.2 生成方式

- 若封面源图已是 `.jfif` → 直接复制一份命名为 `__cover.jfif`
- 若封面源图是动态图 → 用封面参数（360x504, fit=fill, gravity=sm, q=80）通过 imgproxy 生成 `__cover.jfif`
- 封面参数固定，不受 `GALLERY_ARCHIVE_W/H/FIT/Q` 影响

### 6.3 CBZ 中排序

`__cover.jfif` 以 `__` 前缀自然排序第一，确保画廊前端加载时作为封面展示。

---

## 7. misc 目录

### 7.1 内容

```
misc/
├── bgm.mp3       ← 视频/音频文件
├── readme.txt    ← 其他非图片文件
└── info.pdf
```

### 7.2 位置

misc/ 位于分组对应的物理目录下（处理后的路径）。

### 7.3 CBZ 排除

misc/ **不会**被打包进 CBZ。处理完成后，若目录下仅有 misc/ 内容，该目录保留不删除。

---

## 8. 状态标记

### 8.1 处理中

在 `root/` 下创建 `.gallery_processing` 锁文件，处理完成后删除。防止并发处理同一目录。

### 8.2 处理成功

- 无 misc 内容 → 整目录删除
- 有 misc 内容 → 保留目录（源图片文件已删除）

### 8.3 处理失败

```
3844145/ → 3844145_failed/
└── .gallery_error   ← 失败原因
```

- 下次扫描跳过 `_failed` 结尾的目录
- `.gallery_error` 内容为最后一次失败的 error message

---

## 9. 跳过逻辑

扫描时判断目录是否需要处理：

```
skip = true 若:
  ├── 目录名以 _failed 结尾
  ├── 目录下存在 .gallery_processing（正在处理中）
  └── 目录下无任何图片文件且无压缩包
```

---

## 10. 与现有 `/api/gallerize` 的关系

Gallery Archive **完全替代**现有的 `/api/gallerize`：

| 特性 | 旧 gallerize v1 | 新 Gallery Archive |
|------|----------------|-------------------|
| 后缀名 | `.jiff` | `.jfif` |
| 封面名 | `##cover.jiff` | `__cover.jfif` |
| 触发方式 | 手动 POST /api/gallerize | 定时自动 + 手动 API |
| 解压支持 | 无 | zip/tar/xz/gz/rar/7z/cbz/cbr/pdf |
| 输出 | 原地整理 | 打包 CBZ → 移动到 archived/ |
| 图片转换 | 固定 WebP | 可配置 WebP/AVIF |
| 封面参数 | 固定 360x504 g:sm q:80 | 使用转换参数 |
| 动态图 | 不涉及 | 保留原格式，不转换 |
| 体积过滤 | 无 | < MIN_KB 自动删除 |
| 分组打包 | 不支持 | 智能章节分组 |

旧 `gallerize.go` 将被完全重写。API 路由 `/api/gallerize` 保留，手动调用时触发和处理同一次扫描任务。

---

## 11. 代码结构

```
internal/
├── config/config.go          ← 新增 GALLERY_* 配置项
├── api/
│   └── gallerize.go          ← 重写（替换旧 gallerizeV1）
├── archive/                   ← 新增包
│   ├── engine.go             ← processOne() 主流程
│   ├── scanner.go            ← 定时扫描主循环
│   ├── tree.go               ← 分组树构建
│   ├── converter.go          ← 图片转换 + 封面生成
│   ├── packer.go             ← CBZ 打包 + 移动
│   ├── detector.go           ← 动态图检测 + 文件分类
│   ├── unpack.go             ← 解压调度
│   └── unpack/
│       ├── zip.go            ← Go 原生 zip/tar
│       ├── rar.go            ← unrar 调用
│       ├── sevenzip.go       ← 7z 调用
│       ├── xz.go             ← xz 命令调用
│       └── pdf.go            ← pdfimages/pdftoppm 调用
├── img/handler.go            ← 保留 detectAnimated（共享）
├── proxy/imgproxy.go         ← 不变
└── router/dispatcher.go      ← 路由不变
```

### 11.1 关键函数签名

```go
// engine.go
func ProcessOne(rootPath string, cfg *config.Config) error
func archiveGroup(group *Group, cfg *config.Config, client *proxy.ImgproxyClient) error

// scanner.go
func StartScanner(cfg *config.Config)        // 启动定时 goroutine
func ScanOnce(cfg *config.Config) error       // 执行一次扫描+处理

// tree.go
type Group struct {
    Name       string          // CBZ 名（如 "3844145-ch01"）
    DirPath    string          // 物理目录路径
    Images     []FileEntry     // 所有图片文件
    NonImages  []FileEntry     // 非图片文件
    Parent     *Group          // 父级分组
}

func BuildTree(rootPath string, cfg *config.Config) ([]*Group, error)

// converter.go
func ConvertImage(client *proxy.ImgproxyClient, src, dst string, cfg *config.Config) error
func GenerateCover(client *proxy.ImgproxyClient, src, dst string, cfg *config.Config) error

// packer.go
func PackCBZ(group *Group, cfg *config.Config) (string, error)
func MoveToArchive(cbzPath string, cfg *config.Config) error

// detector.go
func IsImageExt(name string) bool
func IsMediaExt(name string) bool           // 视频/音频
func IsArchiveExt(name string) bool         // 压缩包
func DetectAnimated(path string) bool       // 动态图检测（复用 img/handler.go）

// unpack.go
func UnpackAll(dirPath string) error        // 遍历目录解压所有压缩包
```

### 11.2 配置结构体新增字段

```go
type Config struct {
    // ... 现有字段 ...

    // Gallery Archive
    GalleryAutoEnabled   bool
    GalleryScanDir       string
    GalleryArchiveDir    string
    GalleryScanInterval  int    // 秒
    GalleryArchiveFmt    string // "webp" / "avif"
    GalleryArchiveW      int
    GalleryArchiveH      int
    GalleryArchiveFit    string // "contain" / "cover" / "fill"
    GalleryArchiveQ      int    // 1-100
    GalleryArchiveMinKB      int    // KB
    GalleryArchiveMinChapter int
    GalleryArchiveConcurrency int  // 0=auto(CPU-2)
}
```

---

## 12. 手动 API

`POST /api/gallerize` 保留，作为手动触发的入口。

### 请求格式

```json
{
  "path": "3844145",
  "type": "v2"
}
```

- `path`：相对于 `GALLERY_SCAN_DIR` 的目录名
- `type`：`"v2"`（旧 `"v1"` 不再支持）
- 转换参数使用环境变量配置，不在请求中指定

### 响应格式

```json
{
  "ok": true,
  "path": "3844145",
  "type": "v2",
  "cbz": [
    "3844145.cbz",
    "3844145-ch01.cbz"
  ],
  "stats": {
    "total": 15,
    "converted": 12,
    "skipped_animated": 2,
    "removed_small": 1,
    "errors": 0
  }
}
```

### 错误响应

```json
{
  "ok": false,
  "error": "internal",
  "message": "unrar failed: exit status 1"
}
```

---

## 13. 日志

关键进度使用 `WARN` 级别，默认 `PLUS_LOG_LEVEL=warn` 即可看到：

```
level=WARN msg="archive scanner started" scan_dir=/data/... interval=30m0s
level=WARN msg="archive processing" dir=3844145
level=WARN msg="packed cbz" group=3844145-ch01 file=3844145-ch01.cbz
level=WARN msg="archive done" dir=3844145 duration=22.6s cbz=2 converted=87 errors=0
level=WARN msg="scan interrupted by shutdown"
level=WARN msg="received signal" signal=terminated
```

详细调试信息使用 `DEBUG` 级别（转换失败、文件过滤等）。

---

## 14. 安全考虑

- 所有路径操作经过 `filepath.Clean` + 前缀校验（防路径遍历）
- `.gallery_processing` 锁文件防止并发
- 压缩包解压前检查文件大小（防炸弹）
- 外部命令调用使用 `exec.CommandContext` + timeout
- 解压到 `.tmp/` 临时子目录，处理完清理

---

## 15. 优雅关闭

收到 `SIGTERM` / `SIGINT` 信号时：

```
1. archive.Shutdown() → 设置原子标志 + 关闭 stopScan channel
2. archive.WaitShutdown() → 等待当前 ProcessOne 完成
   - 已在处理的分组继续完成（不中断当前目录）
   - 未开始的分组/目录被跳过
3. srv.Shutdown(ctx) → HTTP 服务优雅关闭（30s 超时）
4. 进程 exit(0)
```

关键点：
- **当前正在转换的图片不会被中断**，会全部完成
- scanner goroutine 通过 `select { case <-stopScan }` 立即退出，无需等待 ticker
- docker stop 的默认 10s 超时可能不够，建议 `docker stop -t 120 imgproxy_plus` 给足时间
