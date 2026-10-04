package source

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"jellyfin-extra/server/internal/jellyfin"
)

func TestProxyInjectsTokenAndForwardsRange(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Videos/item1/stream" || r.URL.Query().Get("static") != "true" || r.URL.Query().Get("mediaSourceId") != "ms1" {
			t.Errorf("upstream got %s", r.URL)
		}
		if got := r.Header.Get("Authorization"); got != `MediaBrowser Token="secret"` {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Range"); got != "bytes=10-19" {
			t.Errorf("Range = %q", got)
		}
		w.Header().Set("Content-Range", "bytes 10-19/100")
		w.WriteHeader(http.StatusPartialContent)
		io.WriteString(w, "0123456789")
	}))
	defer upstream.Close()

	jf, _ := jellyfin.New(upstream.URL)
	p := NewProxy(jf)
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	in := p.Register("sess1", "item1", "ms1", "secret")
	req, _ := http.NewRequest(http.MethodGet, in, nil)
	req.Header.Set("Range", "bytes=10-19")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || string(body) != "0123456789" || resp.Header.Get("Content-Range") != "bytes 10-19/100" {
		t.Fatalf("got %d %q %q", resp.StatusCode, body, resp.Header.Get("Content-Range"))
	}

	p.Unregister("sess1")
	resp, err = http.Get(in)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("after Unregister status = %d, want 404", resp.StatusCode)
	}
}
