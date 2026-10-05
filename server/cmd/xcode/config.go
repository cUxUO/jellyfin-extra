package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"jellyfin-extra/server/internal/session"
)

// config 對應 xcode.json。這個檔案含有內網位址，不進版控，範本見 xcode.example.json。
type config struct {
	Listen              string `json:"listen"`
	JellyfinURL         string `json:"jellyfinUrl"`
	FFmpegPath          string `json:"ffmpegPath"`
	WorkDir             string `json:"workDir"`
	MaxSessions         int    `json:"maxSessions"`
	IdleTimeoutSeconds  int    `json:"idleTimeoutSeconds"`
	ReadyTimeoutSeconds int    `json:"readyTimeoutSeconds"`
	// StallTimeoutSeconds：ffmpeg 啟動後這麼久一段都沒轉出就重新啟動（0 表示不重試）。
	StallTimeoutSeconds int `json:"stallTimeoutSeconds"`
	SegmentSeconds      int `json:"segmentSeconds"`
	// LogSource 記錄每個原始檔請求（除錯用）。
	LogSource bool `json:"logSource"`
}

func loadConfig(path string) (config, error) {
	c := config{
		Listen:              ":8097",
		MaxSessions:         3,
		IdleTimeoutSeconds:  120,
		ReadyTimeoutSeconds: 30,
		StallTimeoutSeconds: 5,
		SegmentSeconds:      3,
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("config: %w", err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("config %s: %w", path, err)
	}

	// 相對路徑以設定檔所在目錄為準，不受啟動時工作目錄影響。一律轉成絕對路徑：
	// ffmpeg 以 session 目錄為工作目錄啟動，相對的 ffmpegPath 會被當成相對於 session 目錄而找不到
	// （例如用 -config xcode.json 啟動時）。
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return c, fmt.Errorf("config: %w", err)
	}
	for _, p := range []*string{&c.FFmpegPath, &c.WorkDir} {
		if *p != "" && !filepath.IsAbs(*p) {
			*p = filepath.Join(base, *p)
		}
	}

	switch {
	case c.JellyfinURL == "":
		return c, errors.New("config: jellyfinUrl is required")
	case c.FFmpegPath == "":
		return c, errors.New("config: ffmpegPath is required")
	case c.WorkDir == "":
		return c, errors.New("config: workDir is required")
	case c.MaxSessions < 1 || c.IdleTimeoutSeconds < 10 || c.ReadyTimeoutSeconds < 1 || c.StallTimeoutSeconds < 0 || c.SegmentSeconds < 1:
		return c, errors.New("config: maxSessions, idleTimeoutSeconds (>=10), readyTimeoutSeconds, segmentSeconds must be positive")
	}
	return c, nil
}

func (c config) session() session.Config {
	return session.Config{
		FFmpegPath:     c.FFmpegPath,
		WorkDir:        c.WorkDir,
		MaxSessions:    c.MaxSessions,
		IdleTimeout:    time.Duration(c.IdleTimeoutSeconds) * time.Second,
		ReadyTimeout:   time.Duration(c.ReadyTimeoutSeconds) * time.Second,
		StallTimeout:   time.Duration(c.StallTimeoutSeconds) * time.Second,
		SegmentSeconds: c.SegmentSeconds,
	}
}
