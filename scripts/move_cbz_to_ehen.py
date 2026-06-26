#!/usr/bin/env python3
"""
将 /onas/16t4/archived/ 下的 CBZ 文件迁移到 /onas/16t4/ehen/
目录结构: ehen/{category}/{uploader}/{gallery_id}-{file_name}.cbz
并更新 noco21.eh_gallery-260620 表的 reader_url 字段。

用法:
  set -a; source .env; set +a
  python3 scripts/move_cbz_to_ehen.py --dry-run    # 预览
  python3 scripts/move_cbz_to_ehen.py               # 执行
"""

import os
import sys
import re
import json
import argparse
import urllib.parse
from urllib.request import Request, urlopen

import psycopg2

ARCHIVED_DIR = "/onas/16t4/archived"
EHEN_DIR = "/onas/16t4/ehen"
META_WEBHOOK = "https://n8n.c.gatepro.cn/webhook/search_gallery_by_id_or_filename"

DB_HOST = os.environ.get("PGHOST", "64.186.236.113")
DB_PORT = int(os.environ.get("PGPORT", "5432"))
DB_USER = os.environ.get("PGUSER", "n8n")
DB_PASS = os.environ.get("PGPASSWORD", "")
DB_NAME = os.environ.get("PGDATABASE", "noco21")

# 标准 e-hentai categories
KNOWN_CATEGORIES = {
    "Misc", "Doujinshi", "Manga", "Artist CG", "Game CG",
    "Image Set", "Cosplay", "Western", "Non-H", "Private",
}

TARGET_FILE_MODE = 0o666
TARGET_DIR_MODE = 0o777


def db_connect():
    return psycopg2.connect(
        host=DB_HOST, port=DB_PORT, user=DB_USER,
        password=DB_PASS, dbname=DB_NAME,
    )


def parse_filename(cbz_path):
    """解析 CBZ 文件名，支持三种格式:
    1) {gid}_{token}-{file_name}.cbz  (完整格式)
    2) {gid}_{token}.cbz              (无 file_name)
    3) 其他                             (不可解析，返回原始文件名)
    """
    name = os.path.basename(cbz_path)
    if name.endswith(".cbz"):
        name = name[:-4]

    # 格式1: {gid}_{token}-{file_name}
    m = re.match(r"^(\d+)_([0-9a-fA-F]+)-(.*)$", name)
    if m:
        return {
            "gallery_id": m.group(1),
            "token": m.group(2),
            "file_name": m.group(3).strip(),
        }

    # 格式2: {gid}_{token} (无 file_name)
    m = re.match(r"^(\d+)_([0-9a-fA-F]+)$", name)
    if m:
        return {
            "gallery_id": m.group(1),
            "token": m.group(2),
            "file_name": "",
        }

    # 格式3: 无法解析，返回整个名称作为 file_name 用于 webhook 检索
    return {
        "gallery_id": None,
        "token": None,
        "file_name": name,
    }


def query_metadata_by_url(conn, gallery_id, token):
    """通过 e-hentai URL 精确查询 eh_page-260604，返回 category、uploader、title"""
    url = f"https://e-hentai.org/g/{gallery_id}/{token}/"
    with conn.cursor() as cur:
        cur.execute(
            'SELECT category, uploader, title FROM "eh_page-260604" WHERE url = %s', (url,)
        )
        row = cur.fetchone()
    if row and row[0]:
        return {"category": row[0].strip(), "uploader": (row[1] or "").strip(), "title": row[2]}
    return None


def query_metadata_by_gid(conn, gallery_id):
    """仅通过 gallery_id 模糊查询 eh_page-260604（token 不匹配但 gid 相同）"""
    pattern = f"%e-hentai.org/g/{gallery_id}/%"
    with conn.cursor() as cur:
        cur.execute(
            'SELECT category, uploader, title, url FROM "eh_page-260604" WHERE url LIKE %s LIMIT 1',
            (pattern,),
        )
        row = cur.fetchone()
    if row and row[0]:
        return {"category": row[0].strip(), "uploader": (row[1] or "").strip(), "title": row[2], "url": row[3]}
    return None


def query_metadata_by_title(conn, file_name):
    """通过 file_name 模糊匹配 eh_page-260604 的 title 字段，返回含 url 的完整信息"""
    keyword = file_name[:50] if len(file_name) > 50 else file_name
    pattern = f"%{keyword}%"
    with conn.cursor() as cur:
        cur.execute(
            'SELECT category, uploader, title, url FROM "eh_page-260604" WHERE title LIKE %s LIMIT 1',
            (pattern,),
        )
        row = cur.fetchone()
    if row and row[0]:
        return {"category": row[0].strip(), "uploader": (row[1] or "").strip(), "title": row[2], "url": row[3]}
    return None


