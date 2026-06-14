package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"imgproxy_plus/internal/archive"
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

	archive.StartScanner(cfg)

	slog.Info("imgproxy_plus starting", "port", cfg.HTTPPort, "data_root", cfg.DataRoot, "html_root", htmlRoot, "imgproxy_url", cfg.ImgproxyURL)

	srv := &http.Server{Addr: addr, Handler: dispatcher}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		slog.Warn("received signal", "signal", sig.String())

		archive.Shutdown()

		archive.WaitShutdown()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("server shutdown error", "error", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}

	slog.Warn("server stopped")
}
