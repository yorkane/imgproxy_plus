# 处理选项完整参考

所有选项格式：`/%选项名:%参数1:%参数2:...`
参数分隔符默认 `:`，可通过 `IMGPROXY_ARGUMENTS_SEPARATOR` 修改。

## 一、尺寸与缩放

| 选项 | 缩写 | 参数 | 说明 |
|------|------|------|------|
| `resize` | `rs` | resizing_type, width, height, enlarge, extend | 元选项，定义全部五个参数 |
| `size` | `s` | width, height, enlarge, extend | 元选项，定义尺寸相关参数 |
| `resizing_type` | `rt` | 类型：`fit`/`fill`/`fill-down`/`force`/`auto` | 缩放模式，默认 `fit` |
| `resizing_algorithm` | `ra` | `nearest`/`linear`/`cubic`/`lanczos2`/`lanczos3` | 缩放算法，默认 `lanczos3` |
| `width` | `w` | 像素值 | 目标宽度，0 时按比例计算 |
| `height` | `h` | 像素值 | 目标高度，0 时按比例计算 |
| `min-width` | `mw` | 像素值 | 最小宽度 |
| `min-height` | `mh` | 像素值 | 最小高度 |
| `zoom` | `z` | zoom_x_y 或 zoom_x, zoom_y | 缩放倍数，值 > 0 |
| `dpr` | `dpr` | 倍数 | 用于 HiDPI 屏幕，值 > 0 |
| `enlarge` | `el` | 布尔值（`1`/`t`/`true`） | 是否允许放大图片 |
| `extend` | `ex` | 布尔值, gravity | 是否扩展画布至目标尺寸 |
| `extend_aspect_ratio` | `exar` | 布尔值, gravity | 是否按宽高比扩展画布 |

### 缩放类型说明
- `fit`：等比缩放，图片完全放入目标尺寸内
- `fill`：缩放并裁剪，填满目标尺寸
- `fill-down`：同 `fill`，但只缩小不放大
- `force`：强制拉伸至目标尺寸
- `auto`：根据 gravity 选择 `fit` 或 `fill`

## 二、裁切与定位

| 选项 | 缩写 | 参数 | 说明 |
|------|------|------|------|
| `crop` | `c` | width, height, gravity | 先裁切再缩放 |
| `gravity` | `g` | type, x_offset, y_offset | 裁切/定位引导 |
| `gravity_type` | `gt` | 类型 | 重力类型（`no`/`sm`/`fp`/`so`/`ce`/`no`/`ea`/`we` 等） |
| `gravity_x` | `gx` | 偏移量 | 重力 X 偏移 |
| `gravity_y` | `gy` | 偏移量 | 重力 Y 偏移 |
| `trim` | `t` | threshold, color, equal_hor, equal_ver | 自动裁剪背景边距 |
| `padding` | `pd` | top, right, bottom, left | 内边距（CSS 语法） |

### 重力类型详解
- `no`：无（默认）
- `ce`：居中
- `ea`：东（右）
- `we`：西（左）
- `no`：北（上）
- `so`：南（下）
- `noea`：东北（右上）
- `nowe`：西北（左上）
- `soea`：东南（右下）
- `sowe`：西南（左下）
- `sm`：智能裁剪（自动检测最感兴趣区域）
- `fp`：焦点（x:y），需配合 `gravity_x`/`gravity_y` 指定焦点坐标

## 三、旋转与翻转

| 选项 | 缩写 | 参数 | 说明 |
|------|------|------|------|
| `auto_rotate` | `ar` | 布尔值 | 是否根据 EXIF 数据自动旋转，默认 true |
| `rotate` | `rot` | 角度 | 旋转角度（0/90/180/270 等） |
| `flip` | `fl` | horizontal, vertical | 水平/垂直翻转 |

## 四、颜色与背景

| 选项 | 缩写 | 参数 | 说明 |
|------|------|------|------|
| `background` | `bg` | R:G:B 或 hex 颜色 | 背景色填充（如 `255:0:0` 或 `ff0000`） |

