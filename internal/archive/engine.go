package archive

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"imgproxy_plus/internal/config"
	"imgproxy_plus/internal/proxy"
)

type ArchiveResult struct {
	CBZ   []string `json:"cbz"`
	Stats Stats    `json:"stats"`
}

type Stats struct {
	Total           int `json:"total"`
	Converted       int `json:"converted"`
	SkippedAnimated int `json:"skipped_animated"`
	RemovedSmall    int `json:"removed_small"`
	Errors          int `json:"errors"`
}

func ProcessOne(rootPath string, cfg *config.Config) (*ArchiveResult, error) {
	lockFile, err := OpenProcessing(rootPath)
	if err != nil {
		return nil, fmt.Errorf("already processing or locked: %w", err)
	}
	lockFile.Close()
	defer CleanupDir(rootPath)

	start := time.Now()
	slog.Warn("archive processing", "dir", filepath.Base(rootPath))
	UpdateStatus(true, filepath.Base(rootPath))
	defer UpdateStatus(false, "")

	if err := UnpackAll(rootPath); err != nil {
		MarkFailed(rootPath, err)
		LogEvent("ERROR", "unpack failed", filepath.Base(rootPath), "", map[string]interface{}{"converted": 0})
		return nil, err
	}

	if err := flattenExtractDir(rootPath); err != nil {
		MarkFailed(rootPath, err)
		LogEvent("ERROR", "flatten failed", filepath.Base(rootPath), "", nil)
		return nil, err
	}

	RemoveEmptyDirs(rootPath)

	groups, err := BuildTree(rootPath, cfg)
	if err != nil {
		MarkFailed(rootPath, err)
		LogEvent("ERROR", "build tree failed", filepath.Base(rootPath), "", nil)
		return nil, err
	}

	if len(groups) == 0 {
		slog.Info("no images to process", "dir", filepath.Base(rootPath))
		LogEvent("INFO", "skip", filepath.Base(rootPath), "", map[string]interface{}{"reason": "no images"})
		CleanupDir(rootPath)
		return &ArchiveResult{}, nil
	}

	slog.Info("groups built", "count", len(groups))
	LogEvent("INFO", "start", filepath.Base(rootPath), "", map[string]interface{}{
		"groups": len(groups), "images": countTotal(groups),
	})

	os.MkdirAll(cfg.GalleryArchiveDir, 0755)

	client := proxy.NewImgproxyClient(cfg.ImgproxyURL, cfg.ImgproxyKey, cfg.ImgproxySalt)

	result := &ArchiveResult{}
	stats := Stats{}

	for _, group := range groups {
		if IsShuttingDown() {
			slog.Warn("archive interrupted", "group", group.Name)
			break
		}
		if IsShuttingDown() {
			slog.Warn("processing interrupted by shutdown", "group", group.Name)
			break
		}
		groupStats, cbzPath, groupErr := processGroup(group, client, cfg)
		if groupErr != nil {
			slog.Error("group processing failed", "group", group.Name, "error", groupErr)
			stats.Errors++
			continue
		}

		stats.Total += groupStats.Total
		stats.Converted += groupStats.Converted
		stats.SkippedAnimated += groupStats.SkippedAnimated
		stats.RemovedSmall += groupStats.RemovedSmall

		if cbzPath != "" {
			result.CBZ = append(result.CBZ, filepath.Base(cbzPath))
		}
	}

	if len(result.CBZ) == 0 && stats.Errors > 0 {
		MarkFailed(rootPath, fmt.Errorf("all groups failed"))
		return nil, fmt.Errorf("all groups failed")
	}

	cleanupSourceFiles(rootPath)
	RemoveEmptyDirs(rootPath)

	if HasMediaFiles(rootPath) {
		miscDir := filepath.Join(rootPath, "misc")
		os.MkdirAll(miscDir, 0755)
		moveRemainingMedia(rootPath, miscDir)
	}

	if HasAnyContent(rootPath) {
		slog.Info("directory retained", "dir", filepath.Base(rootPath), "reason", "content remaining")
	} else {
		CleanupDir(rootPath)
		os.Remove(rootPath)
		slog.Info("directory removed", "dir", filepath.Base(rootPath))
	}

	result.Stats = stats
	slog.Warn("archive done", "dir", filepath.Base(rootPath), "duration", time.Since(start).String(), "cbz", len(result.CBZ), "converted", stats.Converted, "errors", stats.Errors)
	LogEvent("OK", "done", filepath.Base(rootPath), "", map[string]interface{}{
		"cbz": len(result.CBZ), "converted": stats.Converted, "duration": time.Since(start),
	})

	// Notify external system (n8n) that this gallery finished archiving.
	// Fire-and-forget; skipped when GALLERY_COMPLETE_WEBHOOK_URL is empty.
	if len(result.CBZ) > 0 {
		FireCompleteWebhook(cfg.GalleryCompleteWebhookURL, filepath.Base(rootPath), result.CBZ, stats, start)
	}

	return result, nil
}

