package sys

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestTelegramChannelHeadFailureModes documents the states that are allowed
// to re-enter the channel queue. These are deliberately table-driven so a
// future status change cannot silently turn a waiting head into a terminal
// job or a hot retry loop.
func TestTelegramChannelHeadFailureModes(t *testing.T) {
	tests := []struct {
		name       string
		job        telegramJobRecord
		wantReason string
		minDelay   time.Duration
		maxDelay   time.Duration
	}{
		{
			name:       "media waiting remains retryable",
			job:        telegramJobRecord{Status: telegramJobStatusRetryWait, LastDispatchError: "媒体仍在异步处理中：display"},
			wantReason: "媒体仍在异步处理中：display",
			minDelay:   0,
			maxDelay:   2 * time.Second,
		},
		{
			name:       "protocol fallback remains retryable",
			job:        telegramJobRecord{Status: telegramJobStatusRetryWait, LastDispatchError: telegramMediaFallbackReason("verify")},
			wantReason: telegramMediaFallbackReason("verify"),
			minDelay:   0,
			maxDelay:   2 * time.Second,
		},
		{
			name:       "canceled send is delayed for reconciliation",
			job:        telegramJobRecord{Status: telegramJobStatusSending, LastDispatchError: telegramPublishLogError(context.Canceled)},
			wantReason: "任务正在发送或等待对账",
			minDelay:   29 * time.Second,
			maxDelay:   31 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now()
			hold, reason := telegramChannelJobRequeueAt(tt.job)
			if tt.name == "canceled send is delayed for reconciliation" {
				if !strings.Contains(reason, "context canceled") {
					t.Fatalf("reason=%q, want cancellation evidence", reason)
				}
			} else if reason != tt.wantReason {
				t.Fatalf("reason=%q, want %q", reason, tt.wantReason)
			}
			if delay := time.Until(time.UnixMilli(hold)); delay < tt.minDelay || delay > tt.maxDelay {
				t.Fatalf("requeue delay=%s, want [%s,%s]", delay, tt.minDelay, tt.maxDelay)
			}
			if time.UnixMilli(hold).Before(before) {
				t.Fatal("waiting head must never be requeued in the past")
			}
			if got := telegramChannelJobExecutionState(tt.job.Status.String(), tt.job.SendPhase.String(), tt.job.DispatchStatus.String()); got != telegramChannelExecutionWaiting && tt.job.Status != telegramJobStatusSending {
				t.Fatalf("waiting job became blocking/terminal: %q", got)
			}
		})
	}
}

func TestTelegramChannelHeadContextCanceledDoesNotBecomeTerminal(t *testing.T) {
	job := telegramJobRecord{
		Status:            telegramJobStatusSending,
		DispatchStatus:    tgDispatchStatusProcessing,
		LastDispatchError: telegramPublishLogError(errors.Join(errors.New("Telegram API"), context.Canceled)),
	}

	if !strings.Contains(job.LastDispatchError, "context canceled") {
		t.Fatalf("cancellation reason was lost: %q", job.LastDispatchError)
	}
	if got := telegramChannelJobExecutionState(job.Status.String(), "", job.DispatchStatus.String()); got != telegramChannelExecutionBlocking {
		t.Fatalf("canceled in-flight job state=%q, want blocking until recovery", got)
	}
	if got := telegramChannelJobExecutionState("failed", "", tgDispatchStatusDone); got != telegramChannelExecutionTerminal {
		t.Fatalf("only explicit failed terminal state should release head, got %q", got)
	}
	if hold, _ := telegramChannelJobRequeueAt(job); time.Until(time.UnixMilli(hold)) < 29*time.Second {
		t.Fatal("canceled in-flight job must be delayed for reconciliation")
	}
}

func TestTelegramChannelHeadLeaseExpiryRemainsRecoverable(t *testing.T) {
	// A processing claim is still a channel blocker while its lease is live.
	// Once the sender reports the lease as expired, recovery may move it back
	// to a waiting state; neither state is terminal or superseded.
	if got := telegramChannelJobExecutionState(telegramJobStatusPending, "", tgDispatchStatusProcessing); got != telegramChannelExecutionBlocking {
		t.Fatalf("live processing lease state=%q, want blocking", got)
	}
	for _, status := range []string{telegramJobStatusPending, telegramJobStatusSending, telegramJobStatusRetryWait, telegramJobStatusConfirming} {
		if !telegramAttemptRecoverableJobStatus(status) {
			t.Fatalf("lease-expired %s job is not recoverable", status)
		}
		if got := telegramChannelJobExecutionState(status, "", tgDispatchStatusIdle); got == telegramChannelExecutionTerminal {
			t.Fatalf("lease-expired %s job became terminal", status)
		}
	}
	for _, status := range []string{telegramJobStatusSent, telegramJobStatusFailed, telegramJobStatusSuperseded} {
		if telegramAttemptRecoverableJobStatus(status) {
			t.Fatalf("terminal %s job became recoverable", status)
		}
	}
}
