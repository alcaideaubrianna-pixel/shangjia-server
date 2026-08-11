package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestAccountTaskPersistenceIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_TELEGRAM_COLLECTOR_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_TELEGRAM_COLLECTOR_INTEGRATION=1 to run database integration test")
	}
	ctx := context.Background()
	service := &sAccountTasks{}
	seed := time.Now().UnixNano()
	payload, _ := json.Marshal(map[string]int64{"taskId": seed})
	submit := &sysin.AccountTaskSubmit{
		TenantID: seed, AccountID: seed, TaskType: sysin.AccountTaskTypeHistoryPage,
		TaskKey: fmt.Sprintf("integration:%d", seed), Priority: 10, Payload: payload, MaxAttempts: 2,
	}
	taskID, err := service.Submit(ctx, submit)
	if err != nil {
		t.Fatalf("submit account task: %v", err)
	}
	duplicateID, err := service.Submit(ctx, submit)
	if err != nil || duplicateID != taskID {
		t.Fatalf("duplicate task id=%d want=%d err=%v", duplicateID, taskID, err)
	}
	loaded, err := service.Get(ctx, taskID)
	if err != nil || loaded == nil || loaded.ID != taskID || loaded.Status != sysin.AccountTaskStatusPending {
		t.Fatalf("get task=%+v err=%v", loaded, err)
	}
	lease := &sysin.AccountLease{AccountID: seed, InstanceID: "integration-owner", Epoch: seed, ExpiresAt: time.Now().Add(time.Minute)}
	tasks, err := service.Claim(ctx, lease, 1, time.Minute)
	if err != nil || len(tasks) != 1 || tasks[0].ID != taskID {
		t.Fatalf("claim tasks=%+v err=%v", tasks, err)
	}
	wrongLease := &sysin.AccountLease{AccountID: seed, InstanceID: "wrong-owner", Epoch: seed + 1}
	if err = service.Complete(ctx, taskID, wrongLease, nil); err == nil {
		t.Fatal("stale lease must not complete account task")
	}
	result, _ := json.Marshal(map[string]bool{"ok": true})
	if err = service.Complete(ctx, taskID, lease, result); err != nil {
		t.Fatalf("complete account task: %v", err)
	}
	columns := dao.TgCollectorAccountTask.Columns()
	row, err := dao.TgCollectorAccountTask.Ctx(ctx).WherePri(taskID).One()
	if err != nil || row[columns.Status].String() != sysin.AccountTaskStatusCompleted {
		t.Fatalf("completed status=%s err=%v", row[columns.Status].String(), err)
	}

	recoverySubmit := &sysin.AccountTaskSubmit{
		TenantID: seed, AccountID: seed, TaskType: sysin.AccountTaskTypeHistoryPage,
		TaskKey: fmt.Sprintf("integration-recovery:%d", seed), Priority: 1, Payload: payload, MaxAttempts: 2,
	}
	recoveryID, err := service.Submit(ctx, recoverySubmit)
	if err != nil {
		t.Fatalf("submit recovery task: %v", err)
	}
	recoveryTasks, err := service.Claim(ctx, lease, 1, time.Millisecond)
	if err != nil || len(recoveryTasks) != 1 || recoveryTasks[0].ID != recoveryID {
		t.Fatalf("claim recovery tasks=%+v err=%v", recoveryTasks, err)
	}
	time.Sleep(5 * time.Millisecond)
	recovered, err := service.RecoverExpired(ctx, 10)
	if err != nil || recovered < 1 {
		t.Fatalf("recover expired=%d err=%v", recovered, err)
	}
	recoveryRow, err := dao.TgCollectorAccountTask.Ctx(ctx).WherePri(recoveryID).One()
	if err != nil || recoveryRow[columns.Status].String() != sysin.AccountTaskStatusFailedRetry {
		t.Fatalf("recovery status=%s err=%v", recoveryRow[columns.Status].String(), err)
	}

	t.Cleanup(func() {
		_, _ = dao.TgCollectorAccountTask.Ctx(ctx).Where(columns.TenantId, seed).Delete()
		_, _ = g.DB().Exec(ctx, "SELECT 1")
	})
}
