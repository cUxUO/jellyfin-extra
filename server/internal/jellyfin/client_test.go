package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCurrentUserSendsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Users/Me" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != `MediaBrowser Token="good"` {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"Id":"u1","Name":"jack"}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	u, err := c.CurrentUser(context.Background(), "good")
	if err != nil || u.ID != "u1" {
		t.Fatalf("CurrentUser = %+v, %v", u, err)
	}
	if _, err := c.CurrentUser(context.Background(), "bad"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("bad token err = %v, want ErrUnauthorized", err)
	}
}

func TestPlaybackInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/jf/Items/abc/PlaybackInfo":
			if r.URL.Query().Get("userId") != "u1" {
				t.Errorf("userId = %q", r.URL.Query().Get("userId"))
			}
			w.Write([]byte(`{"MediaSources":[{"Id":"ms1","RunTimeTicks":600000000,
				"MediaStreams":[{"Type":"Video","Index":0,"Codec":"hevc","Width":3840,"Height":2160,"VideoRange":"HDR","VideoRangeType":"HDR10"},
				{"Type":"Audio","Index":1,"Codec":"eac3","Channels":6,"IsDefault":true}]}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c, _ := New(srv.URL + "/jf")
	info, err := c.PlaybackInfo(context.Background(), "t", "u1", "abc")
	if err != nil {
		t.Fatal(err)
	}
	ms := info.MediaSources[0]
	if ms.ID != "ms1" || len(ms.MediaStreams) != 2 || ms.MediaStreams[0].VideoRange != "HDR" || ms.MediaStreams[1].Channels != 6 {
		t.Fatalf("unexpected %+v", ms)
	}
	if _, err := c.PlaybackInfo(context.Background(), "t", "u1", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing err = %v", err)
	}
}

func TestStreamURL(t *testing.T) {
	c, _ := New("http://jf:8096/base")
	got := c.StreamURL("abc", "ms1")
	want := "http://jf:8096/base/Videos/abc/stream?mediaSourceId=ms1&static=true"
	if got != want {
		t.Fatalf("StreamURL = %q, want %q", got, want)
	}
}
