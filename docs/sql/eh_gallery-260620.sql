-- ============================================================
-- eh_gallery-260620 : EH 归档画廊 (archived galleries)
-- Created via NocoBase API (collection) + SQL unique index. 2026-06-20
-- Purpose: Records galleries that imgproxy_plus has archived.
--   One row per gallery_id (unique). Multi-CBZ galleries use cbz_names.
-- FK: page_id -> eh_page-260604.id  (m2o, logical name 'page')
-- Upsert key: gallery_id (UNIQUE INDEX eh_gallery-260620_gallery_id_uk)
--
-- NOTE: This table is created/managed via NocoBase, NOT raw SQL.
--       This DDL is for reference/migration/version-control only.
--       The live table lives in noco21.public (NocoBase schema).
--
-- UPDATE 2026-06-20: cover_url, reader_url, cbz_name widened to text
--   because URL-encoded CBZ names can exceed varchar(255).
-- ============================================================

CREATE TABLE IF NOT EXISTS public."eh_gallery-260620" (
    id            bigserial    PRIMARY KEY,
    "createdAt"   timestamptz  DEFAULT now(),
    "updatedAt"   timestamptz  DEFAULT now(),
    "createdById" bigint,
    "updatedById" bigint,

    gallery_id     varchar(255),          -- 画廊ID (e-hentai numeric gid)
    gallery_token  varchar(255),          -- 画廊Token (10 hex)
    url            varchar(255),          -- 原画廊URL
    title          varchar(255),          -- 标题
    title_jpn      varchar(255),          -- 日文标题
    category       varchar(255),          -- 分类 (Doujinshi/Manga/Artist CG/...)
    uploader       varchar(255),          -- 上传者
    rating         double precision,      -- 评分
    page_count     bigint,                -- 页数
    file_size      bigint,                -- 原始字节大小 (from eh_torrent.fsize)
    cbz_name       text,                  -- 主CBZ文件名
    cbz_names      text,                  -- 全部CBZ文件名 (newline-separated)
    cover_url      text,                  -- 封面图URL (imgproxy_plus /zip/.../__cover.jfif)
    reader_url     text,                  -- 阅读器URL (imgproxy_plus /or-gallery?path=...)
    archived_at    timestamptz,           -- 归档时间
    archive_status varchar(255),          -- archived/failed/backfilled
    page_id        bigint                 -- FK -> eh_page-260604.id (no DB constraint, indexed)
);

-- NocoBase auto-creates: _pkey(id), _created_by_id, _updated_by_id, _page_id
CREATE UNIQUE INDEX IF NOT EXISTS "eh_gallery-260620_gallery_id_uk"
    ON public."eh_gallery-260620" (gallery_id);
