package youtube

import (
	"context"
	"strings"
	"testing"

	"github.com/govdbot/govd/internal/database"
	"github.com/govdbot/govd/internal/models"
	"github.com/govdbot/govd/internal/util"
)

func testExtractorContext() *models.ExtractorContext {
	ctx, cancel := context.WithCancel(context.Background())
	return &models.ExtractorContext{
		ContentID:  "dQw4w9WgXcQ",
		ContentURL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		Extractor:  Extractor,
		Context:    ctx,
		CancelFunc: cancel,
	}
}

func TestValidateHTTPURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid https", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", false},
		{"valid http", "http://example.com/video.mp4", false},
		{"empty", "", true},
		{"relative", "/videos/abc", true},
		{"non-http", "ftp://example.com/video.mp4", true},
		{"malformed", "://bad", true},
		{"no host", "https://", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateHTTPURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateHTTPURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestParseYtDlpFormats_MuxedFormat(t *testing.T) {
	data := &YtDlpResponse{
		Title:     "Test Video",
		Duration:  212,
		Uploader:  "Test Channel",
		Thumbnail: "https://i.ytimg.com/vi/abc/maxresdefault.jpg",
		Formats: []*YtDlpFormat{
			{
				FormatID: "18",
				URL:      "https://rr1---sn.example.googlevideo.com/videoplayback?sig=secret",
				VCodec:   "avc1.42001E",
				ACodec:   "mp4a.40.2",
				Width:    640,
				Height:   360,
				TBR:      500,
				Protocol: "https",
			},
		},
	}

	formats, err := ParseYtDlpFormats(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(formats) != 1 {
		t.Fatalf("expected 1 format, got %d", len(formats))
	}

	f := formats[0]
	if f.Type != database.MediaTypeVideo {
		t.Fatalf("expected video type, got %v", f.Type)
	}
	if f.VideoCodec != database.MediaCodecAvc {
		t.Fatalf("expected avc codec, got %v", f.VideoCodec)
	}
	if f.AudioCodec != database.MediaCodecAac {
		t.Fatalf("expected aac codec, got %v", f.AudioCodec)
	}
	if f.Width != 640 || f.Height != 360 {
		t.Fatalf("unexpected dimensions: %dx%d", f.Width, f.Height)
	}
	if f.Duration != 212 {
		t.Fatalf("unexpected duration: %d", f.Duration)
	}
	if f.Title != "Test Video" || f.Artist != "Test Channel" {
		t.Fatalf("unexpected metadata: title=%q artist=%q", f.Title, f.Artist)
	}
	if len(f.ThumbnailURL) != 1 {
		t.Fatalf("expected thumbnail URL, got %v", f.ThumbnailURL)
	}
	if f.Bitrate != 500000 {
		t.Fatalf("unexpected bitrate: %d", f.Bitrate)
	}
}

func TestParseYtDlpFormats_SeparateVideoAndAudio(t *testing.T) {
	data := &YtDlpResponse{
		Title:    "Test Video",
		Duration: 120,
		Uploader: "Channel",
		Formats: []*YtDlpFormat{
			{
				FormatID: "137",
				URL:      "https://rr1---sn.example.googlevideo.com/videoplayback?v=1",
				VCodec:   "avc1.640028",
				ACodec:   "none",
				Width:    1920,
				Height:   1080,
				VBR:      4000,
				Protocol: "https",
			},
			{
				FormatID: "140",
				URL:      "https://rr1---sn.example.googlevideo.com/videoplayback?a=1",
				VCodec:   "none",
				ACodec:   "mp4a.40.2",
				ABR:      128,
				Protocol: "https",
			},
		},
	}

	formats, err := ParseYtDlpFormats(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(formats) != 2 {
		t.Fatalf("expected 2 formats, got %d", len(formats))
	}

	video := formats[0]
	audio := formats[1]
	if video.VideoCodec != database.MediaCodecAvc || video.AudioCodec != "" {
		t.Fatalf("unexpected video format: video=%v audio=%v", video.VideoCodec, video.AudioCodec)
	}
	if audio.AudioCodec != database.MediaCodecAac || audio.VideoCodec != "" {
		t.Fatalf("unexpected audio format: video=%v audio=%v", audio.VideoCodec, audio.AudioCodec)
	}
	if audio.Type != database.MediaTypeAudio {
		t.Fatalf("expected audio type, got %v", audio.Type)
	}
}

func TestParseYtDlpFormats_RejectsBadURLs(t *testing.T) {
	data := &YtDlpResponse{
		Title:    "Test",
		Duration: 60,
		Formats: []*YtDlpFormat{
			{FormatID: "1", URL: "", VCodec: "avc1.4d401e", ACodec: "none", Protocol: "https"},
			{FormatID: "2", URL: "/relative", VCodec: "avc1.4d401e", ACodec: "none", Protocol: "https"},
			{FormatID: "3", URL: "ftp://example.com/v.mp4", VCodec: "avc1.4d401e", ACodec: "none", Protocol: "https"},
		},
	}
	_, err := ParseYtDlpFormats(data)
	if err == nil || !strings.Contains(err.Error(), "no formats found") {
		t.Fatalf("expected no formats error, got %v", err)
	}
}

func TestParseYtDlpFormats_SkipsNonHTTPProtocols(t *testing.T) {
	data := &YtDlpResponse{
		Title:    "Test",
		Duration: 60,
		Formats: []*YtDlpFormat{
			{
				FormatID: "hls",
				URL:      "https://manifest.example.com/index.m3u8",
				VCodec:   "avc1.4d401e",
				ACodec:   "mp4a.40.2",
				Protocol: "m3u8_native",
			},
			{
				FormatID: "137",
				URL:      "https://rr1---sn.example.googlevideo.com/videoplayback?v=1",
				VCodec:   "avc1.640028",
				ACodec:   "none",
				Protocol: "https",
			},
		},
	}
	formats, err := ParseYtDlpFormats(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(formats) != 1 {
		t.Fatalf("expected 1 https format, got %d", len(formats))
	}
}

func TestParseYtDlpFormats_NoFormats(t *testing.T) {
	_, err := ParseYtDlpFormats(&YtDlpResponse{Title: "Empty"})
	if err == nil || !strings.Contains(err.Error(), "no formats found") {
		t.Fatalf("expected no formats error, got %v", err)
	}
}

func TestFormatSelection_PrefersMuxedH264(t *testing.T) {
	data := &YtDlpResponse{
		Title:    "Test",
		Duration: 60,
		Formats: []*YtDlpFormat{
			{
				FormatID: "137",
				URL:      "https://example.com/video.mp4",
				VCodec:   "avc1.640028",
				ACodec:   "none",
				Height:   1080,
				VBR:      3000,
				Protocol: "https",
			},
			{
				FormatID: "140",
				URL:      "https://example.com/audio.m4a",
				VCodec:   "none",
				ACodec:   "mp4a.40.2",
				ABR:      128,
				Protocol: "https",
			},
		},
	}
	formats, err := ParseYtDlpFormats(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	item := &models.MediaItem{}
	item.AddFormats(formats...)

	video := item.GetDefaultVideoFormat()
	if video == nil || video.FormatID != "137" {
		t.Fatalf("expected H.264 video format 137, got %#v", video)
	}
	if video.AudioCodec != "" {
		t.Fatalf("expected video-only format for merge pipeline, got audio codec %v", video.AudioCodec)
	}

	audio := item.GetDefaultAudioFormat()
	if audio == nil || audio.FormatID != "140" {
		t.Fatalf("expected audio format 140, got %#v", audio)
	}
}

func TestFormatSelection_PrefersMuxedOverAdaptive(t *testing.T) {
	data := &YtDlpResponse{
		Title:    "Short",
		Duration: 58,
		Formats: []*YtDlpFormat{
			{
				FormatID: "137",
				URL:      "https://example.com/video-1080.mp4",
				VCodec:   "avc1.640028",
				ACodec:   "none",
				Height:   1920,
				FileSize: 13980281,
				VBR:      3000,
				Protocol: "https",
			},
			{
				FormatID: "18",
				URL:      "https://example.com/video-muxed.mp4",
				VCodec:   "avc1.42001E",
				ACodec:   "mp4a.40.2",
				Height:   640,
				FileSize: 4365090,
				TBR:      600,
				Protocol: "https",
			},
		},
	}
	formats, err := ParseYtDlpFormats(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	item := &models.MediaItem{}
	item.AddFormats(formats...)

	video := item.GetDefaultVideoFormat()
	if video == nil || video.FormatID != "18" {
		t.Fatalf("expected muxed format 18, got %#v", video)
	}
	if video.AudioCodec == "" {
		t.Fatalf("expected muxed format to include audio")
	}
}

func TestSanitizeYtDlpError(t *testing.T) {
	raw := `ERROR: Sign in to confirm your age
Cookie: VISITOR_INFO1_LIVE=abc123
https://rr1---sn.example.googlevideo.com/videoplayback?expire=123&sig=secret`
	sanitized := sanitizeYtDlpError(raw)
	if strings.Contains(sanitized, "VISITOR_INFO1_LIVE") {
		t.Fatalf("cookie leaked into sanitized error: %q", sanitized)
	}
	if strings.Contains(sanitized, "googlevideo.com") {
		t.Fatalf("signed URL leaked into sanitized error: %q", sanitized)
	}
	if !strings.Contains(sanitized, "Sign in to confirm your age") {
		t.Fatalf("expected error message preserved, got %q", sanitized)
	}
}

func TestMapYtDlpStderr(t *testing.T) {
	tests := []struct {
		stderr string
		want   error
	}{
		{"ERROR: Sign in to confirm your age", util.ErrAgeRestricted},
		{"ERROR: Private video", util.ErrUnavailable},
		{"ERROR: Video unavailable", util.ErrUnavailable},
		{"ERROR: Use --cookies-from-browser or --cookies for authentication", util.ErrAuthenticationNeeded},
	}

	for _, tt := range tests {
		got := mapYtDlpStderr(tt.stderr)
		if got == nil || got.Error() != tt.want.Error() {
			t.Fatalf("mapYtDlpStderr(%q) = %v, want %v", tt.stderr, got, tt.want)
		}
	}
}

func TestCheckYtDlpAvailability(t *testing.T) {
	tests := []struct {
		name       string
		data       *YtDlpResponse
		wantErr    bool
		wantSubstr string
	}{
		{"live", &YtDlpResponse{LiveStatus: "is_live"}, true, "live streams"},
		{"upcoming", &YtDlpResponse{LiveStatus: "is_upcoming"}, true, "live streams"},
		{"private", &YtDlpResponse{Availability: "private"}, true, util.ErrUnavailable.Error()},
		{"needs auth", &YtDlpResponse{Availability: "needs_auth"}, true, util.ErrAuthenticationNeeded.Error()},
		{"ok", &YtDlpResponse{Availability: "public", LiveStatus: "not_live"}, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkYtDlpAvailability(tt.data)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("checkYtDlpAvailability() = %v, want substring %q", err, tt.wantSubstr)
			}
		})
	}
}

type fakeYtDlpRunner struct {
	stdout []byte
	stderr string
	runErr error
}

func (f *fakeYtDlpRunner) Run(_ context.Context, _ string) ([]byte, string, error) {
	return f.stdout, f.stderr, f.runErr
}

func TestGetVideoFromYtDlp_Integration(t *testing.T) {
	original := ytDlpExec
	t.Cleanup(func() { ytDlpExec = original })

	ytDlpExec = &fakeYtDlpRunner{
		stdout: []byte(`{
			"id": "dQw4w9WgXcQ",
			"title": "Never Gonna Give You Up",
			"duration": 213,
			"uploader": "Rick Astley",
			"thumbnail": "https://i.ytimg.com/vi/dQw4w9WgXcQ/maxresdefault.jpg",
			"availability": "public",
			"live_status": "not_live",
			"formats": [
				{
					"format_id": "18",
					"url": "https://rr1---sn.example.googlevideo.com/videoplayback?id=1",
					"vcodec": "avc1.42001E",
					"acodec": "mp4a.40.2",
					"width": 640,
					"height": 360,
					"tbr": 500,
					"protocol": "https"
				}
			]
		}`),
	}

	ctx := testExtractorContext()
	media, err := GetVideoFromYtDlp(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if media.Caption != "Never Gonna Give You Up" {
		t.Fatalf("unexpected caption: %q", media.Caption)
	}
	if len(media.Items) != 1 || len(media.Items[0].Formats) != 1 {
		t.Fatalf("unexpected media items: %#v", media.Items)
	}
}

func TestGetVideoFromYtDlp_UnavailableVideo(t *testing.T) {
	original := ytDlpExec
	t.Cleanup(func() { ytDlpExec = original })

	ytDlpExec = &fakeYtDlpRunner{
		stderr: "ERROR: Video unavailable",
		runErr: context.DeadlineExceeded,
	}

	ctx := testExtractorContext()
	_, err := GetVideoFromYtDlp(ctx)
	if err != util.ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

func TestGetVideoFromYtDlp_NoFormatsInJSON(t *testing.T) {
	original := ytDlpExec
	t.Cleanup(func() { ytDlpExec = original })

	ytDlpExec = &fakeYtDlpRunner{
		stdout: []byte(`{"title":"Empty","duration":0,"formats":[]}`),
	}

	ctx := testExtractorContext()
	_, err := GetVideoFromYtDlp(ctx)
	if err == nil || !strings.Contains(err.Error(), "no formats found") {
		t.Fatalf("expected no formats error, got %v", err)
	}
}

func TestGetVideoFromYtDlp_InvalidContentURL(t *testing.T) {
	ctx := testExtractorContext()
	ctx.ContentURL = "/watch?v=bad"
	_, err := GetVideoFromYtDlp(ctx)
	if err == nil || !strings.Contains(err.Error(), "invalid video URL") {
		t.Fatalf("expected invalid URL error, got %v", err)
	}
}

func TestGetVideoFromInv_ErrorPropagation(t *testing.T) {
	ctx := testExtractorContext()
	_, err := GetVideoFromInv(ctx)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected not configured error, got %v", err)
	}
}

func TestGetVideo_PrefersYtDlp(t *testing.T) {
	original := ytDlpExec
	t.Cleanup(func() { ytDlpExec = original })

	ytDlpExec = &fakeYtDlpRunner{
		stdout: []byte(`{
			"title": "From yt-dlp",
			"duration": 60,
			"formats": [{
				"format_id": "18",
				"url": "https://example.com/video.mp4",
				"vcodec": "avc1.42001E",
				"acodec": "mp4a.40.2",
				"protocol": "https"
			}]
		}`),
	}

	ctx := testExtractorContext()
	media, err := GetVideo(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if media.Caption != "From yt-dlp" {
		t.Fatalf("expected yt-dlp result, got caption %q", media.Caption)
	}
}
