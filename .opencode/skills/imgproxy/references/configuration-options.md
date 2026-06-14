# 配置环境变量完整参考

所有配置通过环境变量设置。

## 目录

- [一、服务器设置](#一服务器设置)
- [二、URL 签名](#二url-签名)
- [三、安全设置](#三安全设置)
- [四、Cookie](#四cookie)
- [五、压缩质量](#五压缩质量)
- [六、JPEG 设置](#六jpeg-设置)
- [七、PNG 设置](#七png-设置)
- [八、WebP 设置](#八webp-设置)
- [九、AVIF 设置](#九avif-设置)
- [十、JPEG XL 设置](#十jpeg-xl-设置)
- [十一、自动格式检测](#十一自动格式检测)
- [十二、SVG 处理](#十二svg-处理)
- [十三、色彩空间](#十三色彩空间)
- [十四、客户端提示](#十四客户端提示)
- [十五、水印](#十五水印)
- [十六、智能裁剪](#十六智能裁剪)
- [十七、预设](#十七预设)
- [十八、图像源](#十八图像源)
- [十九、源 URL 处理](#十九源-url-处理)
- [二十、监控](#二十监控)
- [二十一、错误报告](#二十一错误报告)
- [二十二、日志](#二十二日志)
- [二十三、内存调优](#二十三内存调优)
- [二十四、杂项](#二十四杂项)

## 一、服务器设置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_BIND` | `:8080` | 监听地址 |
| `IMGPROXY_NETWORK` | `tcp` | 网络类型 |
| `IMGPROXY_TIMEOUT` | `10` | 处理响应超时（秒） |
| `IMGPROXY_GRACEFUL_STOP_TIMEOUT` | `2 × TIMEOUT` | 优雅停止等待时间（秒） |
| `IMGPROXY_READ_REQUEST_TIMEOUT` | `10` | 读取 HTTP 请求超时（秒） |
| `IMGPROXY_WRITE_RESPONSE_TIMEOUT` | `10` | 写入 HTTP 响应超时（秒） |
| `IMGPROXY_KEEP_ALIVE_TIMEOUT` | `10` | HTTP keep-alive 超时（秒），0 为禁用 |
| `IMGPROXY_CLIENT_KEEP_ALIVE_TIMEOUT` | `90` | 下载源图片的客户端 keep-alive 超时（秒） |
| `IMGPROXY_DOWNLOAD_TIMEOUT` | `5` | 下载源图片超时（秒） |
| `IMGPROXY_WORKERS` | CPU核数×2 | 最大并发处理数，Lambda 中自动设为 1 |
| `IMGPROXY_REQUESTS_QUEUE_SIZE` | `0` | 请求队列大小，0 为无限制 |
| `IMGPROXY_MAX_CLIENTS` | `2048` | 最大连接数，0 为无限制 |
| `IMGPROXY_TTL` | `31536000` | Cache-Control max-age（秒，约 1 年） |
| `IMGPROXY_CACHE_CONTROL_PASSTHROUGH` | - | 透传源图片的 Cache-Control/Expires 头 |
| `IMGPROXY_SET_CANONICAL_HEADER` | - | 设置 rel="canonical" 头指向源图片 URL |
| `IMGPROXY_SO_REUSEPORT` | - | 启用 SO_REUSEPORT（仅 Linux/macOS） |
| `IMGPROXY_PATH_PREFIX` | - | URL 路径前缀 |
| `IMGPROXY_USER_AGENT` | `imgproxy/%version` | User-Agent |
| `IMGPROXY_USE_ETAG` | - | 启用 ETag 头 |
| `IMGPROXY_ETAG_BUSTER` | - | 全局 ETag 破坏值 |
| `IMGPROXY_USE_LAST_MODIFIED` | - | 启用 Last-Modified 头 |
| `IMGPROXY_LAST_MODIFIED_BUSTER` | - | 全局 Last-Modified 破坏时间戳（RFC3339） |
| `IMGPROXY_ENABLE_DEBUG_HEADERS` | - | 启用调试头（X-Origin-*、X-Result-*） |

## 二、URL 签名

| 变量 | 说明 |
|------|------|
| `IMGPROXY_KEY` | 十六进制编码的签名密钥（可多个，逗号分隔） |
| `IMGPROXY_SALT` | 十六进制编码的盐值 |
| `IMGPROXY_SIGNATURE_SIZE` | 签名使用的字节数，默认 `32` |
| `IMGPROXY_TRUSTED_SIGNATURES` | 受信任签名列表（逗号分隔），匹配时跳过签名验证 |

## 三、安全设置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_MAX_SRC_RESOLUTION` | `50` | 源图片最大分辨率（Mpx） |
| `IMGPROXY_MAX_SRC_FILE_SIZE` | `0` | 源图片最大文件大小（字节），0 为不检查 |
| `IMGPROXY_MAX_ANIMATION_FRAMES` | `1` | 最大动画帧数 |
| `IMGPROXY_MAX_ANIMATION_FRAME_RESOLUTION` | `0` | 动画帧最大分辨率（Mpx），0 表示对所有帧求和检查 |
| `IMGPROXY_MAX_RESULT_DIMENSION` | `0` | 结果图片最大边长（像素），0 为不限制 |
| `IMGPROXY_ALLOWED_PROCESSING_OPTIONS` | 空 | 允许的处理选项列表（逗号分隔），空为全部允许 |
| `IMGPROXY_MAX_CHAINED_PIPELINES` | `0` | 最大链式流水线数，0 为不限制 |
| `IMGPROXY_MAX_REDIRECTS` | `10` | 最大重定向次数 |
| `IMGPROXY_SECRET` | - | Bearer 令牌认证，请求需带 `Authorization: Bearer <secret>` |
| `IMGPROXY_ALLOW_ORIGIN` | - | CORS 允许的 origin |
| `IMGPROXY_ALLOWED_SOURCES` | 空 | 允许的源 URL 前缀白名单，支持通配符 `*`，空为允许所有 |
| `IMGPROXY_ALLOW_LOOPBACK_SOURCE_ADDRESSES` | `false` | 允许回环地址 |
| `IMGPROXY_ALLOW_LINK_LOCAL_SOURCE_ADDRESSES` | `false` | 允许链路本地地址 |
| `IMGPROXY_ALLOW_PRIVATE_SOURCE_ADDRESSES` | `true` | 允许私网地址 |
| `IMGPROXY_PNG_UNLIMITED` | - | 禁用 PNG 块限制（可能耗尽内存） |
| `IMGPROXY_SVG_UNLIMITED` | - | 禁用 SVG 文件大小限制（默认 10MB） |
| `IMGPROXY_SANITIZE_SVG` | `true` | 清除 SVG 中的脚本（XSS 防护） |
| `IMGPROXY_IGNORE_SSL_VERIFICATION` | - | 忽略 SSL 验证（仅开发环境） |
| `IMGPROXY_ALLOW_SECURITY_OPTIONS` | `false` | 允许 URL 中使用安全相关处理选项（**谨慎启用**） |
| `IMGPROXY_SKIP_PROCESSING_FORMATS` | - | 不处理的格式列表（仅当请求格式与源格式相同时生效） |

## 四、Cookie

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_COOKIE_PASSTHROUGH` | `false` | 透传 Cookie 至源图片请求 |
| `IMGPROXY_COOKIE_BASE_URL` | - | 指定 Cookie 的作用域 URL |
| `IMGPROXY_COOKIE_PASSTHROUGH_ALL` | `false` | 透传所有 Cookie（需谨慎） |

## 五、压缩质量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_QUALITY` | `80` | 默认质量（%） |
| `IMGPROXY_FORMAT_QUALITY` | - | 按格式单独设置质量，如 `jpeg=70,avif=40` |

## 六、JPEG 设置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_JPEG_PROGRESSIVE` | `false` | 渐进式 JPEG |

## 七、PNG 设置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_PNG_INTERLACED` | `false` | PNG 交织 |
| `IMGPROXY_PNG_QUANTIZE` | - | PNG 量化（需 libvips 支持） |
| `IMGPROXY_PNG_QUANTIZATION_COLORS` | `256` | 量化颜色数 |

## 八、WebP 设置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_WEBP_EFFORT` | `4` | WebP 编码努力度（1-6） |
| `IMGPROXY_WEBP_PRESET` | - | WebP 预设（default/photo/picture/drawing/icon/text） |

## 九、AVIF 设置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_AVIF_SPEED` | `8` | AVIF 速度（0-9） |

## 十、JPEG XL 设置

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_JXL_EFFORT` | `4` | JPEG XL 努力度（1-9） |

## 十一、自动格式检测

优先级：JXL > AVIF > WebP。

| 变量 | 说明 |
|------|------|
| `IMGPROXY_AUTO_WEBP` | 根据 Accept 头自动使用 WebP |
| `IMGPROXY_ENFORCE_WEBP` | 强制使用 WebP |
| `IMGPROXY_AUTO_AVIF` | 自动使用 AVIF |
| `IMGPROXY_ENFORCE_AVIF` | 强制使用 AVIF |
| `IMGPROXY_AUTO_JXL` | 自动使用 JPEG XL |
| `IMGPROXY_ENFORCE_JXL` | 强制使用 JPEG XL |
| `IMGPROXY_PREFERRED_FORMATS` | 首选格式，默认 `jpeg,png,gif` |

## 十二、SVG 处理

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_ALWAYS_RASTERIZE_SVG` | `false` | 始终栅格化 SVG |
| `IMGPROXY_SANITIZE_SVG` | `true` | 清除 SVG 中脚本（XSS 防护） |
| `IMGPROXY_SVG_UNLIMITED` | - | 禁用 SVG 文件大小限制 |

## 十三、色彩空间

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_PRESERVE_HDR` | `false` | 保留高位深度图像（不转换为 8 位） |

## 十四、客户端提示

| 变量 | 说明 |
|------|------|
| `IMGPROXY_ENABLE_CLIENT_HINTS` | 启用 Width/DPR 客户端提示 |

## 十五、水印

| 变量 | 说明 |
|------|------|
| `IMGPROXY_WATERMARK_DATA` | Base64 编码的水印图片数据 |
| `IMGPROXY_WATERMARK_PATH` | 本地水印文件路径 |
| `IMGPROXY_WATERMARK_URL` | 水印图片 URL |
| `IMGPROXY_WATERMARK_OPACITY` | 基础不透明度 |

## 十六、智能裁剪

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_SMART_CROP_ADVANCED` | - | 启用高级智能裁剪（Pro 功能，此处不展开） |

OSS 版本默认支持简单智能裁剪（`gravity=sm`），基于 libvips 实现。

## 十七、预设

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_PRESETS` | - | 处理预设定义，如 `default=resizing_type:fill/enlarge:1` |
| `IMGPROXY_PRESETS_SEPARATOR` | `,` | 预设分隔符 |
| `IMGPROXY_PRESETS_PATH` | - | 处理预设文件路径 |
| `IMGPROXY_ONLY_PRESETS` | `false` | 仅允许预设模式，禁止 URL 中直接使用处理选项 |

## 十八、图像源

### 本地文件系统

| 变量 | 说明 |
|------|------|
| `IMGPROXY_LOCAL_FILESYSTEM_ROOT` | 本地文件系统根目录 |
| `IMGPROXY_SOURCE_URL_QUERY_SEPARATOR` | 非 HTTP 源 URL 的查询分隔符，默认 `?` |

### Amazon S3

| 变量 | 说明 |
|------|------|
| `IMGPROXY_USE_S3` | 启用 S3 源 |
| `IMGPROXY_S3_REGION` | S3 区域 |
| `IMGPROXY_S3_ENDPOINT` | S3 端点（兼容 MinIO 等） |
| `IMGPROXY_S3_ALLOWED_BUCKETS` | 允许的 Bucket 白名单 |
| `IMGPROXY_S3_DENIED_BUCKETS` | 拒绝的 Bucket 黑名单 |

### Google Cloud Storage

| 变量 | 说明 |
|------|------|
| `IMGPROXY_USE_GCS` | 启用 GCS 源 |
| `IMGPROXY_GCS_KEY` | GCS 服务账号密钥 JSON 路径 |
| `IMGPROXY_GCS_ENDPOINT` | GCS 端点 |
| `IMGPROXY_GCS_ALLOWED_BUCKETS` | 允许的 Bucket 白名单 |

### Azure Blob Storage

| 变量 | 说明 |
|------|------|
| `IMGPROXY_USE_ABS` | 启用 Azure Blob Storage 源 |
| `IMGPROXY_ABS_NAME` | ABS 存储账号名 |
| `IMGPROXY_ABS_KEY` | ABS 访问密钥 |
| `IMGPROXY_ABS_ENDPOINT` | ABS 端点 |
| `IMGPROXY_ABS_ALLOWED_BUCKETS` | 允许的容器白名单 |

### OpenStack Swift

| 变量 | 说明 |
|------|------|
| `IMGPROXY_USE_SWIFT` | 启用 Swift 源 |
| `IMGPROXY_SWIFT_USERNAME` | Swift 用户名 |
| `IMGPROXY_SWIFT_PASSWORD` | Swift 密码 |
| `IMGPROXY_SWIFT_CONTAINER` | Swift 容器 |
| `IMGPROXY_SWIFT_AUTH_URL` | Swift 认证 URL |
| `IMGPROXY_SWIFT_DOMAIN` | Swift 域名 |
| `IMGPROXY_SWIFT_TENANT_NAME` | Swift 租户名 |
| `IMGPROXY_SWIFT_TENANT_ID` | Swift 租户 ID |
| `IMGPROXY_SWIFT_REGION` | Swift 区域 |
| `IMGPROXY_SWIFT_ALLOWED_BUCKETS` | 允许的容器白名单 |

## 十九、源 URL 处理

| 变量 | 说明 |
|------|------|
| `IMGPROXY_BASE_URL` | 基础 URL 前缀，所有相对 URL 以此为基础 |
| `IMGPROXY_URL_REPLACEMENTS` | 模式替换规则（`pattern=replacement`，分号分隔） |
| `IMGPROXY_BASE64_URL_INCLUDES_FILENAME` | Base64/加密 URL 忽略最后一级文件名 |

## 二十、监控

### Prometheus

| 变量 | 说明 |
|------|------|
| `IMGPROXY_PROMETHEUS_BIND` | Prometheus 指标监听地址 |
| `IMGPROXY_PROMETHEUS_NAMESPACE` | Prometheus 命名空间 |

### Datadog

| 变量 | 说明 |
|------|------|
| `IMGPROXY_DATADOG_ENABLE` | 启用 Datadog |
| `IMGPROXY_DATADOG_ENABLE_ADDITIONAL_METRICS` | 启用额外指标 |

### New Relic

| 变量 | 说明 |
|------|------|
| `IMGPROXY_NEW_RELIC_KEY` | New Relic License Key |
| `IMGPROXY_NEW_RELIC_APP_NAME` | 应用名称 |

### OpenTelemetry

| 变量 | 说明 |
|------|------|
| `IMGPROXY_OPEN_TELEMETRY_ENABLE` | 启用 OpenTelemetry |
| `IMGPROXY_OPEN_TELEMETRY_ENABLE_METRICS` | 启用指标 |
| `IMGPROXY_OPEN_TELEMETRY_ENABLE_LOGS` | 启用日志 |

### CloudWatch

| 变量 | 说明 |
|------|------|
| `IMGPROXY_CLOUD_WATCH_SERVICE_NAME` | CloudWatch 服务名 |
| `IMGPROXY_CLOUD_WATCH_NAMESPACE` | CloudWatch 命名空间 |
| `IMGPROXY_CLOUD_WATCH_REGION` | CloudWatch 区域 |

## 二十一、错误报告

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_REPORT_DOWNLOADING_ERRORS` | `true` | 报告下载错误 |
| `IMGPROXY_DEVELOPMENT_ERRORS_MODE` | - | 显示详细错误信息 |

### Bugsnag

| 变量 | 说明 |
|------|------|
| `IMGPROXY_BUGSNAG_KEY` | Bugsnag API Key |
| `IMGPROXY_BUGSNAG_STAGE` | Bugsnag 阶段 |

### Honeybadger

| 变量 | 说明 |
|------|------|
| `IMGPROXY_HONEYBADGER_KEY` | Honeybadger API Key |
| `IMGPROXY_HONEYBADGER_ENV` | 环境名称 |

### Sentry

| 变量 | 说明 |
|------|------|
| `IMGPROXY_SENTRY_DSN` | Sentry DSN |
| `IMGPROXY_SENTRY_ENVIRONMENT` | 环境名称 |
| `IMGPROXY_SENTRY_RELEASE` | 发布版本 |

### Airbrake

| 变量 | 说明 |
|------|------|
| `IMGPROXY_AIRBRAKE_PROJECT_ID` | Airbrake 项目 ID |
| `IMGPROXY_AIRBRAKE_PROJECT_KEY` | Airbrake 项目 Key |
| `IMGPROXY_AIRBRAKE_ENVIRONMENT` | 环境名称 |

## 二十二、日志

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_LOG_FORMAT` | - | 日志格式（pretty/structured/json/gcp） |
| `IMGPROXY_LOG_LEVEL` | - | 日志级别（error/warn/info/debug） |
| `IMGPROXY_SYSLOG_ENABLE` | - | 启用 syslog |
| `IMGPROXY_SYSLOG_LEVEL` | - | syslog 级别 |
| `IMGPROXY_SYSLOG_NETWORK` | - | syslog 网络类型 |
| `IMGPROXY_SYSLOG_ADDRESS` | - | syslog 地址 |
| `IMGPROXY_SYSLOG_TAG` | - | syslog 标签 |

## 二十三、内存调优

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_DOWNLOAD_BUFFER_SIZE` | `0` | 下载缓冲区初始大小（字节） |
| `IMGPROXY_FREE_MEMORY_INTERVAL` | `10` | 内存释放间隔（秒） |
| `IMGPROXY_BUFFER_POOL_CALIBRATION_THRESHOLD` | `1024` | 缓冲池校准阈值 |
| `IMGPROXY_MALLOC` | - | malloc 实现（malloc/jemalloc/tcmalloc，仅 Docker 环境） |

## 二十四、杂项

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `IMGPROXY_ARGUMENTS_SEPARATOR` | `:` | 处理选项参数分隔符 |
| `IMGPROXY_USE_LINEAR_COLORSPACE` | `false` | 线性色彩空间处理 |
| `IMGPROXY_DISABLE_SHRINK_ON_LOAD` | `false` | 禁用加载时缩小 |
| `IMGPROXY_STRIP_METADATA` | `true` | 去除元数据 |
| `IMGPROXY_KEEP_COPYRIGHT` | `true` | 保留版权信息 |
| `IMGPROXY_STRIP_COLOR_PROFILE` | `true` | 转换并移除 ICC 颜色配置 |
| `IMGPROXY_AUTO_ROTATE` | `true` | 根据 EXIF 自动旋转 |
| `IMGPROXY_ENFORCE_THUMBNAIL` | `false` | 强制使用内嵌缩略图（heic/avif） |
| `IMGPROXY_RETURN_ATTACHMENT` | `false` | Content-Disposition: attachment |
| `IMGPROXY_FAIL_ON_DEPRECATION` | - | 使用废弃配置时报错退出 |
| `IMGPROXY_PDF_NO_BACKGROUND` | - | PDF 不加白色背景 |
