// Package session 管理轉碼工作：依請求轉出片段、拖曳時重新啟動 ffmpeg、閒置回收、清理暫存檔。
package session

import (
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
	ErrBusy      = errors.New("session: too many active sessions")
	ErrNotFound  = errors.New("session: not found")
	ErrNoSegment = errors.New("session: segment out of range")
	ErrNotReady  = errors.New("session: transcode did not produce the segment in time")
)

// idPattern 也用來辨識 WorkDir 裡哪些目錄是我們建立的，清理時只碰這些。
var idPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func ValidID(id string) bool { return idPattern.MatchString(id) }

// lookahead：請求的片段在 ffmpeg 目前進度之後幾段以內就等它轉出，超過就從該段重新啟動。
// ffmpeg 至少以 2 倍速前進，等兩段約 3 秒，和重新啟動的成本差不多。
const lookahead = 2

// maxStallRetries：一次執行超過 StallTimeout 還沒轉出任何一段時，從同一段重新啟動的次數上限。
const maxStallRetries = 2

type Config struct {
	FFmpegPath   string
	WorkDir      string
	MaxSessions  int // NVENC 消費級驅動有同時編碼數上限
	IdleTimeout  time.Duration
	ReadyTimeout time.Duration
	// StallTimeout：一次 ffmpeg 執行在這段時間內一段都沒轉出就重新啟動（0 表示不重試）。
	// FFmpeg 經 HTTP 跳轉大型 MKV 時，延後解析的 Cues 索引有機率不完整（上游問題，8.1 與 9.0 皆然），
	// 只能從較前面的位置循序讀到目標，大檔要讀好幾 GB；同樣的跳轉重新啟動通常就正常。
	StallTimeout   time.Duration
	SegmentSeconds int
}

type Params struct {
	UserID         string
	ItemID         string
	MediaSourceID  string
	Token          string // 只交給 source registry，不保存在 Session
	StartSeconds   float64
	RunTimeSeconds float64
	Plan           profile.Plan

	// 以下只用於監控頁顯示
	Title    string // 片名（集數含影集名稱）
	UserName string
	Client   string // 播放端位址；經反向代理時是轉送前的位址
	Profile  string
}

type Session struct {
	ID       string
	Dir      string
	UserID   string
	ItemID   string
	Plan     profile.Plan
	Segments int    // 播放清單裡的總段數
	Playlist []byte // 給播放端的完整 VOD 清單
	Created  time.Time
	Title    string
	UserName string
	Client   string
	Profile  string

	stats sessionStats

	input      string
	mu         sync.Mutex // 保護 run 與重新啟動的決定
	run        *run
	lastAccess atomic.Int64
}

// run 是一次 ffmpeg 執行，從 startSeg 開始往後轉。
type run struct {
	startSeg int
	started  time.Time
	attempt  int // 因停滯重新啟動的次數
	cancel   context.CancelFunc
	done     chan struct{}
	err      error
	stderr   *tail
	next     int // 從 startSeg 起連續存在的下一段，快取用，只在 Session.mu 下讀寫
}

func (r *run) finished() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// Path 回傳 session 目錄內的檔案路徑；name 必須先經過呼叫端驗證。
func (s *Session) Path(name string) string { return filepath.Join(s.Dir, name) }

func (s *Session) touch() { s.lastAccess.Store(time.Now().UnixNano()) }

func (s *Session) segmentExists(n int) bool {
	_, err := os.Stat(s.Path(ffmpeg.SegmentName(n)))
	return err == nil
}

// frontier 回傳目前這次執行從起點連續轉出的下一段編號。呼叫端必須持有 s.mu。
func (s *Session) frontier(r *run) int {
	for r.next < s.Segments && s.segmentExists(r.next) {
		r.next++
	}
	return r.next
}

