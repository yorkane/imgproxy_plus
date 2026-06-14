package archive

import (
	"archive/zip"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"imgproxy_plus/internal/config"
	"imgproxy_plus/internal/proxy"
)

type MigrateResult struct {
	Total   int              `json:"total"`
	Fixed   int              `json:"fixed"`
	Skipped int              `json:"skipped"`
	Errors  []string         `json:"errors,omitempty"`
	Details []MigrateDetail  `json:"details,omitempty"`
}

type MigrateDetail struct {
	File      string `json:"file"`
	OldCovers []string `json:"old_covers"`
	NewCover  string `json:"new_cover,omitempty"`
	Error     string `json:"error,omitempty"`
}

func MigrateCovers(cfg *config.Config) (*MigrateResult, error) {
	entries, err := os.ReadDir(cfg.GalleryArchiveDir)
	if err != nil {
		return nil, fmt.Errorf("read archive dir: %w", err)
	}

	client := proxy.NewImgproxyClient(cfg.ImgproxyURL, cfg.ImgproxyKey, cfg.ImgproxySalt)

	result := &MigrateResult{}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".cbz") {
			continue
		}
		result.Total++

		cbzPath := filepath.Join(cfg.GalleryArchiveDir, entry.Name())
		detail, err := migrateOneCbz(cbzPath, client, cfg)
		if err != nil {
			slog.Warn("migrate failed", "file", entry.Name(), "error", err)
			detail.Error = err.Error()
			result.Errors = append(result.Errors, entry.Name()+": "+err.Error())
		}

		if detail != nil {
			result.Details = append(result.Details, *detail)
			if detail.Error == "" && len(detail.OldCovers) > 0 {
				result.Fixed++
			} else if len(detail.OldCovers) == 0 {
				result.Skipped++
			}
		}
	}

	return result, nil
}

func migrateOneCbz(cbzPath string, client *proxy.ImgproxyClient, cfg *config.Config) (*MigrateDetail, error) {
	detail := &MigrateDetail{File: filepath.Base(cbzPath)}

	r, err := zip.OpenReader(cbzPath)
	if err != nil {
		return detail, fmt.Errorf("open cbz: %w", err)
	}
	defer r.Close()

	var imageFiles []*zip.File
	hasOldCover := false

	for _, f := range r.File {
		name := filepath.Base(f.Name)
		lower := strings.ToLower(name)
		if lower == "##cover.jiff" || lower == "##cover.jfif" || lower == "--cover.jfif" || lower == "__cover.jfif" {
			hasOldCover = true
			detail.OldCovers = append(detail.OldCovers, name)
			continue
		}
		if IsImageExt(name) {
			imageFiles = append(imageFiles, f)
		}
	}

	if !hasOldCover {
		return detail, nil
	}

	sort.Slice(imageFiles, func(i, j int) bool {
		return naturalCmp(imageFiles[i].Name, imageFiles[j].Name) < 0
	})

	if len(imageFiles) == 0 {
		return detail, fmt.Errorf("no images left after removing old covers")
	}

	tmpDir, err := os.MkdirTemp("", "migrate-cbz-")
	if err != nil {
		return detail, fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	for _, f := range r.File {
		name := filepath.Base(f.Name)
		lower := strings.ToLower(name)
		if lower == "##cover.jiff" || lower == "##cover.jfif" || lower == "--cover.jfif" || lower == "__cover.jfif" {
			continue
		}
		dstPath := filepath.Join(tmpDir, name)
		os.MkdirAll(filepath.Dir(dstPath), 0755)
		src, err := f.Open()
		if err != nil {
			return detail, fmt.Errorf("open %s: %w", f.Name, err)
		}
		dst, err := os.Create(dstPath)
		if err != nil {
			src.Close()
			return detail, fmt.Errorf("create %s: %w", dstPath, err)
		}
		io.Copy(dst, src)
		src.Close()
		dst.Close()
	}

	coverSrc := imageFiles[0]
	coverSrcPath := filepath.Join(tmpDir, coverSrc.Name)
	coverDstPath := filepath.Join(tmpDir, "__cover.jfif")

	if err := convertCover(client, coverSrcPath, coverDstPath); err != nil {
		return detail, fmt.Errorf("generate cover: %w", err)
	}

	if _, err := os.Stat(coverDstPath); os.IsNotExist(err) {
		return detail, fmt.Errorf("cover not created")
	}

	detail.NewCover = coverSrc.Name

	backupPath := cbzPath + ".bak"
	if err := os.Rename(cbzPath, backupPath); err != nil {
		return detail, fmt.Errorf("backup: %w", err)
	}

	newCbz, err := os.Create(cbzPath)
	if err != nil {
		os.Rename(backupPath, cbzPath)
		return detail, fmt.Errorf("create new cbz: %w", err)
	}

	zw := zip.NewWriter(newCbz)

	tmpEntries, _ := os.ReadDir(tmpDir)
	for _, te := range tmpEntries {
		if te.IsDir() {
			continue
		}
		src, err := os.Open(filepath.Join(tmpDir, te.Name()))
		if err != nil {
			continue
		}
		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:   te.Name(),
			Method: zip.Store,
		})
		if err != nil {
			src.Close()
			continue
		}
		io.Copy(w, src)
		src.Close()
	}

	zw.Close()
	newCbz.Close()

	os.Remove(backupPath)

	slog.Warn("migrated cbz cover", "file", detail.File,
		"old_covers", strings.Join(detail.OldCovers, ","),
		"new_cover", detail.NewCover,
		"cover_mtime", time.Now())

	return detail, nil
}