func processGroup(group *Group, client *proxy.ImgproxyClient, cfg *config.Config) (Stats, string, error) {
	stats := Stats{}

	os.MkdirAll(group.DirPath, 0755)
	SetGroupProgress(group.Name, 0, len(group.Images))

	for i := range group.Images {
		img := &group.Images[i]
		cleanSrc := filepath.Clean(img.AbsPath)
		cleanGroup := filepath.Clean(group.DirPath)
		if filepath.Dir(cleanSrc) != cleanGroup {
			dst := filepath.Join(group.DirPath, img.Name)
			// Handle filename conflicts from merged subdirs: prefix with parent dir name
			if _, err := os.Stat(dst); err == nil {
				parentDir := filepath.Base(filepath.Dir(cleanSrc))
				dst = filepath.Join(group.DirPath, parentDir+"_"+img.Name)
			}
			if err := copyFile(cleanSrc, dst); err == nil {
				os.Remove(cleanSrc)
				img.AbsPath = dst
			}
		}
	}

	type convResult struct {
		path      string
		converted bool
		index     int
	}

	sem := make(chan struct{}, cfg.GalleryArchiveConcurrency)
	var mu sync.Mutex
	var results []convResult
	statsConverted := 0
	statsAnimated := 0
	var wg sync.WaitGroup

	for i, img := range group.Images {
		if IsShuttingDown() {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, entry FileEntry) {
			defer wg.Done()
			defer func() { <-sem }()

			dstPath, converted, err := convertOrCopy(client, entry, group.DirPath, cfg)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				slog.Warn("convert failed", "file", entry.Name, "error", err)
				stats.Errors++
				return
			}
			results = append(results, convResult{path: dstPath, converted: converted, index: idx})
			if converted {
				statsConverted++
			} else {
				statsAnimated++
			}
			SetGroupProgress(group.Name, len(results), len(group.Images))
		}(i, img)
	}
	wg.Wait()
	stats.Converted += statsConverted
	stats.SkippedAnimated += statsAnimated

	coverIdx := selectCoverIndex(group)
	if coverIdx >= 0 {
		src := group.Images[coverIdx]
		convertedName := srcToDstName(src)
		srcForCover := filepath.Join(group.DirPath, convertedName)
		if _, err := os.Stat(srcForCover); err == nil {
			copyFileCover(client, srcForCover, group.DirPath)
		} else {
			if _, srcErr := os.Stat(src.AbsPath); srcErr == nil {
				GenerateCover(client, src.AbsPath, group.DirPath, cfg)
			}
		}
	}

	for _, img := range group.Images {
		cleanSrc := filepath.Clean(img.AbsPath)
		cleanGroup := filepath.Clean(group.DirPath)
		if filepath.Dir(cleanSrc) != cleanGroup || !strings.HasSuffix(cleanSrc, ".jfif") {
			dstName := srcToDstName(img)
			dstPath := filepath.Join(group.DirPath, dstName)
			cleanDst := filepath.Clean(dstPath)
			if cleanSrc != cleanDst {
				os.Remove(cleanSrc)
			}
		}
	}

	for _, r := range results {
		if FilterSmallFile(r.path, cfg.GalleryArchiveMinKB) {
			slog.Debug("removing small file", "file", filepath.Base(r.path))
			os.Remove(r.path)
			stats.RemovedSmall++
		}
	}

	miscDir := filepath.Join(group.DirPath, "misc")
	moveMediaFiles(group.DirPath, miscDir)
	for _, ni := range group.NonImages {
		niAbsPath := filepath.Clean(ni.AbsPath)
		os.MkdirAll(miscDir, 0755)
		miscDst := filepath.Join(miscDir, ni.Name)
		if niAbsPath != filepath.Clean(miscDst) {
			os.Rename(niAbsPath, miscDst)
		}
	}

	cbzPath, err := PackCBZ(group, cfg)
	if err != nil {
		return stats, "", fmt.Errorf("pack %s: %w", group.Name, err)
	}

	slog.Warn("packed cbz", "group", group.Name, "file", filepath.Base(cbzPath))
	LogEvent("INFO", "packed", filepath.Base(group.DirPath), group.Name, map[string]interface{}{"cbz": 1})

	return stats, cbzPath, nil
}

