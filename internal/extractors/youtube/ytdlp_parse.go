package youtube

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/govdbot/govd/internal/config"
	"github.com/govdbot/govd/internal/database"
	"github.com/govdbot/govd/internal/models"
)

var (
	signedURLPattern = regexp.MustCompile(`https?://[^\s"']+`)
	jsonBlobPattern  = regexp.MustCompile(`\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}`)
)

func validateHTTPURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty URL")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("malformed URL: %w", err)
	}
	if !parsed.IsAbs() {
		return "", fmt.Errorf("relative URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("non-HTTP URL")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("malformed URL: missing host")
	}
	return parsed.String(), nil
}

func ParseYtDlpFormats(data *YtDlpResponse) ([]*models.MediaFormat, error) {
	if len(data.Formats) == 0 {
		return nil, fmt.Errorf("no formats found")
	}

	duration := int32(data.Duration)
	thumbnail := validateThumbnailURL(data.Thumbnail)

	formats := make([]*models.MediaFormat, 0, len(data.Formats))
	for _, f := range data.Formats {
		parsed, ok := parseYtDlpFormat(f, data, duration, thumbnail)
		if !ok {
			continue
		}
		formats = append(formats, parsed)
	}

	if len(formats) == 0 {
		return nil, fmt.Errorf("no formats found")
	}
	return formats, nil
}

func parseYtDlpFormat(
	f *YtDlpFormat,
	data *YtDlpResponse,
	duration int32,
	thumbnail string,
) (*models.MediaFormat, bool) {
	if f == nil {
		return nil, false
	}
	if isYtDlpFormatSkipped(f) {
		return nil, false
	}

	mediaURL, err := validateHTTPURL(f.URL)
	if err != nil {
		return nil, false
	}

	vCodec := parseYtDlpVideoCodec(f.VCodec)
	aCodec := parseYtDlpAudioCodec(f.ACodec)

	mediaType, vCodec, aCodec := classifyYtDlpStream(vCodec, aCodec)
	if mediaType == "" {
		return nil, false
	}

	bitrate := ytDlpBitrate(f)
	fileSize := ytDlpFileSize(f)
	if fileSize > 0 && fileSize > config.Env.MaxFileSize {
		return nil, false
	}

	format := &models.MediaFormat{
		Type:       mediaType,
		VideoCodec: vCodec,
		AudioCodec: aCodec,
		FormatID:   f.FormatID,
		Width:      int32(f.Width),
		Height:     int32(f.Height),
		Bitrate:    bitrate,
		FileSize:   fileSize,
		Duration:   duration,
		URL:        []string{mediaURL},
		Title:      data.Title,
		Artist:     data.Uploader,
		DownloadSettings: &models.DownloadSettings{
			ChunkSize: 10 * 1024 * 1024,
		},
	}
	if thumbnail != "" && mediaType == database.MediaTypeVideo {
		format.ThumbnailURL = []string{thumbnail}
	}
	return format, true
}

func isYtDlpFormatSkipped(f *YtDlpFormat) bool {
	if f.VCodec == "none" && f.ACodec == "none" {
		return true
	}
	if strings.Contains(strings.ToLower(f.FormatNote), "storyboard") {
		return true
	}
	if strings.Contains(f.URL, "dubbed-auto") {
		return true
	}
	protocol := strings.ToLower(f.Protocol)
	if protocol != "" && protocol != "http" && protocol != "https" {
		return true
	}
	return false
}

func classifyYtDlpStream(
	vCodec database.MediaCodec,
	aCodec database.MediaCodec,
) (database.MediaType, database.MediaCodec, database.MediaCodec) {
	switch {
	case vCodec != "" && aCodec != "":
		return database.MediaTypeVideo, vCodec, aCodec
	case vCodec != "":
		return database.MediaTypeVideo, vCodec, ""
	case aCodec != "":
		return database.MediaTypeAudio, "", aCodec
	default:
		return "", "", ""
	}
}

func parseYtDlpVideoCodec(codec string) database.MediaCodec {
	if codec == "" || codec == "none" {
		return ""
	}
	switch {
	case strings.HasPrefix(codec, "avc1"), strings.HasPrefix(codec, "avc3"):
		return database.MediaCodecAvc
	case strings.HasPrefix(codec, "hev1"), strings.HasPrefix(codec, "hvc1"):
		return database.MediaCodecHevc
	case strings.HasPrefix(codec, "av01"):
		return database.MediaCodecAv1
	case strings.HasPrefix(codec, "vp9"), strings.HasPrefix(codec, "vp09"):
		return database.MediaCodecVp9
	case strings.HasPrefix(codec, "vp8"):
		return database.MediaCodecVp9
	default:
		return ""
	}
}

func parseYtDlpAudioCodec(codec string) database.MediaCodec {
	if codec == "" || codec == "none" {
		return ""
	}
	switch {
	case strings.HasPrefix(codec, "mp4a"):
		return database.MediaCodecAac
	case strings.HasPrefix(codec, "opus"):
		return database.MediaCodecOpus
	case strings.HasPrefix(codec, "mp3"):
		return database.MediaCodecMp3
	case strings.HasPrefix(codec, "flac"):
		return database.MediaCodecFlac
	case strings.HasPrefix(codec, "vorbis"):
		return database.MediaCodecVorbis
	default:
		return ""
	}
}

func ytDlpFileSize(f *YtDlpFormat) int64 {
	if f.FileSize > 0 {
		return f.FileSize
	}
	return f.FileSizeApprox
}

func ytDlpBitrate(f *YtDlpFormat) int64 {
	if f.TBR > 0 {
		return int64(f.TBR * 1000)
	}
	var total float64
	if f.VBR > 0 {
		total += f.VBR
	}
	if f.ABR > 0 {
		total += f.ABR
	}
	if total > 0 {
		return int64(total * 1000)
	}
	return 0
}

func validateThumbnailURL(raw string) string {
	if raw == "" {
		return ""
	}
	validated, err := validateHTTPURL(raw)
	if err != nil {
		return ""
	}
	return validated
}

func sanitizeYtDlpError(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	lines := strings.Split(raw, "\n")
	sanitized := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "cookie") ||
			strings.Contains(lower, "set-cookie") ||
			strings.Contains(lower, "authorization") ||
			strings.Contains(lower, "bearer ") {
			continue
		}
		line = signedURLPattern.ReplaceAllString(line, "<url>")
		line = jsonBlobPattern.ReplaceAllString(line, "<json>")
		sanitized = append(sanitized, line)
	}

	result := strings.Join(sanitized, "; ")
	if len(result) > 500 {
		result = result[:500] + "..."
	}
	return result
}
