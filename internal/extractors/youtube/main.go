package youtube

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/govdbot/govd/internal/logger"
	"github.com/govdbot/govd/internal/models"
	"github.com/govdbot/govd/internal/util"

	"github.com/bytedance/sonic"
)

var Extractor = &models.Extractor{
	ID:          "youtube",
	DisplayName: "YouTube",

	URLPattern: regexp.MustCompile(`(?:https?:)?(?:\/\/)?(?:(?:www|m)\.)?(?:youtube(?:-nocookie)?\.com\/(?:(?:watch\?(?:.*&)?v=)|(?:embed\/)|(?:v\/)|(?:shorts\/))|youtu\.be\/)(?P<id>[\w-]{11})(?:[?&].*)?`),
	Host: []string{
		"youtube",
		"youtu",
		"youtube-nocookie",
	},

	GetFunc: func(ctx *models.ExtractorContext) (*models.ExtractorResponse, error) {
		video, err := GetVideo(ctx)
		if err != nil {
			return nil, err
		}
		return &models.ExtractorResponse{
			Media: video,
		}, nil
	},
}

func GetVideo(ctx *models.ExtractorContext) (*models.Media, error) {
	media, err := GetVideoFromYtDlp(ctx)
	if err == nil {
		return media, nil
	}
	if hasInvidiousConfig(ctx) {
		invMedia, invErr := GetVideoFromInv(ctx)
		if invErr == nil {
			return invMedia, nil
		}
	}
	return nil, err
}

func hasInvidiousConfig(ctx *models.ExtractorContext) bool {
	if ctx.Config == nil {
		return false
	}
	for _, instance := range ctx.Config.Instance {
		if instance != "" {
			return true
		}
	}
	return false
}

func GetVideoFromInv(ctx *models.ExtractorContext) (*models.Media, error) {
	if !hasInvidiousConfig(ctx) {
		return nil, fmt.Errorf("youtube invidious not configured")
	}
	var lastErr error
	for i := range ctx.Config.Instance {
		instance, err := GetInvInstance(ctx, i)
		if err != nil {
			lastErr = err
			continue
		}
		media, err := GetFromInstance(ctx, instance)
		if err == nil {
			return media, nil
		}
		lastErr = err
		ctx.Debugf("invidious instance %s failed: %v", instance, err)
	}
	if lastErr != nil {
		return nil, fmt.Errorf("all invidious instances failed: %w", lastErr)
	}
	return nil, fmt.Errorf("no invidious instances configured")
}

func GetFromInstance(ctx *models.ExtractorContext, instance string) (*models.Media, error) {
	videoID := ctx.ContentID
	reqURL := instance +
		invEndpoint +
		videoID +
		"?local=true" // proxied CDN

	ctx.Debugf("proxied invidious api: %s", reqURL)

	resp, err := ctx.Fetch(
		http.MethodGet,
		reqURL, nil,
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	logger.WriteFile("inv_youtube_response", resp)

	var data *InvResponse
	decoder := sonic.ConfigFastest.NewDecoder(resp.Body)
	err = decoder.Decode(&data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	switch data.Error {
	case "This video may be inappropriate for some users.":
		return nil, util.ErrAgeRestricted
	default:
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("bad response: %s", resp.Status)
		}
	}

	formats := ParseInvFormats(data, instance)
	if len(formats) == 0 {
		return nil, fmt.Errorf("no formats found")
	}

	media := ctx.NewMedia()
	media.SetCaption(data.Title)
	item := media.NewItem()
	item.AddFormats(formats...)

	return media, nil
}
