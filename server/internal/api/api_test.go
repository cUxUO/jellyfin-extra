package api

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jellyfin-extra/server/internal/jellyfin"
	"jellyfin-extra/server/internal/profile"
	"jellyfin-extra/server/internal/session"
)

const (
	itemID = "0123456789abcdef0123456789abcdef"
	sessID = "fedcba9876543210fedcba9876543210"
)

type fakeJF struct{}

func (fakeJF) CurrentUser(_ context.Context, token string) (*jellyfin.User, error) {
	if token != "good" {
		return nil, jellyfin.ErrUnauthorized
	}
	return &jellyfin.User{ID: "u1"}, nil
}

func (fakeJF) PlaybackInfo(_ context.Context, _, _, id string) (*jellyfin.PlaybackInfo, error) {
	if id != itemID {
		return nil, jellyfin.ErrNotFound
	}
	return &jellyfin.PlaybackInfo{MediaSources: []jellyfin.MediaSource{{
		ID: itemID, RunTimeTicks: 600 * ticksPerSecond,
		MediaStreams: []jellyfin.MediaStream{
			{Type: "Video", Index: 0, Codec: "hevc", Width: 3840, Height: 2160, VideoRange: "HDR"},
			{Type: "Audio", Index: 1, Codec: "eac3", Channels: 6},
		},
	}}}, nil
}

type fakeSessions struct {
	dir     string
	started *session.Params
	stopped string
}

func (f *fakeSessions) Start(_ context.Context, p session.Params) (*session.Session, error) {
	f.started = &p
	return &session.Session{ID: sessID, Dir: f.dir, Plan: p.Plan}, nil
}

func (f *fakeSessions) Get(id string) (*session.Session, bool) {
	if id != sessID {
		return nil, false
	}
	return &session.Session{ID: sessID, Dir: f.dir}, true
}

func (f *fakeSessions) Stop(id string) bool { f.stopped = id; return id == sessID }

func newServer(t *testing.T) (*httptest.Server, *fakeSessions) {
	t.Helper()
	fs := &fakeSessions{dir: t.TempDir()}
	s := &Server{JF: fakeJF{}, Sessions: fs, Profiles: profile.Profiles, Log: log.New(io.Discard, "", 0)}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv, fs
}

func post(t *testing.T, url, token, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url+"/v1/sessions", strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", `MediaBrowser Client="test", Token="`+token+`"`)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestCreateSession(t *testing.T) {
	srv, fs := newServer(t)
	resp := post(t, srv.URL, "good", `{"itemId":"`+itemID+`","profile":"zenpad10","startTimeTicks":900000000}`)
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, b)
	}
	var got createResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if got.SessionID != sessID || got.Playlist != "v1/sessions/"+sessID+"/index.m3u8" {
		t.Errorf("response = %+v", got)
	}
	if got.Video.Width != 1280 || got.Video.Height != 720 || !got.Video.Tonemap || got.AudioStreamIndex != 1 {
		t.Errorf("video = %+v audio = %d", got.Video, got.AudioStreamIndex)
	}
	p := fs.started
	if p == nil || p.Token != "good" || p.UserID != "u1" || p.StartSeconds != 90 || p.MediaSourceID != itemID {
		t.Errorf("params = %+v", p)
	}
}

func TestCreateSessionErrors(t *testing.T) {
	srv, _ := newServer(t)
	cases := []struct {
		name, token, body string
		want              int
	}{
		{"no token", "", `{"itemId":"` + itemID + `","profile":"zenpad10"}`, 401},
		{"bad token", "bad", `{"itemId":"` + itemID + `","profile":"zenpad10"}`, 401},
		{"bad json", "good", `{`, 400},
		{"path in item id", "good", `{"itemId":"../../Users","profile":"zenpad10"}`, 400},
		{"unknown profile", "good", `{"itemId":"` + itemID + `","profile":"tv"}`, 400},
		{"missing item", "good", `{"itemId":"ffffffffffffffffffffffffffffffff","profile":"zenpad10"}`, 404},
		{"start past end", "good", `{"itemId":"` + itemID + `","profile":"zenpad10","startTimeTicks":6000000000}`, 400},
	}
	for _, c := range cases {
		if got := post(t, srv.URL, c.token, c.body).StatusCode; got != c.want {
			t.Errorf("%s: status %d, want %d", c.name, got, c.want)
		}
	}
}

func TestServeFile(t *testing.T) {
	srv, fs := newServer(t)
	os.WriteFile(filepath.Join(fs.dir, "index.m3u8"), []byte("#EXTM3U\n#EXT-X-PLAYLIST-TYPE:EVENT\n"), 0o644)
	os.WriteFile(filepath.Join(fs.dir, "seg_00000.ts"), []byte("ts"), 0o644)
	os.WriteFile(filepath.Join(fs.dir, "secret.txt"), []byte("no"), 0o644)

	check := func(path string, want int, ctype string) {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want || (ctype != "" && resp.Header.Get("Content-Type") != ctype) {
			t.Errorf("GET %s = %d %q, want %d %q", path, resp.StatusCode, resp.Header.Get("Content-Type"), want, ctype)
		}
	}
	base := "/v1/sessions/" + sessID + "/"
	check(base+"index.m3u8", 200, "application/vnd.apple.mpegurl")
	check(base+"seg_00000.ts", 200, "video/mp2t")
	check(base+"seg_00001.ts", 404, "") // 還沒轉出來
	check(base+"secret.txt", 404, "")
	check("/v1/sessions/"+strings.Repeat("0", 32)+"/index.m3u8", 404, "")
}

func TestDeleteSession(t *testing.T) {
	srv, fs := newServer(t)
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/sessions/"+sessID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || fs.stopped != sessID {
		t.Fatalf("status %d stopped %q", resp.StatusCode, fs.stopped)
	}
}

func TestTokenFrom(t *testing.T) {
	cases := map[string]string{
		`MediaBrowser Token="abc123"`:                                      "abc123",
		`MediaBrowser Client="x", Device="y", Token="abc123", Version="1"`: "abc123",
		`MediaBrowser token=abc123`:                                        "abc123",
		`Bearer abc123`:                                                    "",
		``:                                                                 "",
	}
	for h, want := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", h)
		if got := tokenFrom(r); got != want {
			t.Errorf("tokenFrom(%q) = %q, want %q", h, got, want)
		}
	}
}

func TestPlaylistStartsAtBeginning(t *testing.T) {
	srv, fs := newServer(t)
	os.WriteFile(filepath.Join(fs.dir, "index.m3u8"), []byte("#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:3,\nseg_00000.ts\n"), 0o644)
	resp, err := http.Get(srv.URL + "/v1/sessions/" + sessID + "/index.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	want := "#EXTM3U\n#EXT-X-START:TIME-OFFSET=0,PRECISE=YES\n#EXT-X-VERSION:3\n#EXTINF:3,\nseg_00000.ts\n"
	if string(b) != want {
		t.Fatalf("playlist =\n%s\nwant\n%s", b, want)
	}
	// 已有 EXT-X-START 或格式不對時不動
	for _, in := range []string{"#EXTM3U\n#EXT-X-START:TIME-OFFSET=5\n", "garbage", ""} {
		if got := string(startAtBeginning([]byte(in))); got != in {
			t.Errorf("startAtBeginning(%q) = %q", in, got)
		}
	}
}
