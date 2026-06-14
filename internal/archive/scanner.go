package archive

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"imgproxy_plus/internal/config"
)

var shuttingDown atomic.Bool
var scanDone = make(chan struct{})
var stopScan = make(chan struct{})

func IsShuttingDown() bool {
	return shuttingDown.Load()
}

func Shutdown() {
	shuttingDown.Store(true)
	close(stopScan)
	slog.Warn("archive scanner shutting down, waiting for current task...")
}

func WaitShutdown() {
	<-scanDone
	slog.Warn("archive scanner stopped")
}

func StartScanner(cfg *config.Config) {
	if !cfg.GalleryAutoEnabled {
		return
	}

	os.MkdirAll(cfg.GalleryScanDir, 0755)
	os.MkdirAll(cfg.GalleryArchiveDir, 0755)

	interval := time.Duration(cfg.GalleryScanInterval) * time.Second

	slog.Warn("archive scanner started",
		"scan_dir", cfg.GalleryScanDir,
		"archive_dir", cfg.GalleryArchiveDir,
		"interval", interval.String(),
	)

	go func() {
		defer close(scanDone)
		if !shuttingDown.Load() {
			ScanOnce(cfg)
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopScan:
				return
			case <-ticker.C:
				if shuttingDown.Load() {
					return
				}
				ScanOnce(cfg)
			}
		}
	}()
}

func ScanOnce(cfg *config.Config) {
	slog.Info("scanning for directories", "dir", cfg.GalleryScanDir)

	entries, err := os.ReadDir(cfg.GalleryScanDir)
	if err != nil {
		slog.Error("scan failed", "dir", cfg.GalleryScanDir, "error", err)
		return
	}

	for _, entry := range entries {
		if shuttingDown.Load() {
			slog.Warn("scan interrupted by shutdown")
			return
		}
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		if strings.HasPrefix(name, ".") {
			continue
		}
		if strings.HasSuffix(name, "_failed") {
			continue
		}

		dirPath := filepath.Join(cfg.GalleryScanDir, name)

		if _, err := os.Stat(filepath.Join(dirPath, ".gallery_processing")); err == nil {
			continue
		}

		if !dirNeedsProcessing(dirPath) {
			slog.Debug("skipping directory", "dir", name, "reason", "no content")
			continue
		}

		result, err := ProcessOne(dirPath, cfg)
		if err != nil {
			slog.Warn("processing failed", "dir", name, "error", err)
			continue
		}

		slog.Warn("archive done",
			"dir", name,
			"cbz", len(result.CBZ),
			"converted", result.Stats.Converted,
			"animated", result.Stats.SkippedAnimated,
			"removed_small", result.Stats.RemovedSmall,
		)
	}
}

func dirNeedsProcessing(dirPath string) bool {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name() == ".tmp" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			return true
		}
		if IsImageExt(e.Name()) || IsArchiveExt(e.Name()) || IsMediaExt(e.Name()) {
			return true
		}
	}
	return false
}
