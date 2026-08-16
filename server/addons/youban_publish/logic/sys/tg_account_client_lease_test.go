package sys

import (
	"context"
	"testing"

	"github.com/gogf/gf/v2/errors/gerror"
)

func TestTelegramAccountBusyErrorSurvivesWrap(t *testing.T) {
	err := gerror.Wrap(&telegramAccountBusyError{tgAccountId: 33, err: context.DeadlineExceeded}, "Inline推送失败")
	if !isTelegramAccountBusyError(err) {
		t.Fatalf("wrapped error should remain a Telegram account busy error: %v", err)
	}
}

func TestCollectDisplayEventPassedEarlyCheck(t *testing.T) {
	for _, status := range []string{"prechecked", "media_pending", "media_ready", "processed", "dispatched"} {
		if !collectDisplayEventPassedEarlyCheck(status) {
			t.Fatalf("status %q must allow verify media", status)
		}
	}
	for _, status := range []string{"pending", "group_collecting", "ignored", "failed", ""} {
		if collectDisplayEventPassedEarlyCheck(status) {
			t.Fatalf("status %q must block verify media", status)
		}
	}
}
