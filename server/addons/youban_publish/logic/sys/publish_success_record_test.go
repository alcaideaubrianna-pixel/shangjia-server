package sys

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMessagePushOperationUsesQuickPushAction(t *testing.T) {
	operations := []string{
		"message_push:6:1785690000000000000:21:5593648889:hash",
		"message_push_plan:3:1785690000000000000:6:21",
	}
	for _, operationNo := range operations {
		if action := publishSuccessRecordAction(operationNo); action != publishSuccessTypeQuick {
			t.Fatalf("operation %s classified as %s", operationNo, action)
		}
	}
	if message := publishJobRecordMessage(publishSuccessTypeQuick, "failed"); message != "快速推送失败" {
		t.Fatalf("unexpected failed message: %s", message)
	}
	if message := publishSuccessRecordMessage(publishSuccessTypeQuick); message != "快速推送成功" {
		t.Fatalf("unexpected success message: %s", message)
	}
}

func TestBoundPublishSuccessRecordMessage(t *testing.T) {
	short := "Telegram 发送失败"
	if got := boundPublishSuccessRecordMessage(short); got != short {
		t.Fatalf("short message = %q, want %q", got, short)
	}

	long := strings.Repeat("错误", 200)
	got := boundPublishSuccessRecordMessage(long)
	if !utf8.ValidString(got) {
		t.Fatal("bounded message is not valid UTF-8")
	}
	if count := utf8.RuneCountInString(got); count != publishSuccessRecordMessageMaxRunes {
		t.Fatalf("bounded message runes = %d, want %d", count, publishSuccessRecordMessageMaxRunes)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("bounded message = %q, want ellipsis suffix", got)
	}
}
