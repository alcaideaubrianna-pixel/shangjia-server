package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

func TestAccountTaskOperationWaitHonorsCancellation(t *testing.T) {
	var gate sync.Mutex
	gate.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- lockAccountTaskOperation(ctx, &gate) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("gate wait: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled task is still blocked on another send")
	}
	gate.Unlock()
	if !gate.TryLock() {
		t.Fatal("cancelled waiter acquired or leaked the gate")
	}
	gate.Unlock()
}

func TestAccountTaskOperationGateRejectsExpiredTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var gate sync.Mutex
	for _, candidate := range []*sync.Mutex{nil, &gate} {
		if err := lockAccountTaskOperation(ctx, candidate); !errors.Is(err, context.Canceled) {
			t.Fatalf("expired task admitted: %v", err)
		}
	}
	if err := lockAccountTaskOperation(context.Background(), &gate); err != nil {
		t.Fatal(err)
	}
	gate.Unlock()
}

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

type testTimeoutAccountTasks struct {
	collectorservice.IAccountTasks
	failed chan *sysin.AccountTaskFailure
}

func (s *testTimeoutAccountTasks) Fail(_ context.Context, in *sysin.AccountTaskFailure) error {
	s.failed <- in
	return nil
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

func TestAccountTaskMediaFallbackDoesNotBlockControlOperations(t *testing.T) {
	var operationGate sync.Mutex
	var mediaFallbackGate sync.Mutex
	if got := accountTaskOperationGate(sysin.AccountTaskTypeMessageMediaFallback, &operationGate, &mediaFallbackGate); got != &mediaFallbackGate {
		t.Fatal("media fallback tasks must be serialized on their dedicated gate")
	}
	if got := accountTaskOperationGate(sysin.AccountTaskTypeMediaDownload, &operationGate, &mediaFallbackGate); got != nil {
		t.Fatal("media download must not hold the account control-operation gate")
	}
	if got := accountTaskOperationGate(sysin.AccountTaskTypeMessageReconcile, &operationGate, &mediaFallbackGate); got != &operationGate {
		t.Fatal("reconciliation must retain the account control-operation gate")
	}
}

func TestAccountTaskTimeoutResetsOnlyMediaFallbackSession(t *testing.T) {
	for _, taskType := range []string{
		sysin.AccountTaskTypeMessagePushInline,
		sysin.AccountTaskTypeMessageReconcile,
		sysin.AccountTaskTypeMessageMediaFallback,
		sysin.AccountTaskTypeMessageDeleteFallback,
	} {
		if !accountTaskRequiresSessionResetOnTimeout(&sysin.AccountTask{TaskType: taskType}) {
			t.Fatalf("message task %s timeout must reset its stuck account session", taskType)
		}
	}
	for _, taskType := range []string{
		sysin.AccountTaskTypeMediaDownload,
		sysin.AccountTaskTypeHistoryPage,
		sysin.AccountTaskTypeDialogCacheRefresh,
	} {
		if accountTaskRequiresSessionResetOnTimeout(&sysin.AccountTask{TaskType: taskType}) {
			t.Fatalf("task %s must retain its existing timeout behavior", taskType)
		}
	}
}

func TestMediaFallbackWatchdogPersistsFailureAndResetsSession(t *testing.T) {
	tasks := &testTimeoutAccountTasks{failed: make(chan *sysin.AccountTaskFailure, 1)}
	collectorservice.RegisterAccountTasks(tasks)
	defer collectorservice.RegisterAccountTasks(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	reset := make(chan struct{}, 1)
	stop := watchAccountTaskTimeout(ctx,
		&sysin.AccountTask{ID: 91, AccountID: 67, TaskType: sysin.AccountTaskTypeMessageMediaFallback},
		&sysin.AccountLease{AccountID: 67, InstanceID: "test", Epoch: 3},
		func() { reset <- struct{}{} },
	)
	defer stop()

	select {
	case failure := <-tasks.failed:
		if failure.TaskID != 91 || failure.NotStarted {
			t.Fatalf("unexpected failure payload: %+v", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("watchdog did not persist the timed out media fallback")
	}
	select {
	case <-reset:
	case <-time.After(time.Second):
		t.Fatal("watchdog did not reset the stuck account session")
	}
}
