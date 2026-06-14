---
name: imgproxy
description: imgproxy 快速安全的独立图片处理服务器专用 skill。当用户提到 imgproxy、图片处理、图片缩放、图片优化、按需图片处理、搭建图片处理服务时，务必使用本 skill。也适用于处理 IMGPROXY_* 环境变量配置、图片 URL 签名生成、图片格式转换（JPEG/PNG/WebP/AVIF）、图片处理流水线配置等场景。
---

# imgproxy Skill

imgproxy 是一个快速、安全的独立图片处理服务器。它按需缩放、处理和转换图片，将图片处理工作从应用中剥离出来。

## 一、核心设计原则

- **简洁**：2 分钟内可启动运行，最少配置。HTTPS 由反向代理/CDN 处理，圆角蒙版通过 CSS 实现，不重复造轮子。
- **速度**：底层使用 libvips 图片处理库，内存占用极低，处理速度极快。
- **安全**：URL 签名防止滥用、源图片类型前置检查、尺寸前置检查防止图片炸弹、HTTP 头部授权、源白名单、大小限制。

## 二、安装方式

### Docker（推荐）

```bash
docker pull ghcr.io/imgproxy/imgproxy:latest
docker run -p 8080:8080 -it ghcr.io/imgproxy/imgproxy:latest
```

### 导出二进制包

```bash
docker run -u0 --rm -it -v $(pwd):/dist ghcr.io/imgproxy/imgproxy:latest-amd64 imgproxy-build-package deb /dist
```

支持导出 DEB、RPM、TAR 包。

### Kubernetes（Helm）

```bash
helm repo add imgproxy https://helm.imgproxy.net/
helm upgrade -i imgproxy imgproxy/imgproxy
```

### 源码编译

```bash
# Ubuntu
sudo apt-get install libvips-dev
CGO_LDFLAGS_ALLOW="-s|-w" go build -o /usr/local/bin/imgproxy

# macOS
brew install vips go
PKG_CONFIG_PATH="$(brew --prefix libffi)/lib/pkgconfig" \
  CGO_LDFLAGS_ALLOW="-s|-w" \
  CGO_CFLAGS_ALLOW="-Xpreprocessor" \
  go build -o /usr/local/bin/imgproxy
```

## 三、快速开始

启动服务后，生成第一个图片处理 URL：

```
http://localhost:8080/insecure/rs:fill:300:400/g:sm/aHR0cHM6Ly9tLm1l/ZGlhLWFtYXpvbi5j/b20vaW1hZ2VzL00v/TVY1QllUY3hOamhr/WmpndE5Ea3dPQzAw/TXpReUxUaGxaRFV0/Tm1OaU1UaGtZek5r/T0dKbFhrRXlYa0Zx/Y0djQC5fVjFfRk1q/cGdfVVgyMTYwXy5q/cGc.jpg
```

URL 结构解析：
| 部分 | 说明 |
|------|------|
| `/insecure` | 非签名模式（仅测试用） |
| `rs:fill:300:400` | 缩放模式 fill，宽 300px，高 400px |
| `/g:sm` | 智能重力（自动选择最感兴趣区域） |
| 最后一段 | Base64 编码的源图片 URL |

**生产环境务必配置签名，不要使用 `/insecure`。**

## 四、配置环境变量

imgproxy 通过环境变量配置。以下按类别列出常用配置项，完整列表见 `references/configuration-options.md`。