def call_webhook(gallery_id=None, file_name=None, gallery_token=None):
    """调用 n8n meta API webhook（同步模式），返回解析后的元数据。
    
    支持三种查询：
    - gallery_id 查询             → 返回扁平 record
    - gallery_id + gallery_token  → 返回扁平 record（更精确）
    - file_name 查询              → 返回 {"match": {...}}
    
    返回统一格式: {category, uploader, title, url, gallery_id}
    """
    body = {}
    if gallery_id:
        body["gallery_id"] = int(gallery_id)
    if gallery_token:
        body["gallery_token"] = gallery_token
    if file_name:
        body["file_name"] = file_name
    data = json.dumps(body).encode()
    req = Request(META_WEBHOOK, data=data,
                  headers={"Content-Type": "application/json", "Accept": "application/json"}, method="POST")
    try:
        with urlopen(req, timeout=120) as resp:
            raw = resp.read().decode()
            if not raw or not raw.strip():
                return None
            result = json.loads(raw)

            # 格式1: {"match": {...}}  (file_name 查询)
            if isinstance(result, dict) and "match" in result and result["match"]:
                item = result["match"]
                return {
                    "category": item.get("category", "").strip(),
                    "uploader": item.get("uploader", "").strip(),
                    "title": item.get("title", "").strip(),
                    "url": item.get("url", "").strip(),
                    "gallery_id": str(item.get("gallery_id", "")),
                }

            # 格式2: 扁平 record (gallery_id / gallery_id+token 查询)
            if isinstance(result, dict) and "id" in result:
                return {
                    "category": result.get("category", "").strip(),
                    "uploader": result.get("uploader", "").strip(),
                    "title": result.get("title", "").strip(),
                    "url": result.get("url", "").strip(),
                    "gallery_id": str(result.get("id", "")),
                }

            # 格式3: 数组
            if isinstance(result, list) and result:
                item = result[0].get("json", result[0])
                gid = item.get("id") or item.get("gallery_id", "")
                return {
                    "category": item.get("category", "").strip(),
                    "uploader": item.get("uploader", "").strip(),
                    "title": item.get("title", "").strip(),
                    "url": item.get("url", "").strip(),
                    "gallery_id": str(gid) if gid else "",
                }

            return None
    except Exception:
        return None


def decode_uploader(uploader):
    """URL 解码 uploader 名称，处理数据库中的百分号编码"""
    if not uploader:
        return "unknown"
    if "%" in uploader:
        try:
            decoded = urllib.parse.unquote(uploader)
            return decoded
        except Exception:
            pass
    return uploader


def safe_path_component(name):
    """处理文件系统不安全字符，替换 / 和不可见字符"""
    if not name:
        return "unknown"
    name = name.replace("/", "_")
    name = name.replace("\x00", "")
    return name.strip()


def extract_gid_from_url(url):
    """从 e-hentai URL 中提取 gallery_id，如 https://e-hentai.org/g/3980433/51956f6e57/ -> 3980433"""
    if not url:
        return None
    m = re.search(r"e-hentai\.org/g/(\d+)/", url)
    return m.group(1) if m else None


def update_reader_url(conn, gallery_id, reader_url):
    """更新 eh_gallery-260620 表的 reader_url"""
    with conn.cursor() as cur:
        cur.execute(
            'UPDATE "eh_gallery-260620" SET reader_url = %s WHERE gallery_id = %s',
            (reader_url, gallery_id),
        )
        return cur.rowcount


