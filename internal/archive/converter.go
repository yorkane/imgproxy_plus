package archive

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"imgproxy_plus/internal/config"
	"imgproxy_plus/internal/proxy"
)

func ConvertImage(client *proxy.ImgproxyClient, srcPath, dstPath string, cfg *config.Config) error {
	rt := "fit"
	switch cfg.GalleryArchiveFit {
	case "cover":
		rt = "fill"
	case "fill":
		rt = "force"
	}

	resizeOpt := fmt.Sprintf("rs:%s:%d:%d", rt, cfg.GalleryArchiveW, cfg.GalleryArchiveH)
	qualityOpt := fmt.Sprintf("q:%d", cfg.GalleryArchiveQ)
	formatOpt := fmt.Sprintf("format:%s", cfg.GalleryArchiveFmt)

	u := client.BuildProcessURL(resizeOpt, "", qualityOpt, formatOpt, "local:///"+srcPath)

	return downloadToFile(u, dstPath)
}

func convertOrCopy(client *proxy.ImgproxyClient, src FileEntry, dstDir string, cfg *config.Config) (string, bool, error) {
	if src.IsAnimated {
		dstPath := filepath.Join(dstDir, src.Name)
		if err := copyFile(src.AbsPath, dstPath); err != nil {
			return "", false, fmt.Errorf("copy animated %s: %w", src.Name, err)
		}
		return dstPath, false, nil
	}

	baseName := strings.TrimSuffix(src.Name, filepath.Ext(src.Name))
	dstPath := filepath.Join(dstDir, baseName+".jfif")

	if err := ConvertImage(client, src.AbsPath, dstPath, cfg); err != nil {
		return "", false, fmt.Errorf("convert %s: %w", src.Name, err)
	}

	return dstPath, true, nil
}

func GenerateCover(client *proxy.ImgproxyClient, srcPath, dstDir string, cfg *config.Config) (string, error) {
	dstPath := filepath.Join(dstDir, "__cover.jfif")
	if err := convertCover(client, srcPath, dstPath); err != nil {
		return "", fmt.Errorf("cover: %w", err)
	}
	return dstPath, nil
}

func copyFileCover(client *proxy.ImgproxyClient, srcPath, dstDir string) (string, error) {
	dstPath := filepath.Join(dstDir, "__cover.jfif")
	if err := convertCover(client, srcPath, dstPath); err != nil {
		return "", err
	}
	return dstPath, nil
}

func FilterSmallFile(path string, minKB int) bool {
	if filepath.Base(path) == "__cover.jfif" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	return info.Size() < int64(minKB)*1024
}

func convertCover(client *proxy.ImgproxyClient, srcPath, dstPath string) error {
	resizeOpt := "rs:fill:360:504"
	gravityOpt := "g:obj:face"
	qualityOpt := "q:80"
	formatOpt := "format:webp"
	u := client.BuildProcessURL(resizeOpt, gravityOpt, qualityOpt, formatOpt, "local:///"+srcPath)
	return downloadToFile(u, dstPath)
}

func downloadToFile(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("imgproxy returned %d", resp.StatusCode)
	}

	os.MkdirAll(filepath.Dir(path), 0755)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	io.Copy(f, resp.Body)
	return nil
}

func copyFile(src, dst string) error {
	os.MkdirAll(filepath.Dir(dst), 0755)

	srcF, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcF.Close()

	dstF, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstF.Close()

	_, err = io.Copy(dstF, srcF)
	return err
}

func moveMediaFiles(rootPath, miscDir string) error {
	return moveFilesByPredicate(rootPath, miscDir, func(name string) bool {
		return IsMediaExt(name)
	})
}

func moveNonImageFiles(rootPath, miscDir string) error {
	return moveFilesByPredicate(rootPath, miscDir, func(name string) bool {
		return !IsImageExt(name) && !IsArchiveExt(name)
	})
}

func moveFilesByPredicate(rootPath, miscDir string, predicate func(string) bool) error {
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return err
	}

	moved := false
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if predicate(e.Name()) {
			os.MkdirAll(miscDir, 0755)
			src := filepath.Join(rootPath, e.Name())
			dst := filepath.Join(miscDir, e.Name())
			if err := os.Rename(src, dst); err != nil {
				slog.Warn("move file failed", "src", src, "dst", dst, "error", err)
				continue
			}
			moved = true
		}
	}
	if moved {
		slog.Info("moved media files", "to", miscDir)
	}
	return nil
}
