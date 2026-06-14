package archive

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"imgproxy_plus/internal/archive/unpack"
)

func UnpackAll(dirPath string) error {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("readdir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !IsArchiveExt(entry.Name()) {
			continue
		}

		archivePath := filepath.Join(dirPath, entry.Name())
		destBase := filepath.Join(dirPath, ".tmp", strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))

		slog.Info("unpacking", "archive", entry.Name())

		var unpackErr error
		ext := strings.ToLower(filepath.Ext(entry.Name()))

		switch {
		case ext == ".zip" || ext == ".cbz":
			unpackErr = unpack.UnpackZip(archivePath, destBase)
		case ext == ".rar" || ext == ".cbr":
			unpackErr = unpack.UnpackRar(archivePath, destBase)
		case ext == ".7z":
			unpackErr = unpack.Unpack7z(archivePath, destBase)
		case ext == ".pdf":
			unpackErr = unpack.UnpackPdf(archivePath, destBase)
		case ext == ".xz" || ext == ".txz":
			unpackErr = unpack.UnpackXz(archivePath, destBase)
		default:
			unpackErr = unpack.UnpackTar(archivePath, destBase)
		}

		if unpackErr != nil {
			slog.Error("unpack failed", "archive", entry.Name(), "error", unpackErr)
			return fmt.Errorf("unpack %s: %w", entry.Name(), unpackErr)
		}

		if err := os.Remove(archivePath); err != nil {
			slog.Warn("remove archive failed", "path", archivePath, "error", err)
		}

		extractUnpacked(destBase, dirPath)
	}

	return nil
}

func extractUnpacked(tmpDir, rootDir string) {
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		src := filepath.Join(tmpDir, entry.Name())
		dst := filepath.Join(rootDir, entry.Name())

		if entry.IsDir() {
			if _, err := os.Stat(dst); err == nil {
				extractUnpacked(src, dst)
				os.Remove(src)
				continue
			}
			if err := os.Rename(src, dst); err != nil {
				moveAllFiles(src, rootDir)
			}
		} else {
			os.Rename(src, dst)
		}
	}

	os.Remove(tmpDir)
}

func moveAllFiles(srcDir, dstDir string) {
	filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(srcDir, path)
		dst := filepath.Join(dstDir, rel)
		os.MkdirAll(filepath.Dir(dst), 0755)
		os.Rename(path, dst)
		return nil
	})
	os.RemoveAll(srcDir)
}
