package unpack

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func UnpackZip(archivePath, destDir string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("zip open: %w", err)
	}
	defer reader.Close()

	os.MkdirAll(destDir, 0755)

	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}

		cleanName := filepath.Clean(f.Name)
		if strings.HasPrefix(cleanName, "..") {
			continue
		}

		dstPath := filepath.Join(destDir, cleanName)
		os.MkdirAll(filepath.Dir(dstPath), 0755)

		src, err := f.Open()
		if err != nil {
			return fmt.Errorf("zip open %s: %w", f.Name, err)
		}

		dst, err := os.Create(dstPath)
		if err != nil {
			src.Close()
			return fmt.Errorf("create %s: %w", dstPath, err)
		}

		_, copyErr := io.Copy(dst, src)
		src.Close()
		dst.Close()

		if copyErr != nil {
			return fmt.Errorf("copy %s: %w", f.Name, copyErr)
		}
	}

	return nil
}