### 服务器设置
| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_BIND` | `:8080` | 监听地址 |
| `IMGPROXY_WORKERS` | CPU核数×2 | 最大并发处理数 |
| `IMGPROXY_TIMEOUT` | `10` | 处理响应超时（秒） |
| `IMGPROXY_READ_REQUEST_TIMEOUT` | `10` | 读取 HTTP 请求超时（秒） |
| `IMGPROXY_WRITE_RESPONSE_TIMEOUT` | `10` | 写入 HTTP 响应超时（秒） |
| `IMGPROXY_DOWNLOAD_TIMEOUT` | `5` | 下载源图片超时（秒） |
| `IMGPROXY_KEEP_ALIVE_TIMEOUT` | `10` | HTTP keep-alive 超时（秒） |
| `IMGPROXY_MAX_CLIENTS` | `2048` | 最大连接数，0 为无限制 |
| `IMGPROXY_TTL` | `31536000` | Cache-Control max-age（秒） |
| `IMGPROXY_USE_ETAG` | - | 启用 ETag 头 |
| `IMGPROXY_PATH_PREFIX` | - | URL 路径前缀 |
| `IMGPROXY_USER_AGENT` | `imgproxy/%version` | User-Agent |
| `IMGPROXY_ENABLE_DEBUG_HEADERS` | - | 启用调试头（X-Origin-*、X-Result-*） |

### URL 签名
| 变量 | 说明 |
|------|------|
| `IMGPROXY_KEY` | 十六进制编码的签名密钥（可多个，逗号分隔） |
| `IMGPROXY_SALT` | 十六进制编码的盐值 |
| `IMGPROXY_SIGNATURE_SIZE` | 签名使用的字节数，默认 32 |
| `IMGPROXY_TRUSTED_SIGNATURES` | 受信任签名列表，逗号分隔 |

### 安全设置
| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_ALLOWED_SOURCES` | 空（允许所有） | 允许的源 URL 前缀白名单，支持通配符 `*` |
| `IMGPROXY_MAX_SRC_RESOLUTION` | `50` | 源图片最大分辨率（Mpx） |
| `IMGPROXY_MAX_SRC_FILE_SIZE` | `0` | 源图片最大文件大小（字节），0 为不检查 |
| `IMGPROXY_MAX_ANIMATION_FRAMES` | `1` | 最大动画帧数 |
| `IMGPROXY_MAX_RESULT_DIMENSION` | `0` | 结果图片最大边长（像素），0 为不限制 |
| `IMGPROXY_SECRET` | - | Bearer 令牌认证 |
| `IMGPROXY_ALLOW_ORIGIN` | - | CORS 允许的 origin |
| `IMGPROXY_SANITIZE_SVG` | `true` | 清除 SVG 中的脚本（XSS 防护） |
| `IMGPROXY_MAX_REDIRECTS` | `10` | 最大重定向次数 |
| `IMGPROXY_ALLOW_SECURITY_OPTIONS` | `false` | 允许 URL 中使用安全相关处理选项（谨慎启用） |
| `IMGPROXY_SKIP_PROCESSING_FORMATS` | - | 不处理的格式列表 |

### 压缩质量
| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_QUALITY` | `80` | 默认质量（%） |
| `IMGPROXY_FORMAT_QUALITY` | - | 按格式单独设置质量，如 `jpeg=70,avif=40` |
| `IMGPROXY_JPEG_PROGRESSIVE` | `false` | 渐进式 JPEG |
| `IMGPROXY_PNG_INTERLACED` | `false` | PNG 交织 |
| `IMGPROXY_PNG_QUANTIZE` | - | PNG 量化 |
| `IMGPROXY_PNG_QUANTIZATION_COLORS` | `256` | 量化颜色数 |
| `IMGPROXY_WEBP_EFFORT` | `4` | WebP 编码努力度（1-6） |
| `IMGPROXY_AVIF_SPEED` | `8` | AVIF 速度（0-9） |
| `IMGPROXY_JXL_EFFORT` | `4` | JPEG XL 努力度（1-9） |

### 自动格式检测
| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_AUTO_WEBP` | - | 根据 Accept 头自动使用 WebP |
| `IMGPROXY_ENFORCE_WEBP` | - | 强制使用 WebP |
| `IMGPROXY_AUTO_AVIF` | - | 自动使用 AVIF |
| `IMGPROXY_ENFORCE_AVIF` | - | 强制使用 AVIF |
| `IMGPROXY_AUTO_JXL` | - | 自动使用 JPEG XL |
| `IMGPROXY_ENFORCE_JXL` | - | 强制使用 JPEG XL |
| `IMGPROXY_PREFERRED_FORMATS` | `jpeg,png,gif` | 首选格式列表 |

