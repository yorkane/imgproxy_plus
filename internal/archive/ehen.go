package archive

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"imgproxy_plus/internal/config"

	_ "github.com/lib/pq"
)

// ehenTargetFileMode is the permission set on moved CBZ files (rw-rw-rw-).
const ehenTargetFileMode = 0666

// ehenTargetDirMode is the permission set on created directories (rwxrwxrwx).
const ehenTargetDirMode = 0777

// ehenFilenameRe matches CBZ filenames in the format {gid}_{token}-{file_name}.cbz
// or {gid}_{token}.cbz (no file_name). Groups: (gid) (token) (file_name)
var ehenFilenameRe = regexp.MustCompile(`^(\d+)_([0-9a-fA-F]+)-(.*)$`)

// ehenFilenameNoDashRe matches {gid}_{token}.cbz (no file_name). Groups: (gid) (token)
var ehenFilenameNoDashRe = regexp.MustCompile(`^(\d+)_([0-9a-fA-F]+)$`)

// ehenGIDFromURLRe extracts gallery_id from an e-hentai URL.
var ehenGIDFromURLRe = regexp.MustCompile(`e-hentai\.org/g/(\d+)/`)

// ehenMeta holds resolved metadata for an e-hentai gallery.
type ehenMeta struct {
	Category  string
	Uploader  string
	Title     string
	URL       string
	GalleryID string
}

// ehenStats tracks per-file and aggregate statistics for ehen routing.
type ehenStats struct {
	Moved     int
	DBUpdated int
	NoMeta    int
	Webhook   int
	Errors    int
}

// ehenParsed holds the result of parsing a CBZ filename.
type ehenParsed struct {
	GalleryID string
	Token     string
	FileName  string
}

// parseEHENFilename parses a CBZ filename (without .cbz extension).
// Supports three formats:
//   1) {gid}_{token}-{file_name}  (full format)
//   2) {gid}_{token}              (no file_name)
//   3) other                      (unparseable, whole name returned as FileName)
func parseEHENFilename(cbzName string) ehenParsed {
	if strings.HasSuffix(cbzName, ".cbz") {
		cbzName = cbzName[:len(cbzName)-4]
	}

	if m := ehenFilenameRe.FindStringSubmatch(cbzName); m != nil {
		return ehenParsed{GalleryID: m[1], Token: m[2], FileName: strings.TrimSpace(m[3])}
	}

	if m := ehenFilenameNoDashRe.FindStringSubmatch(cbzName); m != nil {
		return ehenParsed{GalleryID: m[1], Token: m[2], FileName: ""}
	}

	return ehenParsed{FileName: cbzName}
}

// openMetaDB opens a PostgreSQL connection for metadata lookups.
func openMetaDB(cfg *config.Config) (*sql.DB, error) {
	if cfg.PGPass == "" {
		return nil, fmt.Errorf("PGPASSWORD not set")
	}
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable connect_timeout=10",
		cfg.PGHost, cfg.PGPort, cfg.PGUser, cfg.PGPass, cfg.PGDatabase,
	)
	return sql.Open("postgres", dsn)
}

// queryMetaByURL looks up metadata in eh_page-260604 by exact e-hentai URL.
func queryMetaByURL(db *sql.DB, gid, token string) *ehenMeta {
	ehURL := fmt.Sprintf("https://e-hentai.org/g/%s/%s/", gid, token)
	row := db.QueryRow(
		`SELECT category, uploader, title FROM "eh_page-260604" WHERE url = $1`, ehURL,
	)
	var cat, up, title sql.NullString
	if err := row.Scan(&cat, &up, &title); err != nil || !cat.Valid || cat.String == "" {
		return nil
	}
	return &ehenMeta{
		Category: strings.TrimSpace(cat.String),
		Uploader: strings.TrimSpace(up.String),
		Title:    title.String,
	}
}