func countTotal(groups []*Group) int {
	n := 0
	for _, g := range groups {
		n += len(g.Images)
	}
	return n
}

func selectCoverIndex(group *Group) int {
	if len(group.Images) == 0 {
		return -1
	}
	for i, img := range group.Images {
		if HasCoverWord(img.Name) {
			return i
		}
	}
	return 0
}

func srcToDstName(img FileEntry) string {
	if img.IsAnimated || IsJfifExt(img.Name) {
		return img.Name
	}
	baseName := filepath.Base(img.Name)
	return baseName[:len(baseName)-len(filepath.Ext(baseName))] + ".jfif"
}

func cleanupSourceFiles(rootPath string) {
	filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		basename := filepath.Base(path)
		if basename == "__cover.jfif" {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(basename), ".webp") {
			return nil
		}
		if IsImageExt(basename) && !IsJfifExt(basename) {
			os.Remove(path)
		}
		return nil
	})
}

func moveRemainingMedia(rootPath, miscDir string) {
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return
	}
	os.MkdirAll(miscDir, 0755)
	for _, e := range entries {
		name := e.Name()
		if name == "misc" || name == ".tmp" || strings.HasPrefix(name, ".") {
			continue
		}
		src := filepath.Join(rootPath, name)
		if e.IsDir() {
			moveRemainingMedia(src, filepath.Join(miscDir, name))
		} else if IsMediaExt(name) {
			dst := filepath.Join(miscDir, name)
			os.Rename(src, dst)
		}
	}
}

func flattenExtractDir(rootPath string) error {
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == ".tmp" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		subDir := filepath.Join(rootPath, e.Name())
		flattenRecursive(subDir)
	}
	return nil
}

func flattenRecursive(dirPath string) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() && e.Name() != ".tmp" && !strings.HasPrefix(e.Name(), ".") {
			flattenRecursive(filepath.Join(dirPath, e.Name()))
		}
	}

	tryFlattenWrapper(dirPath)
}

func tryFlattenWrapper(dirPath string) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}

	var subDirs []string
	hasNonDirFiles := false
	for _, e := range entries {
		if e.IsDir() {
			subDirs = append(subDirs, e.Name())
		} else {
			hasNonDirFiles = true
		}
	}

	if !hasNonDirFiles && len(subDirs) > 1 {
		for _, sd := range subDirs {
			subSubPath := filepath.Join(dirPath, sd)
			subSubEntries, _ := os.ReadDir(subSubPath)
			for _, sse := range subSubEntries {
				if sse.IsDir() {
					src := filepath.Join(subSubPath, sse.Name())
					dst := filepath.Join(dirPath, sse.Name())
					if _, err := os.Stat(dst); err != nil {
						os.Rename(src, dst)
					}
				}
			}
		}
	}
}


