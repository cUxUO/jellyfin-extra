// Package session 管理轉碼工作：啟動 ffmpeg、等第一批片段、閒置回收、清理暫存檔。
package session

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"jellyfin-extra/server/internal/ffmpeg"
	"jellyfin-extra/server/internal/profile"
	"jellyfin-extra/server/internal/source"
)

var (
	ErrBusy     = errors.New("session: too many active sessions")
	ErrNotReady = errors.New("session: transcode did not produce segments in time")
)

// idPattern 也用來辨識 WorkDir 裡哪些目錄是我們建立的，清理時只碰這些。
var idPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func ValidID(id string) bool { return idPattern.MatchString(id) }

type Config struct {
	FFmpegPath     string
	WorkDir        string
	MaxSessions    int // NVENC 消費級驅動有同時編碼數上限
	IdleTimeout    time.Duration
	ReadyTimeout   time.Duration
	SegmentSeconds int
}

type Params struct {
	UserID        string
	ItemID        string
	MediaSourceID string
	Token         string // 只交給 source registry，不保存在 Session
	StartSeconds  float64
	Plan          profile.Plan
}

type Session struct {
	ID           string
	Dir          string
	UserID       string
	ItemID       string
	StartSeconds float64
	Plan         profile.Plan

	cancel     context.CancelFunc
	done       chan struct{}
	waitErr    error
	stderr     *tail
	lastAccess atomic.Int64
}

// Path 回傳 session 目錄內的檔案路徑；name 必須先經過呼叫端驗證。
func (s *Session) Path(name string) string { return filepath.Join(s.Dir, name) }

// Finished 表示 ffmpeg 已結束（轉完或失敗），檔案仍可讀到被回收為止。
func (s *Session) Finished() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *Session) touch() { s.lastAccess.Store(time.Now().UnixNano()) }

type Manager struct {
	cfg Config
	src source.Registry
	log *log.Logger

	// command 讓測試可以換掉 ffmpeg
	command func(ctx context.Context, name string, args ...string) *exec.Cmd

	mu       sync.Mutex
	sessions map[string]*Session
	pending  int // 啟動中、尚未就緒的數量，也佔名額
}

func NewManager(cfg Config, src source.Registry, logger *log.Logger) (*Manager, error) {
	if err := os.MkdirAll(cfg.WorkDir, 0o755); err != nil {
		return nil, fmt.Errorf("session: work dir: %w", err)
	}
	m := &Manager{cfg: cfg, src: src, log: logger, command: exec.CommandContext, sessions: map[string]*Session{}}
	m.removeStale()
	return m, nil
}

// removeStale 清掉上次執行留下的 session 目錄（例如程式被強制結束）。
func (m *Manager) removeStale() {
	entries, err := os.ReadDir(m.cfg.WorkDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && ValidID(e.Name()) {
			removeAll(filepath.Join(m.cfg.WorkDir, e.Name()))
		}
	}
}

func (m *Manager) Start(ctx context.Context, p Params) (*Session, error) {
	m.mu.Lock()
	if len(m.sessions)+m.pending >= m.cfg.MaxSessions {
		m.mu.Unlock()
		return nil, ErrBusy
	}
	m.pending++
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.pending--
		m.mu.Unlock()
	}()

	s := &Session{
		ID: newID(), UserID: p.UserID, ItemID: p.ItemID,
		StartSeconds: p.StartSeconds, Plan: p.Plan,
		done: make(chan struct{}), stderr: &tail{max: 4096},
	}
	s.Dir = filepath.Join(m.cfg.WorkDir, s.ID)
	s.touch()

	if err := m.launch(s, p); err != nil {
		m.cleanup(s)
		return nil, err
	}
	if err := m.waitReady(ctx, s); err != nil {
		m.cleanup(s)
		return nil, err
	}

	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	m.log.Printf("session %s ready: item=%s user=%s start=%.1fs %dx%d %dkbps hw=%v tonemap=%v",
		s.ID, s.ItemID, s.UserID, s.StartSeconds, s.Plan.Width, s.Plan.Height, s.Plan.VideoBitrate/1000, s.Plan.HWDecode, s.Plan.Tonemap)
	return s, nil
}

