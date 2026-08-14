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
