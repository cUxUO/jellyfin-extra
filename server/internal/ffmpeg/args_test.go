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
	a := Args(Job{Input: "http://127.0.0.1:1/src/x", StartSeconds: 90.5, SegmentSeconds: 3, Plan: basePlan()})

	if value(a, "-hwaccel") != "cuda" || value(a, "-hwaccel_output_format") != "cuda" {
		t.Errorf("missing cuda hwaccel: %v", a)
	}
	// -ss 必須在 -i 之前才是快速跳轉
	if ss, in := slices.Index(a, "-ss"), slices.Index(a, "-i"); ss < 0 || ss > in || value(a, "-ss") != "90.500" {
		t.Errorf("-ss misplaced: %v", a)
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
	if a[len(a)-1] != PlaylistName || value(a, "-hls_segment_filename") != SegmentPattern {
		t.Errorf("output must be relative names: %v", a[len(a)-3:])
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
	if slices.Contains(a, "-ss") {
		t.Error("-ss must be omitted when starting at 0")
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
