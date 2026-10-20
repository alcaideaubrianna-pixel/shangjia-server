package sys

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"hotgo/addons/telegram_collector/model/input/sysin"
)

// New task constants must pass the actual submission gate, not just have a
// registered consumer. Reading the declarations prevents a hand-written test
// list from silently omitting the next plugin task type.
func TestValidateAccountTaskSubmitCoversDeclaredTypes(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "../../model/input/sysin/account_task.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	ast.Inspect(f, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 || !strings.HasPrefix(spec.Names[0].Name, "AccountTaskType") {
			return true
		}
		literal, ok := spec.Values[0].(*ast.BasicLit)
		if !ok {
			t.Fatalf("unsupported task constant declaration: %s", spec.Names[0].Name)
		}
		kind, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Fatal(err)
		}
		count++
		t.Run(kind, func(t *testing.T) {
			in := &sysin.AccountTaskSubmit{TaskType: kind, TaskKey: "test:1", HistoryTaskID: 1, MediaOwnerAccountID: 1, Media: &sysin.CollectorMediaItem{FileID: "test-file"}}
			if err := validateAccountTaskSubmit(in); err != nil {
				t.Fatalf("declared task %s cannot be submitted: %v", spec.Names[0].Name, err)
			}
		})
		return true
	})
	if count == 0 {
		t.Fatal("no account task declarations found")
	}
}

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
		{name: "bot manager", in: &sysin.AccountTaskSubmit{TaskType: sysin.AccountTaskTypeBotManager}},
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
	for _, status := range []string{sysin.AccountTaskStatusPending, sysin.AccountTaskStatusProcessing, sysin.AccountTaskStatusFailedRetry, sysin.AccountTaskStatusFinalizing} {
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
		sysin.AccountTaskTypeChannelBotAttach,
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
