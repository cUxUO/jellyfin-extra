package profile

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"jellyfin-extra/server/internal/jellyfin"
)

func TestFitBox(t *testing.T) {
	cases := []struct{ w, h, maxW, maxH, wantW, wantH int }{
		{1920, 1080, 1920, 1080, 1920, 1080},
		{3840, 2160, 1920, 1080, 1920, 1080},
		{3840, 2160, 1280, 800, 1280, 720},   // 16:9 進 16:10 框，寬受限
		{1440, 1080, 1280, 800, 1066, 800},   // 4:3 進 16:10 框，高受限
		{1920, 800, 1280, 800, 1280, 532},    // 2.4:1 電影
		{720, 480, 1920, 1080, 720, 480},     // 不放大
		{1921, 1081, 4000, 4000, 1920, 1080}, // 奇數取偶
	}
	for _, c := range cases {
		w, h := FitBox(c.w, c.h, c.maxW, c.maxH)
		if w != c.wantW || h != c.wantH {
			t.Errorf("FitBox(%d,%d,%d,%d) = %d×%d, want %d×%d", c.w, c.h, c.maxW, c.maxH, w, h, c.wantW, c.wantH)
		}
	}
}

func source(streams ...jellyfin.MediaStream) jellyfin.MediaSource {
	return jellyfin.MediaSource{ID: "ms", MediaStreams: streams}
}

