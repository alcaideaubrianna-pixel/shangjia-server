package sys

import (
	"strings"
	"testing"

	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestValidateAccountTaskSubmitSupportsAllRegisteredTaskTypes(t *testing.T) {
	tests := []struct {
		name string
		in   *sysin.AccountTaskSubmit
	}{
		{name: "history", in: &sysin.AccountTaskSubmit{TaskType: sysin.AccountTaskTypeHistoryPage, HistoryTaskID: 1}},
		{name: "material import history", in: &sysin.AccountTaskSubmit{TaskType: sysin.AccountTaskTypeMaterialImportHistoryPage, TaskKey: "material-import:1:offset:0"}},
		{name: "username diagnostic", in: &sysin.AccountTaskSubmit{TaskType: sysin.AccountTaskTypeUsernameResolveDiagnostic}},
		{name: "dialog refresh", in: &sysin.AccountTaskSubmit{TaskType: sysin.AccountTaskTypeDialogCacheRefresh}},
		{name: "message push inline", in: &sysin.AccountTaskSubmit{TaskType: sysin.AccountTaskTypeMessagePushInline}},
		{name: "message reconcile", in: &sysin.AccountTaskSubmit{TaskType: sysin.AccountTaskTypeMessageReconcile}},
		{name: "managed bot username check", in: &sysin.AccountTaskSubmit{TaskType: sysin.AccountTaskTypeManagedBotUsernameCheck}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateAccountTaskSubmit(test.in); err != nil {
				t.Fatalf("task type %q rejected: %v", test.in.TaskType, err)
			}
		})
	}
}

func TestValidateAccountTaskSubmitRejectsUnknownTaskType(t *testing.T) {
	err := validateAccountTaskSubmit(&sysin.AccountTaskSubmit{TaskType: "unknown_task"})
	if err == nil || !strings.Contains(err.Error(), "不支持的Telegram账号任务类型") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAccountTaskTerminal(t *testing.T) {
	for _, status := range []string{sysin.AccountTaskStatusCompleted, sysin.AccountTaskStatusDead, sysin.AccountTaskStatusCancelled} {
		if !accountTaskTerminal(status) {
			t.Fatalf("status %q should be terminal", status)
		}
	}
	for _, status := range []string{sysin.AccountTaskStatusPending, sysin.AccountTaskStatusProcessing, sysin.AccountTaskStatusFailedRetry} {
		if accountTaskTerminal(status) {
			t.Fatalf("status %q should not be terminal", status)
		}
	}
}

func TestAccountTaskCanRevive(t *testing.T) {
	for _, taskType := range []string{
		sysin.AccountTaskTypeHistoryPage,
		sysin.AccountTaskTypeMaterialImportHistoryPage,
		sysin.AccountTaskTypeMediaDownload,
		sysin.AccountTaskTypeMessageReconcile,
		sysin.AccountTaskTypeMessageMediaFallback,
		sysin.AccountTaskTypeManagedBotUsernameCheck,
	} {
		if !accountTaskCanRevive(taskType) {
			t.Fatalf("task type %q should be revivable", taskType)
		}
	}
	if accountTaskCanRevive(sysin.AccountTaskTypeDialogCacheRefresh) {
		t.Fatal("dialog cache refresh should not revive a terminal task")
	}
}