type Manager struct {
	cfg Config
	src source.Registry
	log *log.Logger

	// command 讓測試可以換掉 ffmpeg
	command func(ctx context.Context, name string, args ...string) *exec.Cmd

	mu       sync.Mutex
	sessions map[string]*Session
	pending  int // 啟動中、尚未就緒的數量，也佔名額

	totals totals
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

// Start 建立 session，從 StartSeconds 所在的段落開始轉，等那一段轉出後才回傳。
func (m *Manager) Start(ctx context.Context, p Params) (*Session, error) {
	m.mu.Lock()
	if len(m.sessions)+m.pending >= m.cfg.MaxSessions {
		m.mu.Unlock()
		m.totals.busy.Add(1)
		return nil, ErrBusy
	}
	m.pending++
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.pending--
		m.mu.Unlock()
	}()

	seg := m.cfg.SegmentSeconds
	s := &Session{
		ID: newID(), UserID: p.UserID, ItemID: p.ItemID, Plan: p.Plan, Created: time.Now(),
		Title: p.Title, UserName: p.UserName, Client: p.Client, Profile: p.Profile,
		Segments: ffmpeg.SegmentCount(p.RunTimeSeconds, seg),
		Playlist: ffmpeg.VODPlaylist(p.RunTimeSeconds, seg),
	}
	s.Dir = filepath.Join(m.cfg.WorkDir, s.ID)
	s.touch()
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}
	s.input = m.src.Register(s.ID, p.ItemID, p.MediaSourceID, p.Token)

	startSeg := min(int(p.StartSeconds)/seg, s.Segments-1)
	s.mu.Lock()
	err := m.startRun(s, startSeg)
	s.mu.Unlock()
	if err == nil {
		err = m.waitSegment(ctx, s, startSeg)
	}
	if err != nil {
		m.cleanup(s)
		m.totals.failed.Add(1)
		return nil, err
	}

	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	m.totals.started.Add(1)
	s.stats.lastSegment.Store(int64(startSeg))
	m.log.Printf("session %s ready: item=%s user=%s start=seg%d/%d %dx%d %dkbps hw=%v tonemap=%v",
		s.ID, s.ItemID, s.UserID, startSeg, s.Segments, s.Plan.Width, s.Plan.Height, s.Plan.VideoBitrate/1000, s.Plan.HWDecode, s.Plan.Tonemap)
	return s, nil
}

// Segment 確保第 n 段存在並回傳檔案路徑。片段已轉出就直接回傳；在 ffmpeg 進度附近就等它；
// 否則（使用者拖曳到遠處或往回拖到沒轉過的地方）從第 n 段重新啟動 ffmpeg。
func (m *Manager) Segment(ctx context.Context, id string, n int) (string, error) {
	s, ok := m.Get(id)
	if !ok {
		return "", ErrNotFound
	}
	if n < 0 || n >= s.Segments {
		return "", ErrNoSegment
	}
	if s.segmentExists(n) {
		return s.Path(ffmpeg.SegmentName(n)), nil
	}

	s.mu.Lock()
	if r := s.run; r == nil || n < r.startSeg || n > s.frontier(r)+lookahead || r.finished() {
		// 已結束的執行不會再產生片段：成功結束代表 n 在這次起點之前，失敗則重試一次
		if err := m.startRun(s, n); err != nil {
			s.mu.Unlock()
			return "", err
		}
	}
	s.mu.Unlock()

	if err := m.waitSegment(ctx, s, n); err != nil {
		return "", err
	}
	return s.Path(ffmpeg.SegmentName(n)), nil
}

// startRun 停掉目前的 ffmpeg（若有），從第 startSeg 段重新啟動。呼叫端必須持有 s.mu。
func (m *Manager) startRun(s *Session, startSeg int) error {
	return m.startRunAttempt(s, startSeg, 0)
}

