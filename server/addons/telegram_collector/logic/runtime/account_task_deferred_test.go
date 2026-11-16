package runtime

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"
)

type testAccountAdmissionDeferred struct{}

func (testAccountAdmissionDeferred) Error() string                        { return "channel lease busy" }
func (testAccountAdmissionDeferred) AccountTaskNotStarted() bool          { return true }
func (testAccountAdmissionDeferred) AccountTaskRetryDelay() time.Duration { return 17 * time.Second }

func TestAccountTaskChannelAdmissionDoesNotExhaustSendAttempts(t *testing.T) {
	task := &sysin.AccountTask{AttemptCount: 5, MaxAttempts: 5}
	deferred := fmt.Errorf("handler admission: %w", testAccountAdmissionDeferred{})
	notStarted := accountTaskFailureNotStarted(nil, deferred)
	if !notStarted || accountTaskFailureShouldFinalize(task, notStarted) {
		t.Fatal("fifth channel-slot wait must return its claim attempt and must not finalize the job")
	}
	if retryDelay(deferred) != 17*time.Second {
		t.Fatal("wrapped admission error lost its bounded persistent retry delay")
	}
}

func TestAccountTaskSendUnknownRetainsFailureAccounting(t *testing.T) {
	task := &sysin.AccountTask{AttemptCount: 5, MaxAttempts: 5}
	for _, err := range []error{context.DeadlineExceeded, context.Canceled, errors.New("unknown remote delivery")} {
		notStarted := accountTaskFailureNotStarted(nil, err)
		if notStarted || !accountTaskFailureShouldFinalize(task, notStarted) {
			t.Fatalf("send error must not be reclassified as admission: %v", err)
		}
	}
	if !accountTaskFailureNotStarted(context.DeadlineExceeded, nil) {
		t.Fatal("runtime gate failure must preserve the existing not-started contract")
	}
}
