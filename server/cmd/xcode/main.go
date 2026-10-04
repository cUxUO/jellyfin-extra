// xcode 是 Windows 上的轉碼伺服器：替播放程式向 Jellyfin 取原始檔，用 NVENC 即時轉成 HLS。
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"jellyfin-extra/server/internal/api"
	"jellyfin-extra/server/internal/jellyfin"
	"jellyfin-extra/server/internal/profile"
	"jellyfin-extra/server/internal/session"
	"jellyfin-extra/server/internal/source"
)

func main() {
	exe, _ := os.Executable()
	configPath := flag.String("config", filepath.Join(filepath.Dir(exe), "xcode.json"), "config file")
	flag.Parse()

	logger := log.New(os.Stderr, "", log.LstdFlags)
	if err := run(*configPath, logger); err != nil {
		logger.Fatal(err)
	}
}

func run(configPath string, logger *log.Logger) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	if err := killChildrenOnExit(); err != nil {
		return err
	}
	if _, err := os.Stat(cfg.FFmpegPath); err != nil {
		return err
	}

	jf, err := jellyfin.New(cfg.JellyfinURL)
	if err != nil {
		return err
	}
	proxy := source.NewProxy(jf)
	if cfg.LogSource {
		proxy.Log = logger
	}
	if err := proxy.Start(); err != nil {
		return err
	}
	defer proxy.Close()

	mgr, err := session.NewManager(cfg.session(), proxy, logger)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	mgrDone := make(chan struct{})
	go func() {
		mgr.Run(ctx)
		close(mgrDone)
	}()

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           (&api.Server{JF: jf, Sessions: mgr, Profiles: profile.Profiles, Log: logger}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	logger.Printf("xcode listening on %s, jellyfin=%s, workDir=%s, maxSessions=%d", cfg.Listen, cfg.JellyfinURL, cfg.WorkDir, cfg.MaxSessions)
	err = srv.ListenAndServe()
	stop()
	<-mgrDone // 停掉所有 ffmpeg、清掉暫存
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
