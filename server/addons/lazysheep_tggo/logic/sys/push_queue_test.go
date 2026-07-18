package sys

import (
	"errors"
	"testing"

	"github.com/go-telegram/bot"
)

func TestIsAmbiguousTelegramSendError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "media group response header timeout",
			err:  errors.New(`error do request for method sendMediaGroup, Post "https://api.telegram.org/bot***/sendMediaGroup": net/http: timeout awaiting response headers`),
			want: true,
		},
		{
			name: "single video context deadline",
			err:  errors.New(`error do request for method sendVideo: context deadline exceeded`),
			want: true,
		},
		{
			name: "telegram rate limit remains retryable",
			err:  &bot.TooManyRequestsError{RetryAfter: 11},
			want: false,
		},
		{
			name: "bangchat pull timeout is not telegram send",
			err:  errors.New(`采集 BangChat 消息失败: Get "https://seats.bangchat.top": context deadline exceeded`),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAmbiguousTelegramSendError(tt.err); got != tt.want {
				t.Fatalf("isAmbiguousTelegramSendError() = %v, want %v", got, tt.want)
			}
		})
	}
}