def main():
    parser = argparse.ArgumentParser(description="CBZ 归档迁移工具")
    parser.add_argument("--dry-run", action="store_true", help="预览模式，不实际移动文件")
    args = parser.parse_args()

    if not DB_PASS:
        print("ERROR: PGPASSWORD 环境变量未设置")
        sys.exit(1)

    conn = db_connect()

    # 扫描 archived 目录下所有 .cbz 文件
    cbz_files = sorted([
        os.path.join(ARCHIVED_DIR, f)
        for f in os.listdir(ARCHIVED_DIR)
        if f.endswith(".cbz")
    ])
    total = len(cbz_files)
    print(f"扫描到 {total} 个 CBZ 文件")
    print(f"{'[DRY-RUN 模式]' if args.dry_run else '[执行模式]'}\n")

    stats = {
        "moved": 0, "db_updated": 0, "db_no_record": 0,
        "exists": 0, "no_meta": 0, "webhook": 0, "error": 0,
    }

    for i, src_path in enumerate(cbz_files):
        info = parse_filename(src_path)
        gid = info["gallery_id"]
        token = info["token"]
        fname = safe_path_component(info["file_name"]) if info["file_name"] else ""

        # === 获取元数据：DB 优先，webhook 同步兜底 ===
        meta = None

        if gid and token:
            meta = query_metadata_by_url(conn, gid, token)

        if not meta and gid:
            meta = query_metadata_by_gid(conn, gid)

        if not meta and fname:
            meta = query_metadata_by_title(conn, fname)

        if not meta:
            # 策略4: 同步 webhook（已改为 responseNode 模式）
            identifier = f"gallery_id={gid}" if gid else f"file_name={fname[:40]}"
            print(f"[WEBHOOK] 查询 {identifier}")
            meta = call_webhook(gallery_id=gid, file_name=fname if fname else None, gallery_token=token)
            stats["webhook"] += 1
            if meta:
                print(f"  -> webhook 返回成功")
                # webhook 返回的 gid 可能和文件名中的不同，用 webhook 的精确值
                if not gid and meta.get("gallery_id"):
                    gid = meta["gallery_id"]
                elif meta.get("gallery_id") and meta["gallery_id"] != gid:
                    pass  # 保持原来的 gid，webhook 的 gid 可能更准但没有 token

        if not meta:
            label = f"gid={gid}" if gid else fname[:40]
            print(f"[NO-META] {label} (元数据缺失，跳过)")
            stats["no_meta"] += 1
            continue

        # 从 URL 中提取 gid（file_name 查询时可能没有 gid）
        if not gid and meta.get("url"):
            gid = extract_gid_from_url(meta["url"])

        # 从元数据中补充 file_name（无 file_name 的格式如 {gid}_{token}.cbz）
        if not fname and meta.get("title"):
            fname = safe_path_component(decode_uploader(meta["title"]))

        category = safe_path_component(meta["category"])
        uploader_raw = decode_uploader(meta["uploader"])
        uploader = safe_path_component(uploader_raw)

        if not category:
            label = f"gid={gid}" if gid else fname[:40]
            print(f"[NO-CAT] {label} (category 为空)")
            stats["no_meta"] += 1
            continue

        # === 构造目标路径 ===
        if fname:
            target_filename = f"{gid}-{fname}.cbz" if gid else f"{fname}.cbz"
        else:
            target_filename = f"{gid}.cbz"
        target_dir = os.path.join(EHEN_DIR, category, uploader)
        target_path = os.path.join(target_dir, target_filename)

        # 检查是否已存在
        if os.path.exists(target_path):
            reader_url = f"/ehen/{category}/{uploader}/{target_filename}"
            if not args.dry_run and gid:
                updated = update_reader_url(conn, gid, reader_url)
                if updated > 0:
                    print(f"[EXISTS] {category}/{uploader}/{target_filename} (DB OK)")
                    stats["db_updated"] += 1
                else:
                    print(f"[EXISTS] {category}/{uploader}/{target_filename} (no DB)")
                    stats["db_no_record"] += 1
                conn.commit()
            else:
                print(f"[EXISTS] {category}/{uploader}/{target_filename}")
            stats["exists"] += 1
            continue

        # === 执行或预览 ===
        reader_url = f"/ehen/{category}/{uploader}/{target_filename}"

        if args.dry_run:
            print(f"[DRY-RUN] {os.path.basename(src_path)} -> {category}/{uploader}/{target_filename}")
            stats["moved"] += 1
        else:
            try:
                # 创建目标目录（含父目录），设置权限
                os.makedirs(target_dir, mode=TARGET_DIR_MODE, exist_ok=True)

                # 原子移动
                os.rename(src_path, target_path)

                # 设置文件权限为 rw-rw-rw-（允许访问和删除）
                os.chmod(target_path, TARGET_FILE_MODE)

                # 更新数据库（仅当有 gid 时）
                if gid:
                    updated = update_reader_url(conn, gid, reader_url)
                    conn.commit()
                    if updated > 0:
                        stats["db_updated"] += 1
                        print(f"[OK] {category}/{uploader}/{target_filename}")
                    else:
                        stats["db_no_record"] += 1
                        print(f"[OK] {category}/{uploader}/{target_filename} (无 DB 记录)")
                else:
                    print(f"[OK] {category}/{uploader}/{target_filename} (无 gid, 未更新 DB)")

                stats["moved"] += 1

            except Exception as e:
                print(f"[ERR] {os.path.basename(src_path)}: {e}")
                conn.rollback()
                stats["error"] += 1

        # 进度提示
        if (i + 1) % 50 == 0:
            print(f"--- 进度: {i + 1}/{total} ---")

    conn.close()

    # === 统计报告 ===
    print(f"\n{'='*50}")
    print(f"  {'DRY-RUN' if args.dry_run else '执行'} 完成")
    print(f"  总文件: {total}")
    print(f"  已移动: {stats['moved']}")
    print(f"  已存在: {stats['exists']}")
    print(f"  无元数据: {stats['no_meta']}")
    print(f"  Webhook 触发: {stats['webhook']}")
    print(f"  DB 已更新: {stats['db_updated']}")
    print(f"  DB 无记录: {stats['db_no_record']}")
    print(f"  错误: {stats['error']}")
    print(f"{'='*50}")


if __name__ == "__main__":
    main()
