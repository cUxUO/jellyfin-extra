package ffmpeg

import (
	"slices"
	"strings"
	"testing"

	"jellyfin-extra/server/internal/profile"
)

// value 回傳旗標後面的值；旗標不存在時回傳 ""。
func value(args []string, flag string) string {
	i := slices.Index(args, flag)
	if i < 0 || i+1 >= len(args) {
		return ""
	}
	return args[i+1]
}

func basePlan() profile.Plan {
	return profile.Plan{
		VideoIndex: 1, AudioIndex: 3, VideoInput: 1, AudioInput: 3, Width: 1280, Height: 720,
		VideoBitrate: 4_000_000, AudioBitrate: 128_000,
		HWDecode: true, H264Profile: "high", H264Level: "4.0",
	}
}

func TestArgsHardwarePath(t *testing.T) {
	a := Args(Job{Input: "http://127.0.0.1:1/src/x", StartSegment: 30, SegmentSeconds: 3, Plan: basePlan()})

	if value(a, "-hwaccel") != "cuda" || value(a, "-hwaccel_output_format") != "cuda" {
		t.Errorf("missing cuda hwaccel: %v", a)
	}
	// -ss 必須在 -i 之前才是快速跳轉；位置是段落邊界
	if ss, in := slices.Index(a, "-ss"), slices.Index(a, "-i"); ss < 0 || ss > in || value(a, "-ss") != "90" {
		t.Errorf("-ss misplaced: %v", a)
	}
	// 中途啟動時，時間戳記與檔名編號都要對齊整部片
	if value(a, "-output_ts_offset") != "90" || value(a, "-start_number") != "30" {
		t.Errorf("offset=%q start_number=%q", value(a, "-output_ts_offset"), value(a, "-start_number"))
	}
	if got := value(a, "-vf"); got != "scale_cuda=w=1280:h=720:format=yuv420p" {
		t.Errorf("-vf = %q", got)
	}
	if value(a, "-maxrate") != "6000000" || value(a, "-bufsize") != "8000000" {
		t.Errorf("rate control: maxrate=%s bufsize=%s", value(a, "-maxrate"), value(a, "-bufsize"))
	}
	if value(a, "-force_key_frames") != "expr:gte(t,n_forced*3)" {
		t.Errorf("keyframes = %q", value(a, "-force_key_frames"))
	}
	maps := []string{}
	for i, v := range a {
		if v == "-map" {
			maps = append(maps, a[i+1])
		}
	}
	if !slices.Equal(maps, []string{"0:1", "0:3"}) {
		t.Errorf("maps = %v", maps)
	}
	// ffmpeg 自己的清單不對外，播放端拿的是伺服器產生的 index.m3u8
	if a[len(a)-1] != "ffmpeg.m3u8" || value(a, "-hls_segment_filename") != SegmentPattern {
		t.Errorf("outputs: %v", a[len(a)-3:])
	}
}

func TestArgsTonemapAndSoftware(t *testing.T) {
	p := basePlan()
	p.Tonemap = true
	if vf := value(Args(Job{Input: "in", Plan: p}), "-vf"); !strings.HasPrefix(vf, "scale_cuda=w=1280:h=720,tonemap_cuda=") {
		t.Errorf("tonemap -vf = %q", vf)
	}

	p = basePlan()
	p.HWDecode = false
	a := Args(Job{Input: "in", Plan: p})
	if slices.Contains(a, "-hwaccel") {
		t.Error("software path must not request hwaccel")
	}
	if vf := value(a, "-vf"); vf != "scale=1280:720:flags=bicubic,format=yuv420p" {
		t.Errorf("software -vf = %q", vf)
	}
	if slices.Contains(a, "-ss") || value(a, "-start_number") != "0" || value(a, "-output_ts_offset") != "0" {
		t.Error("starting at 0 must not seek and must number from 0")
	}
}

func TestArgsNoAudio(t *testing.T) {
	p := basePlan()
	p.AudioIndex, p.AudioInput = -1, -1
	a := Args(Job{Input: "in", Plan: p})
	if !slices.Contains(a, "-an") || slices.Contains(a, "-c:a") {
		t.Errorf("no-audio args wrong: %v", a)
	}
}

func TestVODPlaylist(t *testing.T) {
	got := string(VODPlaylist(7.5, 3))
	want := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:4\n#EXT-X-MEDIA-SEQUENCE:0\n" +
		"#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-INDEPENDENT-SEGMENTS\n" +
		"#EXTINF:3.000000,\nseg_00000.ts\n" +
		"#EXTINF:3.000000,\nseg_00001.ts\n" +
		"#EXTINF:1.500000,\nseg_00002.ts\n" +
		"#EXT-X-ENDLIST\n"
	if got != want {
		t.Fatalf("playlist =\n%s\nwant\n%s", got, want)
	}
	if n := SegmentCount(6, 3); n != 2 {
		t.Errorf("SegmentCount(6,3) = %d, want 2", n)
	}
	// 剛好整除時最後一段不是 0 秒
	if !strings.Contains(string(VODPlaylist(6, 3)), "#EXTINF:3.000000,\nseg_00001.ts\n#EXT-X-ENDLIST") {
		t.Error("exact multiple produced a wrong last segment")
	}
	// 一部 101 分鐘的片：2034 段
	if n := SegmentCount(6101.12, 3); n != 2034 {
		t.Errorf("SegmentCount = %d", n)
	}
}
