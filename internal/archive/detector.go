package archive

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var imgExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".jfif": true,
	".png": true, ".webp": true, ".gif": true, ".bmp": true,
	".tiff": true, ".tif": true, ".avif": true, ".svg": true,
	".ico": true, ".heic": true, ".heif": true, ".jxl": true, ".pic": true,
}

var mediaExts = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".mov": true, ".wmv": true,
	".flv": true, ".webm": true, ".mpg": true, ".mpeg": true, ".m4v": true, ".3gp": true,
	".mp3": true, ".aac": true, ".ogg": true, ".wav": true, ".flac": true,
	".m4a": true, ".wma": true, ".opus": true, ".ape": true, ".aiff": true,
}

var archiveExts = map[string]bool{
	".zip": true, ".cbz": true,
	".tar": true,
	".gz": true, ".tgz": true,
	".xz": true, ".txz": true,
	".rar": true, ".cbr": true,
	".7z": true,
	".pdf": true,
}

func IsImageExt(name string) bool {
	return imgExts[strings.ToLower(filepath.Ext(name))]
}

func IsJfifExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".jfif"
}

func IsMediaExt(name string) bool {
	return mediaExts[strings.ToLower(filepath.Ext(name))]
}

func IsArchiveExt(name string) bool {
	return archiveExts[strings.ToLower(filepath.Ext(name))]
}

func IsCBZExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".cbz"
}

func DetectAnimated(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, 32)
	n, _ := io.ReadFull(f, buf)
	if n < 4 {
		return false
	}

	if bytes.HasPrefix(buf, []byte("\x89PNG")) {
		return false
	}

	if bytes.HasPrefix(buf, []byte("RIFF")) && n >= 12 &&
		bytes.Equal(buf[8:12], []byte("WEBP")) {
		if n >= 21 && bytes.HasPrefix(buf[12:], []byte("VP8X")) {
			if buf[20]&0x10 != 0 {
				return true
			}
		}
		return false
	}

	if bytes.HasPrefix(buf, []byte("GIF8")) {
		return true
	}

	return false
}

func HasAnimatedWebP(dirPath string) bool {
	var found bool
	filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || found {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) == ".webp" && DetectAnimated(path) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func HasCoverWord(name string) bool {
	lower := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	return strings.Contains(lower, "cover")
}
