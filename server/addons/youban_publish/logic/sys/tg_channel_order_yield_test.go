package sys

import (
	"context"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestTelegramChannelOrderWaitYieldsOnlyUntouchedJob(t *testing.T) {
	reason := "等待同频道前序推送任务完成"
	for _, status := range []JobStatus{telegramJobStatusPending, telegramJobStatusRetryWait} {
		job := telegramJobRecord{Id: 3472880, Priority: tgJobPriorityInteractive, Status: status, LastDispatchError: reason}
		decision := telegramChannelQueueDecisionFor(job)
		if !decision.YieldHead || decision.BlockReason != telegramChannelBlockOrderWait {
			t.Fatalf("untouched order waiter retained head: %+v", decision)
		}
		if telegramChannelEffectivePriority(job, true) != tgJobPriorityHistory {
			t.Fatal("media readiness bypassed unrelated cleanup wait")
		}
		hold, gotReason := telegramChannelJobRequeueAt(job)
		if d := time.Until(time.UnixMilli(hold)); d < 29*time.Second || d > 31*time.Second || gotReason != reason {
			t.Fatalf("order wait must back off: %s %q", d, gotReason)
		}
		if job.Priority != tgJobPriorityInteractive {
			t.Fatal("durable priority changed")
		}
	}
	for _, job := range []telegramJobRecord{
		{Status: telegramJobStatusSending},
		{Status: "unknown"},
		{Status: telegramJobStatusConfirming},
		{Status: telegramJobStatusPending, SendPhase: telegramSendPhaseDisplayConfirmed},
		{Status: telegramJobStatusRetryWait, SendPhase: telegramSendPhaseVerifySending},
		{Status: telegramJobStatusPending, TimeoutCleanupAt: gtime.Now()},
	} {
		job.LastDispatchError = reason
		job.Priority = tgJobPriorityInteractive
		if telegramChannelQueueDecisionFor(job).YieldHead {
			t.Fatalf("started/unknown job yielded: %+v", job)
		}
		if telegramChannelEffectivePriority(job, true) != tgJobPriorityInteractive {
			t.Fatalf("started/unknown job downgraded: %+v", job)
		}
	}
}

func TestTelegramChannelOrderYieldProducerReplayAndClaim(t *testing.T) {
	reliableChannelTestRedis(t, false)
	ctx := context.Background()
	const channelID int64 = 9999109
	job := telegramJobRecord{Id: 3472880, Priority: tgJobPriorityInteractive, Status: telegramJobStatusPending, LastDispatchError: "等待同频道前序推送任务完成"}
	now := time.Now()
	if err := enqueueTelegramChannelJob(ctx, channelID, job.Id, job.Priority, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	claim, err := claimTelegramChannelJob(ctx, channelID)
	if err != nil || claim.JobID != job.Id {
		t.Fatalf("initial claim: %+v %v", claim, err)
	}
	// Worker yields while preserving the exact token claim and the durable Job.
	if ok, err := settleTelegramChannelClaim(ctx, claim, true, telegramChannelEffectivePriority(job, true), now.Add(30*time.Second)); err != nil || !ok {
		t.Fatalf("yield settle=%v %v", ok, err)
	}
	// Repeated Outbox enqueue must not promote the waiter back into urgent.
	for i := 0; i < 3; i++ {
		priority, due := telegramChannelEnqueuePlacement(job, job.Priority, now.Add(time.Second), now)
		if priority != tgJobPriorityHistory || due.Before(now.Add(30*time.Second)) {
			t.Fatalf("producer lost backoff: %d %s", priority, due)
		}
		if err := enqueueTelegramChannelJob(ctx, channelID, job.Id, priority, due); err != nil {
			t.Fatal(err)
		}
	}
	// A different ready profile now progresses, without removing the old barrier.
	if err := enqueueTelegramChannelJob(ctx, channelID, 3465089, tgJobPriorityDefault, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	next, err := claimTelegramChannelJob(ctx, channelID)
	if err != nil || next.JobID != 3465089 {
		t.Fatalf("ready work starved by blocked urgent: %+v %v", next, err)
	}
	if ok, err := settleTelegramChannelClaim(ctx, next, false, tgJobPriorityDefault, now); err != nil || !ok {
		t.Fatalf("ready ACK=%v %v", ok, err)
	}
	if stale, err := settleTelegramChannelClaim(ctx, claim, false, tgJobPriorityInteractive, now); err != nil || stale {
		t.Fatalf("stale claim changed queue: %v %v", stale, err)
	}
	if early, err := claimTelegramChannelJob(ctx, channelID); err != nil || early.JobID != 0 {
		t.Fatalf("waiter became immediately executable: %+v %v", early, err)
	}
	// On its next due retry it is still the same Job, then ordinary send gates
	// decide if the predecessor has been reconciled; it is never auto-sent here.
	if err := enqueueTelegramChannelJob(ctx, channelID, job.Id, telegramChannelEffectivePriority(job), now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	retry, err := claimTelegramChannelJob(ctx, channelID)
	if err != nil || retry.JobID != job.Id || !retry.History {
		t.Fatalf("yielded Job lost: %+v %v", retry, err)
	}
	if job.Priority != tgJobPriorityInteractive {
		t.Fatal("producer rewrote durable priority")
	}
}

func TestTelegramChannelOrderYieldSurvivesExpiredOutboxDeadline(t *testing.T) {
	job := telegramJobRecord{Status: telegramJobStatusPending, Priority: tgJobPriorityInteractive, NextRetryAt: gtime.Now().Add(-time.Minute), LastDispatchError: "等待同频道前序推送任务完成"}
	if reason := telegramChannelEnqueueDispatchReason(job, 0); reason != job.LastDispatchError {
		t.Fatalf("zero-delay recovery erased order wait: %q", reason)
	}
	hold, _ := telegramChannelJobRequeueAt(job)
	if time.Until(time.UnixMilli(hold)) < 29*time.Second {
		t.Fatal("expired persisted retry defeated bounded backoff")
	}
	// Simulate the persisted producer reason being read by the worker again.
	job.LastDispatchError = telegramChannelEnqueueDispatchReason(job, 0)
	if priority := telegramChannelEffectivePriority(job, true); priority != tgJobPriorityHistory {
		t.Fatalf("reload promoted blocked urgent: %d", priority)
	}
	other := telegramJobRecord{Status: telegramJobStatusPending, LastDispatchError: "temporary old error"}
	if telegramChannelEnqueueDispatchReason(other, 0) != "" || telegramChannelEnqueueDispatchReason(other, time.Second) != other.LastDispatchError {
		t.Fatal("unrelated enqueue reason behavior changed")
	}
}
