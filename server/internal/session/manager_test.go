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
	"strings"
	"testing"
	"time"

	"jellyfin-extra/server/internal/profile"
)

// TestHelperProcess 不是真的測試：被 Manager 當成 ffmpeg 啟動，行為由環境變數決定。
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv("FAKE_FFMPEG")
	if mode == "" {
		return
	}
	switch mode {
	case "ok":
		os.WriteFile("seg_00000.ts", []byte("x"), 0o644)
		os.WriteFile("index.m3u8", []byte("#EXTM3U\n#EXTINF:3,\nseg_00000.ts\n#EXTINF:3,\nseg_00001.ts\n"), 0o644)
		time.Sleep(time.Minute) // 直到被結束
	case "fail":
		fmt.Fprintln(os.Stderr, "Unknown decoder 'bogus'")
		os.Exit(1)
	}
	os.Exit(0)
}

type fakeRegistry struct{ registered map[string]bool }

func (f *fakeRegistry) Register(id, _, _, _ string) string {
	f.registered[id] = true
	return "http://127.0.0.1:1/src/" + id
}
func (f *fakeRegistry) Unregister(id string) { delete(f.registered, id) }

func newTestManager(t *testing.T, mode string, max int) (*Manager, *fakeRegistry) {
	t.Helper()
	reg := &fakeRegistry{registered: map[string]bool{}}
	m, err := NewManager(Config{
		FFmpegPath: os.Args[0], WorkDir: t.TempDir(), MaxSessions: max,
		IdleTimeout: time.Minute, ReadyTimeout: 5 * time.Second, SegmentSeconds: 3,
	}, reg, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	m.command = func(ctx context.Context, name string, _ ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, name, "-test.run=^TestHelperProcess$")
		cmd.Env = append(os.Environ(), "FAKE_FFMPEG="+mode)
		return cmd
	}
	return m, reg
}

var plan = profile.Plan{Width: 1280, Height: 720, VideoBitrate: 3_000_000, AudioIndex: -1, AudioInput: -1, H264Profile: "high", H264Level: "4.0"}

func TestStartReadyAndStop(t *testing.T) {
	m, reg := newTestManager(t, "ok", 1)
	s, err := m.Start(context.Background(), Params{ItemID: "item", Token: "tok", Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	if !ValidID(s.ID) || !reg.registered[s.ID] {
		t.Fatalf("id=%q registered=%v", s.ID, reg.registered)
	}
	if _, err := os.Stat(s.Path("index.m3u8")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(context.Background(), Params{Plan: plan}); !errors.Is(err, ErrBusy) {
		t.Fatalf("second start err = %v, want ErrBusy", err)
	}
	if _, ok := m.Get(s.ID); !ok {
		t.Fatal("Get failed")
	}

	if !m.Stop(s.ID) {
		t.Fatal("Stop returned false")
	}
	if _, err := os.Stat(s.Dir); !os.IsNotExist(err) {
		t.Fatalf("session dir still exists: %v", err)
	}
	if reg.registered[s.ID] || !s.Finished() {
		t.Fatal("not cleaned up")
	}
}

func TestStartFailureReportsStderr(t *testing.T) {
	m, reg := newTestManager(t, "fail", 1)
	_, err := m.Start(context.Background(), Params{Plan: plan})
	if err == nil || !strings.Contains(err.Error(), "Unknown decoder") {
		t.Fatalf("err = %v, want ffmpeg stderr", err)
	}
	if len(reg.registered) != 0 || len(m.sessions) != 0 || m.pending != 0 {
		t.Fatal("failed start leaked state")
	}
}

func TestReapIdleAndStaleDirs(t *testing.T) {
	m, _ := newTestManager(t, "ok", 2)
	s, err := m.Start(context.Background(), Params{Plan: plan})
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
