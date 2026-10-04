// Package ffmpeg 把轉碼計畫轉成 ffmpeg 參數。參數一律由這裡產生，不接受用戶端輸入。
package ffmpeg

import (
	"fmt"
	"strconv"

	"jellyfin-extra/server/internal/profile"
)

const (
	PlaylistName   = "index.m3u8"
	SegmentPattern = "seg_%05d.ts"
)

type Job struct {
	Input          string  // ffmpeg 讀取的網址（本機 source proxy）
	StartSeconds   float64 // 從片中這個位置開始
	SegmentSeconds int
	Plan           profile.Plan
}

// Args 產生 ffmpeg 參數。ffmpeg 的工作目錄必須是 session 目錄，輸出檔名是相對路徑，
// 這樣播放清單裡的片段網址也是相對的，經過反向代理改路徑也不受影響。
func Args(j Job) []string {
	p := j.Plan
	seg := j.SegmentSeconds
	if seg <= 0 {
		seg = 3
	}

	a := []string{"-hide_banner", "-nostdin", "-loglevel", "warning", "-y"}
	if p.HWDecode {
		a = append(a, "-hwaccel", "cuda", "-hwaccel_output_format", "cuda")
	}
	// 一開始盡快轉出一段緩衝，之後限制在 2 倍速，避免整部片一口氣轉完佔滿 GPU 和硬碟
	a = append(a, "-readrate", "2", "-readrate_initial_burst", "30")
	if j.StartSeconds > 0 {
		a = append(a, "-ss", strconv.FormatFloat(j.StartSeconds, 'f', 3, 64))
	}
	a = append(a, "-i", j.Input)

	a = append(a, "-map", fmt.Sprintf("0:%d", p.VideoInput))
	if p.AudioInput >= 0 {
		a = append(a, "-map", fmt.Sprintf("0:%d", p.AudioInput))
	}
	a = append(a, "-sn", "-dn", "-map_metadata", "-1", "-map_chapters", "-1")

	a = append(a, "-vf", videoFilter(p))
	a = append(a,
		"-c:v", "h264_nvenc", "-preset", "p4", "-rc", "vbr",
		"-b:v", strconv.FormatInt(p.VideoBitrate, 10),
		"-maxrate", strconv.FormatInt(p.VideoBitrate*3/2, 10),
		"-bufsize", strconv.FormatInt(p.VideoBitrate*2, 10),
		"-profile:v", p.H264Profile, "-level:v", p.H264Level,
		"-spatial-aq", "1",
		// 每段開頭都是 IDR，和 fps 無關；拖曳與切段都依賴這點
		"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", seg), "-forced-idr", "1",
	)

	if p.AudioInput >= 0 {
		a = append(a, "-c:a", "aac", "-ac", "2", "-ar", "48000", "-b:a", strconv.FormatInt(p.AudioBitrate, 10))
	} else {
		a = append(a, "-an")
	}

	a = append(a,
		"-max_muxing_queue_size", "2048",
		"-f", "hls",
		"-hls_time", strconv.Itoa(seg),
		"-hls_list_size", "0",
		"-hls_playlist_type", "event",
		// temp_file：片段和清單寫完才改名，播放端不會讀到寫一半的檔案
		"-hls_flags", "temp_file+independent_segments",
		"-hls_segment_type", "mpegts",
		"-hls_segment_filename", SegmentPattern,
		PlaylistName,
	)
	return a
}

func videoFilter(p profile.Plan) string {
	size := fmt.Sprintf("w=%d:h=%d", p.Width, p.Height)
	switch {
	case p.HWDecode && p.Tonemap:
		// 參數與 Jellyfin 本身用的 tonemap_cuda 一致；縮放先做，tonemap 的運算量較小
		return "scale_cuda=" + size + "," +
			"tonemap_cuda=format=yuv420p:p=bt709:t=bt709:m=bt709:tonemap=bt2390:peak=100:desat=0"
	case p.HWDecode:
		return "scale_cuda=" + size + ":format=yuv420p"
	default:
		// NVDEC 不支援的格式走 CPU 解碼與縮放，NVENC 直接吃系統記憶體的畫面
		return fmt.Sprintf("scale=%d:%d:flags=bicubic,format=yuv420p", p.Width, p.Height)
	}
}
