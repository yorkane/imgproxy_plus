package unpack

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func UnpackTar(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	var reader io.Reader = f

	ext := strings.ToLower(filepath.Ext(archivePath))
	if ext == ".gz" || ext == ".tgz" || strings.HasSuffix(strings.ToLower(archivePath), ".tar.gz") {
		gzReader, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("gzip: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	tr := tar.NewReader(reader)
	os.MkdirAll(destDir, 0755)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}

		cleanName := filepath.Clean(header.Name)
		if strings.HasPrefix(cleanName, "..") {
			continue
		}

		dstPath := filepath.Join(destDir, cleanName)
		os.MkdirAll(filepath.Dir(dstPath), 0755)

		dst, err := os.Create(dstPath)
		if err != nil {
			return fmt.Errorf("create %s: %w", dstPath, err)
		}

		_, copyErr := io.Copy(dst, tr)
		dst.Close()

		if copyErr != nil {
			return fmt.Errorf("copy %s: %w", header.Name, copyErr)
		}
	}

	return nil
}

func UnpackXz(archivePath, destDir string) error {
	if strings.HasSuffix(strings.ToLower(archivePath), ".txz") ||
		strings.HasSuffix(strings.ToLower(archivePath), ".tar.xz") {
		return unpackXzTar(archivePath, destDir)
	}
	return fmt.Errorf("unsupported xz format: %s", archivePath)
}

func unpackXzTar(archivePath, destDir string) error {
	xzCmd := exec.Command("xz", "-dc", archivePath)

	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("pipe: %w", err)
	}
	xzCmd.Stdout = pw

	if err := xzCmd.Start(); err != nil {
		return fmt.Errorf("xz start: %w", err)
	}
	pw.Close()

	tr := tar.NewReader(pr)
	os.MkdirAll(destDir, 0755)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			xzCmd.Wait()
			return fmt.Errorf("tar read: %w", err)
		}

		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}

		cleanName := filepath.Clean(header.Name)
		if strings.HasPrefix(cleanName, "..") {
			continue
		}

		dstPath := filepath.Join(destDir, cleanName)
		os.MkdirAll(filepath.Dir(dstPath), 0755)

		dst, err := os.Create(dstPath)
		if err != nil {
			xzCmd.Wait()
			return fmt.Errorf("create %s: %w", dstPath, err)
		}

		_, copyErr := io.Copy(dst, tr)
		dst.Close()

		if copyErr != nil {
			xzCmd.Wait()
			return fmt.Errorf("copy %s: %w", header.Name, copyErr)
		}
	}

	return xzCmd.Wait()
}
