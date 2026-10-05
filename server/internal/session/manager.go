// Package session 管理轉碼工作：依請求轉出片段、拖曳時重新啟動 ffmpeg、閒置回收、清理暫存檔。
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
	"jellyfin-extra/server/internal/proc"
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

// maxRetries：一次執行一段都沒轉出就停滯（超過 StallTimeout）或失敗結束時，從同一段重新啟動的次數上限。
const maxRetries = 2

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
	// Plans 是各軌的轉碼計畫，第一軌畫質最高、開播時先轉它。多於一軌時 index.m3u8 是主播放清單，
	// 播放端依頻寬在各軌間切換（自適應）。
	Plans []profile.Plan

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
	Variants []*Variant
	Segments int    // 播放清單裡的總段數（各軌相同）
	Playlist []byte // index.m3u8：一軌時是完整 VOD 清單，多軌時是主播放清單
	Created  time.Time
	Title    string
	UserName string
	Client   string
	Profile  string

	stats sessionStats

	input       string
	mu          sync.Mutex // 保護各軌的 run 與重新啟動的決定
	lastAccess  atomic.Int64
	lastVariant atomic.Int32 // 播放端最近請求的軌，監控頁顯示用
}

// Variant 是 session 的一軌（一種解析度）。各軌片段時間點一致，播放端可以隨時切換。
type Variant struct {
	Index    int
	Plan     profile.Plan
	Dir      string // 這一軌的片段目錄；只有一軌時就是 session 目錄
	Playlist []byte // 多軌時的 v{n}.m3u8

	run         *run         // 只在 Session.mu 下讀寫
	lastRequest atomic.Int64 // 最近一次請求這一軌片段的時間
	waiters     atomic.Int32 // 正在等這一軌片段的請求數
}

// Plan 是開播時那一軌（畫質最高）的計畫。
func (s *Session) Plan() profile.Plan { return s.Variants[0].Plan }

// Variant 回傳第 i 軌。
func (s *Session) Variant(i int) (*Variant, bool) {
	if i < 0 || i >= len(s.Variants) {
		return nil, false
	}
	return s.Variants[i], true
}

// PlaylistFile 回傳播放端請求的播放清單：index.m3u8，多軌時另有 v{n}.m3u8。
func (s *Session) PlaylistFile(name string) ([]byte, bool) {
	if name == ffmpeg.PlaylistName {
		return s.Playlist, true
	}
	if len(s.Variants) > 1 {
		for _, v := range s.Variants {
			if name == ffmpeg.VariantPlaylistName(v.Index) {
				return v.Playlist, true
			}
		}
	}
	return nil, false
}

// label 是 log 裡的 session 名稱，多軌時帶軌道編號。
func (s *Session) label(v *Variant) string {
	if len(s.Variants) > 1 {
		return fmt.Sprintf("%s v%d", s.ID, v.Index)
	}
	return s.ID
}