func TestBuildHDRToZenPad(t *testing.T) {
	src := source(
		jellyfin.MediaStream{Type: "Video", Index: 0, Codec: "mjpeg", Width: 600, Height: 900},
		jellyfin.MediaStream{Type: "Video", Index: 1, Codec: "hevc", Width: 3840, Height: 2160, BitDepth: 10, VideoRange: "HDR"},
		jellyfin.MediaStream{Type: "Audio", Index: 2, Codec: "truehd", Channels: 8},
		jellyfin.MediaStream{Type: "Audio", Index: 3, Codec: "ac3", Channels: 6, IsDefault: true},
	)
	plan, err := Build(Profiles["zenpad10"], src, Request{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.VideoIndex != 1 || plan.AudioIndex != 3 {
		t.Errorf("streams = v%d a%d, want v1 a3", plan.VideoIndex, plan.AudioIndex)
	}
	if !plan.HWDecode || !plan.Tonemap {
		t.Errorf("HWDecode=%v Tonemap=%v, want both", plan.HWDecode, plan.Tonemap)
	}
	if plan.Width != 1280 || plan.Height != 720 {
		t.Errorf("size = %d×%d", plan.Width, plan.Height)
	}
	// 1280×720 在 1280×800 框的 90%
	if plan.VideoBitrate != 3_600_000 {
		t.Errorf("bitrate = %d", plan.VideoBitrate)
	}
}

func TestBuildAudioSelectionAndCaps(t *testing.T) {
	two := 2
	src := source(
		jellyfin.MediaStream{Type: "Video", Index: 0, Codec: "h264", Width: 1920, Height: 1080, BitDepth: 10, BitRate: 3_000_000},
		jellyfin.MediaStream{Type: "Audio", Index: 1, Codec: "aac"},
		jellyfin.MediaStream{Type: "Audio", Index: 2, Codec: "aac"},
	)
	plan, err := Build(Profiles["ipad-air1"], src, Request{AudioStreamIndex: &two})
	if err != nil {
		t.Fatal(err)
	}
	if plan.AudioIndex != 2 {
		t.Errorf("audio = %d, want 2", plan.AudioIndex)
	}
	if plan.HWDecode {
		t.Error("10-bit H.264 must not use NVDEC")
	}
	if plan.VideoBitrate != 3_000_000 {
		t.Errorf("bitrate = %d, want capped at source 3M", plan.VideoBitrate)
	}

	plan, _ = Build(Profiles["ipad-air1"], src, Request{MaxBitrate: 1_500_000})
	if plan.AudioIndex != 1 {
		t.Errorf("default audio = %d, want first (1)", plan.AudioIndex)
	}
	if plan.VideoBitrate != 1_500_000-192_000 {
		t.Errorf("bitrate = %d, want total cap minus audio", plan.VideoBitrate)
	}

	plan, _ = Build(Profiles["ipad-air1"], src, Request{MaxBitrate: 600_000})
	if plan.VideoBitrate != minVideoBitrate {
		t.Errorf("bitrate = %d, want floor %d", plan.VideoBitrate, minVideoBitrate)
	}
}

func TestBuildNoAudioNoVideo(t *testing.T) {
	plan, err := Build(Profiles["ipad-air1"], source(jellyfin.MediaStream{Type: "Video", Codec: "vp9", Width: 640, Height: 360}), Request{})
	if err != nil || plan.AudioIndex != -1 || plan.AudioInput != -1 || plan.AudioBitrate != 0 {
		t.Fatalf("plan = %+v, err = %v", plan, err)
	}
	if _, err := Build(Profiles["ipad-air1"], source(jellyfin.MediaStream{Type: "Audio", Codec: "flac"}), Request{}); err != ErrNoVideo {
		t.Fatalf("err = %v, want ErrNoVideo", err)
	}
}

// Jellyfin 12.1 把外掛字幕排在 Index 0，內嵌串流往後推；ffmpeg 要用原始檔裡的編號。
// 實際案例：Platinum.Data.mkv（外掛 srt + h264 + ac3）。
func TestBuildMapsAroundExternalStreams(t *testing.T) {
	src := source(
		jellyfin.MediaStream{Type: "Subtitle", Index: 0, Codec: "subrip", IsExternal: true},
		jellyfin.MediaStream{Type: "Video", Index: 1, Codec: "h264", Width: 1920, Height: 1080, BitDepth: 8},
		jellyfin.MediaStream{Type: "Audio", Index: 2, Codec: "ac3", Channels: 6, IsDefault: true},
	)
	plan, err := Build(Profiles["zenpad10"], src, Request{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.VideoIndex != 1 || plan.AudioIndex != 2 {
		t.Errorf("jellyfin indexes = v%d a%d, want v1 a2", plan.VideoIndex, plan.AudioIndex)
	}
	if plan.VideoInput != 0 || plan.AudioInput != 1 {
		t.Errorf("ffmpeg inputs = v%d a%d, want v0 a1", plan.VideoInput, plan.AudioInput)
	}

	// 內嵌封面是原始檔裡的串流，要計入；外部串流夾在中間也不影響
	src = source(
		jellyfin.MediaStream{Type: "EmbeddedImage", Index: 0, Codec: "mjpeg"},
		jellyfin.MediaStream{Type: "Video", Index: 1, Codec: "hevc", Width: 1920, Height: 1080},
		jellyfin.MediaStream{Type: "Subtitle", Index: 2, Codec: "ass", IsExternal: true},
		jellyfin.MediaStream{Type: "Audio", Index: 3, Codec: "aac"},
	)
	plan, _ = Build(Profiles["zenpad10"], src, Request{})
	if plan.VideoInput != 1 || plan.AudioInput != 2 {
		t.Errorf("with cover art: inputs = v%d a%d, want v1 a2", plan.VideoInput, plan.AudioInput)
	}
}

func TestBuildBurnSubtitle(t *testing.T) {
	src := source(
		jellyfin.MediaStream{Type: "Subtitle", Index: 0, Codec: "ass", IsExternal: true},
		jellyfin.MediaStream{Type: "Video", Index: 1, Codec: "hevc", Width: 3840, Height: 2160},
		jellyfin.MediaStream{Type: "Audio", Index: 2, Codec: "truehd"},
		jellyfin.MediaStream{Type: "Subtitle", Index: 3, Codec: "subrip"},
		jellyfin.MediaStream{Type: "Subtitle", Index: 4, Codec: "PGSSUB"},
		jellyfin.MediaStream{Type: "Subtitle", Index: 5, Codec: "PGSSUB", IsExternal: true},
	)
	idx := func(i int) *int { return &i }

	plan, err := Build(Profiles["zenpad10"], src, Request{SubtitleStreamIndex: idx(4)})
	if err != nil {
		t.Fatal(err)
	}
	// 外掛 ass 排在 Index 0，PGS 在原始檔裡是第 3 條（video, audio, subrip, pgs）
	if plan.SubtitleIndex != 4 || plan.SubtitleInput != 3 {
		t.Errorf("subtitle = %d / input %d, want 4 / 3", plan.SubtitleIndex, plan.SubtitleInput)
	}
	plan, _ = Build(Profiles["zenpad10"], src, Request{})
	if plan.SubtitleIndex != -1 || plan.SubtitleInput != -1 {
		t.Errorf("no subtitle requested but got %d/%d", plan.SubtitleIndex, plan.SubtitleInput)
	}
	for _, i := range []int{0, 3, 5, 9} { // 外掛文字、內嵌文字、外掛圖形、不存在
		if _, err := Build(Profiles["zenpad10"], src, Request{SubtitleStreamIndex: idx(i)}); !errors.Is(err, ErrSubtitle) {
			t.Errorf("subtitle %d: err = %v, want ErrSubtitle", i, err)
		}
	}
}

func TestProfileVideoCodec(t *testing.T) {
	src := source(jellyfin.MediaStream{Type: "Video", Index: 0, Codec: "h264", Width: 1920, Height: 1080})
	for name, want := range map[string]string{"ipad-air1": "h264", "zenpad10": "hevc"} {
		plan, err := Build(Profiles[name], src, Request{})
		if err != nil {
			t.Fatal(err)
		}
		if plan.VideoCodec != want {
			t.Errorf("%s: VideoCodec = %q, want %q", name, plan.VideoCodec, want)
		}
	}
}

func TestFromCapsAndLadder(t *testing.T) {
	src := source(jellyfin.MediaStream{Type: "Video", Index: 0, Codec: "hevc", Width: 1920, Height: 804, BitRate: 20_000_000},
		jellyfin.MediaStream{Type: "Audio", Index: 1, Codec: "flac"})

	// ZenPad 實測：HEVC、H.264 硬解都到 1920×1088 → 選 HEVC
	p, err := FromCaps("P028", Capabilities{Decoders: []Decoder{{"h264", 1920, 1088}, {"hevc", 1920, 1088}}})
	if err != nil || p.VideoCodec != "hevc" || p.MaxWidth != 1920 || p.MaxHeight != 1088 {
		t.Fatalf("profile = %+v, %v", p, err)
	}
	top, err := Build(p, src, Request{})
	if err != nil || top.Width != 1920 || top.Height != 804 || top.CodecLevel != "4" {
		t.Fatalf("top = %dx%d level %s, %v", top.Width, top.Height, top.CodecLevel, err)
	}
	plans := Ladder(p, top, 4)
	var got []string
	for _, v := range plans {
		got = append(got, fmt.Sprintf("%dx%d", v.Width, v.Height))
		if v.VideoBitrate > top.VideoBitrate || v.VideoBitrate < minVideoBitrate {
			t.Errorf("%dx%d bitrate %d out of range", v.Width, v.Height, v.VideoBitrate)
		}
	}
	if strings.Join(got, " ") != "1920x804 1280x536 852x356 640x268" {
		t.Errorf("ladder = %v", got)
	}
	if c := plans[1].CodecsAttr(); c != "hvc1.1.6.L93.90,mp4a.40.2" {
		t.Errorf("codecs = %s", c)
	}

	// 選 720p：放進 1280×720
	plan, _ := Build(p, src, Request{MaxHeight: 720})
	if plan.Width != 1280 || plan.Height != 536 {
		t.Errorf("720p = %dx%d", plan.Width, plan.Height)
	}
	// 螢幕大小 1280×800（ZenPad）：寬度受限
	plan, _ = Build(p, source(jellyfin.MediaStream{Type: "Video", Codec: "h264", Width: 1920, Height: 1080}), Request{MaxWidth: 1280, MaxHeight: 800})
	if plan.Width != 1280 || plan.Height != 720 {
		t.Errorf("screen box = %dx%d", plan.Width, plan.Height)
	}

	// HEVC 只到 640×480（太小）就改用 H.264
	p, _ = FromCaps("x", Capabilities{Decoders: []Decoder{{"hevc", 640, 480}, {"h264", 1280, 720}}})
	if p.VideoCodec != "h264" || p.MaxHeight != 720 || p.CodecLevel != "3.1" {
		t.Errorf("fallback profile = %+v", p)
	}
	top, _ = Build(p, src, Request{})
	if c := top.CodecsAttr(); c != "avc1.64001f,mp4a.40.2" {
		t.Errorf("codecs = %s", c)
	}
	// 新手機常回報 8192×8192：照收，輸出仍限制在 1080p
	p, err = FromCaps("phone", Capabilities{Decoders: []Decoder{{"h264", 8192, 8192}, {"hevc", 8192, 8192}}})
	if err != nil || p.VideoCodec != "hevc" || p.MaxWidth != 1920 || p.MaxHeight != 1088 {
		t.Errorf("phone profile = %+v, %v", p, err)
	}
	if _, err := FromCaps("x", Capabilities{Decoders: []Decoder{{"h264", 20000, 1080}}}); err == nil {
		t.Error("absurd size accepted")
	}
	if _, err := FromCaps("x", Capabilities{Decoders: []Decoder{{"av1", 1920, 1080}}}); err == nil {
		t.Error("no usable decoder accepted")
	}
}
