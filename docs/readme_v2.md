# 项目状态闭环 + 归档画廊展示（v2）

> 本文档记录 2026-06-19 ~ 2026-06-20 完成的系统升级：
> 1. 补全 aria2/imgproxy_plus → noco21 的状态闭环
> 2. 新建 `eh_gallery-260620` 表展示归档画廊（含封面 + 阅读器深链）
> 3. n8n 两个工作流：状态同步、归档入册
> 4. imgproxy_plus 加 webhook 通知归档完成
> 5. 回填 220 个现有 CBZ
>
> 同时记录开发过程中踩过的坑与经验，作为团队知识沉淀。

---

## 一、最终交付物

### 1.1 组件清单

| 组件 | 位置 | 状态 |
|---|---|---|
| NocoBase 表 `eh_gallery-260620` | noco21.public | ✅ 221 行 |
| NocoBase UI「🖼️ 归档画廊」+「📖 归档画廊管理」 | noco.c.gatepro.cn 桌面 | ✅ GridCard 封面图+标题（已修复 subKey）+ Table |
| n8n 工作流 `aria2-status-sync` (id `FgOaPqRJbZySpxgN`) | n8n.c.gatepro.cn | ✅ ACTIVE，每 5min |
| n8n 工作流 `gallery-archive-complete` (id `yapWLNgBVDzmBTo7`) | n8n.c.gatepro.cn | ✅ ACTIVE，webhook |
| imgproxy_plus webhook 代码 | `/opt/opencode/imgproxy_plus` | ✅ 代码就绪，**待 QNAP docker compose build** |
| imgproxy_plus `.env` 修复 | 同上 `.env` | ✅ 已修 |
| `aria2_status` enum 加 `archived` | eh_page-260604 / eh_torrent-260604 | ✅ |

### 1.2 数据现状（2026-06-20）

```
eh_gallery-260620: 221 行（220 backfilled + 1 archived）
eh_torrent-260604: total=1761  archived=214  active=39  ok=182
eh_page-260604:    total=2525  archived=214  active=0   ok=225
```

### 1.3 状态流转全景

```
scrape-torrent 写入 aria2_gid + status='ok'
        ↓
aria2 下载: active → complete → removed
        ↓
[工作流 A 每 5min] aria2-status-sync
   查 tellActive + tellWaiting → 以 gid 为 key → UPDATE eh_torrent/eh_page.aria2_status
        ↓
imgproxy_plus 归档完成（engine.go ProcessOne 末尾）
        ↓
[webhook] POST https://n8n.c.gatepro.cn/webhook/gallery-archive-complete
   payload: {source_dir, gallery_id, gallery_token, cbz[], total_pages, ...}
        ↓
[工作流 B] gallery-archive-complete
   ① Build payload: 拼 cover_url + reader_url
   ② Lookup eh_page by url
   ③ Lookup eh_torrent by page_id
   ④ Upsert eh_gallery-260620
   ⑤ Mark torrent/page → aria2_status='archived'
        ↓
NocoBase UI: 🖼️ 归档画廊（GridCard，封面 + 标题 + 状态）
```

---

## 二、关键设计决策

### 2.1 aria2 完成信号 → DB 映射

| 候选键 | 可靠性 | 备注 |
|---|---|---|
| `infoHash` → `eh_torrent.hash` | ❌ 5/16 | workflow 改了 torrent 内嵌 `4:name` 字段，BT infohash 重算 |
| 解析目录名 `{gid}_{token}` | ⚠️ 14/16 | 2 个异常（裸 `/downloads`、跨画廊） |
| **`aria2_gid` → `eh_torrent.aria2_gid`** | ✅ 15/16 | **唯一可靠键**，1 个 NULL 是数据 bug |

→ 工作流 A 以 `aria2_gid` 为匹配键。

### 2.2 imgproxy_plus 归档完成信号

