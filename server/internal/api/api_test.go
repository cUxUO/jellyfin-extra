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

func (fakeJF) Item(_ context.Context, _, _, id string) (*jellyfin.Item, error) {
	return &jellyfin.Item{Name: "Test Movie", ProductionYear: 2024}, nil
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
	variants  []int // 每次 Segment 請求的軌道
	served    []int
}

func (f *fakeSessions) Start(_ context.Context, p session.Params) (*session.Session, error) {
	f.started = &p
	return &session.Session{ID: sessID, Dir: f.dir, Variants: []*session.Variant{{Plan: p.Plans[0], Dir: f.dir}}}, nil
}

func (f *fakeSessions) Get(id string) (*session.Session, bool) {
	if id != sessID {
		return nil, false
	}
	// 兩軌（自適應）：index.m3u8 是主播放清單，另有 v0.m3u8、v1.m3u8
	return &session.Session{ID: sessID, Dir: f.dir, Playlist: []byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nv0.m3u8\n"),
		Variants: []*session.Variant{
			{Index: 0, Dir: f.dir, Playlist: []byte("#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\nv0_seg_00000.ts\n")},
			{Index: 1, Dir: f.dir, Playlist: []byte("#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\nv1_seg_00000.ts\n")},
		}}, true
}

// Segment 模擬 session.Manager：第 0–1 段存在，第 2 段「轉出」後回傳，其餘超出範圍。
func (f *fakeSessions) Segment(_ context.Context, id string, variant, n int) (string, error) {
	f.requested = append(f.requested, n)
	f.variants = append(f.variants, variant)
	switch {
	case id != sessID:
		return "", session.ErrNotFound
	case n > 2:
		return "", session.ErrNoSegment
	}
	return filepath.Join(f.dir, fmt.Sprintf("seg_%05d.ts", n)), nil
}

func (f *fakeSessions) Stop(id string) bool { f.stopped = id; return id == sessID }

func (f *fakeSessions) RecordServed(_ *session.Session, n int, _ int64) {
	f.served = append(f.served, n)
}

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
	// 監控頁用的資訊
	if p.Title != "Test Movie (2024)" || p.Client != "127.0.0.1" || p.Profile != "zenpad10" {
		t.Errorf("display params: title=%q client=%q profile=%q", p.Title, p.Client, p.Profile)
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
	if body := check(base+"index.m3u8", 200, "application/vnd.apple.mpegurl"); !strings.Contains(body, "STREAM-INF") {
		t.Errorf("master playlist body = %q", body)
	}
	if body := check(base+"v1.m3u8", 200, "application/vnd.apple.mpegurl"); !strings.Contains(body, "v1_seg_00000.ts") {
		t.Errorf("variant playlist body = %q", body)
	}
	check(base+"v2.m3u8", 404, "")
	check(base+"seg_00000.ts", 200, "video/mp2t")
	check(base+"seg_00002.ts", 200, "video/mp2t")
	check(base+"v1_seg_00001.ts", 200, "video/mp2t")
	check(base+"seg_00099.ts", 404, "") // 超出片長
	check(base+"secret.txt", 404, "")
	check(base+"v10_seg_00001.ts", 404, "")
	check(base+"ffmpeg.m3u8", 404, "") // ffmpeg 自己的清單不對外
	check("/v1/sessions/"+strings.Repeat("0", 32)+"/index.m3u8", 404, "")
	if !slices.Equal(fs.requested, []int{0, 2, 1, 99}) || !slices.Equal(fs.variants, []int{0, 0, 1, 0}) {
		t.Errorf("segments requested from manager = %v, variants %v", fs.requested, fs.variants)
	}
	// 只有實際送出的片段計入監控數據
	if !slices.Equal(fs.served, []int{0, 2, 1}) {
		t.Errorf("segments recorded as served = %v", fs.served)
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
	if got.SubtitleStreamIndex != 3 || fs.started.Plans[0].SubtitleInput != 3 {
		t.Errorf("subtitle = %d, plan input = %d", got.SubtitleStreamIndex, fs.started.Plans[0].SubtitleInput)
	}
}

func TestCreateSessionFromCapabilities(t *testing.T) {
	srv, fs := newServer(t)
	caps := `"capabilities":{"decoders":[{"codec":"h264","maxWidth":1920,"maxHeight":1088},{"codec":"hevc","maxWidth":1920,"maxHeight":1088}]}`

	// 自適應：HEVC，最高 1080p，另有較低的幾軌
	resp := post(t, srv.URL, "good", `{"itemId":"`+itemID+`","profile":"P028",`+caps+`,"adaptive":true}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var got createResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Video.Codec != "hevc" || got.Video.Height != 1080 || len(got.Variants) != 4 || len(fs.started.Plans) != 4 {
		t.Fatalf("video = %+v variants = %+v", got.Video, got.Variants)
	}
	if fs.started.Profile != "P028" || got.Variants[1].Height != 720 || got.Variants[3].Height != 360 {
		t.Errorf("profile %q variants %+v", fs.started.Profile, got.Variants)
	}

	// 選 720p、不自適應：只有一軌
	resp = post(t, srv.URL, "good", `{"itemId":"`+itemID+`",`+caps+`,"maxHeight":720}`)
	got = createResponse{}
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Video.Width != 1280 || got.Video.Height != 720 || len(got.Variants) != 0 || len(fs.started.Plans) != 1 {
		t.Errorf("720p: video = %+v variants = %d", got.Video, len(got.Variants))
	}

	// 只有 H.264 硬解
	resp = post(t, srv.URL, "good", `{"itemId":"`+itemID+`","capabilities":{"decoders":[{"codec":"h264","maxWidth":1280,"maxHeight":720}]}}`)
	got = createResponse{}
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Video.Codec != "h264" || got.Video.Height != 720 {
		t.Errorf("h264 only: video = %+v", got.Video)
	}

	for name, body := range map[string]string{
		"no decoder": `{"itemId":"` + itemID + `","capabilities":{"decoders":[{"codec":"vp9","maxWidth":1920,"maxHeight":1080}]}}`,
		"bad size":   `{"itemId":"` + itemID + `","capabilities":{"decoders":[{"codec":"h264","maxWidth":0,"maxHeight":1080}]}}`,
		"bad height": `{"itemId":"` + itemID + `",` + caps + `,"maxHeight":-1}`,
	} {
		if code := post(t, srv.URL, "good", body).StatusCode; code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, code)
		}
	}
}
