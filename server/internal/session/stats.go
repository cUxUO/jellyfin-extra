package session

import (
	"os"
	"sort"
	"sync/atomic"
	"time"
)

// sessionStats 是單一 session 的計數，給監控頁用。
type sessionStats struct {
	restarts       atomic.Int64 // 拖曳到遠處而重新啟動 ffmpeg
	stallRetries   atomic.Int64 // 一段都沒轉出（停滯或失敗）而重新啟動
	segmentsServed atomic.Int64
	bytesServed    atomic.Int64
	lastSegment    atomic.Int64 // 播放端最近請求的片段，約等於播放位置
}

// totals 是伺服器啟動以來的累計。
type totals struct {
	started        atomic.Int64
	failed         atomic.Int64
	busy           atomic.Int64
	restarts       atomic.Int64
	stallRetries   atomic.Int64
	segmentsServed atomic.Int64
	bytesServed    atomic.Int64
}

// RecordServed 記錄播放端取走第 n 段（size 位元組）。
func (m *Manager) RecordServed(s *Session, n int, size int64) {
	s.stats.lastSegment.Store(int64(n))
	s.stats.segmentsServed.Add(1)
	s.stats.bytesServed.Add(size)
	m.totals.segmentsServed.Add(1)
	m.totals.bytesServed.Add(size)
}

// SessionInfo 是監控頁顯示的一個 session，不含 token。
type SessionInfo struct {
	ID             string    `json:"id"`
	ItemID         string    `json:"itemId"`
	Title          string    `json:"title"`
	UserName       string    `json:"userName"`
	Client         string    `json:"client"`
	Profile        string    `json:"profile"`
	Created        time.Time `json:"created"`
	LastAccess     time.Time `json:"lastAccess"`
	SegmentSeconds int       `json:"segmentSeconds"`
	Segments       int       `json:"segments"`

	Codec         string `json:"codec"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	VideoBitrate  int64  `json:"videoBitrate"`
	AudioBitrate  int64  `json:"audioBitrate"`
	SourceWidth   int    `json:"sourceWidth"`
	SourceHeight  int    `json:"sourceHeight"`
	HWDecode      bool   `json:"hwDecode"`
	Tonemap       bool   `json:"tonemap"`
	AudioIndex    int    `json:"audioIndex"`
	SubtitleIndex int    `json:"subtitleIndex"`

	// 目前這次 ffmpeg 執行：從 RunStart 段開始，已連續轉出到 Frontier 段之前
	Running     bool    `json:"running"`
	RunStart    int     `json:"runStart"`
	Frontier    int     `json:"frontier"`
	RunSeconds  float64 `json:"runSeconds"`
	Speed       float64 `json:"speed"` // 轉碼速度（倍速）
	LastSegment int     `json:"lastSegment"`

	Restarts       int64 `json:"restarts"`
	StallRetries   int64 `json:"stallRetries"`
	SegmentsServed int64 `json:"segmentsServed"`
	BytesServed    int64 `json:"bytesServed"`
}

// Totals 是伺服器啟動以來的累計。
type Totals struct {
	Started        int64 `json:"started"`
	Failed         int64 `json:"failed"`
	Busy           int64 `json:"busy"`
	Restarts       int64 `json:"restarts"`
	StallRetries   int64 `json:"stallRetries"`
	SegmentsServed int64 `json:"segmentsServed"`
	BytesServed    int64 `json:"bytesServed"`
}

type Snapshot struct {
	MaxSessions int           `json:"maxSessions"`
	Starting    int           `json:"starting"` // 啟動中、還沒就緒
	Sessions    []SessionInfo `json:"sessions"`
	Totals      Totals        `json:"totals"`
}

// Snapshot 回傳目前所有 session 的狀態，依建立時間排序。不會更新最後存取時間。
func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	list := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	starting := m.pending
	m.mu.Unlock()
	sort.Slice(list, func(i, j int) bool { return list[i].Created.Before(list[j].Created) })

	now := time.Now()
	seg := m.cfg.SegmentSeconds
	infos := make([]SessionInfo, 0, len(list))
	for _, s := range list {
		p := s.Plan
		info := SessionInfo{
			ID: s.ID, ItemID: s.ItemID, Title: s.Title, UserName: s.UserName, Client: s.Client, Profile: s.Profile,
			Created: s.Created, LastAccess: time.Unix(0, s.lastAccess.Load()),
			SegmentSeconds: seg, Segments: s.Segments,
			Codec: p.VideoCodec, Width: p.Width, Height: p.Height, VideoBitrate: p.VideoBitrate, AudioBitrate: p.AudioBitrate,
			SourceWidth: p.SourceWidth, SourceHeight: p.SourceHeight, HWDecode: p.HWDecode, Tonemap: p.Tonemap,
			AudioIndex: p.AudioIndex, SubtitleIndex: p.SubtitleIndex,
			LastSegment:    int(s.stats.lastSegment.Load()),
			Restarts:       s.stats.restarts.Load(),
			StallRetries:   s.stats.stallRetries.Load(),
			SegmentsServed: s.stats.segmentsServed.Load(),
			BytesServed:    s.stats.bytesServed.Load(),
		}
		s.mu.Lock()
		if r := s.run; r != nil {
			info.Running = !r.finished()
			info.RunStart = r.startSeg
			info.Frontier = s.frontier(r)
			info.RunSeconds = now.Sub(r.started).Seconds()
			if info.RunSeconds > 0 {
				info.Speed = float64((info.Frontier-r.startSeg)*seg) / info.RunSeconds
			}
		}
		s.mu.Unlock()
		infos = append(infos, info)
	}
	return Snapshot{
		MaxSessions: m.cfg.MaxSessions,
		Starting:    starting,
		Sessions:    infos,
		Totals: Totals{
			Started: m.totals.started.Load(), Failed: m.totals.failed.Load(), Busy: m.totals.busy.Load(),
			Restarts: m.totals.restarts.Load(), StallRetries: m.totals.stallRetries.Load(),
			SegmentsServed: m.totals.segmentsServed.Load(), BytesServed: m.totals.bytesServed.Load(),
		},
	}
}

// WorkDirUsage 回傳所有 session 目錄的檔案總大小（轉出的片段）。
func (m *Manager) WorkDirUsage() int64 {
	m.mu.Lock()
	dirs := make([]string, 0, len(m.sessions))
	for _, s := range m.sessions {
		dirs = append(dirs, s.Dir)
	}
	m.mu.Unlock()
	var total int64
	for _, d := range dirs {
		total += dirSize(d)
	}
	return total
}

func dirSize(dir string) int64 {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var n int64
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !info.IsDir() {
			n += info.Size()
		}
	}
	return n
}

// WorkDir 是 session 暫存目錄所在的位置（監控頁用來查剩餘空間）。
func (m *Manager) WorkDir() string { return m.cfg.WorkDir }
