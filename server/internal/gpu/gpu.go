// Package gpu 用 nvidia-smi 讀取 GPU 狀態，給監控頁用。
package gpu

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"jellyfin-extra/server/internal/proc"
)

// Stats 是一張 GPU 的即時狀態；nvidia-smi 回報 N/A 的欄位為 -1。
type Stats struct {
	Name         string  `json:"name"`
	Driver       string  `json:"driver"`
	Utilization  float64 `json:"utilization"` // %
	Encoder      float64 `json:"encoder"`     // NVENC 使用率 %
	Decoder      float64 `json:"decoder"`     // NVDEC 使用率 %
	MemoryUsed   float64 `json:"memoryUsed"`  // MiB
	MemoryTotal  float64 `json:"memoryTotal"` // MiB
	Temperature  float64 `json:"temperature"` // °C
	Power        float64 `json:"power"`       // W
	PowerLimit   float64 `json:"powerLimit"`  // W
	Fan          float64 `json:"fan"`         // %
	ClockMHz     float64 `json:"clockMHz"`
	EncSessions  float64 `json:"encSessions"`  // NVENC 工作階段數（所有程式）
	EncFPS       float64 `json:"encFps"`       // NVENC 平均編碼 fps
	EncLatencyUs float64 `json:"encLatencyUs"` // NVENC 平均延遲（微秒）
}

var fields = []string{
	"name", "driver_version", "utilization.gpu", "utilization.encoder", "utilization.decoder",
	"memory.used", "memory.total", "temperature.gpu", "power.draw", "power.limit", "fan.speed", "clocks.gr",
	"encoder.stats.sessionCount", "encoder.stats.averageFps", "encoder.stats.averageLatency",
}

// Monitor 快取最近一次查詢，避免監控頁每次刷新都啟動 nvidia-smi。
type Monitor struct {
	MaxAge time.Duration

	mu    sync.Mutex
	at    time.Time
	stats []Stats
	err   error
}

func NewMonitor() *Monitor { return &Monitor{MaxAge: 2 * time.Second} }

// Query 回傳每張 GPU 的狀態；沒有 nvidia-smi（例如在 Linux 容器內開發）時回傳錯誤。
func (m *Monitor) Query(ctx context.Context) ([]Stats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.at.IsZero() && time.Since(m.at) < m.MaxAge {
		return m.stats, m.err
	}
	m.stats, m.err = query(ctx)
	m.at = time.Now()
	return m.stats, m.err
}

func query(ctx context.Context) ([]Stats, error) {
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil, errors.New("gpu: nvidia-smi not found")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--query-gpu="+strings.Join(fields, ","), "--format=csv,noheader,nounits")
	proc.NoWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gpu: nvidia-smi: %w", err)
	}
	return parse(string(out))
}

func parse(out string) ([]Stats, error) {
	var list []Stats
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		v := strings.Split(line, ",")
		if len(v) != len(fields) {
			return nil, fmt.Errorf("gpu: unexpected nvidia-smi output: %q", line)
		}
		for i := range v {
			v[i] = strings.TrimSpace(v[i])
		}
		f := func(i int) float64 {
			n, err := strconv.ParseFloat(v[i], 64)
			if err != nil {
				return -1 // [N/A]、[Not Supported]
			}
			return n
		}
		list = append(list, Stats{
			Name: v[0], Driver: v[1], Utilization: f(2), Encoder: f(3), Decoder: f(4),
			MemoryUsed: f(5), MemoryTotal: f(6), Temperature: f(7), Power: f(8), PowerLimit: f(9),
			Fan: f(10), ClockMHz: f(11), EncSessions: f(12), EncFPS: f(13), EncLatencyUs: f(14),
		})
	}
	return list, nil
}
