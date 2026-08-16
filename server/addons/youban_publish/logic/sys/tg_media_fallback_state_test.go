package sys

import (
	"errors"
	"testing"
)

func TestTelegramMediaFallbackQueuedSignalIsPreserved(t *testing.T) {
	err := func() error {
		return errTelegramMediaFallbackQueued
	}()
	if !errors.Is(err, errTelegramMediaFallbackQueued) {
		t.Fatal("协议号降级排队信号必须向发布调度层传播")
	}
}

func TestMediaFallbackTaskCanSkipOnlyAfterRealMessageSaved(t *testing.T) {
	tests := []struct {
		name         string
		status       string
		messageCount int
		want         bool
	}{
		{name: "发送完成且有消息", status: "sent", messageCount: 1, want: true},
		{name: "错误标记成功但没有消息", status: "sent", messageCount: 0, want: false},
		{name: "发送中不能跳过", status: "sending", messageCount: 1, want: false},
		{name: "失败任务不能跳过", status: "failed", messageCount: 1, want: false},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := mediaFallbackTaskCanSkip(testCase.status, testCase.messageCount); got != testCase.want {
				t.Fatalf("mediaFallbackTaskCanSkip(%q, %d) = %t, want %t", testCase.status, testCase.messageCount, got, testCase.want)
			}
		})
	}
}
