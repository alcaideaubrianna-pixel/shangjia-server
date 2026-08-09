package sys

import (
	"testing"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestTelegramRecoveryTimeTextUsesApplicationWallClock(t *testing.T) {
	value := gtime.NewFromStr("2026-08-09 17:07:13")
	if got := telegramRecoveryTimeText(value); got != "2026-08-09 17:07:13" {
		t.Fatalf("telegramRecoveryTimeText() = %q, want %q", got, "2026-08-09 17:07:13")
	}
}

func TestTelegramRecoveryTimeTextNil(t *testing.T) {
	if got := telegramRecoveryTimeText(nil); got != "" {
		t.Fatalf("telegramRecoveryTimeText(nil) = %q, want empty string", got)
	}
}
