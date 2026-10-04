// Package api 是播放程式呼叫的 HTTP 介面。
//
//	POST   /v1/sessions               開始轉碼，回傳播放清單路徑
//	GET    /v1/sessions/{id}/{file}   播放清單與片段
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
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
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
}

// Sessions 是 api 用到的 session 管理功能，測試時可替換。
type Sessions interface {
	Start(ctx context.Context, p session.Params) (*session.Session, error)
	Get(id string) (*session.Session, bool)
	Stop(id string) bool
}

type Server struct {
	JF       Jellyfin
	Sessions Sessions
	Profiles map[string]profile.Profile
	Log      *log.Logger
}

var (
	// Jellyfin 的 ID 是 32 位 hex，部分 API 會回傳帶連字號的 GUID 形式
	jellyfinID = regexp.MustCompile(`^[0-9a-fA-F]{32}$|^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	fileName   = regexp.MustCompile(`^(index\.m3u8|seg_[0-9]{5}\.ts)$`)
	tokenField = regexp.MustCompile(`(?i)\bToken="?([^",\s]+)"?`)
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
	StartTimeTicks   int64  `json:"startTimeTicks"` // Jellyfin 的時間單位，1 tick = 100ns
	MaxBitrate       int64  `json:"maxBitrate"`
}

type videoInfo struct {
	Width    int   `json:"width"`
	Height   int   `json:"height"`
	Bitrate  int64 `json:"bitrate"`
	HWDecode bool  `json:"hwDecode"`
	Tonemap  bool  `json:"tonemap"`
}

type createResponse struct {
	SessionID        string    `json:"sessionId"`
	Playlist         string    `json:"playlist"`
	StartTimeTicks   int64     `json:"startTimeTicks"` // 播放清單的 0 秒對應片中的這個位置
	RunTimeTicks     int64     `json:"runTimeTicks"`
	AudioStreamIndex int       `json:"audioStreamIndex"` // -1 表示沒有音軌
	Video            videoInfo `json:"video"`
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
	if req.StartTimeTicks < 0 || (src.RunTimeTicks > 0 && req.StartTimeTicks >= src.RunTimeTicks) {
		writeError(w, http.StatusBadRequest, "startTimeTicks out of range")
		return
	}

	plan, err := profile.Build(prof, src, profile.Request{AudioStreamIndex: req.AudioStreamIndex, MaxBitrate: req.MaxBitrate})
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	sess, err := s.Sessions.Start(ctx, session.Params{
		UserID: user.ID, ItemID: req.ItemID, MediaSourceID: src.ID, Token: token,
		StartSeconds: float64(req.StartTimeTicks) / ticksPerSecond, Plan: plan,
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
		SessionID:        sess.ID,
		Playlist:         "v1/sessions/" + sess.ID + "/" + ffmpeg.PlaylistName,
		StartTimeTicks:   req.StartTimeTicks,
		RunTimeTicks:     src.RunTimeTicks,
		AudioStreamIndex: plan.AudioIndex,
		Video: videoInfo{
			Width: plan.Width, Height: plan.Height, Bitrate: plan.VideoBitrate,
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
	if !session.ValidID(id) || !fileName.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	sess, ok := s.Sessions.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(sess.Path(name))
	if err != nil {
		// 片段還沒轉出來，或已被回收
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, "stat failed", http.StatusInternalServerError)
		return
	}
	if name == ffmpeg.PlaylistName {
		b, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, "read failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeContent(w, r, name, st.ModTime(), bytes.NewReader(startAtBeginning(b)))
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, name, st.ModTime(), f)
}

// startAtBeginning 在清單加上 EXT-X-START。轉碼中的 EVENT 清單還沒有 ENDLIST，
// AVPlayer 和 ExoPlayer 都會把它當直播、從最新的幾段開始播，跳過開頭。
func startAtBeginning(playlist []byte) []byte {
	const tag = "#EXT-X-START:TIME-OFFSET=0,PRECISE=YES\n"
	if bytes.Contains(playlist, []byte("#EXT-X-START")) {
		return playlist
	}
	head, rest, ok := bytes.Cut(playlist, []byte("\n"))
	if !ok || !bytes.HasPrefix(head, []byte("#EXTM3U")) {
		return playlist
	}
	out := make([]byte, 0, len(playlist)+len(tag))
	out = append(out, head...)
	out = append(out, '\n')
	out = append(out, tag...)
	return append(out, rest...)
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !session.ValidID(id) || !s.Sessions.Stop(id) {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
