// Package logring 保留最近幾行 log，給監控頁的事件列表用。
package logring

import (
	"bytes"
	"sync"
)

// Ring 是 io.Writer：把寫入的內容切成行，只保留最後 Max 行。
// 與 log.Logger 搭配時每次 Write 剛好是一行；不完整的行會等到換行才收下。
type Ring struct {
	mu      sync.Mutex
	max     int
	lines   []string
	partial []byte
}

func New(max int) *Ring { return &Ring{max: max} }

func (r *Ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.partial = append(r.partial, p...)
	for {
		i := bytes.IndexByte(r.partial, '\n')
		if i < 0 {
			break
		}
		r.lines = append(r.lines, string(bytes.TrimRight(r.partial[:i], "\r")))
		r.partial = r.partial[i+1:]
	}
	if over := len(r.lines) - r.max; over > 0 {
		r.lines = append([]string(nil), r.lines[over:]...)
	}
	return len(p), nil
}

// Lines 回傳目前保留的行，舊的在前。
func (r *Ring) Lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...)
}