### 图像源
| 变量 | 说明 |
|------|------|
| `IMGPROXY_LOCAL_FILESYSTEM_ROOT` | 本地文件系统根目录 |
| `IMGPROXY_USE_S3` | 启用 S3 源 |
| `IMGPROXY_S3_REGION` | S3 区域 |
| `IMGPROXY_S3_ENDPOINT` | S3 端点（兼容 MinIO 等） |
| `IMGPROXY_USE_GCS` | 启用 Google Cloud Storage 源 |
| `IMGPROXY_GCS_KEY` | GCS 服务账号密钥 JSON 路径 |
| `IMGPROXY_USE_ABS` | 启用 Azure Blob Storage 源 |
| `IMGPROXY_ABS_NAME` | ABS 存储账号名 |
| `IMGPROXY_ABS_KEY` | ABS 访问密钥 |
| `IMGPROXY_USE_SWIFT` | 启用 OpenStack Swift 源 |

### 水印
| 变量 | 说明 |
|------|------|
| `IMGPROXY_WATERMARK_DATA` | Base64 编码的水印图片数据 |
| `IMGPROXY_WATERMARK_PATH` | 本地水印文件路径 |
| `IMGPROXY_WATERMARK_URL` | 水印图片 URL |
| `IMGPROXY_WATERMARK_OPACITY` | 基础不透明度 |

### 监控与日志
| 变量 | 说明 |
|------|------|
| `IMGPROXY_PROMETHEUS_BIND` | Prometheus 指标监听地址 |
| `IMGPROXY_LOG_FORMAT` | 日志格式（pretty/structured/json/gcp） |
| `IMGPROXY_LOG_LEVEL` | 日志级别（error/warn/info/debug） |