| 方案 | 评估 | 选择 |
|---|---|---|
| 轮询 `/api/archive-status` ring buffer | 限 200 条，busy 时丢事件 | ❌ |
| 改源码加 webhook（engine.go:147） | 可靠，~65 行 go 代码 | ✅ |

→ engine.go `ProcessOne` 末尾（`LogEvent("OK","done",...)` 之后）调 `FireCompleteWebhook`，
   payload 带 `source_dir`（`{gid}_{token}`），gallery_id/token 由 `strings.SplitN(source_dir,"_",2)` 派生。

### 2.3 `eh_gallery-260620` 字段设计

仿 `eh_page-260604`，去爬虫字段，加归档字段：
- 业务键 `gallery_id`（建 UNIQUE INDEX）
- `cover_url` / `reader_url` / `cbz_name` / `cbz_names` 用 **`text`** 类型（URL-encoded CBZ 名可能超 255 字符）
- `page_id` m2o → eh_page-260604，复用物理列 `page_id`
- `archive_status` enum: `archived` / `failed` / `backfilled`

### 2.4 imgproxy_plus URL 模式

```
封面:  https://q.ws.gatepro.cn:99/gly/zip/ssd1/aria2/archived/{cbz}/__cover.jfif
阅读器: https://q.ws.gatepro.cn:99/gly/or-gallery?path={URL-encoded-full-path}
```

- `cover_url`：CBZ 名用 `encodeURI`（slash-safe）
- `reader_url`：整个 path 用 `encodeURIComponent` **一次**（CBZ 名不能预编码，否则 `%5B` → `%255B` 双重编码）
- 缺 `__cover.jfif` 的 CBZ fallback 到第一张 `.jfif`（65/220 个早期归档版本无 cover）

### 2.5 imgproxy_plus 实际鉴权

配置了 Basic Auth + IP 白名单，但 **APISIX 容器 IP 在白名单 `172.16/12` 内，对外实际免认证**。
NocoBase `<img>` 标签可直接引用 URL，浏览器无障碍访问。

---

## 三、踩过的坑（重要经验）

### 3.1 n8n 工作流陷阱

#### 坑 1：HTTP Request v4.1 body 格式

**错误**：用 `bodyParameters.parameters`（v3 格式）→ 实际 body 为空 → aria2 报错。

**正确**：
```json
{
  "sendBody": true,
  "contentType": "json",
  "specifyBody": "json",
  "jsonBody": "<raw JSON string>"
}
```

#### 坑 2：Webhook `responseMode: onReceived` 导致后台执行失败

**现象**：webhook 立即返回 200，但工作流后台执行报 `error`，DB 不更新。
**根因**：`onReceived` 模式下 n8n 立即响应后，执行上下文不稳定。
**解决**：改用 `responseMode: responseNode` + 末尾加 `respondToWebhook` 节点。
**教训**：webhook 工作流**一律用 responseNode 模式**，不要用 onReceived。

#### 坑 3：节点数据传递 - Postgres 输出 vs 上游节点数据

Postgres 节点（update/insert）执行后，下游节点拿到的 `$json` 是 **DB 返回值**，不是上游数据。
要在 update 节点里用上游 Code 节点的字段，必须用 **node reference**：
```javascript
valueToMatchOn: "={{ $('Final record').first().json.torrent_id }}"
```
而不是 `={{ $json.torrent_id }}`（这是 Postgres 自己的输出）。

#### 坑 4：SplitInBatches 无 loop-back 只处理第一批

SplitInBatches 需要「loop back」连接才能处理所有批次。线性链式连接只处理第一批。
**解决**：对 ≤100 条数据**直接用 Code 节点 `runOnceForAllItems`** + node reference join，不要用 SplitInBatches。

#### 坑 5：n8n Public API 限制

- `POST /workflows/:id/run` 不支持（无法 API 触发测试）
- `POST /workflows/:id/activate` 是激活端点（不是 PATCH）
- GET-then-PUT 模式失败：PUT 拒绝额外字段（`staticData`, `versionId`, `shared` 等）
- **必须**用 POST 创建新工作流，不能 PUT 改已有工作流的 node 数量等

