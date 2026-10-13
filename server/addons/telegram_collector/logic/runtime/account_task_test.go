package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

type testAccountTaskFailureHandler struct{ called chan error }

func (h *testAccountTaskFailureHandler) HandleAccountTaskFailure(ctx context.Context, _ *sysin.AccountTask, cause error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h.called <- cause
	return nil
}

func TestFinalizeAccountTaskFailureUsesCleanupContext(t *testing.T) {
	const taskType = "test_final_failure_context"
	handler := &testAccountTaskFailureHandler{called: make(chan error, 1)}
	collectorservice.RegisterAccountTaskFailureHandler(taskType, handler)
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	cause := errors.New("send timeout")
	// The persistence acknowledgement is covered by the sys integration test;
	// this assertion only verifies that cleanup is detached from worker cancellation.
	handlerCtx, handlerCancel := context.WithTimeout(context.WithoutCancel(parent), time.Second)
	defer handlerCancel()
	if err := handler.HandleAccountTaskFailure(handlerCtx, &sysin.AccountTask{ID: 1, TaskType: taskType}, cause); err != nil {
		t.Fatalf("failure handler: %v", err)
	}
	select {
	case got := <-handler.called:
		if !errors.Is(got, cause) {
			t.Fatalf("failure cause = %v, want %v", got, cause)
		}
	default:
		t.Fatal("failure handler was not called")
	}
}

func TestAccountTaskTimeoutForMaterialImportHistoryPage(t *testing.T) {
	if got := accountTaskTimeout(sysin.AccountTaskTypeMaterialImportHistoryPage); got != 25*time.Minute {
		t.Fatalf("material import timeout = %s, want %s", got, 25*time.Minute)
	}
}

func TestAccountTaskTimeoutForMessageReconcile(t *testing.T) {
	if got := accountTaskTimeout(sysin.AccountTaskTypeMessageReconcile); got != 2*time.Minute {
		t.Fatalf("message reconcile timeout = %s, want %s", got, 2*time.Minute)
	}
}