### 内存调优
| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_DOWNLOAD_BUFFER_SIZE` | `0` | 下载缓冲区初始大小 |
| `IMGPROXY_FREE_MEMORY_INTERVAL` | `10` | 内存释放间隔（秒） |
| `IMGPROXY_BUFFER_POOL_CALIBRATION_THRESHOLD` | `1024` | 缓冲池校准阈值 |
| `IMGPROXY_MALLOC` | - | malloc 实现（malloc/jemalloc/tcmalloc） |

### 预设
| 变量 | 说明 |
|------|------|
| `IMGPROXY_PRESETS` | 处理预设定义，如 `default=resizing_type:fill/enlarge:1` |
| `IMGPROXY_PRESETS_SEPARATOR` | 预设分隔符，默认 `,` |
| `IMGPROXY_PRESETS_PATH` | 处理预设文件路径 |
| `IMGPROXY_ONLY_PRESETS` | 仅允许预设模式，默认 `false` |

## 五、URL 结构与处理选项

### URL 格式

```
http://imgproxy.example.com/%签名/%处理选项组/%源图片URL@%扩展名
```

即使签名验证被禁用，签名位置也需要填入占位字符串（如 `insecure` 或 `_`）。

### 源图片 URL 编码方式

**纯文本（Plain）**：
```
/plain/http://example.com/image.jpg@png
```
需要对 URL 进行百分号编码（`%`→`%25`，`?`→`%3F`，`@`→`%40`）。

**Base64 编码**（推荐）：
```
/aHR0cDovL2V4YW1w/bGUuY29tL2ltYWdlL3BpYy5qcGc.png
```
使用 URL-safe Base64 编码，可用 `/` 任意分割。

### 处理选项格式

```
/%选项名:%参数1:%参数2:...
```

参数分隔符默认 `:`，可通过 `IMGPROXY_ARGUMENTS_SEPARATOR` 修改。

**处理选项分类概览**（完整表格见 `references/processing-options.md`）：

| 类别 | 常用选项 | 缩写 |
|------|----------|------|
| 尺寸与缩放 | resize, width, height, dpr, zoom, enlarge, extend | rs, w, h, dpr, z, el, ex |
| 裁切与定位 | crop, gravity, trim, padding | c, g, t, pd |
| 旋转与翻转 | auto_rotate, rotate, flip | ar, rot, fl |
| 颜色与背景 | background | bg |
| 滤镜与特效 | blur, sharpen, pixelate | bl, sh, pix |
| 水印 | watermark | wm |
| 元数据与输出 | quality, format, max_bytes, strip_metadata, cachebuster | q, f, mb, sm, cb |
| 跳过/原始 | skip_processing, raw | skp, raw |
| 其他 | preset, filename, return_attachment, expires | pr, fn, att, exp |

### 完整示例

```
http://imgproxy.example.com/AfrOrF3gWeDA6VOlDG4TzxMv39O7MXnF4CXpKUwGqRM/pr:sharp/rs:fill:300:400:0/g:sm/plain/http://example.com/images/curiosity.jpg@png
```

## 六、URL 签名

强烈建议生产环境启用 URL 签名，防止滥用。

### 配置

```bash
IMGPROXY_KEY=736563726574   # hex 编码密钥
IMGPROXY_SALT=68656c6c6f    # hex 编码盐值
```

生成随机 key/salt：
```bash
echo $(xxd -g 2 -l 64 -p /dev/random | tr -d '\n')
```

### 签名计算步骤

1. **提取路径中签名之后的部分**（含开头的 `/`），如 `/rs:fill:300:400:0/g:sm/aHR0cDovL2V4YW1w/bGUuY29tL2ltYWdl/cy9jdXJpb3NpdHku/anBn.png`
2. **在开头拼接 salt**（原始字符串，非 hex）：`hello/rs:fill...`
3. **计算 HMAC-SHA256**（key 为 hex 解码后）
4. **URL-safe Base64 编码**（`+`→`-`，`/`→`_`，去掉末尾 `=`）
5. **将签名插入 URL 路径最前面**

### 多语言签名示例

**Python**：
```python
import hmac, hashlib, base64
key = bytes.fromhex('736563726574')
salt = b'hello'
path = '/rs:fill:300:400:0/g:sm/aHR0cDovL2V4YW1w/bGUuY29tL2ltYWdl/cy9jdXJpb3NpdHku/anBn.png'
signature = base64.urlsafe_b64encode(hmac.new(key, salt + path.encode(), hashlib.sha256).digest()).rstrip(b'=').decode()
```

**Node.js**：
```javascript
const crypto = require('crypto');
const key = Buffer.from('736563726574', 'hex');
const salt = 'hello';
const path = '/rs:fill:300:400:0/g:sm/aHR0cDovL2V4YW1w/bGUuY29tL2ltYWdl/cy9jdXJpb3NpdHku/anBn.png';
const signature = crypto.createHmac('sha256', key).update(salt + path).digest().toString('base64url');
```

**PHP**：
```php
$key = hex2bin('736563726574');
$salt = 'hello';
$path = '/rs:fill:300:400:0/g:sm/aHR0cDovL2V4YW1w/bGUuY29tL2ltYWdl/cy9jdXJpb3NpdHku/anBn.png';
$signature = rtrim(strtr(base64_encode(hash_hmac('sha256', $salt . $path, $key, true)), '+/', '-_'), '=');
```

## 七、图像源

### HTTP/HTTPS（默认）
直接使用源图片 URL，支持缓存控制、User-Agent、Cookie 透传。

### 本地文件系统
```bash
IMGPROXY_LOCAL_FILESYSTEM_ROOT=/var/images
```
URL 中使用 `local://path/to/image.jpg`。

### Amazon S3
```bash
IMGPROXY_USE_S3=true
IMGPROXY_S3_REGION=us-east-1
IMGPROXY_S3_ENDPOINT=https://s3.amazonaws.com
```
支持 `IMGPROXY_S3_ALLOWED_BUCKETS` 和 `IMGPROXY_S3_DENIED_BUCKETS` 白/黑名单。

### Google Cloud Storage
```bash
IMGPROXY_USE_GCS=true
IMGPROXY_GCS_KEY=/path/to/key.json
```
支持 `IMGPROXY_GCS_ALLOWED_BUCKETS` 白名单。

