package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"jellyfin-extra/server/internal/profile"
)

// TestHelperProcess 不是真的測試：被 Manager 當成 ffmpeg 啟動，行為由環境變數決定。
// "ok" 模式讀 -start_number，從那一段起寫出 4 段，模擬 ffmpeg 往後轉。
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv("FAKE_FFMPEG")
	if mode == "" {
		return
	}
	args := os.Args[slices.Index(os.Args, "--")+1:]
	switch mode {
	case "ok":
		start, _ := strconv.Atoi(args[slices.Index(args, "-start_number")+1])
		for n := start; n < start+4; n++ {
			os.WriteFile(fmt.Sprintf("seg_%05d.ts", n), []byte("x"), 0o644)
		}
		time.Sleep(time.Minute) // 直到被結束
	case "stall-once":
		// 第一次執行卡住不輸出（模擬索引不完整的跳轉），之後的執行正常
		if _, err := os.Stat("stalled"); err != nil {
			os.WriteFile("stalled", nil, 0o644)
			time.Sleep(time.Minute)
		}
		start, _ := strconv.Atoi(args[slices.Index(args, "-start_number")+1])
		for n := start; n < start+4; n++ {
			os.WriteFile(fmt.Sprintf("seg_%05d.ts", n), []byte("x"), 0o644)
		}
		time.Sleep(time.Minute)
	case "fail":
		fmt.Fprintln(os.Stderr, "Unknown decoder 'bogus'")
		os.Exit(1)
	case "fail-once":
		// 第一次執行沒轉出就失敗（模擬偶發的開檔錯誤），之後的執行正常
		if _, err := os.Stat("failed"); err != nil {
			os.WriteFile("failed", nil, 0o644)
			fmt.Fprintln(os.Stderr, "Error opening input")
			os.Exit(1)
		}
		start, _ := strconv.Atoi(args[slices.Index(args, "-start_number")+1])
		for n := start; n < start+4; n++ {
			os.WriteFile(fmt.Sprintf("seg_%05d.ts", n), []byte("x"), 0o644)
		}
		time.Sleep(time.Minute)
	}
	os.Exit(0)
}

type fakeRegistry struct {
	registered map[string]bool
	reading    atomic.Bool // true 時每次查詢讀取量都增加，模擬 ffmpeg 持續讀來源
	read       atomic.Int64
}

func (f *fakeRegistry) Register(id, _, _, _ string) string {
	f.registered[id] = true
	return "http://127.0.0.1:1/src/" + id
}
func (f *fakeRegistry) Unregister(id string) { delete(f.registered, id) }
func (f *fakeRegistry) BytesRead(string) int64 {
	if f.reading.Load() {
		return f.read.Add(1)
	}
	return f.read.Load()
}

func newTestManager(t *testing.T, mode string, max int) (*Manager, *fakeRegistry) {
	t.Helper()
	reg := &fakeRegistry{registered: map[string]bool{}}
	m, err := NewManager(Config{
		FFmpegPath: os.Args[0], WorkDir: t.TempDir(), MaxSessions: max,
		IdleTimeout: time.Minute, ReadyTimeout: 5 * time.Second, StallTimeout: 500 * time.Millisecond, SegmentSeconds: 3,
	}, reg, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	m.command = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, name, append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)...)
		cmd.Env = append(os.Environ(), "FAKE_FFMPEG="+mode)
		return cmd
	}
	return m, reg
}

var plan = profile.Plan{Width: 1280, Height: 720, VideoBitrate: 3_000_000, AudioIndex: -1, AudioInput: -1, SubtitleIndex: -1, SubtitleInput: -1, CodecProfile: "high", CodecLevel: "4.0"}

// 300 秒的片，每段 3 秒，共 100 段
func params(startSeconds float64) Params {
	return Params{ItemID: "item", Token: "tok", StartSeconds: startSeconds, RunTimeSeconds: 300, Plans: []profile.Plan{plan}}
}

func runStart(s *Session) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Variants[0].run.startSeg
}

