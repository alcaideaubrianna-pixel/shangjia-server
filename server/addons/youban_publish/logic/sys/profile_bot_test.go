package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestBotProfileMediaFallbackTitle(t *testing.T) {
	if title := botProfileMediaFallbackTitle(nil, []*sysin.MessageTemplateMediaInp{{MediaType: "video"}}); title != "视频资料" {
		t.Fatalf("unexpected video fallback title: %q", title)
	}
	if title := botProfileMediaFallbackTitle([]*sysin.MessageTemplateMediaInp{{MediaType: "image"}}, nil); title != "图片资料" {
		t.Fatalf("unexpected image fallback title: %q", title)
	}
	if title := botProfileMediaFallbackTitle(nil, nil); title != "" {
		t.Fatalf("expected empty fallback title, got %q", title)
	}
}
