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
	SegmentSeconds      int    `json:"segmentSeconds"`
}

func loadConfig(path string) (config, error) {
	c := config{
		Listen:              ":8097",
		MaxSessions:         3,
		IdleTimeoutSeconds:  120,
		ReadyTimeoutSeconds: 30,
		SegmentSeconds:      3,
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("config: %w", err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("config %s: %w", path, err)
	}

	// 相對路徑以設定檔所在目錄為準，不受啟動時工作目錄影響
	base := filepath.Dir(path)
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
	case c.MaxSessions < 1 || c.IdleTimeoutSeconds < 10 || c.ReadyTimeoutSeconds < 1 || c.SegmentSeconds < 1:
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
		SegmentSeconds: c.SegmentSeconds,
	}
}