func TestStartAndStop(t *testing.T) {
	m, reg := newTestManager(t, "ok", 1)
	s, err := m.Start(context.Background(), params(0))
	if err != nil {
		t.Fatal(err)
	}
	if !ValidID(s.ID) || !reg.registered[s.ID] || s.Segments != 100 {
		t.Fatalf("id=%q registered=%v segments=%d", s.ID, reg.registered, s.Segments)
	}
	if !strings.Contains(string(s.Playlist), "#EXT-X-ENDLIST") {
		t.Error("playlist must be a complete VOD playlist")
	}
	if _, err := m.Start(context.Background(), params(0)); !errors.Is(err, ErrBusy) {
		t.Fatalf("second start err = %v, want ErrBusy", err)
	}

	if !m.Stop(s.ID) {
		t.Fatal("Stop returned false")
	}
	if _, err := os.Stat(s.Dir); !os.IsNotExist(err) {
		t.Fatalf("session dir still exists: %v", err)
	}
	if reg.registered[s.ID] {
		t.Fatal("source not unregistered")
	}
}

func TestStartMidwayAlignsToSegment(t *testing.T) {
	m, _ := newTestManager(t, "ok", 1)
	s, err := m.Start(context.Background(), params(31)) // 31 秒落在第 10 段（30–33 秒）
	if err != nil {
		t.Fatal(err)
	}
	if got := runStart(s); got != 10 {
		t.Fatalf("started at seg %d, want 10", got)
	}
}

func TestSegmentWaitReuseRestart(t *testing.T) {
	m, _ := newTestManager(t, "ok", 1)
	ctx := context.Background()
	s, err := m.Start(ctx, params(0))
	if err != nil {
		t.Fatal(err)
	}

	// 已轉出的段落直接回傳
	if p, err := m.Segment(ctx, s.ID, 0, 1); err != nil || filepath.Base(p) != "seg_00001.ts" {
		t.Fatalf("Segment(1) = %q, %v", p, err)
	}
	// 拖曳到遠處：從第 50 段重新啟動
	if _, err := m.Segment(ctx, s.ID, 0, 50); err != nil {
		t.Fatal(err)
	}
	if got := runStart(s); got != 50 {
		t.Fatalf("after seek run starts at %d, want 50", got)
	}
	// 往回拖到先前轉過的位置：沿用舊檔案，不重新啟動
	if _, err := m.Segment(ctx, s.ID, 0, 2); err != nil {
		t.Fatal(err)
	}
	if got := runStart(s); got != 50 {
		t.Fatalf("existing segment caused a restart (run starts at %d)", got)
	}
	// 往回拖到沒轉過的位置：重新啟動
	if _, err := m.Segment(ctx, s.ID, 0, 20); err != nil {
		t.Fatal(err)
	}
	if got := runStart(s); got != 20 {
		t.Fatalf("backward seek run starts at %d, want 20", got)
	}
	// 超出片長
	if _, err := m.Segment(ctx, s.ID, 0, 100); !errors.Is(err, ErrNoSegment) {
		t.Fatalf("Segment(100) err = %v, want ErrNoSegment", err)
	}
	if _, err := m.Segment(ctx, "ffffffffffffffffffffffffffffffff", 0, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown session err = %v", err)
	}
}

func TestStartFailureReportsStderr(t *testing.T) {
	m, reg := newTestManager(t, "fail", 1)
	_, err := m.Start(context.Background(), params(0))
	if err == nil || !strings.Contains(err.Error(), "Unknown decoder") {
		t.Fatalf("err = %v, want ffmpeg stderr", err)
	}
	if len(reg.registered) != 0 || len(m.sessions) != 0 || m.pending != 0 {
		t.Fatal("failed start leaked state")
	}
}

func TestFailedRunIsRetried(t *testing.T) {
	m, _ := newTestManager(t, "fail-once", 1)
	s, err := m.Start(context.Background(), params(31))
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	start, attempt := s.Variants[0].run.startSeg, s.Variants[0].run.attempt
	s.mu.Unlock()
	if start != 10 || attempt != 1 {
		t.Fatalf("retry from seg %d attempt %d, want seg 10 attempt 1", start, attempt)
	}
	if got := m.Snapshot().Totals.StallRetries; got != 1 {
		t.Errorf("retries counted = %d, want 1", got)
	}
}

