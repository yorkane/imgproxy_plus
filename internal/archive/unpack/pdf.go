package unpack

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func UnpackPdf(archivePath, destDir string) error {
	os.MkdirAll(destDir, 0755)

	imgDir := filepath.Join(destDir, "img")
	os.MkdirAll(imgDir, 0755)

	cmd := exec.Command("pdfimages", "-j", archivePath, filepath.Join(imgDir, "img"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := string(out)
		if len(outStr) > 0 {
			return fmt.Errorf("pdfimages: %s: %w", outStr, err)
		}
	}

	imgs, _ := os.ReadDir(imgDir)
	hasImages := false
	for _, e := range imgs {
		if !e.IsDir() {
			hasImages = true
			break
		}
	}

	if !hasImages {
		return unpackPdfPpm(archivePath, destDir)
	}

	return nil
}

func unpackPdfPpm(archivePath, destDir string) error {
	pageDir := filepath.Join(destDir, "page")
	os.MkdirAll(pageDir, 0755)
	cmd := exec.Command("pdftoppm", "-jpeg", "-r", "150", archivePath, filepath.Join(pageDir, "page"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pdftoppm: %s: %w", string(out), err)
	}
	return nil
}
