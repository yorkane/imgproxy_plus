package config

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
)

type Config struct {
	DataRoot         string
	RamdiskPath      string
	HTTPPort         string
	URLPrefix        string
	ImgproxyURL      string
	LogLevel         string
	AuthUser         string
	AuthPass         string
	AuthIPWhitelist  []*net.IPNet
	ZipExts          map[string]bool
	ZipfsTransparent bool
	APIPageSizeMax   int
	FileAPIDisable   bool
	ImgproxyKey      string
	ImgproxySalt     string

	GalleryAutoEnabled    bool
	GalleryScanDir        string
	GalleryArchiveDir     string
	GalleryScanInterval   int
	GalleryArchiveFmt     string
	GalleryArchiveW       int
	GalleryArchiveH       int
	GalleryArchiveFit     string
	GalleryArchiveQ       int
	GalleryArchiveMinKB      int
	GalleryArchiveMinChapter int
	GalleryArchiveConcurrency int
	GalleryCompleteWebhookURL string

	EhenEnabled      bool
	EhenDir          string
	EhenMetaWebhook  string
	PGHost           string
	PGPort           int
	PGUser           string
	PGPass           string
	PGDatabase       string
}

func Load() *Config {
	c := &Config{
		DataRoot:    env("PLUS_DATA_ROOT", "/data"),
		RamdiskPath: env("PLUS_RAMDISK_PATH", "/mnt/ramdisk"),
		HTTPPort:    env("PLUS_HTTP_PORT", "8080"),
		URLPrefix:   strings.TrimRight(env("PLUS_URL_PREFIX", ""), "/"),
		LogLevel:    env("PLUS_LOG_LEVEL", "warn"),
	}

	imgproxyBind := env("IMGPROXY_BIND", ":8081")
	host := imgproxyBind
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	c.ImgproxyURL = "http://" + host

	authUser := env("AUTH_USER", "")
	if authUser != "" {
		parts := strings.SplitN(authUser, ":", 2)
		c.AuthUser = parts[0]
		if len(parts) > 1 {
			c.AuthPass = parts[1]
		}
	}

	whitelist := env("AUTH_IP_WHITELIST", "")
	for _, cidr := range strings.Split(whitelist, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		if !strings.Contains(cidr, "/") {
			cidr = cidr + "/32"
		}
		_, netw, err := net.ParseCIDR(cidr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: invalid CIDR %q: %v\n", cidr, err)
			continue
		}
		c.AuthIPWhitelist = append(c.AuthIPWhitelist, netw)
	}

	c.ZipExts = map[string]bool{}
	for _, ext := range strings.Split(env("ZIP_EXTS", "zip,cbz"), ",") {
		ext = strings.TrimSpace(strings.ToLower(ext))
		if ext != "" {
			c.ZipExts[ext] = true
		}
	}

	c.ZipfsTransparent = strings.ToLower(env("ZIPFS_TRANSPARENT", "true")) == "true"

	c.FileAPIDisable = strings.ToLower(env("FILEAPI_DISABLE", "false")) == "true"

	max, err := strconv.Atoi(env("API_PAGE_SIZE_MAX", "200"))
	if err != nil || max <= 0 {
		max = 200
	}
	c.APIPageSizeMax = max

	c.ImgproxyKey = env("IMGPROXY_KEY", "")
	c.ImgproxySalt = env("IMGPROXY_SALT", "")

	c.GalleryAutoEnabled = strings.ToLower(env("GALLERY_AUTO_ENABLED", "false")) == "true"
	c.GalleryScanDir = env("GALLERY_SCAN_DIR", "/data/aria2/completed")
	c.GalleryArchiveDir = env("GALLERY_ARCHIVE_DIR", "/data/archived")
	c.GalleryArchiveFmt = env("GALLERY_ARCHIVE_FMT", "webp")
	c.GalleryArchiveFit = env("GALLERY_ARCHIVE_FIT", "cover")

	interval, err := strconv.Atoi(env("GALLERY_SCAN_INTERVAL", "1800"))
	if err != nil || interval <= 0 {
		interval = 1800
	}
	c.GalleryScanInterval = interval

	w, err := strconv.Atoi(env("GALLERY_ARCHIVE_W", "2560"))
	if err != nil || w <= 0 {
		w = 2560
	}
	c.GalleryArchiveW = w

	h, err := strconv.Atoi(env("GALLERY_ARCHIVE_H", "2560"))
	if err != nil || h <= 0 {
		h = 2560
	}
	c.GalleryArchiveH = h

	q, err := strconv.Atoi(env("GALLERY_ARCHIVE_Q", "90"))
	if err != nil || q <= 0 || q > 100 {
		q = 90
	}
	c.GalleryArchiveQ = q

	minKB, err := strconv.Atoi(env("GALLERY_ARCHIVE_MIN_KB", "10"))
	if err != nil || minKB < 0 {
		minKB = 10
	}
	c.GalleryArchiveMinKB = minKB

	minChapter, err := strconv.Atoi(env("GALLERY_ARCHIVE_MIN_CHAPTER", "5"))
	if err != nil || minChapter < 1 {
		minChapter = 5
	}
	c.GalleryArchiveMinChapter = minChapter

	concurrency, err := strconv.Atoi(env("GALLERY_ARCHIVE_CONCURRENCY", "0"))
	if err != nil || concurrency < 0 {
		concurrency = 0
	}
	if concurrency == 0 {
		concurrency = runtime.NumCPU() - 2
		if concurrency < 1 {
			concurrency = 1
		}
	}
	c.GalleryArchiveConcurrency = concurrency

	c.GalleryCompleteWebhookURL = env("GALLERY_COMPLETE_WEBHOOK_URL", "")

	c.EhenEnabled = strings.ToLower(env("EHEN_ENABLED", "true")) == "true"
	c.EhenDir = env("EHEN_DIR", "/data/ehen")
	c.EhenMetaWebhook = env("EHEN_META_WEBHOOK", "https://n8n.c.gatepro.cn/webhook/search_gallery_by_id_or_filename")

	c.PGHost = env("PGHOST", "")
	c.PGUser = env("PGUSER", "n8n")
	c.PGPass = env("PGPASSWORD", "")
	c.PGDatabase = env("PGDATABASE", "noco21")
	pgPort, err := strconv.Atoi(env("PGPORT", "5432"))
	if err != nil || pgPort <= 0 {
		pgPort = 5432
	}
	c.PGPort = pgPort

	return c
}

func (c *Config) IsZipExt(ext string) bool {
	return c.ZipExts[strings.ToLower(ext)]
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
