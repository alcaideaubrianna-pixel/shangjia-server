package sys

import (
	"errors"
	"testing"
)

func TestIsTelegramMediaSourceUnavailableError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "http 404", err: errors.New("下载远程媒体失败：HTTP 404"), want: true},
		{name: "wrapped http 404", err: errors.New("TG展示资料推送失败: 下载远程媒体失败：HTTP 404"), want: true},
		{name: "http 500", err: errors.New("下载远程媒体失败：HTTP 500"), want: false},
		{name: "timeout", err: errors.New("下载远程媒体失败：context deadline exceeded"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isTelegramMediaSourceUnavailableError(test.err); got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}
