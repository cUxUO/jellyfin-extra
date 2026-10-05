// Package api 是播放程式呼叫的 HTTP 介面。
//
//	POST   /v1/sessions               開始轉碼，回傳播放清單路徑
//	GET    /v1/sessions/{id}/{file}   播放清單（整部片的 VOD 清單）與片段（請求時才轉出）
//	DELETE /v1/sessions/{id}          停止轉碼
//	GET    /healthz
//
// 回傳的路徑都是相對路徑（不以 / 開頭），播放程式自行接在伺服器基底網址後面，
// 經反向代理掛在子路徑下（例如 /xcode/）時也不用改。
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"jellyfin-extra/server/internal/ffmpeg"
	"jellyfin-extra/server/internal/jellyfin"
	"jellyfin-extra/server/internal/profile"
	"jellyfin-extra/server/internal/session"
)

// Jellyfin 是 api 用到的 Jellyfin 功能，測試時可替換。
type Jellyfin interface {
	CurrentUser(ctx context.Context, token string) (*jellyfin.User, error)
	PlaybackInfo(ctx context.Context, token, userID, itemID string) (*jellyfin.PlaybackInfo, error)
	Item(ctx context.Context, token, userID, itemID string) (*jellyfin.Item, error)
}

// Sessions 是 api 用到的 session 管理功能，測試時可替換。
type Sessions interface {
	Start(ctx context.Context, p session.Params) (*session.Session, error)
	Get(id string) (*session.Session, bool)
	Segment(ctx context.Context, id string, n int) (string, error)
	Stop(id string) bool
	RecordServed(s *session.Session, n int, size int64)
}

type Server struct {
	JF       Jellyfin
	Sessions Sessions
	Profiles map[string]profile.Profile
	Log      *log.Logger
}

var (
	// Jellyfin 的 ID 是 32 位 hex，部分 API 會回傳帶連字號的 GUID 形式
	jellyfinID  = regexp.MustCompile(`^[0-9a-fA-F]{32}$|^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	segmentName = regexp.MustCompile(`^seg_([0-9]{5})\.ts$`)
	tokenField  = regexp.MustCompile(`(?i)\bToken="?([^",\s]+)"?`)
)

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/sessions", s.createSession)
	mux.HandleFunc("GET /v1/sessions/{id}/{file}", s.serveFile)
	mux.HandleFunc("DELETE /v1/sessions/{id}", s.deleteSession)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	return s.logRequests(mux)
}

type createRequest struct {
	ItemID           string `json:"itemId"`
	MediaSourceID    string `json:"mediaSourceId"`
	Profile          string `json:"profile"`
	AudioStreamIndex *int   `json:"audioStreamIndex"`
	// SubtitleStreamIndex 是要燒進畫面的圖形字幕；文字字幕由播放程式自己向 Jellyfin 取 WebVTT 顯示
	SubtitleStreamIndex *int  `json:"subtitleStreamIndex"`
	StartTimeTicks      int64 `json:"startTimeTicks"` // Jellyfin 的時間單位，1 tick = 100ns
	MaxBitrate          int64 `json:"maxBitrate"`
}

type videoInfo struct {
	Codec    string `json:"codec"` // h264 或 hevc
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Bitrate  int64  `json:"bitrate"`
	HWDecode bool   `json:"hwDecode"`
	Tonemap  bool   `json:"tonemap"`
}

type createResponse struct {
	SessionID string `json:"sessionId"`
	// Playlist 涵蓋整部片（0 秒就是片頭），播放端自行跳到 StartTimeTicks 開始播；
	// 伺服器已先轉好那個位置的片段。
	Playlist            string    `json:"playlist"`
	StartTimeTicks      int64     `json:"startTimeTicks"`
	RunTimeTicks        int64     `json:"runTimeTicks"`
	AudioStreamIndex    int       `json:"audioStreamIndex"`    // -1 表示沒有音軌
	SubtitleStreamIndex int       `json:"subtitleStreamIndex"` // 燒進畫面的字幕，-1 表示沒有
	Video               videoInfo `json:"video"`
}

