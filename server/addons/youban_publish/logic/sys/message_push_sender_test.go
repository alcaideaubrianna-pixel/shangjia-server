package sys

import (
	"errors"
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestMessageTemplateUsesInline(t *testing.T) {
	tests := []struct {
		name     string
		template *sysin.MessageTemplateModel
		want     bool
	}{
		{name: "text", template: &sysin.MessageTemplateModel{SerialNo: "XX123456", Text: "text"}, want: true},
		{name: "single image", template: &sysin.MessageTemplateModel{SerialNo: "XX123456", Media: []*sysin.MessageTemplateMediaModel{{MediaType: "image"}}}, want: true},
		{name: "single video", template: &sysin.MessageTemplateModel{SerialNo: "XX123456", Media: []*sysin.MessageTemplateMediaModel{{MediaType: "video"}}}, want: false},
		{name: "multiple images", template: &sysin.MessageTemplateModel{SerialNo: "XX123456", Media: []*sysin.MessageTemplateMediaModel{{MediaType: "image"}, {MediaType: "image"}}}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := messageTemplateUsesInline(test.template); got != test.want {
				t.Fatalf("messageTemplateUsesInline() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestShouldFallbackListenerBotToAccount(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "bot forbidden", err: errors.New("forbidden, bot is not a member of the group chat"), want: true},
		{name: "invalid token", err: errors.New("unauthorized, invalid bot token"), want: true},
		{name: "chat not found", err: errors.New("bad request, chat not found"), want: true},
		{name: "timeout", err: errors.New("context deadline exceeded"), want: false},
		{name: "rate limit", err: errors.New("too many requests, retry after 30"), want: false},
		{name: "flood wait", err: errors.New("FLOOD_WAIT_20"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldFallbackListenerBotToAccount(test.err); got != test.want {
				t.Fatalf("shouldFallbackListenerBotToAccount() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestListenerAccountFallbackText(t *testing.T) {
	got := listenerAccountFallbackText("通知内容", "查看用户", "https://t.me/example", false)
	want := "通知内容\n\n<a href=\"https://t.me/example\">查看用户</a>"
	if got != want {
		t.Fatalf("listenerAccountFallbackText() = %q, want %q", got, want)
	}
}
