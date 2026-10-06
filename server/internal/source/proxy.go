// Package source 提供 ffmpeg 讀取原始檔的來源。
//
// 目前的實作是在 127.0.0.1 上轉發 Jellyfin 的原始檔串流，由這裡替 ffmpeg 附上 token，
// token 因此不會出現在 ffmpeg 的命令列或 log。日後要改成直接讀 NAS，只需換掉這個實作。
package source

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"jellyfin-extra/server/internal/jellyfin"
)

// Registry 讓 session 管理器取得 ffmpeg 的輸入網址，與來源實作無關。
type Registry interface {
	Register(sessionID, itemID, mediaSourceID, token string) (inputURL string)
	Unregister(sessionID string)
	// BytesRead 是 session 的 ffmpeg 至今從來源讀到的位元組數，用來判斷 ffmpeg 是否卡住不讀。
	BytesRead(sessionID string) int64
}

type entry struct {
	upstream *url.URL
	token    string
	read     *atomic.Int64 // 已轉給 ffmpeg 的位元組數（監控頁用）
	total    *atomic.Int64 // Proxy 的累計
}

type entryKey struct{}

type Proxy struct {
	// Log 不為 nil 時記錄每個請求（Range、狀態、位元組數、耗時），用來追查 ffmpeg 卡在哪個讀取。
	// 網址只含 session ID，不含 token。
	Log *log.Logger

	jf   *jellyfin.Client
	srv  *http.Server
	addr string

	mu      sync.Mutex
	entries map[string]entry
	total   atomic.Int64 // 啟動以來從 Jellyfin 讀取的總量
}

func NewProxy(jf *jellyfin.Client) *Proxy {
	p := &Proxy{jf: jf, entries: map[string]entry{}}
	rp := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			e := r.In.Context().Value(entryKey{}).(entry)
			u := *e.upstream // 每個請求各用一份，避免共用指標
			r.Out.URL = &u
			r.Out.Host = u.Host
			r.Out.Header.Set("Authorization", jellyfin.AuthHeader(e.token))
			// Range 等標頭由 ReverseProxy 原樣帶過去，ffmpeg 的跳轉靠它
		},
		ModifyResponse: func(resp *http.Response) error {
			if t, ok := resp.Request.Context().Value(traceKey{}).(*trace); ok {
				t.upstream = time.Since(t.start)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if t, ok := r.Context().Value(traceKey{}).(*trace); ok {
				t.err = err
			}
			w.WriteHeader(http.StatusBadGateway)
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /src/{id}", func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		e, ok := p.entries[r.PathValue("id")]
		p.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), entryKey{}, e)
		cw := &countingWriter{ResponseWriter: w, read: e.read, total: e.total}
		if p.Log == nil {
			rp.ServeHTTP(cw, r.WithContext(ctx))
			return
		}
		t := &trace{start: time.Now()}
		func() {
			// 用戶端中途斷線時 ReverseProxy 以 ErrAbortHandler 結束，記錄後照樣往上拋
			defer func() {
				if v := recover(); v != nil {
					if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) && t.err == nil {
						t.err = err
					}
					p.logRequest(r, cw, t)
					panic(v)
				}
			}()
			rp.ServeHTTP(cw, r.WithContext(context.WithValue(ctx, traceKey{}, t)))
		}()
		p.logRequest(r, cw, t)
	})
	p.srv = &http.Server{Handler: mux}
	return p
}

type traceKey struct{}

// trace 記錄一個來源請求的時間點；upstream 是收到 Jellyfin 回應標頭的時間。
type trace struct {
	start    time.Time
	upstream time.Duration
	err      error
}

type countingWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
	read   *atomic.Int64
	total  *atomic.Int64
}

func (c *countingWriter) WriteHeader(code int) {
	c.status = code
	c.ResponseWriter.WriteHeader(code)
}

func (c *countingWriter) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	n, err := c.ResponseWriter.Write(b)
	c.bytes += int64(n)
	c.read.Add(int64(n))
	c.total.Add(int64(n))
	return n, err
}

// Flush 讓 ReverseProxy 照常即時送出資料。
func (c *countingWriter) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (p *Proxy) logRequest(r *http.Request, cw *countingWriter, t *trace) {
	id := r.PathValue("id")
	if len(id) > 8 {
		id = id[:8]
	}
	msg := fmt.Sprintf("source %s range=%q status=%d bytes=%d upstream=%s total=%s",
		id, r.Header.Get("Range"), cw.status, cw.bytes, t.upstream.Round(time.Millisecond), time.Since(t.start).Round(time.Millisecond))
	if t.err != nil {
		msg += " err=" + t.err.Error()
	}
	p.Log.Print(msg)
}

// Start 只聽 loopback，外部連不到。
func (p *Proxy) Start() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("source proxy: %w", err)
	}
	p.addr = ln.Addr().String()
	go p.srv.Serve(ln)
	return nil
}

func (p *Proxy) Close() error { return p.srv.Close() }

func (p *Proxy) Register(sessionID, itemID, mediaSourceID, token string) string {
	u, err := url.Parse(p.jf.StreamURL(itemID, mediaSourceID))
	if err != nil {
		// StreamURL 由已驗證的 base URL 組成，不會失敗
		panic(err)
	}
	p.mu.Lock()
	p.entries[sessionID] = entry{upstream: u, token: token, read: new(atomic.Int64), total: &p.total}
	p.mu.Unlock()
	return "http://" + p.addr + "/src/" + sessionID
}

// BytesRead 回傳某個 session 已從 Jellyfin 讀取並轉給 ffmpeg 的位元組數。
func (p *Proxy) BytesRead(sessionID string) int64 {
	p.mu.Lock()
	e, ok := p.entries[sessionID]
	p.mu.Unlock()
	if !ok {
		return 0
	}
	return e.read.Load()
}

// TotalRead 是啟動以來從 Jellyfin 讀取的總位元組數（包含已結束的 session）。
func (p *Proxy) TotalRead() int64 { return p.total.Load() }

func (p *Proxy) Unregister(sessionID string) {
	p.mu.Lock()
	delete(p.entries, sessionID)
	p.mu.Unlock()
}
