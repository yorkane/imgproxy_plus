package archive

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ArchiveEvent is the payload POSTed to GALLERY_COMPLETE_WEBHOOK_URL when a
// gallery directory finishes archiving. gallery_id/gallery_token are parsed
// from the source directory basename ({id}_{token}); they are the only stable
// link back to the e-hentai gallery because imgproxy_plus stores no metadata.
type ArchiveEvent struct {
	Event         string   `json:"event"`          // "archive_complete"
	SourceDir     string   `json:"source_dir"`     // basename of input dir, e.g. "3998280_91f672d62c"
	GalleryID     string   `json:"gallery_id"`     // parsed prefix before first "_"
	GalleryToken  string   `json:"gallery_token"`  // 10-hex token (may include chapter suffix)
	CBZ           []string `json:"cbz"`            // produced CBZ basenames
	TotalPages    int      `json:"total_pages"`    // converted + animated images count
	Converted     int      `json:"converted"`
	Errors        int      `json:"errors"`
	DurationMS    int64    `json:"duration_ms"`
	CompletedAt   string   `json:"completed_at"`   // RFC3339
}

// FireCompleteWebhook notifies an external system (n8n) that a gallery archive
// finished. It is fire-and-forget: runs in a goroutine, never blocks the
// archive pipeline, and only logs on failure. No-op when webhookURL is empty.
func FireCompleteWebhook(webhookURL, sourceDir string, cbz []string, stats Stats, start time.Time) {
	if webhookURL == "" {
		return
	}
	evt := buildEvent(sourceDir, cbz, stats, start)
	go send(webhookURL, evt)
}

func buildEvent(sourceDir string, cbz []string, stats Stats, start time.Time) ArchiveEvent {
	gid, token := parseGalleryID(sourceDir)
	return ArchiveEvent{
		Event:        "archive_complete",
		SourceDir:    sourceDir,
		GalleryID:    gid,
		GalleryToken: token,
		CBZ:          cbz,
		TotalPages:   stats.Converted + stats.SkippedAnimated,
		Converted:    stats.Converted,
		Errors:       stats.Errors,
		DurationMS:   time.Since(start).Milliseconds(),
		CompletedAt:  time.Now().UTC().Format(time.RFC3339),
	}
}

// parseGalleryID splits "3998280_91f672d62c" into ("3998280", "91f672d62c").
// For chapterized dirs the token may carry a suffix ("-ch01"), which the
// consumer can trim. Returns ("","") if the dir name does not match.
func parseGalleryID(sourceDir string) (string, string) {
	idx := strings.Index(sourceDir, "_")
	if idx <= 0 {
		return "", ""
	}
	return sourceDir[:idx], sourceDir[idx+1:]
}

func send(url string, evt ArchiveEvent) {
	body, err := json.Marshal(evt)
	if err != nil {
		slog.Warn("webhook marshal failed", "source_dir", evt.SourceDir, "error", err)
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		slog.Warn("webhook request build failed", "source_dir", evt.SourceDir, "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("webhook send failed", "source_dir", evt.SourceDir, "url", url, "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		slog.Warn("webhook non-2xx", "source_dir", evt.SourceDir, "status", resp.StatusCode)
	} else {
		slog.Info("webhook sent", "source_dir", evt.SourceDir, "gallery_id", evt.GalleryID, "cbz", len(evt.CBZ))
	}
}
