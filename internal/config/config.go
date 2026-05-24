package config

import (
	"fmt"
	"net"
	"os"
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
