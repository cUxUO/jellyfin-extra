package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"jellyfin-extra/server/internal/jellyfin"
	"jellyfin-extra/server/internal/profile"
	"jellyfin-extra/server/internal/session"
)

const (
	itemID      = "0123456789abcdef0123456789abcdef"
	noRuntimeID = "00000000000000000000000000000abc"
	sessID      = "fedcba9876543210fedcba9876543210"
)

type fakeJF struct{}

func (fakeJF) CurrentUser(_ context.Context, token string) (*jellyfin.User, error) {
	if token != "good" {
		return nil, jellyfin.ErrUnauthorized
	}
	return &jellyfin.User{ID: "u1"}, nil
}

func (fakeJF) PlaybackInfo(_ context.Context, _, _, id string) (*jellyfin.PlaybackInfo, error) {
	if id != itemID && id != noRuntimeID {
		return nil, jellyfin.ErrNotFound
	}
	runtime := int64(600 * ticksPerSecond)
	if id == noRuntimeID {
		runtime = 0
	}
	return &jellyfin.PlaybackInfo{MediaSources: []jellyfin.MediaSource{{
		ID: id, RunTimeTicks: runtime,
		MediaStreams: []jellyfin.MediaStream{
			{Type: "Video", Index: 0, Codec: "hevc", Width: 3840, Height: 2160, VideoRange: "HDR"},
			{Type: "Audio", Index: 1, Codec: "eac3", Channels: 6},
			{Type: "Subtitle", Index: 2, Codec: "subrip"},
			{Type: "Subtitle", Index: 3, Codec: "PGSSUB"},
		},
	}}}, nil
}

type fakeSessions struct {
	dir       string
	started   *session.Params
	stopped   string
	requested []int
}

func (f *fakeSessions) Start(_ context.Context, p session.Params) (*session.Session, error) {
	f.started = &p
	return &session.Session{ID: sessID, Dir: f.dir, Plan: p.Plan}, nil
}

func (f *fakeSessions) Get(id string) (*session.Session, bool) {
	if id != sessID {
		return nil, false
	}
	return &session.Session{ID: sessID, Dir: f.dir, Playlist: []byte("#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n")}, true
}

// Segment 模擬 session.Manager：第 0–1 段存在，第 2 段「轉出」後回傳，其餘超出範圍。
func (f *fakeSessions) Segment(_ context.Context, id string, n int) (string, error) {
	f.requested = append(f.requested, n)
	switch {
	case id != sessID:
		return "", session.ErrNotFound
	case n > 2:
		return "", session.ErrNoSegment
	}
	return filepath.Join(f.dir, fmt.Sprintf("seg_%05d.ts", n)), nil
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
	if p == nil || p.Token != "good" || p.UserID != "u1" || p.StartSeconds != 90 || p.RunTimeSeconds != 600 || p.MediaSourceID != itemID {
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
		{"no runtime", "good", `{"itemId":"` + noRuntimeID + `","profile":"zenpad10"}`, 422},
		{"burn text subtitle", "good", `{"itemId":"` + itemID + `","profile":"zenpad10","subtitleStreamIndex":2}`, 422},
	}
	for _, c := range cases {
		if got := post(t, srv.URL, c.token, c.body).StatusCode; got != c.want {
			t.Errorf("%s: status %d, want %d", c.name, got, c.want)
		}
	}
}

func TestServeFile(t *testing.T) {
	srv, fs := newServer(t)
	for _, n := range []string{"seg_00000.ts", "seg_00001.ts", "seg_00002.ts", "secret.txt"} {
		os.WriteFile(filepath.Join(fs.dir, n), []byte("ts"), 0o644)
	}

	check := func(path string, want int, ctype string) string {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != want || (ctype != "" && resp.Header.Get("Content-Type") != ctype) {
			t.Errorf("GET %s = %d %q, want %d %q", path, resp.StatusCode, resp.Header.Get("Content-Type"), want, ctype)
		}
		return string(b)
	}
	base := "/v1/sessions/" + sessID + "/"
	// 播放清單來自 session（伺服器依片長產生），不是磁碟上 ffmpeg 寫的檔案
	if body := check(base+"index.m3u8", 200, "application/vnd.apple.mpegurl"); !strings.Contains(body, "VOD") {
		t.Errorf("playlist body = %q", body)
	}
	check(base+"seg_00000.ts", 200, "video/mp2t")
	check(base+"seg_00002.ts", 200, "video/mp2t")
	check(base+"seg_00099.ts", 404, "") // 超出片長
	check(base+"secret.txt", 404, "")
	check(base+"ffmpeg.m3u8", 404, "") // ffmpeg 自己的清單不對外
	check("/v1/sessions/"+strings.Repeat("0", 32)+"/index.m3u8", 404, "")
	if !slices.Equal(fs.requested, []int{0, 2, 99}) {
		t.Errorf("segments requested from manager = %v", fs.requested)
	}
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

func TestCreateSessionBurnsImageSubtitle(t *testing.T) {
	srv, fs := newServer(t)
	resp := post(t, srv.URL, "good", `{"itemId":"`+itemID+`","profile":"zenpad10","subtitleStreamIndex":3}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var got createResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if got.SubtitleStreamIndex != 3 || fs.started.Plan.SubtitleInput != 3 {
		t.Errorf("subtitle = %d, plan input = %d", got.SubtitleStreamIndex, fs.started.Plan.SubtitleInput)
	}
}