## 五、滤镜与特效

| 选项 | 缩写 | 参数 | 说明 |
|------|------|------|------|
| `blur` | `bl` | sigma | 高斯模糊，sigma 值越大越模糊 |
| `sharpen` | `sh` | sigma | 锐化，sigma 值控制锐化程度 |
| `pixelate` | `pix` | size | 像素化，size 控制像素块大小 |

## 六、水印

| 选项 | 缩写 | 参数 | 说明 |
|------|------|------|------|
| `watermark` | `wm` | opacity, position, x_offset, y_offset, scale | 添加水印 |

### 水印位置
- `ce`：居中
- `no`：北（上中）
- `noea`：东北（右上）
- `ea`：东（右中）
- `soea`：东南（右下）
- `so`：南（下中）
- `sowe`：西南（左下）
- `we`：西（左中）
- `nowe`：西北（左上）
- `re`：重复平铺

水印图片需通过 `IMGPROXY_WATERMARK_*` 环境变量配置。

## 七、元数据与输出

| 选项 | 缩写 | 参数 | 说明 |
|------|------|------|------|
| `strip_metadata` | `sm` | 布尔值 | 是否移除元数据，默认 true |
| `keep_copyright` | `kcr` | 布尔值 | 是否保留版权信息 |
| `strip_color_profile` | `scp` | 布尔值 | 是否移除 ICC 颜色配置，默认 true |
| `enforce_thumbnail` | `eth` | 布尔值 | 强制使用嵌入式缩略图（heic/avif） |
| `quality` | `q` | 0-100 | 输出质量百分比 |
| `format_quality` | `fq` | format1:quality1:format2:quality2:... | 按格式分别设置质量 |
| `max_bytes` | `mb` | 字节数 | 自动降质直到不超过指定大小 |
| `format` | `f` / `ext` | 格式名 | 输出格式（jpg/png/webp/avif/gif/tiff/jxl 等） |
| `cachebuster` | `cb` | 任意字符串 | 缓存破坏器 |
| `expires` | `exp` | Unix 时间戳 | URL 过期时间 |
| `filename` | `fn` | filename, encoded | 设置下载文件名 |
| `return_attachment` | `att` | 布尔值 | 强制 Content-Disposition: attachment 下载 |
| `preset` | `pr` | 预设名称 | 应用预设配置 |
| `skip_processing` | `skp` | extension1, extension2,... | 指定格式跳过处理，保持原样 |
| `raw` | `raw` | 布尔值 | 直接流式输出原图，不做处理 |
| `max_src_resolution` | `msr` | 像素数（Mpx） | 覆盖全局最大源分辨率限制 |
| `max_src_file_size` | `msfs` | 字节数 | 覆盖全局最大源文件大小 |
| `max_animation_frames` | `maf` | 帧数 | 覆盖全局最大动画帧数 |
| `max_animation_frame_resolution` | `mafr` | 像素数 | 覆盖全局最大动画帧分辨率 |
| `max_result_dimension` | `mrd` | 像素数 | 覆盖全局结果图最大尺寸 |

> 备注：`max_src_resolution`、`max_src_file_size`、`max_animation_frames`、`max_animation_frame_resolution`、`max_result_dimension` 属于安全选项，需设置 `IMGPROXY_ALLOW_SECURITY_OPTIONS=true` 才能在 URL 中使用。

## 八、输出格式

在 URL 末尾追加：
- 纯文本模式：`@扩展名`，如 `@png`
- Base64/加密模式：`.扩展名`，如 `.webp`

支持的输出格式取决于编译配置：
- `jpg`（JPEG）
- `png`（PNG）
- `webp`（WebP）
- `avif`（AVIF）
- `gif`（GIF）
- `tiff`（TIFF）
- `heic`（HEIC）
- `jxl`（JPEG XL）

若省略扩展名，imgproxy 会尽量保持源格式输出；若源格式不支持输出，则默认输出 JPEG。