#### 坑 6：webhook 路径冲突

删除工作流后，webhook 注册可能未释放，重建同 path 的工作流激活时报 409。
**解决**：要么换 webhookId，要么等 n8n GC；最稳妥是**不要重复创建同 path 工作流**。

### 3.2 NocoBase 经验

#### 坑 7：BYTEA 字段不被支持

`eh_torrent-260604.torrent_data` (BYTEA) 被 NocoBase 标记 `unsupportedFields`，
导致该集合的 `?fields=` 过滤报 `Invalid SQL column`。
**解决**：查询时不指定 fields，或用 n8n Postgres 节点直连 SQL（绕过 NocoBase API）。

#### 坑 8：nocobase-config skill 的 `X-Free-Auth` 认证

设置 `NOCO_API_KEY=sk-Aria2` 时，skill 自动用 `X-Free-Auth` 头（不是 Bearer Token）。
这是 NocoBase 的「免认证」模式，只读访问 OK，但写入受限。

#### 坑 9：建表时 URL/长文本字段类型

NocoBase `url` interface 默认创建 `varchar(255)`。CBZ 文件名 URL-encoded 后可能超 255。
**解决**：建表后 `ALTER COLUMN ... TYPE text`。

#### 坑 13：GridCard 卡片内容空白（subKey 用错）

**现象**：🖼️ 归档画廊页面卡片容器渲染正常（9 张 `.ant-card`、221 条分页、`eh_gallery-260620:list` 返回 200），
但每张卡片**内容全空白**（无封面、无标题），控制台无报错。一度怀疑是数据问题，但数据 `title`/`cover_url` 都有值。

**根因**：`GridCardItemModel` 被 attach 到了 `GridCardBlockModel` 的 `items`（复数，subType=array），
正确应为 `item`（单数，subType=object）。错误的 subKey 导致卡片模板无法被 FlowEngine 正确 fork，
其下的 `DetailsGridModel` / `JSFieldModel` 全部不渲染、JS 不执行。

**伴随问题**：卡片树里还混入了不存在的 `use: "GridCardCoverModel"`（→ fallback 到 ErrorFlowModel），
以及一批缺 `jsSettings` 的残缺 `JSFieldModel`（只绑了字段路径，无渲染代码 → 静默空白）。

**修复范式**（参照页面 `rs3guz59xm1` / tab `rqski5886hq`）：
1. `flowModels:destroy` 删除无效 `GridCardCoverModel`
2. `flowModels:attach` 把 `GridCardItemModel` re-attach 到 `item/object`
3. 重建 `DetailsGrid → DetailsItem(field=title) → JSField(field=title)` 三层结构
4. JSField 用 React JSX + `ctx.record` 渲染封面图：`<img src={ctx.record.cover_url}>` + 标题，末尾 `ctx.render(<El/>)`
5. 最终 9/9 卡片封面加载成功

> **判断要点**：卡片容器在、数据请求 200、却内容空白 + 无报错 → 几乎必然是 subKey 用错（item vs items）或 JSField 缺 jsSettings。
> 详见 nocobase skill `references/page-types.md`「卡片内容字段渲染」「P5/P6/P7 陷阱」。

### 3.3 数据工程经验

#### 坑 10：URL 双重编码

Python `urllib.parse.quote(cbz)` 后再 `urllib.parse.quote(path)` 会对 `%5B` 二次编码成 `%255B`。
**正确写法**：`urllib.parse.quote(full_path, safe='/%')` 一次性编码。

#### 坑 11：BT infohash 不等于 DB hash

爬虫 workflow 在 `changeTorrentFilename` 步骤里改了 torrent 的 `4:name` 字段，
导致 BT infohash 重算。DB 存的是改前 hash，aria2 用的是改后 hash。
**永远不要用 infohash 做映射键**，用 `aria2_gid`。

