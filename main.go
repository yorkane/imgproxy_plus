package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"imgproxy_plus/internal/config"
	"imgproxy_plus/internal/router"
	"imgproxy_plus/internal/static"
)

func main() {
	cfg := config.Load()

	level := slog.LevelWarn
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	htmlRoot := filepath.Join(exeDir, "html")
	if _, err := os.Stat(htmlRoot); os.IsNotExist(err) {
		htmlRoot = filepath.Join(filepath.Dir(exeDir), "html")
		if _, err := os.Stat(htmlRoot); os.IsNotExist(err) {
			htmlRoot = "html"
		}
	}
	static.Init(htmlRoot, cfg.URLPrefix)

	os.MkdirAll(filepath.Join(cfg.RamdiskPath, ".imgapi-tmp"), 0755)

	dispatcher := router.New(cfg)
	addr := fmt.Sprintf(":%s", cfg.HTTPPort)

	slog.Info("imgproxy_plus starting", "port", cfg.HTTPPort, "data_root", cfg.DataRoot, "html_root", htmlRoot, "imgproxy_url", cfg.ImgproxyURL)

	if err := http.ListenAndServe(addr, dispatcher); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
