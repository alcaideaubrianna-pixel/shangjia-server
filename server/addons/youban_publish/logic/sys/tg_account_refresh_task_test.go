package sys

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestRefreshTgAccountViaTask(t *testing.T) {
	keys := map[string]bool{}
	for _, tc := range []struct {
		name string
		task *collectorin.AccountTask
		err  error
		want string
	}{
		{"completed", &collectorin.AccountTask{Status: collectorin.AccountTaskStatusCompleted}, nil, sysin.PublishTgAccountStatusAuthorized},
		{"dead", &collectorin.AccountTask{Status: collectorin.AccountTaskStatusDead, ErrorMessage: "probe failed"}, nil, sysin.PublishTgAccountStatusFailed},
		{"expired", &collectorin.AccountTask{Status: collectorin.AccountTaskStatusDead, ErrorMessage: "rpc error code 406: AUTH_KEY_DUPLICATED"}, nil, sysin.PublishTgAccountStatusExpired},
		{"cancelled", &collectorin.AccountTask{Status: collectorin.AccountTaskStatusCancelled}, nil, sysin.PublishTgAccountStatusFailed},
		{"missing result", nil, nil, sysin.PublishTgAccountStatusFailed},
		{"submit failed", nil, errors.New("submit failed"), sysin.PublishTgAccountStatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, message := refreshTgAccountViaTask(context.Background(), 12, 7, 8, func(ctx context.Context, in *collectorin.AccountTaskSubmit, poll time.Duration) (*collectorin.AccountTask, error) {
				if in.AccountID != 12 || in.TenantID != 7 || in.TaskType != collectorin.AccountTaskTypeTgAccountRefresh || in.MaxAttempts != 1 || in.Priority != tgJobPriorityUrgent {
					t.Fatalf("unexpected task: %+v", in)
				}
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 20*time.Second {
					t.Fatal("probe must have a bounded deadline")
				}
				var id, tenant, operator int64
				n, err := fmt.Sscanf(in.TaskKey, "tg-account-refresh:%d:%d:%d", &id, &tenant, &operator)
				if err != nil || n != 3 || id != 12 || tenant != 7 || operator != 8 {
					t.Fatalf("handler cannot parse task key: %s", in.TaskKey)
				}
				if keys[in.TaskKey] {
					t.Fatal("probe reused an earlier task key")
				}
				keys[in.TaskKey] = true
				return tc.task, tc.err
			})
			if status != tc.want || message == "" {
				t.Fatalf("got %q %q, want %q", status, message, tc.want)
			}
		})
	}
}

func TestRefreshTgAccountViaTaskCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status, message := refreshTgAccountViaTask(ctx, 12, 7, 8, func(ctx context.Context, _ *collectorin.AccountTaskSubmit, _ time.Duration) (*collectorin.AccountTask, error) {
		t.Fatal("cancelled request must not submit a task")
		return nil, ctx.Err()
	})
	if status != sysin.PublishTgAccountStatusFailed || message == "" {
		t.Fatalf("unexpected cancellation result: %s %s", status, message)
	}
}

func TestHandleTgAccountRefreshTaskRejectsWrongOwner(t *testing.T) {
	s := &sSysPublish{}
	for _, task := range []*collectorin.AccountTask{
		nil,
		{TaskKey: "invalid", AccountID: 12, TenantID: 7},
		{TaskKey: "tg-account-refresh:13:7:8:probe", AccountID: 12, TenantID: 7},
		{TaskKey: "tg-account-refresh:12:9:8:probe", AccountID: 12, TenantID: 7},
	} {
		if err := s.handleTgAccountRefreshTask(context.Background(), nil, task); err == nil {
			t.Fatalf("invalid task accepted: %+v", task)
		}
	}
}