### Azure Blob Storage
```bash
IMGPROXY_USE_ABS=true
IMGPROXY_ABS_NAME=storageaccount
IMGPROXY_ABS_KEY=accesskey
```
支持 `IMGPROXY_ABS_ALLOWED_BUCKETS` 白名单。

### OpenStack Swift
```bash
IMGPROXY_USE_SWIFT=true
```
配置认证 URL、用户名、密码等。支持 `IMGPROXY_SWIFT_ALLOWED_BUCKETS` 白名单。

## 八、安全配置

### 源白名单
```bash
IMGPROXY_ALLOWED_SOURCES=https://example.com,https://*.cdn.com
```
支持通配符 `*`，生产环境务必配置。

### URL 签名
配置 `IMGPROXY_KEY` 和 `IMGPROXY_SALT` 并生成签名 URL，防止未经授权的请求。

### Bearer 令牌认证
```bash
IMGPROXY_SECRET=my-secret-token
```
所有请求需要在 HTTP 头中添加 `Authorization: Bearer my-secret-token`。

### 尺寸与文件大小限制
```bash
IMGPROXY_MAX_SRC_RESOLUTION=50     # 最大 50 Mpx
IMGPROXY_MAX_SRC_FILE_SIZE=10485760 # 最大 10MB
IMGPROXY_MAX_RESULT_DIMENSION=4096  # 结果图最大边长 4096px
```

### 其他安全措施
- `IMGPROXY_SANITIZE_SVG=true`：清除 SVG 中脚本，防止 XSS
- `IMGPROXY_MAX_REDIRECTS=10`：防止重定向循环
- `IMGPROXY_ALLOW_LOOPBACK_SOURCE_ADDRESSES=false`：禁止回环地址
- `IMGPROXY_ALLOW_SECURITY_OPTIONS=false`：禁止 URL 中修改安全参数（谨慎启用）

## 九、缓存

### 缓存策略

OSS 版本不提供内置缓存，建议通过反向代理/CDN 实现缓存。

### 缓存控制

```bash
IMGPROXY_TTL=31536000  # 设置 Cache-Control max-age
IMGPROXY_CACHE_CONTROL_PASSTHROUGH=true  # 透传源图片的缓存头
IMGPROXY_USE_ETAG=true  # 启用 ETag
IMGPROXY_USE_LAST_MODIFIED=true  # 启用 Last-Modified
IMGPROXY_ETAG_BUSTER=  # 全局 ETag 破坏值
IMGPROXY_LAST_MODIFIED_BUSTER=  # 全局 Last-Modified 破坏时间戳
```

### 推荐的生产架构
```
用户 → CDN → 反向代理/Nginx → imgproxy → 源存储(S3/GCS/HTTP)
```
CDN 层自动缓存图片响应，是最简单高效的缓存方案。

## 十、监控集成

### Prometheus
```bash
IMGPROXY_PROMETHEUS_BIND=:9090
IMGPROXY_PROMETHEUS_NAMESPACE=imgproxy
```
在 `/metrics` 端点暴露指标。

### Datadog
```bash
IMGPROXY_DATADOG_ENABLE=true
IMGPROXY_DATADOG_ENABLE_ADDITIONAL_METRICS=true
```

### 错误报告
支持 Bugsnag、Honeybadger、Sentry、Airbrake 集成，配置对应 `IMGPROXY_*_KEY`/`DSN` 变量。

## 十一、内存调优

```bash
IMGPROXY_DOWNLOAD_BUFFER_SIZE=1048576       # 1MB 下载缓冲区
IMGPROXY_FREE_MEMORY_INTERVAL=10            # 每 10 秒释放内存
IMGPROXY_BUFFER_POOL_CALIBRATION_THRESHOLD=1024
IMGPROXY_MALLOC=jemalloc                    # 使用 jemalloc 提升性能
```

## 十二、参考文件

- `references/processing-options.md`：完整的处理选项表格，包含所有 OSS 选项的名称、缩写、参数和说明
- `references/configuration-options.md`：完整的配置环境变量表格，按类别组织