// queryMetaByGID looks up metadata in eh_page-260604 by gallery_id pattern.
func queryMetaByGID(db *sql.DB, gid string) *ehenMeta {
	pattern := fmt.Sprintf("%%e-hentai.org/g/%s/%%", gid)
	row := db.QueryRow(
		`SELECT category, uploader, title, url FROM "eh_page-260604" WHERE url LIKE $1 LIMIT 1`, pattern,
	)
	var cat, up, title, ehURL sql.NullString
	if err := row.Scan(&cat, &up, &title, &ehURL); err != nil || !cat.Valid || cat.String == "" {
		return nil
	}
	return &ehenMeta{
		Category: strings.TrimSpace(cat.String),
		Uploader: strings.TrimSpace(up.String),
		Title:    title.String,
		URL:      ehURL.String,
	}
}

// queryMetaByTitle looks up metadata in eh_page-260604 by fuzzy title match.
func queryMetaByTitle(db *sql.DB, fname string) *ehenMeta {
	keyword := fname
	if len(keyword) > 50 {
		keyword = keyword[:50]
	}
	pattern := fmt.Sprintf("%%%s%%", keyword)
	row := db.QueryRow(
		`SELECT category, uploader, title, url FROM "eh_page-260604" WHERE title LIKE $1 LIMIT 1`, pattern,
	)
	var cat, up, title, ehURL sql.NullString
	if err := row.Scan(&cat, &up, &title, &ehURL); err != nil || !cat.Valid || cat.String == "" {
		return nil
	}
	return &ehenMeta{
		Category: strings.TrimSpace(cat.String),
		Uploader: strings.TrimSpace(up.String),
		Title:    title.String,
		URL:      ehURL.String,
	}
}

// callMetaWebhook calls the n8n synchronous webhook to resolve gallery metadata.
// Accepts gallery_id, gallery_token, and file_name (any combination).
func callMetaWebhook(webhookURL string, gid, token, fname string) *ehenMeta {
	body := map[string]interface{}{}
	if gid != "" {
		body["gallery_id"] = mustAtoi(gid)
	}
	if token != "" {
		body["gallery_token"] = token
	}
	if fname != "" {
		body["file_name"] = fname
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil
	}

	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("ehen webhook request failed", "error", err)
		return nil
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	// Format 1: {"match": {...}} (file_name query)
	if match, ok := result["match"].(map[string]interface{}); ok {
		return webhookMatchToMeta(match)
	}

	// Format 2: flat record with "id" field (gallery_id / gallery_id+token query)
	if _, ok := result["id"]; ok {
		return webhookRecordToMeta(result)
	}

	return nil
}

// webhookMatchToMeta converts a "match" format response to ehenMeta.
func webhookMatchToMeta(m map[string]interface{}) *ehenMeta {
	return &ehenMeta{
		Category:  strField(m, "category"),
		Uploader:  strField(m, "uploader"),
		Title:     strField(m, "title"),
		URL:       strField(m, "url"),
		GalleryID: strField(m, "gallery_id"),
	}
}

// webhookRecordToMeta converts a flat record response to ehenMeta.
func webhookRecordToMeta(m map[string]interface{}) *ehenMeta {
	return &ehenMeta{
		Category:  strField(m, "category"),
		Uploader:  strField(m, "uploader"),
		Title:     strField(m, "title"),
		URL:       strField(m, "url"),
		GalleryID: strField(m, "id"),
	}
}

