# 2026-06-27 更新记录

## ehen CBZ 自动路由集成

### 背景

`/onas/16t4/archived/` 下的 CBZ 文件需要按分类移动到 `/onas/16t4/ehen/{category}/{uploader}/{gallery_id}-{file_name}.cbz`，
并更新 noco21 数据库的 `reader_url` 和 `cover_url`。

### 迁移执行（6月26日）

用 Python 脚本 `scripts/move_cbz_to_ehen.py` 一次性迁移了 754 个 CBZ 文件：

| 项目 | 数量 |
|------|------|
| 总文件 | 755 |
| 成功迁移 | 754 |
| 跳过(无元数据) | 1（webhook 修复后补迁完成） |
| DB reader_url 更新 | 218 |
| DB cover_url 更新 | 221 |
| 权限设置 | 0666 (文件) / 0777 (目录) |
| archived 目录清空 | ✓ |

### Go 集成

Python 脚本逻辑完整移植到 Go：`internal/archive/ehen.go`

**核心函数：** `RouteCBZToEhen(db, cbzPath, cfg) *ehenStats`

**元数据解析级联策略：**
1. DB `eh_page-260604` URL 精确匹配
2. DB `eh_page-260604` gid 模糊匹配
3. DB `eh_page-260604` title 模糊匹配
4. n8n webhook 同步查询 (`gallery_id` + `gallery_token` + `file_name`)

**集成位置：** `engine.go:ProcessOne()` — 每个 `PackCBZ` 生成后立即调用，CBZ 直接路由到 ehen 分类目录。

**新增环境变量：**
- `EHEN_ENABLED=true`
- `EHEN_DIR=/data/ehen`
- `EHEN_META_WEBHOOK=https://n8n.c.gatepro.cn/webhook/search_gallery_by_id_or_filename`

**新增依赖：** `github.com/lib/pq` v1.12.3 (纯 Go PgSQL 驱动，兼容 CGO_ENABLED=0)

### 修改文件清单

| 文件 | 变更 |
|------|------|
| `internal/archive/ehen.go` | **新增** — 元数据解析 + 文件移动 + DB 更新 |
| `internal/archive/engine.go` | 添加 `database/sql` 导入；`ProcessOne` 中调用 `RouteCBZToEhen` |
| `internal/config/config.go` | 新增 `EhenEnabled/EhenDir/EhenMetaWebhook` + PG 配置字段 |
| `go.mod` | 新增 `github.com/lib/pq` v1.12.3 |
| `go.sum` | 更新 |
| `.env` | 新增 `EHEN_*` 配置 |
| `.env.example` | 新增 `EHEN_*` + `PG*` 配置模板 |
| `AGENTS.md` | 新增 ehen 自动路由文档 |
| `scripts/move_cbz_to_ehen.py` | 迁移脚本（保留备用） |

### webhook API 变化

`search_gallery_by_id_or_filename` webhook 已改为**同步模式**（`responseNode`），直接返回 JSON：

- `{"gallery_id": N}` → 返回扁平 record（`id/category/uploader/title/url`）
- `{"gallery_id": N, "gallery_token": "xxx"}` → 同上（更精确）
- `{"file_name": "xxx"}` → 返回 `{"match": {category/uploader/title/url/gallery_id}}`