func (m *Manager) startRunAttempt(s *Session, startSeg, attempt int) error {
	if old := s.run; old != nil {
		old.cancel()
		<-old.done
		m.log.Printf("session %s: restart at seg %d (was from seg %d)", s.ID, startSeg, old.startSeg)
		if attempt > 0 {
			s.stats.stallRetries.Add(1)
			m.totals.stallRetries.Add(1)
		} else {
			s.stats.restarts.Add(1)
			m.totals.restarts.Add(1)
		}
	}
	args := ffmpeg.Args(ffmpeg.Job{
		Input: s.input, StartSegment: startSeg,
		SegmentSeconds: m.cfg.SegmentSeconds, Plan: s.Plan,
	})

	ctx, cancel := context.WithCancel(context.Background())
	r := &run{startSeg: startSeg, started: time.Now(), attempt: attempt, cancel: cancel, done: make(chan struct{}), stderr: &tail{max: 4096}, next: startSeg}
	cmd := m.command(ctx, m.cfg.FFmpegPath, args...)
	cmd.Dir = s.Dir
	cmd.Stderr = r.stderr
	cmd.WaitDelay = 5 * time.Second
	// 參數裡沒有 token（輸入是本機 proxy），可以完整記錄
	m.log.Printf("session %s: ffmpeg %s", s.ID, strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		cancel()
		close(r.done)
		s.run = nil
		return fmt.Errorf("session: start ffmpeg: %w", err)
	}
	s.run = r
	go func() {
		r.err = cmd.Wait()
		close(r.done)
		if r.err != nil && ctx.Err() == nil {
			m.log.Printf("session %s: ffmpeg failed: %v: %s", s.ID, r.err, r.stderr)
		}
	}()
	return nil
}

// waitSegment 等第 n 段出現；負責轉它的 ffmpeg 結束了還沒出現就回報錯誤。
func (m *Manager) waitSegment(ctx context.Context, s *Session, n int) error {
	timeout := time.NewTimer(m.cfg.ReadyTimeout)
	defer timeout.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if s.segmentExists(n) {
			return nil
		}
		s.mu.Lock()
		r := s.run
		s.mu.Unlock()
		var done <-chan struct{}
		if r != nil {
			done = r.done
		}
		select {
		case <-done:
			if s.segmentExists(n) {
				return nil
			}
			s.mu.Lock()
			replaced := s.run != r
			s.mu.Unlock()
			if replaced {
				// 被另一個請求的重新啟動取代（不是失敗），改等新的那次執行
				continue
			}
			if r.err != nil {
				return fmt.Errorf("session: ffmpeg exited: %v: %s", r.err, r.stderr)
			}
			// 成功結束卻沒有這一段：片長比 Jellyfin 記錄的短，最後一段不存在
			return ErrNoSegment
		case <-timeout.C:
			if r != nil {
				return fmt.Errorf("%w: %s", ErrNotReady, r.stderr)
			}
			return ErrNotReady
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			m.retryIfStalled(s, r)
		}
	}
}

// retryIfStalled 在 r 超過 StallTimeout 仍一段都沒轉出時，從同一段重新啟動。
// 多個等待者可能同時發現；只有 r 仍是目前的執行時才重新啟動。
func (m *Manager) retryIfStalled(s *Session, r *run) {
	if r == nil || m.cfg.StallTimeout <= 0 || r.attempt >= maxStallRetries || time.Since(r.started) < m.cfg.StallTimeout {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run != r || r.finished() || s.frontier(r) > r.startSeg {
		return
	}
	m.log.Printf("session %s: no segment from seg %d after %s, retrying (%d/%d)",
		s.ID, r.startSeg, m.cfg.StallTimeout, r.attempt+1, maxStallRetries)
	if err := m.startRunAttempt(s, r.startSeg, r.attempt+1); err != nil {
		m.log.Printf("session %s: retry: %v", s.ID, err)
	}
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
	s.mu.Lock()
	if r := s.run; r != nil {
		r.cancel()
		<-r.done
	}
	s.mu.Unlock()
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