func strField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func mustAtoi(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// decodeUploader handles URL-encoded uploader names from the database.
func decodeUploader(raw string) string {
	if raw == "" {
		return "unknown"
	}
	if strings.Contains(raw, "%") {
		if decoded, err := url.QueryUnescape(raw); err == nil {
			return decoded
		}
	}
	return raw
}

// safePathComponent replaces filesystem-unsafe characters.
func safePathComponent(name string) string {
	if name == "" {
		return "unknown"
	}
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\x00", "")
	return strings.TrimSpace(name)
}

// extractGIDFromURL pulls the gallery_id from an e-hentai URL.
func extractGIDFromURL(ehURL string) string {
	if m := ehenGIDFromURLRe.FindStringSubmatch(ehURL); m != nil {
		return m[1]
	}
	return ""
}

// resolveEhenMeta resolves metadata for a CBZ file using a cascade of strategies:
// 1. DB lookup by exact URL
// 2. DB lookup by gallery_id pattern
// 3. DB lookup by title matching
// 4. Synchronous webhook call
func resolveEhenMeta(db *sql.DB, webhookURL string, parsed ehenParsed, fname string) (*ehenMeta, *ehenStats, string) {
	st := &ehenStats{}
	var meta *ehenMeta
	gid := parsed.GalleryID
	token := parsed.Token

	if gid != "" && token != "" {
		meta = queryMetaByURL(db, gid, token)
	}

	if meta == nil && gid != "" {
		meta = queryMetaByGID(db, gid)
	}

	if meta == nil && fname != "" {
		meta = queryMetaByTitle(db, fname)
	}

	if meta == nil {
		identifier := fmt.Sprintf("gallery_id=%s", gid)
		if gid == "" {
			if len(fname) > 40 {
				identifier = fmt.Sprintf("file_name=%s", fname[:40])
			} else {
				identifier = fmt.Sprintf("file_name=%s", fname)
			}
		}
		slog.Info("ehen webhook query", "identifier", identifier)
		meta = callMetaWebhook(webhookURL, gid, token, fname)
		st.Webhook++
		if meta != nil {
			slog.Info("ehen webhook success", "identifier", identifier, "gallery_id", meta.GalleryID)
		}
	}

	return meta, st, gid
}

// RouteCBZToEhen is the main function to route a freshly-generated CBZ file
// from the archive directory to the ehen directory structure.
// It resolves metadata, moves the file, and updates the database.
//
// Parameters:
//   - db: PostgreSQL connection (may be nil, in which case DB is not updated)
//   - cbzPath: absolute path to the CBZ file at its current location
//   - cfg: application configuration
//
// Returns aggregated stats.
func RouteCBZToEhen(db *sql.DB, cbzPath string, cfg *config.Config) *ehenStats {
	st := &ehenStats{}

	if !cfg.EhenEnabled || cfg.EhenDir == "" {
		return st
	}

	cbzName := filepath.Base(cbzPath)
	parsed := parseEHENFilename(cbzName)
	fname := safePathComponent(parsed.FileName)

	// Resolve metadata
	meta, _, gid := resolveEhenMeta(db, cfg.EhenMetaWebhook, parsed, fname)
	if meta == nil {
		label := fmt.Sprintf("gid=%s", gid)
		if gid == "" {
			if len(fname) > 40 {
				label = fname[:40]
			} else {
				label = fname
			}
		}
		slog.Warn("ehen no metadata", "cbz", cbzName, "label", label)
		st.NoMeta++
		return st
	}

	// Extract gid from webhook URL if needed
	if gid == "" && meta.URL != "" {
		gid = extractGIDFromURL(meta.URL)
		if gid == "" && meta.GalleryID != "" {
			gid = meta.GalleryID
		}
	}

	// Fill file_name from metadata if missing
	if fname == "" && meta.Title != "" {
		fname = safePathComponent(decodeUploader(meta.Title))
	}

	category := safePathComponent(meta.Category)
	uploader := safePathComponent(decodeUploader(meta.Uploader))

	if category == "" {
		slog.Warn("ehen no category", "cbz", cbzName)
		st.NoMeta++
		return st
	}

	// Build target path
	var targetFilename string
	if fname != "" {
		if gid != "" {
			targetFilename = fmt.Sprintf("%s-%s.cbz", gid, fname)
		} else {
			targetFilename = fmt.Sprintf("%s.cbz", fname)
		}
	} else {
		targetFilename = fmt.Sprintf("%s.cbz", gid)
	}

	targetDir := filepath.Join(cfg.EhenDir, category, uploader)
	targetPath := filepath.Join(targetDir, targetFilename)

	// Check if already exists
	if _, err := os.Stat(targetPath); err == nil {
		readerURL := fmt.Sprintf("/ehen/%s/%s/%s", category, uploader, targetFilename)
		slog.Info("ehen already exists", "path", targetPath)
		if db != nil && gid != "" {
			if n := updateGalleryDB(db, gid, readerURL, targetFilename, category, uploader); n > 0 {
				st.DBUpdated++
			}
		}
		st.Moved++ // counted as moved even though already there
		return st
	}

	// Create directory and move file
	if err := os.MkdirAll(targetDir, ehenTargetDirMode); err != nil {
		slog.Error("ehen mkdir failed", "dir", targetDir, "error", err)
		st.Errors++
		return st
	}

	if err := os.Rename(cbzPath, targetPath); err != nil {
		slog.Error("ehen move failed", "src", cbzPath, "dst", targetPath, "error", err)
		st.Errors++
		return st
	}

	if err := os.Chmod(targetPath, ehenTargetFileMode); err != nil {
		slog.Warn("ehen chmod failed", "path", targetPath, "error", err)
	}

	// Update database
	readerURL := fmt.Sprintf("/ehen/%s/%s/%s", category, uploader, targetFilename)
	var dbUpdated int
	if db != nil && gid != "" {
		dbUpdated = updateGalleryDB(db, gid, readerURL, targetFilename, category, uploader)
	}

	if dbUpdated > 0 {
		slog.Info("ehen routed", "cbz", cbzName, "dst", fmt.Sprintf("%s/%s/%s", category, uploader, targetFilename), "db", "ok")
	} else {
		slog.Info("ehen routed", "cbz", cbzName, "dst", fmt.Sprintf("%s/%s/%s", category, uploader, targetFilename), "db", "no record")
	}
	st.Moved++
	st.DBUpdated += dbUpdated
	return st
}

// updateGalleryDB updates eh_gallery-260620 with reader_url and cover_url.
func updateGalleryDB(db *sql.DB, gid, readerURL, targetFilename, category, uploader string) int {
	if db == nil || gid == "" {
		return 0
	}

	// Update reader_url
	result, err := db.Exec(
		`UPDATE "eh_gallery-260620" SET reader_url = $1 WHERE gallery_id = $2`,
		readerURL, gid,
	)
	if err != nil {
		slog.Warn("ehen db update reader_url failed", "gallery_id", gid, "error", err)
		return 0
	}
	n, _ := result.RowsAffected()

	// Update cover_url
	encodedName := url.QueryEscape(targetFilename)
	coverURL := fmt.Sprintf("https://q.ws.gatepro.cn:99/gly/zip/ehen/%s/%s/%s/__cover.jfif",
		category, uploader, encodedName)
	if _, err := db.Exec(
		`UPDATE "eh_gallery-260620" SET cover_url = $1 WHERE gallery_id = $2 AND cover_url IS NOT NULL`,
		coverURL, gid,
	); err != nil {
		slog.Warn("ehen db update cover_url failed", "gallery_id", gid, "error", err)
	}

	// Fill category/uploader if missing
	if _, err := db.Exec(
		`UPDATE "eh_gallery-260620" SET category = $1, uploader = $2 WHERE gallery_id = $3 AND (category IS NULL OR category = '')`,
		category, uploader, gid,
	); err != nil {
		slog.Warn("ehen db fill category/uploader failed", "gallery_id", gid, "error", err)
	}

	return int(n)
}

// AggregateEhenStats merges multiple ehenStats into one.
func AggregateEhenStats(stats ...*ehenStats) *ehenStats {
	agg := &ehenStats{}
	for _, s := range stats {
		if s == nil {
			continue
		}
		agg.Moved += s.Moved
		agg.DBUpdated += s.DBUpdated
		agg.NoMeta += s.NoMeta
		agg.Webhook += s.Webhook
		agg.Errors += s.Errors
	}
	return agg
}
