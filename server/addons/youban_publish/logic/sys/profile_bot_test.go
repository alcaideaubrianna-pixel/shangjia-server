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

func TestBotProfileTitleFromText(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "nickname", text: "昵称：孤岛 s233\n地区：四川 成都", want: "孤岛 s233"},
		{name: "explicit title", text: "标题：杭州新人\n年龄：20", want: "杭州新人"},
		{name: "plain first line", text: "杭州新人\n年龄：20", want: "杭州新人"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := botProfileTitleFromText(test.text); got != test.want {
				t.Fatalf("botProfileTitleFromText() = %q, want %q", got, test.want)
			}
		})
	}
}
