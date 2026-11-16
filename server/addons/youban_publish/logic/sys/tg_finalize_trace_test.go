package sys

import (
	"context"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

func TestTelegramFinalizeTraceStaleJobPreservesDurableDeadline(t *testing.T) {
	deadline := time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339Nano)
	ctx := telegramPublishTraceContext(context.Background(), 42, g.Map{
		"jobDeadline": deadline, "jobRemainingMs": int64(123),
	})
	ctx = telegramPublishJobTraceContext(ctx, telegramJobRecord{Id: 42})
	fields := ctx.Value(telegramPublishTraceContextKey{}).(g.Map)
	if fields["jobDeadline"] != deadline || fields["jobRemainingMs"] != int64(123) {
		t.Fatal("stale job erased durable deadline metadata")
	}
}

func TestTelegramFinalizeTraceStageUsesActualDeadline(t *testing.T) {
	deadline := time.Now().Add(10 * time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	fields := g.Map{
		"deadlineSource": "verify",
		"jobDeadline":    time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339Nano),
		"jobRemainingMs": int64(0),
	}
	telegramPublishStageDeadlineFields(ctx, fields)
	if fields["effectiveDeadline"] != deadline.UTC().Format(time.RFC3339Nano) {
		t.Fatal("stage did not record actual context deadline")
	}
	if fields["deadlineSource"] != "verify" || fields["jobRemainingMs"].(int64) <= fields["stageRemainingMs"].(int64) {
		t.Fatal("stage budget was confused with job budget")
	}
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	telegramPublishStageDeadlineFields(expired, fields)
	if fields["stageRemainingMs"].(int64) >= 0 {
		t.Fatal("remaining budget was not recalculated at emission")
	}
}

func TestTelegramFinalizeTraceJobDeadlineWinsStageBudget(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	for _, phase := range []string{"display", "verify"} {
		fields := g.Map{
			"deadlineSource": phase, "phase": phase, "purpose": phase,
			"jobDeadline": deadline.UTC().Format(time.RFC3339Nano),
		}
		telegramPublishStageDeadlineFields(ctx, fields)
		if fields["deadlineSource"] != "job" || fields["phase"] != phase || fields["purpose"] != phase {
			t.Fatal("job-limited API stage misidentified its deadline source")
		}
	}
}

func TestTelegramFinalizeTraceDetachedDeadlineIsNotExpiredJob(t *testing.T) {
	ctx, cancel := telegramFinalizeContextWithTimeout(context.Background(), "test", time.Second)
	defer cancel()
	fields := g.Map{"jobDeadline": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)}
	telegramPublishStageDeadlineFields(ctx, fields)
	if fields["deadlineSource"] != "finalize" {
		t.Fatal("detached finalization incorrectly inherited expired job source")
	}
}