func TestTailDropsAttachmentWarnings(t *testing.T) {
	tl := &tail{max: 4096}
	io.WriteString(tl, "[http @ 0x1] HTTP error 500\n")
	for i := 5; i < 30; i++ {
		// 分段寫入，模擬 stderr 不照行邊界送來
		io.WriteString(tl, fmt.Sprintf("[in#0/matroska,webm @ 0x2] Could not find codec parameters for stream %d (Attachment: none): unknown codec\nConsider increasing the value for the 'analy", i))
		io.WriteString(tl, "zeduration' (0) and 'probesize' (5000000) options\n")
	}
	io.WriteString(tl, "Error opening input: I/O error")
	got := tl.String()
	if strings.Contains(got, "Attachment") || strings.Contains(got, "Consider increasing") {
		t.Errorf("noise kept: %q", got)
	}
	if !strings.Contains(got, "HTTP error 500") || !strings.HasSuffix(got, "Error opening input: I/O error") {
		t.Errorf("tail = %q", got)
	}
}

func TestReapIdleAndStaleDirs(t *testing.T) {
	m, _ := newTestManager(t, "ok", 2)
	s, err := m.Start(context.Background(), params(0))
	if err != nil {
		t.Fatal(err)
	}
	m.reapIdle(time.Now())
	if _, ok := m.Get(s.ID); !ok {
		t.Fatal("fresh session reaped")
	}
	m.reapIdle(time.Now().Add(2 * time.Minute))
	if _, ok := m.Get(s.ID); ok {
		t.Fatal("idle session not reaped")
	}

	// 上次留下的 session 目錄要清掉，其他目錄不能碰
	stale := filepath.Join(m.cfg.WorkDir, newID())
	keep := filepath.Join(m.cfg.WorkDir, "ffmpeg")
	os.MkdirAll(stale, 0o755)
	os.MkdirAll(keep, 0o755)
	m.removeStale()
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale session dir not removed")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Error("unrelated dir removed")
	}
}

func TestStalledRunIsRestarted(t *testing.T) {
	m, _ := newTestManager(t, "stall-once", 1)
	start := time.Now()
	s, err := m.Start(context.Background(), params(31))
	if err != nil {
		t.Fatal(err)
	}
	if got := runStart(s); got != 10 {
		t.Fatalf("retry started at seg %d, want 10", got)
	}
	s.mu.Lock()
	attempt := s.Variants[0].run.attempt
	s.mu.Unlock()
	if attempt != 1 {
		t.Fatalf("attempt = %d, want 1", attempt)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %s; stall retry should kick in after StallTimeout", d)
	}
}

func TestRunWithoutSourceReadsIsRestartedEarly(t *testing.T) {
	m, _ := newTestManager(t, "stall-once", 1)
	m.cfg.StallTimeout = time.Minute // 只靠沒有讀取來判斷
	m.cfg.IOStallTimeout = 300 * time.Millisecond
	start := time.Now()
	s, err := m.Start(context.Background(), params(31))
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	attempt := s.Variants[0].run.attempt
	s.mu.Unlock()
	if attempt != 1 {
		t.Fatalf("attempt = %d, want 1", attempt)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %s; restart should kick in after IOStallTimeout", d)
	}
}

func TestRunStillReadingIsNotRestartedEarly(t *testing.T) {
	m, reg := newTestManager(t, "stall-once", 1)
	reg.reading.Store(true) // 還在讀（例如 Cues 不完整時循序讀到目標），不算沒有讀取
	m.cfg.StallTimeout = 0
	m.cfg.IOStallTimeout = 200 * time.Millisecond
	m.cfg.ReadyTimeout = time.Second
	if _, err := m.Start(context.Background(), params(31)); !errors.Is(err, ErrNotReady) {
		t.Fatalf("err = %v, want ErrNotReady (no restart while reading)", err)
	}
}