#### 坑 12：早期归档无 cover

imgproxy_plus 早期版本未生成 `__cover.jfif`（65/220 CBZ）。
回填脚本必须 fallback 到 CBZ 内第一张图（通过 imgproxy_plus API ls 查第一张 `.jfif`）。

---

## 四、部署 / 运维指南

### 4.1 imgproxy_plus 部署（QNAP）

```bash
cd /opt/opencode/imgproxy_plus
# 代码改动文件：
#   .env (GALLERY_ARCHIVE_DIR, GALLERY_COMPLETE_WEBHOOK_URL)
#   internal/config/config.go (+GalleryCompleteWebhookURL 字段 + Load)
#   internal/archive/webhook.go (新文件)
#   internal/archive/engine.go (FireCompleteWebhook 调用点 line 152)
docker compose build && docker compose up -d
```

构建后，归档完成自动 POST webhook，无需手动触发。

### 4.2 n8n 工作流

两个工作流已 ACTIVE，**无需手动维护**：
- `aria2-status-sync`：cron 每 5 分钟
- `gallery-archive-complete`：被动 webhook

监控：n8n UI → Executions，过滤 workflowId 查成功率。

### 4.3 回填脚本（一次性）

`/tmp/backfill.py`（已在本次执行完，备份留存）：
- 扫 `/ssd1/aria2/archived/*.cbz`
- 解析 `{gid}_{token}` 前缀
- upsert eh_gallery + 标记 torrent/page 为 archived
- 缺 cover 的 fallback 到第一张图

如需重跑（新增 CBZ 后），调整 SQL 的 ON CONFLICT 逻辑即可。

### 4.4 NocoBase UI

访问 `https://noco.c.gatepro.cn`：
- 桌面 → 📚 EH 图库 → 🖼️ 归档画廊（GridCard 视图，含封面）
- 桌面 → 📚 EH 图库 → 📖 归档画廊管理（Table 视图，全字段）

封面图列直接从 imgproxy_plus 加载，无需认证。

---

## 五、文件清单

```
/opt/opencode/imgproxy_plus/
├── .env                              # ← 修改：GALLERY_ARCHIVE_DIR + WEBHOOK_URL
├── internal/config/config.go         # ← 修改：+GalleryCompleteWebhookURL
├── internal/archive/
│   ├── webhook.go                    # ← 新增：归档完成 webhook（~65 行）
│   └── engine.go                     # ← 修改：line 152 调用 FireCompleteWebhook
├── docs/sql/
│   └── eh_gallery-260620.sql         # ← 新增：DDL 文档
└── docs/
    └── readme_v2.md                  # ← 本文档

n8n.c.gatepro.cn:
├── aria2-status-sync (FgOaPqRJbZySpxgN)         # 新建
└── gallery-archive-complete (yapWLNgBVDzmBTo7)  # 新建

noco21.public:
└── eh_gallery-260620                              # 新建表 + UNIQUE INDEX
```

---

## 六、后续优化建议

| 优先级 | 项 | 说明 |
|---|---|---|
| 高 | 工作流 A 监控 `aria2 complete/error/removing` 状态 | 当前只同步 active+waiting，complete 状态因 tellStopped 为空无法捕获（需 move.sh hook） |
| 中 | imgproxy_plus 加 `/or-gallery?cbz=xxx` 深链路由 | 当前 reader_url 是目录入口，深链到具体 CBZ 需改 SPA JS |
| 中 | NocoBase eh_gallery 加「阅读」按钮 | customRequest 打开 reader_url（仿 eh_page 的下载按钮） |
| 低 | 修复 990 个未提交种子 | 工作流 A 只同步已提交的，未提交的需 scrape-torrent 重跑 |
| 低 | 修复 1 行 `aria2_status='c457cb77782c8228'` 数据 bug | GID 误写入 status 字段 |
| 低 | imgproxy_plus 修复重复 `detectAnimated` 实现 | 3 份不同实现，drift 风险 |
