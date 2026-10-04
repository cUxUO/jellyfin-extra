// Package profile 定義各裝置的輸出規格，並依片源決定轉碼計畫。
package profile

import (
	"errors"
	"fmt"
	"strings"

	"jellyfin-extra/server/internal/jellyfin"
)

// Profile 是一台裝置能穩定硬解的輸出上限。數值來自實測，見 CLAUDE.md。
type Profile struct {
	Name            string
	MaxWidth        int
	MaxHeight       int
	H264Profile     string
	H264Level       string
	MaxVideoBitrate int64 // 在 MaxWidth×MaxHeight 時的目標位元率
	AudioBitrate    int64
}

var Profiles = map[string]Profile{
	// A7：H.264 硬解到 1080p，沒有 HEVC 硬解
	"ipad-air1": {
		Name: "ipad-air1", MaxWidth: 1920, MaxHeight: 1080,
		H264Profile: "high", H264Level: "4.1",
		MaxVideoBitrate: 8_000_000, AudioBitrate: 192_000,
	},
	// MT8163，螢幕 1280×800，送更大的解析度只是浪費頻寬
	"zenpad10": {
		Name: "zenpad10", MaxWidth: 1280, MaxHeight: 800,
		H264Profile: "high", H264Level: "4.0",
		MaxVideoBitrate: 4_000_000, AudioBitrate: 128_000,
	},
}

// Request 是播放程式可以調整的部分。
type Request struct {
	AudioStreamIndex *int  // nil 表示用預設音軌
	MaxBitrate       int64 // 0 表示不限，用於對外連線頻寬有限時
}

// Plan 是一次轉碼的完整決定，ffmpeg 參數由它產生。
type Plan struct {
	VideoIndex   int // Jellyfin 的 MediaStream.Index
	AudioIndex   int // Jellyfin 的 MediaStream.Index，-1 表示沒有音軌
	VideoInput   int // 對應到 ffmpeg 輸入檔的串流編號（-map 0:N）
	AudioInput   int // 同上，-1 表示沒有音軌
	Width        int
	Height       int
	VideoBitrate int64
	AudioBitrate int64
	HWDecode     bool // NVDEC 解碼，整條濾鏡留在 GPU 上
	Tonemap      bool // HDR 轉 SDR
	H264Profile  string
	H264Level    string
}

const minVideoBitrate = 1_000_000

var ErrNoVideo = errors.New("profile: media source has no video stream")

// Build 依裝置規格和片源決定轉碼計畫。
func Build(p Profile, src jellyfin.MediaSource, req Request) (Plan, error) {
	video, ok := pickVideo(src.MediaStreams)
	if !ok {
		return Plan{}, ErrNoVideo
	}
	if video.Width <= 0 || video.Height <= 0 {
		return Plan{}, fmt.Errorf("profile: video stream %d has no dimensions", video.Index)
	}

	plan := Plan{
		VideoIndex:   video.Index,
		VideoInput:   inputIndex(src.MediaStreams, video),
		AudioIndex:   -1,
		AudioInput:   -1,
		HWDecode:     nvdecSupports(video),
		Tonemap:      strings.EqualFold(video.VideoRange, "HDR"),
		H264Profile:  p.H264Profile,
		H264Level:    p.H264Level,
		AudioBitrate: p.AudioBitrate,
	}
	if plan.Tonemap && !plan.HWDecode {
		// HDR 片源幾乎都是 HEVC / AV1 / VP9，NVDEC 都能解；真的遇到再處理 CPU 版的 tonemap
		return Plan{}, fmt.Errorf("profile: HDR %s without NVDEC support is not handled", video.Codec)
	}

	if audio, ok := pickAudio(src.MediaStreams, req.AudioStreamIndex); ok {
		plan.AudioIndex = audio.Index
		plan.AudioInput = inputIndex(src.MediaStreams, audio)
	} else {
		plan.AudioBitrate = 0
	}

	plan.Width, plan.Height = FitBox(video.Width, video.Height, p.MaxWidth, p.MaxHeight)
	plan.VideoBitrate = videoBitrate(p, plan, video, req)
	return plan, nil
}

// inputIndex 把 Jellyfin 的 Index 換成 ffmpeg 輸入檔裡的串流編號。
// Jellyfin 會把外掛字幕等外部串流也編進 Index（12.1 把外掛字幕排在最前面），
// 原始檔裡實際的編號是「排在它前面的內嵌串流數」。內嵌封面（EmbeddedImage）也是內嵌串流，一併計入。
func inputIndex(streams []jellyfin.MediaStream, s jellyfin.MediaStream) int {
	n := 0
	for _, o := range streams {
		if !o.IsExternal && o.Index < s.Index {
			n++
		}
	}
	return n
}

// FitBox 等比例縮小到框內，不放大，寬高取偶數（NVENC 與 yuv420p 都需要）。
func FitBox(w, h, maxW, maxH int) (int, int) {
	if w > maxW || h > maxH {
		// 以較受限的一邊為準
		if w*maxH > h*maxW {
			h = h * maxW / w
			w = maxW
		} else {
			w = w * maxH / h
			h = maxH
		}
	}
	return w &^ 1, h &^ 1
}

func videoBitrate(p Profile, plan Plan, video jellyfin.MediaStream, req Request) int64 {
	// 依像素數比例縮放目標位元率
	br := p.MaxVideoBitrate * int64(plan.Width*plan.Height) / int64(p.MaxWidth*p.MaxHeight)
	// 片源本身位元率很低時，轉碼不會讓畫質變好，只會浪費頻寬
	if video.BitRate > 0 && br > video.BitRate {
		br = video.BitRate
	}
	if req.MaxBitrate > 0 && br > req.MaxBitrate-plan.AudioBitrate {
		br = req.MaxBitrate - plan.AudioBitrate
	}
	if br < minVideoBitrate {
		br = minVideoBitrate
	}
	return br
}

func pickVideo(streams []jellyfin.MediaStream) (jellyfin.MediaStream, bool) {
	for _, s := range streams {
		if s.Type != "Video" || s.IsExternal {
			continue
		}
		switch strings.ToLower(s.Codec) {
		case "mjpeg", "png", "bmp", "gif": // 封面圖
			continue
		}
		return s, true
	}
	return jellyfin.MediaStream{}, false
}

func pickAudio(streams []jellyfin.MediaStream, want *int) (jellyfin.MediaStream, bool) {
	var first, def *jellyfin.MediaStream
	for i := range streams {
		s := &streams[i]
		if s.Type != "Audio" || s.IsExternal {
			continue
		}
		if want != nil && s.Index == *want {
			return *s, true
		}
		if first == nil {
			first = s
		}
		if def == nil && s.IsDefault {
			def = s
		}
	}
	switch {
	case def != nil:
		return *def, true
	case first != nil:
		return *first, true
	}
	return jellyfin.MediaStream{}, false
}

// nvdecSupports 判斷 RTX 3070 Ti（Ampere）的 NVDEC 能不能解這條影像。
func nvdecSupports(s jellyfin.MediaStream) bool {
	switch strings.ToLower(s.Codec) {
	case "h264":
		return s.BitDepth <= 8 // NVDEC 不支援 10-bit H.264
	case "hevc", "vp9", "av1", "mpeg2video", "vc1":
		return true
	}
	return false
}