const ticksPerSecond = 10_000_000

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	token := tokenFrom(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing Authorization: MediaBrowser Token=\"...\"")
		return
	}
	var req createRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !jellyfinID.MatchString(req.ItemID) || (req.MediaSourceID != "" && !jellyfinID.MatchString(req.MediaSourceID)) {
		writeError(w, http.StatusBadRequest, "invalid itemId or mediaSourceId")
		return
	}
	prof, ok := s.Profiles[req.Profile]
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown profile")
		return
	}

	ctx := r.Context()
	user, err := s.JF.CurrentUser(ctx, token)
	if err != nil {
		s.jellyfinError(w, err)
		return
	}
	info, err := s.JF.PlaybackInfo(ctx, token, user.ID, req.ItemID)
	if err != nil {
		s.jellyfinError(w, err)
		return
	}
	src, ok := pickSource(info.MediaSources, req.MediaSourceID)
	if !ok {
		writeError(w, http.StatusNotFound, "media source not found")
		return
	}
	if src.RunTimeTicks <= 0 {
		// 沒有片長就無法預先列出所有片段，也就無法拖曳
		writeError(w, http.StatusUnprocessableEntity, "media source has no runtime")
		return
	}
	if req.StartTimeTicks < 0 || req.StartTimeTicks >= src.RunTimeTicks {
		writeError(w, http.StatusBadRequest, "startTimeTicks out of range")
		return
	}

	plan, err := profile.Build(prof, src, profile.Request{
		AudioStreamIndex:    req.AudioStreamIndex,
		SubtitleStreamIndex: req.SubtitleStreamIndex,
		MaxBitrate:          req.MaxBitrate,
	})
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// 片名只用於監控頁，查不到就顯示 ID
	title := req.ItemID
	if item, err := s.JF.Item(ctx, token, user.ID, req.ItemID); err == nil && item.Name != "" {
		title = item.DisplayTitle()
	}

	sess, err := s.Sessions.Start(ctx, session.Params{
		UserID: user.ID, ItemID: req.ItemID, MediaSourceID: src.ID, Token: token,
		StartSeconds:   float64(req.StartTimeTicks) / ticksPerSecond,
		RunTimeSeconds: float64(src.RunTimeTicks) / ticksPerSecond,
		Plan:           plan,
		Title:          title, UserName: user.Name, Client: clientAddr(r), Profile: prof.Name,
	})
	switch {
	case errors.Is(err, session.ErrBusy):
		writeError(w, http.StatusServiceUnavailable, "transcoder busy")
		return
	case err != nil:
		s.Log.Printf("start session for item %s: %v", req.ItemID, err)
		writeError(w, http.StatusBadGateway, "transcode failed to start")
		return
	}

	writeJSON(w, http.StatusCreated, createResponse{
		SessionID:           sess.ID,
		Playlist:            "v1/sessions/" + sess.ID + "/" + ffmpeg.PlaylistName,
		StartTimeTicks:      req.StartTimeTicks,
		RunTimeTicks:        src.RunTimeTicks,
		AudioStreamIndex:    plan.AudioIndex,
		SubtitleStreamIndex: plan.SubtitleIndex,
		Video: videoInfo{
			Codec: plan.VideoCodec, Width: plan.Width, Height: plan.Height, Bitrate: plan.VideoBitrate,
			HWDecode: plan.HWDecode, Tonemap: plan.Tonemap,
		},
	})
}

func pickSource(sources []jellyfin.MediaSource, id string) (jellyfin.MediaSource, bool) {
	for _, src := range sources {
		if id == "" || src.ID == id {
			return src, true
		}
	}
	return jellyfin.MediaSource{}, false
}

func (s *Server) jellyfinError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, jellyfin.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "jellyfin rejected the token")
	case errors.Is(err, jellyfin.ErrNotFound):
		writeError(w, http.StatusNotFound, "item not found")
	default:
		s.Log.Printf("jellyfin: %v", err)
		writeError(w, http.StatusBadGateway, "jellyfin unavailable")
	}
}

// serveFile 以 session ID 作為存取憑證：ID 是 128 位元亂數，只有建立者知道。
// AVPlayer 不方便替每個片段請求加標頭，所以不在這裡驗 token。
func (s *Server) serveFile(w http.ResponseWriter, r *http.Request) {
	id, name := r.PathValue("id"), r.PathValue("file")
	if !session.ValidID(id) {
		http.NotFound(w, r)
		return
	}
	if name == ffmpeg.PlaylistName {
		sess, ok := s.Sessions.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(sess.Playlist))
		return
	}

	m := segmentName.FindStringSubmatch(name)
	if m == nil {
		http.NotFound(w, r)
		return
	}
	n, _ := strconv.Atoi(m[1])
	// 還沒轉出的片段在這裡等 ffmpeg；拖曳到遠處時會從這一段重新啟動
	path, err := s.Sessions.Segment(r.Context(), id, n)
	switch {
	case errors.Is(err, session.ErrNotFound), errors.Is(err, session.ErrNoSegment):
		http.NotFound(w, r)
		return
	case errors.Is(err, context.Canceled):
		return // 播放端已放棄這個請求（例如又拖曳了）
	case err != nil:
		s.Log.Printf("session %s segment %d: %v", id, n, err)
		http.Error(w, "segment unavailable", http.StatusServiceUnavailable)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, "stat failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, name, st.ModTime(), f)
	if sess, ok := s.Sessions.Get(id); ok {
		s.Sessions.RecordServed(sess, n, st.Size())
	}
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !session.ValidID(id) || !s.Sessions.Stop(id) {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// clientAddr 是監控頁顯示的播放端位址；經反向代理時取 X-Forwarded-For 的第一個位址。
func clientAddr(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		first, _, _ := strings.Cut(fwd, ",")
		return strings.TrimSpace(first) + "（經反向代理）"
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func tokenFrom(r *http.Request) string {
	if m := tokenField.FindStringSubmatch(r.Header.Get("Authorization")); m != nil {
		return m[1]
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// logRequests 不記錄任何標頭，避免 token 進 log。
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.Log.Printf("%s %s %s %d %s", r.RemoteAddr, r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}
