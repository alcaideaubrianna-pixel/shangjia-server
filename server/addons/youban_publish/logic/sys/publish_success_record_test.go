package sys

import "testing"

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
