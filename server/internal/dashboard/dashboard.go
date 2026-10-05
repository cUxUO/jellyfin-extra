// Package dashboard 是轉碼伺服器的監控頁：GPU、各 session 的轉碼狀態、累計數據與最近的事件。
//
//	GET /dashboard        監控頁（HTML，每 2 秒向 /dashboard/data 取資料）
//	GET /dashboard/data   目前狀態（JSON）
//
// 頁面含使用者與片名，只開放內網直接連線：經反向代理（帶 X-Forwarded-For 等標頭）或非私有位址的請求一律拒絕。
package dashboard

import (
	"context"
	_ "embed"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"jellyfin-extra/server/internal/gpu"
	"jellyfin-extra/server/internal/jellyfin"
	"jellyfin-extra/server/internal/logring"
	"jellyfin-extra/server/internal/session"
)

//go:embed dashboard.html
var page []byte

type Sessions interface {
	Snapshot() session.Snapshot
	WorkDirUsage() int64
	WorkDir() string
}

type Source interface {
	BytesRead(sessionID string) int64
	TotalRead() int64
}

type Jellyfin interface {
	PublicInfo(ctx context.Context) (*jellyfin.PublicInfo, error)
}

type Server struct {
	Sessions    Sessions
	Source      Source
	JF          Jellyfin
	JellyfinURL string
	GPU         *gpu.Monitor
	Events      *logring.Ring
	Started     time.Time

	jfMu    sync.Mutex
	jfAt    time.Time
	jfState jellyfinState
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dashboard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(page)
	})
	mux.HandleFunc("GET /dashboard/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(s.data(r.Context()))
	})
	return lanOnly(mux)
}

type data struct {
	Now      time.Time     `json:"now"`
	Server   serverInfo    `json:"server"`
	Jellyfin jellyfinState `json:"jellyfin"`
	GPU      gpuState      `json:"gpu"`
	Disk     diskState     `json:"disk"`
	Sessions sessionsData  `json:"sessions"`
	Events   []string      `json:"events"`
}

type serverInfo struct {
	Version    string    `json:"version"`
	Started    time.Time `json:"started"`
	Goroutines int       `json:"goroutines"`
	HeapBytes  uint64    `json:"heapBytes"`
	GoVersion  string    `json:"goVersion"`
}

type jellyfinState struct {
	URL       string `json:"url"`
	OK        bool   `json:"ok"`
	Name      string `json:"name,omitempty"`
	Version   string `json:"version,omitempty"`
	LatencyMs int64  `json:"latencyMs"`
	Error     string `json:"error,omitempty"`
}

type gpuState struct {
	Available bool        `json:"available"`
	Error     string      `json:"error,omitempty"`
	List      []gpu.Stats `json:"list"`
}

type diskState struct {
	WorkDir   string `json:"workDir"`
	UsedBytes int64  `json:"usedBytes"` // 轉出的片段
	FreeBytes int64  `json:"freeBytes"` // -1 表示查不到
}

type sessionInfo struct {
	session.SessionInfo
	SourceBytes int64 `json:"sourceBytes"` // 已從 Jellyfin 讀取的原始檔位元組
}

type sessionsData struct {
	MaxSessions     int            `json:"maxSessions"`
	Starting        int            `json:"starting"`
	Active          []sessionInfo  `json:"active"`
	Totals          session.Totals `json:"totals"`
	SourceBytesRead int64          `json:"sourceBytesRead"` // 啟動以來讀取原始檔的總量
}

func (s *Server) data(ctx context.Context) data {
	snap := s.Sessions.Snapshot()
	active := make([]sessionInfo, 0, len(snap.Sessions))
	for _, si := range snap.Sessions {
		active = append(active, sessionInfo{SessionInfo: si, SourceBytes: s.Source.BytesRead(si.ID)})
	}

	var g gpuState
	if s.GPU != nil {
		list, err := s.GPU.Query(ctx)
		g = gpuState{Available: err == nil, List: list}
		if err != nil {
			g.Error = err.Error()
		}
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	dir, _ := filepath.Abs(s.Sessions.WorkDir())
	free, err := diskFree(dir)
	if err != nil {
		free = -1
	}
	var events []string
	if s.Events != nil {
		events = s.Events.Lines()
	}
	return data{
		Now: time.Now(),
		Server: serverInfo{
			Version: version(), Started: s.Started, Goroutines: runtime.NumGoroutine(),
			HeapBytes: mem.HeapAlloc, GoVersion: runtime.Version(),
		},
		Jellyfin: s.jellyfin(ctx),
		GPU:      g,
		Disk:     diskState{WorkDir: dir, UsedBytes: s.Sessions.WorkDirUsage(), FreeBytes: free},
		Sessions: sessionsData{MaxSessions: snap.MaxSessions, Starting: snap.Starting, Active: active, Totals: snap.Totals, SourceBytesRead: s.Source.TotalRead()},
		Events:   events,
	}
}

// jellyfin 每 10 秒確認一次 Jellyfin 連得上，並量延遲。
func (s *Server) jellyfin(ctx context.Context) jellyfinState {
	s.jfMu.Lock()
	defer s.jfMu.Unlock()
	if !s.jfAt.IsZero() && time.Since(s.jfAt) < 10*time.Second {
		return s.jfState
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	start := time.Now()
	info, err := s.JF.PublicInfo(ctx)
	st := jellyfinState{URL: s.JellyfinURL, LatencyMs: time.Since(start).Milliseconds()}
	if err != nil {
		st.Error = err.Error()
	} else {
		st.OK, st.Name, st.Version = true, info.ServerName, info.Version
	}
	s.jfState, s.jfAt = st, time.Now()
	return st
}

// version 取自建置時嵌入的 git 資訊，例如「8ecd827」「8ecd827+dirty」。
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	var rev string
	var dirty bool
	for _, kv := range info.Settings {
		switch kv.Key {
		case "vcs.revision":
			rev = kv.Value
		case "vcs.modified":
			dirty = kv.Value == "true"
		}
	}
	if rev == "" {
		return "dev"
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if dirty {
		rev += "+dirty"
	}
	return rev
}

// cgnat 是 Tailscale 等使用的 100.64.0.0/10。
var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// lanOnly 只允許內網直接連線。反向代理（NPM）一定會加 X-Forwarded-For，經它進來的請求即使來源是內網位址也拒絕。
func lanOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Real-IP") != "" || r.Header.Get("Forwarded") != "" {
			http.Error(w, "監控頁只開放內網直接連線", http.StatusForbidden)
			return
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || cgnat.Contains(ip)) {
			http.Error(w, "監控頁只開放內網直接連線", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
