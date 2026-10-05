package main

import (
	"os"
	"path/filepath"
	"testing"
)

// 用相對的 -config 啟動（例如在測試目錄裡執行 xcode.exe -config xcode.json）時，
// ffmpeg 與暫存目錄仍要是絕對路徑：ffmpeg 以 session 目錄為工作目錄啟動，相對路徑會找不到。
func TestLoadConfigRelativePathsBecomeAbsolute(t *testing.T) {
	dir := t.TempDir()
	cfg := `{"jellyfinUrl":"http://127.0.0.1:8096","ffmpegPath":"ffmpeg/ffmpeg.exe","workDir":"sessions"}`
	if err := os.WriteFile(filepath.Join(dir, "xcode.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	c, err := loadConfig("xcode.json")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "ffmpeg", "ffmpeg.exe"); c.FFmpegPath != want {
		t.Errorf("FFmpegPath = %q, want %q", c.FFmpegPath, want)
	}
	if want := filepath.Join(dir, "sessions"); c.WorkDir != want {
		t.Errorf("WorkDir = %q, want %q", c.WorkDir, want)
	}
}
