// Package source 提供 ffmpeg 讀取原始檔的來源。
//
// 目前的實作是在 127.0.0.1 上轉發 Jellyfin 的原始檔串流，由這裡替 ffmpeg 附上 token，
// token 因此不會出現在 ffmpeg 的命令列或 log。日後要改成直接讀 NAS，只需換掉這個實作。
package source

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"

	"jellyfin-extra/server/internal/jellyfin"
)

// Registry 讓 session 管理器取得 ffmpeg 的輸入網址，與來源實作無關。
type Registry interface {
	Register(sessionID, itemID, mediaSourceID, token string) (inputURL string)
	Unregister(sessionID string)
}

type entry struct {
	upstream *url.URL
	token    string
}

type entryKey struct{}

type Proxy struct {
	jf   *jellyfin.Client
	srv  *http.Server
	addr string

	mu      sync.Mutex
	entries map[string]entry
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
		rp.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), entryKey{}, e)))
	})
	p.srv = &http.Server{Handler: mux}
	return p
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
	p.entries[sessionID] = entry{upstream: u, token: token}
	p.mu.Unlock()
	return "http://" + p.addr + "/src/" + sessionID
}

func (p *Proxy) Unregister(sessionID string) {
	p.mu.Lock()
	delete(p.entries, sessionID)
	p.mu.Unlock()
}
