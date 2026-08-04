package sys

import (
	"strings"
	"testing"
)

func TestTruncateTelegramObserveError(t *testing.T) {
	short := "发送失败"
	if got := truncateTelegramObserveError(short); got != short {
		t.Fatalf("short message changed: %q", got)
	}

	long := strings.Repeat("错", telegramObserveLastErrorMaxRunes+10)
	got := truncateTelegramObserveError(long)
	if len([]rune(got)) != telegramObserveLastErrorMaxRunes {
		t.Fatalf("truncated rune length = %d", len([]rune(got)))
	}
}
