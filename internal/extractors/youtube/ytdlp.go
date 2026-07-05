package youtube

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/govdbot/govd/internal/models"
	"github.com/govdbot/govd/internal/util"
)

// Telegram-friendly format: prefer H.264/AAC in MP4, up to 1080p, with muxed fallback.
const telegramFormat = "bestvideo[ext=mp4][vcodec^=avc1][height<=1080]+bestaudio[ext=m4a][acodec^=mp4a]/" +
	"best[ext=mp4][vcodec^=avc1][acodec^=mp4a]/" +
	"bestvideo[ext=mp4][vcodec^=avc1][height<=1080]+bestaudio[ext=m4a]/" +
	"best[ext=mp4]/best"

type ytDlpRunner interface {
	Run(ctx context.Context, videoURL string) (stdout []byte, stderr string, err error)
}

type execYtDlpRunner struct {
	path string
}

func (r *execYtDlpRunner) Run(ctx context.Context, videoURL string) ([]byte, string, error) {
	cmd := exec.CommandContext(
		ctx,
		r.path,
		"--no-playlist",
		"--no-warnings",
		"--js-runtimes=quickjs",
		"--dump-single-json",
		"-f", telegramFormat,
		videoURL,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.Bytes(), stderr.String(), err
}

var ytDlpExec ytDlpRunner = &execYtDlpRunner{path: "yt-dlp"}

func GetVideoFromYtDlp(ctx *models.ExtractorContext) (*models.Media, error) {
	videoURL, err := validateHTTPURL(ctx.ContentURL)
	if err != nil {
		return nil, fmt.Errorf("invalid video URL: %w", err)
	}

	stdout, stderr, err := ytDlpExec.Run(ctx.Context, videoURL)
	if err != nil {
		if mapped := mapYtDlpStderr(stderr); mapped != nil {
			return nil, mapped
		}
		sanitized := sanitizeYtDlpError(stderr)
		if sanitized != "" {
			return nil, fmt.Errorf("yt-dlp failed: %s", sanitized)
		}
		return nil, fmt.Errorf("yt-dlp failed: %w", err)
	}

	var data YtDlpResponse
	if err := sonic.Unmarshal(stdout, &data); err != nil {
		sanitized := sanitizeYtDlpError(string(stdout))
		if sanitized != "" {
			return nil, fmt.Errorf("failed to decode yt-dlp response: %s", sanitized)
		}
		return nil, fmt.Errorf("failed to decode yt-dlp response: %w", err)
	}

	if err := checkYtDlpAvailability(&data); err != nil {
		return nil, err
	}

	formats, err := ParseYtDlpFormats(&data)
	if err != nil {
		return nil, err
	}

	media := ctx.NewMedia()
	media.SetCaption(data.Title)
	item := media.NewItem()
	item.AddFormats(formats...)

	return media, nil
}

func checkYtDlpAvailability(data *YtDlpResponse) error {
	switch data.LiveStatus {
	case "is_live":
		return fmt.Errorf("live streams are not supported")
	case "is_upcoming":
		return fmt.Errorf("upcoming live streams are not supported")
	}
	if data.IsLive {
		return fmt.Errorf("live streams are not supported")
	}

	switch data.Availability {
	case "private":
		return util.ErrUnavailable
	case "premium_only", "needs_auth":
		return util.ErrAuthenticationNeeded
	}
	return nil
}

func mapYtDlpStderr(stderr string) error {
	lower := strings.ToLower(stderr)

	switch {
	case strings.Contains(lower, "sign in to confirm your age"),
		strings.Contains(lower, "age-restricted"),
		strings.Contains(lower, "confirm your age"):
		return util.ErrAgeRestricted
	case strings.Contains(lower, "private video"):
		return util.ErrUnavailable
	case strings.Contains(lower, "video unavailable"),
		strings.Contains(lower, "this video is unavailable"),
		strings.Contains(lower, "has been removed"):
		return util.ErrUnavailable
	case strings.Contains(lower, "sign in") && strings.Contains(lower, "cookies"),
		strings.Contains(lower, "use --cookies"),
		strings.Contains(lower, "login required"),
		strings.Contains(lower, "members-only"):
		return util.ErrAuthenticationNeeded
	case strings.Contains(lower, "live event will begin"),
		strings.Contains(lower, "is live"),
		strings.Contains(lower, "live stream"):
		return fmt.Errorf("live streams are not supported")
	case strings.Contains(lower, "requested format is not available"),
		strings.Contains(lower, "no video formats"):
		return fmt.Errorf("no formats found")
	}
	return nil
}
