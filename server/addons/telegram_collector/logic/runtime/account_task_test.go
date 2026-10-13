package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

type testAccountTaskFailureHandler struct {
	called chan error
	err    error
}

func (h *testAccountTaskFailureHandler) HandleAccountTaskFailure(ctx context.Context, _ *sysin.AccountTask, cause error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h.called <- cause
	return h.err
}

type testFinalizingAccountTasks struct {
	collectorservice.IAccountTasks
	finalized chan int64
}

func (s *testFinalizingAccountTasks) FinalizeFailure(_ context.Context, taskID int64) error {
	s.finalized <- taskID
	return nil
}

func TestFinalizeAccountTaskFailureUsesCleanupContext(t *testing.T) {
	const taskType = "test_final_failure_context"
	handler := &testAccountTaskFailureHandler{called: make(chan error, 1)}
	collectorservice.RegisterAccountTaskFailureHandler(taskType, handler)
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	cause := errors.New("send timeout")
	handlerCtx, handlerCancel := context.WithTimeout(context.WithoutCancel(parent), time.Second)
	defer handlerCancel()
	if err := handleAccountTaskFinalFailure(handlerCtx, &sysin.AccountTask{ID: 1, TaskType: taskType, ErrorMessage: cause.Error()}); err != nil {
		t.Fatalf("failure handler: %v", err)
	}
	select {
	case got := <-handler.called:
		if got == nil || got.Error() != cause.Error() {
			t.Fatalf("failure cause = %v, want %v", got, cause)
		}
	default:
		t.Fatal("failure handler was not called")
	}
}

func TestFinalizeAccountTaskFailureAcknowledgesOnlyAfterHandlerSuccess(t *testing.T) {
	tasks := &testFinalizingAccountTasks{finalized: make(chan int64, 2)}
	collectorservice.RegisterAccountTasks(tasks)
	defer collectorservice.RegisterAccountTasks(nil)

	const failedType = "test_final_failure_retry"
	failedHandler := &testAccountTaskFailureHandler{called: make(chan error, 1), err: errors.New("database unavailable")}
	collectorservice.RegisterAccountTaskFailureHandler(failedType, failedHandler)
	finalizeAccountTaskFailure(context.Background(), &sysin.AccountTask{ID: 11, TaskType: failedType, ErrorMessage: "send failed"})
	select {
	case id := <-tasks.finalized:
		t.Fatalf("failure callback error must keep task finalizing, finalized task %d", id)
	default:
	}

	const successType = "test_final_failure_success"
	successHandler := &testAccountTaskFailureHandler{called: make(chan error, 1)}
	collectorservice.RegisterAccountTaskFailureHandler(successType, successHandler)
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	finalizeAccountTaskFailure(parent, &sysin.AccountTask{ID: 12, TaskType: successType, ErrorMessage: "send failed"})
	select {
	case id := <-tasks.finalized:
		if id != 12 {
			t.Fatalf("finalized task = %d, want 12", id)
		}
	default:
		t.Fatal("successful failure callback must acknowledge finalization")
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
