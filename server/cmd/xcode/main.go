// xcode 是 Windows 上的轉碼伺服器：替播放程式向 Jellyfin 取原始檔，用 NVENC 即時轉成 HLS。
//
// Windows 上以 GUI 程式建置（-H=windowsgui）：雙擊啟動、常駐系統匣，從系統匣結束。
// 從命令列或 SSH 執行時加 -notray。log 寫在 exe 旁的 xcode.log。
package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	"jellyfin-extra/server/internal/api"
	"jellyfin-extra/server/internal/dashboard"
	"jellyfin-extra/server/internal/gpu"
	"jellyfin-extra/server/internal/jellyfin"
	"jellyfin-extra/server/internal/logring"
	"jellyfin-extra/server/internal/profile"
	"jellyfin-extra/server/internal/session"
	"jellyfin-extra/server/internal/source"
)

func main() {
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	configPath := flag.String("config", filepath.Join(dir, "xcode.json"), "config file")
	noTray := flag.Bool("notray", false, "do not show the tray icon (for running from a console or SSH)")
	flag.Parse()

	// 最近的 log 同時留在記憶體，給監控頁的事件列表；GUI 程式沒有主控台，另外寫進檔案
	events := logring.New(200)
	logPath := filepath.Join(dir, "xcode.log")
	out := []io.Writer{events, quiet{os.Stderr}}
	if f, err := openRotatingLog(logPath, 10<<20); err == nil {
		defer f.Close()
		out = append(out, f)
	}
	logger := log.New(io.MultiWriter(out...), "", log.LstdFlags)

	if !*noTray && trayAvailable {
		runWithTray(*configPath, logPath, logger, events)
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, *configPath, logger, events, nil); err != nil {
		logger.Fatal(err)
	}
}

// quiet 忽略寫入錯誤：GUI 程式的 stderr 可能是無效的 handle，不能讓 io.MultiWriter 因此中斷。
type quiet struct{ w io.Writer }

func (q quiet) Write(p []byte) (int, error) {
	q.w.Write(p)
	return len(p), nil
}

// status 給系統匣顯示的執行狀態。
type status struct {
	DashboardURL string
	Active       func() int // 目前的轉碼 session 數
}

// run 啟動伺服器直到 ctx 結束；onReady（可為 nil）在開始接受連線前呼叫。
func run(ctx context.Context, configPath string, logger *log.Logger, events *logring.Ring, onReady func(status)) error {
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

	ctx, stop := context.WithCancel(ctx)
	defer stop()
	mgrDone := make(chan struct{})
	go func() {
		mgr.Run(ctx)
		close(mgrDone)
	}()

	dash := &dashboard.Server{
		Sessions: mgr, Source: proxy, JF: jf, JellyfinURL: cfg.JellyfinURL,
		GPU: gpu.NewMonitor(), Events: events, Started: time.Now(),
	}
	// 監控頁另外掛，不經 api 的請求記錄（每 2 秒輪詢會洗掉事件列表）
	mux := http.NewServeMux()
	mux.Handle("/dashboard", dash.Handler())
	mux.Handle("/dashboard/", dash.Handler())
	mux.Handle("/", (&api.Server{JF: jf, Sessions: mgr, Profiles: profile.Profiles, Log: logger}).Handler())
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		stop()
		<-mgrDone
		return err
	}
	if onReady != nil {
		onReady(status{
			DashboardURL: dashboardURL(ln.Addr()),
			Active:       func() int { return len(mgr.Snapshot().Sessions) },
		})
	}
	logger.Printf("xcode listening on %s, jellyfin=%s, workDir=%s, maxSessions=%d, dashboard at /dashboard", cfg.Listen, cfg.JellyfinURL, cfg.WorkDir, cfg.MaxSessions)
	err = srv.Serve(ln)
	stop()
	<-mgrDone // 停掉所有 ffmpeg、清掉暫存
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// dashboardURL 是本機開監控頁用的網址；聽所有介面時用 127.0.0.1（監控頁允許 loopback）。
func dashboardURL(addr net.Addr) string {
	host, port := "127.0.0.1", ""
	if a, ok := addr.(*net.TCPAddr); ok {
		port = strconv.Itoa(a.Port)
		if !a.IP.IsUnspecified() {
			host = a.IP.String()
		}
	}
	return "http://" + net.JoinHostPort(host, port) + "/dashboard"
}
