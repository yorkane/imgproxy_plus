# imgproxy_plus 经验总结 2026-06-21

## 1. APISIX 路由配置陷阱

### Host 端口匹配
edge 网关可能剥离 `:99` 端口号，导致 Host 头变成不带端口的域名。APISIX 路由的 `hosts` 需要同时包含带端口和不带端口的变体，否则会匹配失败返回 403。

### proxy-rewrite 与 PLUS_URL_PREFIX 双重处理
- APISIX 的 `proxy-rewrite` 和 Go 应用的 `PLUS_URL_PREFIX` 不能同时去前缀
- 如果 Go 层已配置 `PLUS_URL_PREFIX=/gly`，APISIX 路由 **不要** 加 `proxy-rewrite` 去 `/gly`
- 否则请求路径被两次去前缀，Go 层收到不带前缀的路径 → `stripPrefix` 不匹配 → 404

### 路由优先级
```json
"priority": 10  // 更具体的路由给更高优先级
```
当多个路由的 `hosts` 和 `uri` 有重叠时，用 `priority` 控制匹配顺序。

## 2. 归档系统配置

| 环境变量 | 说明 |
|----------|------|
| `GALLERY_AUTO_ENABLED=true` | 启用定期扫描 |
| `GALLERY_SCAN_DIR` | 扫描源目录 |
| `GALLERY_ARCHIVE_DIR` | CBZ 输出目录 (默认 `/data/archived`, 对应 HDD) |
| `GALLERY_SCAN_INTERVAL` | 扫描间隔(秒)，默认 1800 (30分钟) |

CBZ 直接输出到 `GALLERY_ARCHIVE_DIR`，**无需额外移动步骤**。

## 3. 动画 WebP 检测 Bug

**VP8X chunk 结构**（偏移量从文件头开始）:
```
Offset 0-3:  "RIFF"
Offset 4-7:  文件大小 (LE)
Offset 8-11: "WEBP"
Offset 12-15: "VP8X"
Offset 16-19: Chunk Size (LE, 通常 10)
Offset 20:   Flags 字节
  bit 0 (0x01): ICCP
  bit 1 (0x02): ALPHA
  bit 2 (0x04): EXIF
  bit 3 (0x08): XMP
  bit 4 (0x10): ANIMATION ← 正确位
```

**两个 bug 位置**:
- `internal/img/handler.go` `detectAnimated()`: 读 `buf[16]&0x10` → 错误！`buf[16]` 是 Chunk Size，不是 Flags
- `internal/archive/detector.go` `DetectAnimated()`: 读 `buf[20]&0x02` → 错误！`0x02` 是 ALPHA 位，不是 ANIMATION

**正确写法**: `buf[20] & 0x10`

## 4. 跨设备移动目录

`os.Rename` 在不同挂载点/文件系统之间失败:`invalid cross-device link`

**解决方案**: 先尝 rename，失败则 copy + delete
```go
func moveDir(src, dst string) error {
    err := os.Rename(src, dst)
    if err == nil {
        return nil
    }
    // 跨设备错误才 fallback，其他错误直接返回
    if !strings.Contains(err.Error(), "cross-device") {
        return err
    }
    if err := copyDir(src, dst); err != nil {
        return fmt.Errorf("copy: %w", err)
    }
    os.RemoveAll(src)
    return nil
}
```

## 5. Go 语法注意

```go
// ❌ 错误: err 作用域仅限于 if 块
if err := os.Rename(src, dst); err == nil {
    return nil
}
// err 在这里不可见！

// ✅ 正确: err 在函数作用域内
err := os.Rename(src, dst)
if err == nil {
    return nil
}
// err 仍然可访问
```

## 6. 视频播放器键盘冲突隔离

多个视图（reader / gallery / video player）共用键盘事件时:
- `videoOnKey()`: 顶部加 `if (!this.videoPlayerShow) return;` 只在播放器可见时处理
- `onKeydown()`: 顶部加 `if (this.videoPlayerShow) return;` 避免视频播放时触发 gallery/reader 快捷键
- 视频音量用 `ArrowUp`/`ArrowDown` 调节，用 `b`/`B`/`Esc` 返回

## 7. 前端音量持久化

视频音量存 `localStorage` 键 `or-video-volume`，打开新视频时自动恢复:
```js
// open 时恢复
g.videoVolume = LS.int('or-video-volume', 80);
// setVolume 时保存
LS.set('or-video-volume', vol);
```
