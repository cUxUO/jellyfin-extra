// Package ffmpeg 把轉碼計畫轉成 ffmpeg 參數，並產生給播放端的 HLS 播放清單。
// 參數一律由這裡產生，不接受用戶端輸入。
package ffmpeg

import (
	"fmt"
	"math"
	"strconv"

	"jellyfin-extra/server/internal/profile"
)

const (
	// PlaylistName 是給播放端的清單，由伺服器依片長產生（見 VODPlaylist），不是 ffmpeg 寫的。
	PlaylistName = "index.m3u8"
	// ffmpegPlaylist 是 hls muxer 必須寫的清單，伺服器不使用、也不對外提供。
	ffmpegPlaylist = "ffmpeg.m3u8"
	SegmentPattern = "seg_%05d.ts"
)

type Job struct {
	Input          string // ffmpeg 讀取的網址（本機 source proxy）
	StartSegment   int    // 從第幾段開始轉；拖曳到還沒轉出的位置時，從那一段重新啟動
	SegmentSeconds int
	Plan           profile.Plan
}

// SegmentName 是第 n 段的檔名，和 SegmentPattern 一致。
func SegmentName(n int) string { return fmt.Sprintf("seg_%05d.ts", n) }

// Args 產生 ffmpeg 參數。ffmpeg 的工作目錄必須是 session 目錄，輸出檔名是相對路徑。
//
// 每段固定 SegmentSeconds 秒、以 IDR 開頭；從中途啟動時用 -output_ts_offset 和 -start_number
// 讓時間戳記與編號都對齊整部片，不同次啟動轉出的片段可以混在同一個播放清單裡。
func Args(j Job) []string {
	p := j.Plan
	seg := segmentSeconds(j.SegmentSeconds)
	start := strconv.Itoa(j.StartSegment * seg)

	a := []string{"-hide_banner", "-nostdin", "-loglevel", "warning", "-y"}
	if p.HWDecode {
		a = append(a, "-hwaccel", "cuda", "-hwaccel_output_format", "cuda")
	}
	// 一開始盡快轉出一段緩衝，之後限制在 2 倍速，避免整部片一口氣轉完佔滿 GPU 和硬碟
	a = append(a, "-readrate", "2", "-readrate_initial_burst", "30")
	if j.StartSegment > 0 {
		a = append(a, "-ss", start)
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
		// 每段開頭都是 IDR，和 fps 無關；t 從這次啟動的 0 起算，起點本身是段落邊界，所以仍對齊整部片
		"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", seg), "-forced-idr", "1",
	)

	if p.AudioInput >= 0 {
		a = append(a, "-c:a", "aac", "-ac", "2", "-ar", "48000", "-b:a", strconv.FormatInt(p.AudioBitrate, 10))
	} else {
		a = append(a, "-an")
	}

	a = append(a,
		"-max_muxing_queue_size", "2048",
		"-output_ts_offset", start,
		"-f", "hls",
		"-hls_time", strconv.Itoa(seg),
		"-hls_list_size", "0",
		"-start_number", strconv.Itoa(j.StartSegment),
		// temp_file：片段寫完才改名，播放端不會讀到寫一半的檔案
		"-hls_flags", "temp_file+independent_segments",
		"-hls_segment_type", "mpegts",
		"-hls_segment_filename", SegmentPattern,
		ffmpegPlaylist,
	)
	return a
}

// SegmentCount 是片長切成每段 seg 秒後的段數。
func SegmentCount(runTimeSeconds float64, seg int) int {
	seg = segmentSeconds(seg)
	n := int(math.Ceil(runTimeSeconds / float64(seg)))
	if n < 1 {
		n = 1
	}
	return n
}

// VODPlaylist 依片長一次列出所有片段。播放端因此知道完整長度，可以拖曳到任何位置；
// 片段實際上是被請求時才轉出（見 session.Manager.Segment）。
func VODPlaylist(runTimeSeconds float64, seg int) []byte {
	seg = segmentSeconds(seg)
	n := SegmentCount(runTimeSeconds, seg)
	b := []byte("#EXTM3U\n#EXT-X-VERSION:3\n")
	// 強制關鍵幀落在第一個 >= 邊界的幀，實際長度可能略長於 seg，目標長度留一秒餘裕
	b = fmt.Appendf(b, "#EXT-X-TARGETDURATION:%d\n", seg+1)
	b = append(b, "#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-INDEPENDENT-SEGMENTS\n"...)
	for i := 0; i < n; i++ {
		d := float64(seg)
		if i == n-1 {
			d = runTimeSeconds - float64(seg*(n-1))
			if d <= 0 {
				d = float64(seg)
			}
		}
		b = fmt.Appendf(b, "#EXTINF:%.6f,\n%s\n", d, SegmentName(i))
	}
	return append(b, "#EXT-X-ENDLIST\n"...)
}

func segmentSeconds(seg int) int {
	if seg <= 0 {
		return 3
	}
	return seg
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