func (m *Manager) launch(s *Session, p Params) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("session: %w", err)
	}
	input := m.src.Register(s.ID, p.ItemID, p.MediaSourceID, p.Token)
	args := ffmpeg.Args(ffmpeg.Job{
		Input: input, StartSeconds: p.StartSeconds,
		SegmentSeconds: m.cfg.SegmentSeconds, Plan: p.Plan,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cmd := m.command(ctx, m.cfg.FFmpegPath, args...)
	cmd.Dir = s.Dir
	cmd.Stderr = s.stderr
	cmd.WaitDelay = 5 * time.Second
	// 參數裡沒有 token（輸入是本機 proxy），可以完整記錄
	m.log.Printf("session %s: ffmpeg %s", s.ID, strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		cancel()
		close(s.done)
		return fmt.Errorf("session: start ffmpeg: %w", err)
	}
	s.cancel = cancel
	go func() {
		s.waitErr = cmd.Wait()
		close(s.done)
		if s.waitErr != nil && ctx.Err() == nil {
			m.log.Printf("session %s: ffmpeg failed: %v: %s", s.ID, s.waitErr, s.stderr)
		} else if ctx.Err() == nil {
			m.log.Printf("session %s: ffmpeg finished", s.ID)
		}
	}()
	return nil
}

// waitReady 等到播放清單裡至少有兩段，播放端一開始就有緩衝。
func (m *Manager) waitReady(ctx context.Context, s *Session) error {
	timeout := time.NewTimer(m.cfg.ReadyTimeout)
	defer timeout.Stop()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		if segmentCount(s.Path(ffmpeg.PlaylistName)) >= 2 {
			return nil
		}
		select {
		case <-s.done:
			// 片子短於兩段時，ffmpeg 正常結束也算就緒
			if s.waitErr == nil && segmentCount(s.Path(ffmpeg.PlaylistName)) >= 1 {
				return nil
			}
			return fmt.Errorf("session: ffmpeg exited: %v: %s", s.waitErr, s.stderr)
		case <-timeout.C:
			return fmt.Errorf("%w: %s", ErrNotReady, s.stderr)
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func segmentCount(playlist string) int {
	b, err := os.ReadFile(playlist)
	if err != nil {
		return 0
	}
	return bytes.Count(b, []byte("#EXTINF"))
}

// Get 取得 session，同時更新最後存取時間。
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()
	if ok {
		s.touch()
	}
	return s, ok
}

func (m *Manager) Stop(id string) bool {
	m.mu.Lock()
	s, ok := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()
	if ok {
		m.cleanup(s)
		m.log.Printf("session %s stopped", id)
	}
	return ok
}

func (m *Manager) cleanup(s *Session) {
	if s.cancel != nil {
		s.cancel()
	}
	<-s.done
	m.src.Unregister(s.ID)
	removeAll(s.Dir)
}

// Run 定期回收閒置的 session，直到 ctx 結束，結束時停掉全部。
func (m *Manager) Run(ctx context.Context) {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			m.stopAll()
			return
		case <-tick.C:
			m.reapIdle(time.Now())
		}
	}
}

func (m *Manager) reapIdle(now time.Time) {
	var idle []string
	m.mu.Lock()
	for id, s := range m.sessions {
		if now.Sub(time.Unix(0, s.lastAccess.Load())) > m.cfg.IdleTimeout {
			idle = append(idle, id)
		}
	}
	m.mu.Unlock()
	for _, id := range idle {
		m.log.Printf("session %s idle, reaping", id)
		m.Stop(id)
	}
}

func (m *Manager) stopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Stop(id)
	}
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// removeAll 在 Windows 上重試：ffmpeg 剛結束時檔案可能還被占用一下。
func removeAll(dir string) {
	for i := 0; i < 10; i++ {
		if err := os.RemoveAll(dir); err == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// tail 只保留 ffmpeg stderr 的最後一段，用於錯誤訊息。
type tail struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-t.max:]...)
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.buf))
}
