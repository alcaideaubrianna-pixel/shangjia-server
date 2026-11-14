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

func TestCycleOperationsUseCycleAction(t *testing.T) {
	for _, operationNo := range []string{"cycle_batch:12:34:56", "profile_cycle:run-1"} {
		if got := publishSuccessRecordAction(operationNo); got != publishSuccessTypeCycle {
			t.Fatalf("operation %q classified as %q, want %q", operationNo, got, publishSuccessTypeCycle)
		}
	}
	if got := publishSuccessRecordMessage(publishSuccessTypeCycle); got != "循环上架推送成功" {
		t.Fatalf("cycle success message = %q", got)
	}
}

func TestFullPushProfileRecordIsNotBatchSuccess(t *testing.T) {
	if got := publishSuccessRecordMessage(publishSuccessTypeFull); got != "资料推送成功" {
		t.Fatalf("full push profile message = %q, want per-profile success", got)
	}
	if got := publishJobRecordMessage(publishSuccessTypeFull, "success"); got != "资料推送成功" {
		t.Fatalf("full push terminal profile message = %q, want per-profile success", got)
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

func TestPublishRecordStatusFromJob(t *testing.T) {
	tests := map[string]string{
		"sent":         "success",
		"pending":      "pending",
		"sending":      "sending",
		"failed_retry": "failed_retry",
		"failed":       "failed",
		"superseded":   "superseded",
		"unknown":      "unknown",
		"unexpected":   "pending",
	}
	for input, want := range tests {
		if got := publishRecordStatusFromJob(input); got != want {
			t.Fatalf("publishRecordStatusFromJob(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPublishJobRecordMessageTerminalStates(t *testing.T) {
	if got := publishJobRecordMessage(publishSuccessTypeCollect, "superseded"); got != "TG任务已被新操作替换" {
		t.Fatalf("superseded message = %q", got)
	}
	if got := publishJobRecordMessage(publishSuccessTypeCollect, "unknown"); got != "TG发送结果待人工确认" {
		t.Fatalf("unknown message = %q", got)
	}
}