// run 是一次 ffmpeg 執行，從 startSeg 開始往後轉。
type run struct {
	startSeg int
	started  time.Time
	attempt  int // 一段都沒轉出而重新啟動的次數（停滯或失敗）
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

func (s *Session) touch() { s.lastAccess.Store(time.Now().UnixNano()) }

// segmentPath 是第 n 段在這一軌目錄裡的路徑。
func (v *Variant) segmentPath(n int) string { return filepath.Join(v.Dir, ffmpeg.SegmentName(n)) }

func (v *Variant) segmentExists(n int) bool {
	_, err := os.Stat(v.segmentPath(n))
	return err == nil
}

// frontier 回傳 v 的這次執行從起點連續轉出的下一段編號。呼叫端必須持有 s.mu。
func (s *Session) frontier(v *Variant, r *run) int {
	for r.next < s.Segments && v.segmentExists(r.next) {
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

	if len(p.Plans) == 0 {
		return nil, errors.New("session: no transcode plan")
	}
	seg := m.cfg.SegmentSeconds
	s := &Session{
		ID: newID(), UserID: p.UserID, ItemID: p.ItemID, Created: time.Now(),
		Title: p.Title, UserName: p.UserName, Client: p.Client, Profile: p.Profile,
		Segments: ffmpeg.SegmentCount(p.RunTimeSeconds, seg),
	}
	s.Dir = filepath.Join(m.cfg.WorkDir, s.ID)
	if len(p.Plans) == 1 {
		s.Variants = []*Variant{{Plan: p.Plans[0], Dir: s.Dir}}
		s.Playlist = ffmpeg.VODPlaylist(p.RunTimeSeconds, seg)
	} else {
		vs := make([]ffmpeg.Variant, len(p.Plans))
		for i, plan := range p.Plans {
			s.Variants = append(s.Variants, &Variant{
				Index: i, Plan: plan, Dir: filepath.Join(s.Dir, fmt.Sprintf("v%d", i)),
				Playlist: ffmpeg.VariantVODPlaylist(p.RunTimeSeconds, seg, i),
			})
			vs[i] = ffmpeg.VariantOf(plan)
		}
		s.Playlist = ffmpeg.MasterPlaylist(vs)
	}
	s.touch()
	for _, v := range s.Variants {
		if err := os.MkdirAll(v.Dir, 0o755); err != nil {
			return nil, fmt.Errorf("session: %w", err)
		}
	}
	s.input = m.src.Register(s.ID, p.ItemID, p.MediaSourceID, p.Token)

	startSeg := min(int(p.StartSeconds)/seg, s.Segments-1)
	v0 := s.Variants[0]
	v0.lastRequest.Store(time.Now().UnixNano())
	s.mu.Lock()
	err := m.startRun(s, v0, startSeg)
	s.mu.Unlock()
	if err == nil {
		err = m.waitSegment(ctx, s, v0, startSeg)
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
	top := s.Plan()
	m.log.Printf("session %s ready: item=%s user=%s start=seg%d/%d %s %dx%d %dkbps hw=%v tonemap=%v variants=%d",
		s.ID, s.ItemID, s.UserID, startSeg, s.Segments, top.VideoCodec, top.Width, top.Height, top.VideoBitrate/1000, top.HWDecode, top.Tonemap, len(s.Variants))
	return s, nil
}

// Segment 確保第 variant 軌的第 n 段存在並回傳檔案路徑。片段已轉出就直接回傳；在 ffmpeg 進度附近就等它；
// 否則（使用者拖曳到遠處、往回拖到沒轉過的地方、或播放端切到這一軌）從第 n 段重新啟動這一軌的 ffmpeg。
func (m *Manager) Segment(ctx context.Context, id string, variant, n int) (string, error) {
	s, ok := m.Get(id)
	if !ok {
		return "", ErrNotFound
	}
	v, ok := s.Variant(variant)
	if !ok || n < 0 || n >= s.Segments {
		return "", ErrNoSegment
	}
	v.lastRequest.Store(time.Now().UnixNano())
	s.lastVariant.Store(int32(variant))
	if v.segmentExists(n) {
		return v.segmentPath(n), nil
	}
	v.waiters.Add(1)
	defer v.waiters.Add(-1)

	s.mu.Lock()
	if r := v.run; r == nil || n < r.startSeg || n > s.frontier(v, r)+lookahead || r.finished() {
		// 已結束的執行不會再產生片段：成功結束代表 n 在這次起點之前，失敗則重試一次
		if err := m.startRun(s, v, n); err != nil {
			s.mu.Unlock()
			return "", err
		}
	}
	s.mu.Unlock()

	if err := m.waitSegment(ctx, s, v, n); err != nil {
		return "", err
	}
	return v.segmentPath(n), nil
}

// startRun 停掉目前的 ffmpeg（若有），從第 startSeg 段重新啟動。呼叫端必須持有 s.mu。
func (m *Manager) startRun(s *Session, v *Variant, startSeg int) error {
	return m.startRunAttempt(s, v, startSeg, 0)
}

func (m *Manager) startRunAttempt(s *Session, v *Variant, startSeg, attempt int) error {
	if old := v.run; old != nil {
		old.cancel()
		<-old.done
		m.log.Printf("session %s: restart at seg %d (was from seg %d)", s.label(v), startSeg, old.startSeg)
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
		SegmentSeconds: m.cfg.SegmentSeconds, Plan: v.Plan,
	})

	ctx, cancel := context.WithCancel(context.Background())
	r := &run{startSeg: startSeg, started: time.Now(), attempt: attempt, cancel: cancel, done: make(chan struct{}), stderr: &tail{max: 4096}, next: startSeg}
	cmd := m.command(ctx, m.cfg.FFmpegPath, args...)
	proc.NoWindow(cmd)
	cmd.Dir = v.Dir
	cmd.Stderr = r.stderr
	cmd.WaitDelay = 5 * time.Second
	// 參數裡沒有 token（輸入是本機 proxy），可以完整記錄
	m.log.Printf("session %s: ffmpeg %s", s.label(v), strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		cancel()
		close(r.done)
		v.run = nil
		return fmt.Errorf("session: start ffmpeg: %w", err)
	}
	v.run = r
	go func() {
		r.err = cmd.Wait()
		close(r.done)
		if r.err != nil && ctx.Err() == nil {
			m.log.Printf("session %s: ffmpeg failed: %v: %s", s.label(v), r.err, r.stderr)
		}
	}()
	return nil
}

// waitSegment 等第 n 段出現；負責轉它的 ffmpeg 結束了還沒出現就回報錯誤。
func (m *Manager) waitSegment(ctx context.Context, s *Session, v *Variant, n int) error {
	timeout := time.NewTimer(m.cfg.ReadyTimeout)
	defer timeout.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if v.segmentExists(n) {
			return nil
		}
		s.mu.Lock()
		r := v.run
		s.mu.Unlock()
		var done <-chan struct{}
		if r != nil {
			done = r.done
		}
		select {
		case <-done:
			if v.segmentExists(n) {
				return nil
			}
			s.mu.Lock()
			replaced := v.run != r
			s.mu.Unlock()
			if replaced {
				// 被另一個請求的重新啟動取代（不是失敗），改等新的那次執行
				continue
			}
			if r.err != nil {
				if m.retryFailed(s, v, r) {
					continue
				}
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
			m.retryIfStalled(s, v, r)
		}
	}
}

// retryIfStalled 在 r 超過 StallTimeout 仍一段都沒轉出時，從同一段重新啟動。
// 多個等待者可能同時發現；只有 r 仍是目前的執行時才重新啟動。
func (m *Manager) retryIfStalled(s *Session, v *Variant, r *run) {
	if r == nil || m.cfg.StallTimeout <= 0 || r.attempt >= maxRetries || time.Since(r.started) < m.cfg.StallTimeout {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.run != r || r.finished() || s.frontier(v, r) > r.startSeg {
		return
	}
	m.log.Printf("session %s: no segment from seg %d after %s, retrying (%d/%d)",
		s.label(v), r.startSeg, m.cfg.StallTimeout, r.attempt+1, maxRetries)
	if err := m.startRunAttempt(s, v, r.startSeg, r.attempt+1); err != nil {
		m.log.Printf("session %s: retry: %v", s.label(v), err)
	}
}

// retryFailed 在 r 一段都沒轉出就失敗結束時，從同一段重新啟動，回傳是否該改等新的執行。
// 偶發的開檔或讀取錯誤（例如拖曳後立刻失敗、從同一位置再開就正常）不必讓播放端看到錯誤。
func (m *Manager) retryFailed(s *Session, v *Variant, r *run) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.run != r {
		return true // 另一個等待者已經重新啟動
	}
	if r.attempt >= maxRetries || s.frontier(v, r) > r.startSeg {
		return false
	}
	m.log.Printf("session %s: ffmpeg failed before seg %d, retrying (%d/%d)", s.label(v), r.startSeg, r.attempt+1, maxRetries)
	if err := m.startRunAttempt(s, v, r.startSeg, r.attempt+1); err != nil {
		m.log.Printf("session %s: retry: %v", s.label(v), err)
		return false
	}
	return true
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
	for _, v := range s.Variants {
		if r := v.run; r != nil {
			r.cancel()
			<-r.done
		}
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
			m.stopIdleVariants(time.Now())
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

// variantIdle：多軌時，一軌這麼久沒被請求（播放端已切到別軌）就停掉它的 ffmpeg，不讓它在背景佔 GPU。
// 已轉出的片段保留，切回來時沿用。
const variantIdle = 15 * time.Second

func (m *Manager) stopIdleVariants(now time.Time) {
	m.mu.Lock()
	list := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		if len(s.Variants) > 1 {
			list = append(list, s)
		}
	}
	m.mu.Unlock()
	for _, s := range list {
		s.mu.Lock()
		for _, v := range s.Variants {
			r := v.run
			if r == nil || r.finished() || v.waiters.Load() > 0 || now.Sub(time.Unix(0, v.lastRequest.Load())) < variantIdle {
				continue
			}
			r.cancel()
			<-r.done
			v.run = nil
			m.log.Printf("session %s: idle variant, stopped", s.label(v))
		}
		s.mu.Unlock()
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
// 附件（MKV 內嵌字型）探測不到參數的警告會濾掉：動畫常有幾十個字型，每個兩行，會把真正的錯誤擠出去。
type tail struct {
	mu          sync.Mutex
	buf         []byte
	partial     []byte // 還沒收到換行的最後一行
	max         int
	afterAttach bool // 上一行是附件警告，接著的「Consider increasing…」也一起濾掉
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.partial = append(t.partial, p...)
	for {
		i := bytes.IndexByte(t.partial, '\n')
		if i < 0 {
			break
		}
		t.line(t.partial[:i+1])
		t.partial = t.partial[i+1:]
	}
	if len(t.partial) > t.max {
		t.partial = t.partial[len(t.partial)-t.max:]
	}
	t.partial = append([]byte(nil), t.partial...)
	return len(p), nil
}

func (t *tail) line(l []byte) {
	attach := bytes.Contains(l, []byte("Could not find codec parameters")) && bytes.Contains(l, []byte("(Attachment:"))
	hint := t.afterAttach && bytes.HasPrefix(l, []byte("Consider increasing the value for the 'analyzeduration'"))
	t.afterAttach = attach
	if attach || hint {
		return
	}
	t.buf = append(t.buf, l...)
	if len(t.buf) > t.max {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-t.max:]...)
	}
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.buf) + string(t.partial))
}