func TestSnapshotCountsRestartsAndServed(t *testing.T) {
	m, _ := newTestManager(t, "ok", 2)
	ctx := context.Background()
	p := params(0)
	p.Title, p.UserName, p.Client, p.Profile = "Movie (2024)", "alice", "192.168.0.9", "zenpad10"
	s, err := m.Start(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Segment(ctx, s.ID, 0, 50); err != nil { // 跳到遠處：重新啟動一次
		t.Fatal(err)
	}
	m.RecordServed(s, 50, 1000)
	if _, err := m.Start(ctx, params(0)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(ctx, params(0)); !errors.Is(err, ErrBusy) {
		t.Fatalf("third start err = %v", err)
	}

	snap := m.Snapshot()
	if snap.MaxSessions != 2 || len(snap.Sessions) != 2 {
		t.Fatalf("snapshot: max=%d sessions=%d", snap.MaxSessions, len(snap.Sessions))
	}
	got := snap.Sessions[0] // 依建立時間排序
	if got.ID != s.ID || got.Title != "Movie (2024)" || got.UserName != "alice" || got.Client != "192.168.0.9" || got.Profile != "zenpad10" {
		t.Errorf("display fields = %+v", got)
	}
	if got.Restarts != 1 || got.RunStart != 50 || got.LastSegment != 50 || got.SegmentsServed != 1 || got.BytesServed != 1000 {
		t.Errorf("counters = %+v", got)
	}
	if !got.Running || got.Frontier <= got.RunStart {
		t.Errorf("run state: running=%v start=%d frontier=%d", got.Running, got.RunStart, got.Frontier)
	}
	tot := snap.Totals
	if tot.Started != 2 || tot.Busy != 1 || tot.Restarts != 1 || tot.SegmentsServed != 1 || tot.BytesServed != 1000 {
		t.Errorf("totals = %+v", tot)
	}
	if m.WorkDirUsage() <= 0 {
		t.Error("work dir usage should count segment files")
	}
}

func TestVariants(t *testing.T) {
	m, _ := newTestManager(t, "ok", 1)
	ctx := context.Background()
	low := plan
	low.Width, low.Height = 640, 360
	p := params(0)
	p.Plans = []profile.Plan{plan, low}
	s, err := m.Start(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(s.Playlist), "v1.m3u8") || !strings.Contains(string(s.Playlist), "RESOLUTION=640x360") {
		t.Fatalf("master playlist = %s", s.Playlist)
	}
	if pl, ok := s.PlaylistFile("v1.m3u8"); !ok || !strings.Contains(string(pl), "v1_seg_00000.ts") {
		t.Fatalf("v1 playlist = %s", pl)
	}

	// 切到第 1 軌：在自己的目錄裡另外啟動 ffmpeg，第 0 軌照常
	path, err := m.Segment(ctx, s.ID, 1, 5)
	if err != nil || path != filepath.Join(s.Dir, "v1", "seg_00005.ts") {
		t.Fatalf("v1 seg 5 = %q, %v", path, err)
	}
	if _, err := m.Segment(ctx, s.ID, 2, 0); !errors.Is(err, ErrNoSegment) {
		t.Errorf("missing variant err = %v", err)
	}
	if got := m.Snapshot().Sessions[0]; got.Variant != 1 || got.Variants != 2 || got.Height != 360 {
		t.Errorf("snapshot variant = %d/%d %dp", got.Variant, got.Variants, got.Height)
	}

	// 第 0 軌很久沒被請求：停掉它的 ffmpeg，第 1 軌留著
	s.Variants[0].lastRequest.Store(time.Now().Add(-time.Minute).UnixNano())
	m.stopIdleVariants(time.Now())
	s.mu.Lock()
	v0, v1 := s.Variants[0].run, s.Variants[1].run
	s.mu.Unlock()
	if v0 != nil || v1 == nil || v1.finished() {
		t.Errorf("after idle stop: v0 run %v, v1 run %v", v0, v1)
	}
	// 切回第 0 軌：已轉出的片段直接沿用
	if p, err := m.Segment(ctx, s.ID, 0, 0); err != nil || filepath.Base(p) != "seg_00000.ts" {
		t.Errorf("v0 seg 0 = %q, %v", p, err)
	}
}
