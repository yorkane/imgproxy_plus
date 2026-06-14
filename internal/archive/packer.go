package archive

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"imgproxy_plus/internal/config"
)

func PackCBZ(group *Group, cfg *config.Config) (string, error) {
	os.MkdirAll(group.DirPath, 0755)

	entries, err := os.ReadDir(group.DirPath)
	if err != nil {
		return "", fmt.Errorf("read dir %s: %w", group.DirPath, err)
	}

	var cbzFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if IsJfifExt(name) || (IsImageExt(name) && !IsJfifExt(name)) {
			cbzFiles = append(cbzFiles, name)
		}
	}

	if len(cbzFiles) == 0 {
		return "", fmt.Errorf("no files to pack in %s", group.DirPath)
	}

	archivedFiles := make(map[string]bool)
	for _, f := range cbzFiles {
		archivedFiles[f] = true
	}

	cbzName := group.Name + ".cbz"
	os.MkdirAll(cfg.GalleryArchiveDir, 0755)
	cbzPath := filepath.Join(cfg.GalleryArchiveDir, cbzName)

	cbzFile, err := os.Create(cbzPath)
	if err != nil {
		return "", fmt.Errorf("create cbz %s: %w", cbzPath, err)
	}
	defer cbzFile.Close()

	zw := zip.NewWriter(cbzFile)
	defer zw.Close()

	for _, name := range cbzFiles {
		srcPath := filepath.Join(group.DirPath, name)
		src, err := os.Open(srcPath)
		if err != nil {
			return "", fmt.Errorf("open %s: %w", name, err)
		}

		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:   name,
			Method: zip.Store,
		})
		if err != nil {
			src.Close()
			return "", fmt.Errorf("create zip entry %s: %w", name, err)
		}

		_, copyErr := io.Copy(w, src)
		src.Close()
		if copyErr != nil {
			return "", fmt.Errorf("copy to zip %s: %w", name, copyErr)
		}
	}

	zw.Close()
	cbzFile.Close()

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if archivedFiles[e.Name()] {
			os.Remove(filepath.Join(group.DirPath, e.Name()))
		}
	}

	return cbzPath, nil
}

func RemoveEmptyDirs(rootPath string) {
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			subDir := filepath.Join(rootPath, e.Name())
			if e.Name() == ".tmp" {
				os.RemoveAll(subDir)
				continue
			}
			RemoveEmptyDirs(subDir)
		}
	}
	entries, _ = os.ReadDir(rootPath)
	hasContent := false
	for _, e := range entries {
		if e.Name() == ".gallery_error" || e.Name() == ".gallery_processing" {
			continue
		}
		d := filepath.Join(rootPath, e.Name())
		if e.IsDir() && strings.HasPrefix(e.Name(), ".") {
			os.RemoveAll(d)
			continue
		}
		hasContent = true
	}
	if !hasContent {
		os.Remove(rootPath)
	}
}
