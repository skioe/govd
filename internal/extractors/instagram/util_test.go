package instagram

import (
	"net/url"
	"strings"
	"testing"

	"github.com/govdbot/govd/internal/database"
	"github.com/govdbot/govd/internal/models"
)

func testExtractorContext() *models.ExtractorContext {
	return &models.ExtractorContext{
		ContentID:  "DaRyLcXv6pF",
		ContentURL: "https://www.instagram.com/reel/DaRyLcXv6pF/",
		Extractor:  Extractor,
	}
}

func TestParseGQLMedia_VideoEmptyURL(t *testing.T) {
	ctx := testExtractorContext()
	_, err := ParseGQLMedia(ctx, &Media{
		Typename: "GraphVideo",
		VideoURL: "",
	})
	if err == nil {
		t.Fatal("expected error for empty video URL")
	}
	if !strings.Contains(err.Error(), "GQL video URL is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseGQLMedia_VideoValidURL(t *testing.T) {
	ctx := testExtractorContext()
	media, err := ParseGQLMedia(ctx, &Media{
		Typename: "GraphVideo",
		VideoURL: "https://cdninstagram.com/video.mp4",
		Dimensions: &Dimensions{
			Width:  720,
			Height: 1280,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(media.Items) != 1 || len(media.Items[0].Formats) != 1 {
		t.Fatalf("expected one item with one format, got %#v", media.Items)
	}
	format := media.Items[0].Formats[0]
	if format.URL[0] != "https://cdninstagram.com/video.mp4" {
		t.Fatalf("unexpected video URL: %q", format.URL[0])
	}
	if format.Type != database.MediaTypeVideo {
		t.Fatalf("expected video type, got %v", format.Type)
	}
}

func TestParseGQLMedia_ImageEmptyURL(t *testing.T) {
	ctx := testExtractorContext()
	_, err := ParseGQLMedia(ctx, &Media{
		Typename:   "GraphImage",
		DisplayURL: "",
	})
	if err == nil {
		t.Fatal("expected error for empty image URL")
	}
	if !strings.Contains(err.Error(), "GQL image URL is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseGQLMedia_UnsupportedTypename(t *testing.T) {
	ctx := testExtractorContext()
	_, err := ParseGQLMedia(ctx, &Media{
		Typename: "GraphUnknown",
	})
	if err == nil {
		t.Fatal("expected error for unsupported typename")
	}
	if !strings.Contains(err.Error(), "unsupported media typename") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseGQLMedia_SidecarEmptyChildURL(t *testing.T) {
	ctx := testExtractorContext()
	_, err := ParseGQLMedia(ctx, &Media{
		Typename: "GraphSidecar",
		EdgeSidecarToChildren: &EdgeSidecarToChildren{
			Edges: []*EdgeNode{
				{
					Node: &Media{
						Typename:   "GraphImage",
						DisplayURL: "",
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected error for sidecar child with empty URL")
	}
	if !strings.Contains(err.Error(), "sidecar image URL is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseGQLMedia_NilDimensions(t *testing.T) {
	ctx := testExtractorContext()
	media, err := ParseGQLMedia(ctx, &Media{
		Typename: "GraphVideo",
		VideoURL: "https://cdninstagram.com/video.mp4",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	format := media.Items[0].Formats[0]
	if format.Width != 0 || format.Height != 0 {
		t.Fatalf("expected zero dimensions, got %dx%d", format.Width, format.Height)
	}
}

func TestParseGQLMedia_VideoMissingThumbnail(t *testing.T) {
	ctx := testExtractorContext()
	media, err := ParseGQLMedia(ctx, &Media{
		Typename:   "GraphVideo",
		VideoURL:   "https://cdninstagram.com/video.mp4",
		DisplayURL: "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	format := media.Items[0].Formats[0]
	if len(format.ThumbnailURL) != 0 {
		t.Fatalf("expected no thumbnail, got %#v", format.ThumbnailURL)
	}
	if format.URL[0] != "https://cdninstagram.com/video.mp4" {
		t.Fatalf("unexpected video URL: %q", format.URL[0])
	}
}

func TestGetCDNURL_NoURI(t *testing.T) {
	_, err := GetCDNURL("https://igram.world/proxy?foo=bar")
	if err == nil {
		t.Fatal("expected error when uri parameter is missing")
	}
	if !strings.Contains(err.Error(), "no CDN uri") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCDNURL_EmptyURI(t *testing.T) {
	_, err := GetCDNURL("https://igram.world/proxy?uri=")
	if err == nil {
		t.Fatal("expected error when uri parameter is empty")
	}
	if !strings.Contains(err.Error(), "no CDN uri") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCDNURL_InvalidScheme(t *testing.T) {
	_, err := GetCDNURL("https://igram.world/proxy?uri=" + url.QueryEscape("file:///etc/passwd"))
	if err == nil {
		t.Fatal("expected error for non-HTTP scheme")
	}
	if !strings.Contains(err.Error(), "invalid scheme") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCDNURL_PercentEncodedHTTPS(t *testing.T) {
	wrapped := "https://igram.world/proxy?uri=" + url.QueryEscape("https://cdn.example.com/media/video.mp4")
	cdnURL, err := GetCDNURL(wrapped)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cdnURL != "https://cdn.example.com/media/video.mp4" {
		t.Fatalf("unexpected CDN URL: %q", cdnURL)
	}
}
