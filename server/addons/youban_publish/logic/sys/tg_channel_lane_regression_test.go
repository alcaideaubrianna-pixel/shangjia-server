package sys

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
)

func TestTelegramChannelLaneRedisMigrationPreservesJob(t *testing.T) {
	server := miniredis.RunT(t)
	previousConfig := g.Cfg().GetAdapter()
	adapter, err := gcfg.NewAdapterContent(fmt.Sprintf(`{"redis":{"default":{"address":%q}}}`, server.Addr()))
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	t.Cleanup(func() { g.Cfg().SetAdapter(previousConfig) })
	client, err := gredis.New(&gredis.Config{Address: server.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	shared := g.Redis()
	previousRedis := shared.GetAdapter()
	shared.SetAdapter(client.GetAdapter())
	t.Cleanup(func() { shared.SetAdapter(previousRedis); _ = client.Close(context.Background()) })
	ctx := context.Background()
	job := telegramJobRecord{Id: 501, Priority: tgJobPriorityDefault, Status: telegramJobStatusRetryWait, LastDispatchError: "媒体仍在异步处理中"}
	const channelID int64 = 9990501
	due := time.Now().Add(-time.Minute)
	enqueue := func(id int64, priority int) {
		t.Helper()
		if err := enqueueTelegramChannelJob(ctx, channelID, id, priority, due); err != nil {
			t.Fatal(err)
		}
	}
	enqueue(job.Id, telegramChannelEffectivePriority(job, false))
	enqueue(502, tgJobPriorityBulk)
	claim, err := dequeueTelegramChannelHeadWithScore(ctx, channelID)
	if err != nil || claim.JobID != 502 {
		t.Fatalf("ready work did not pass media wait: %+v %v", claim, err)
	}
	claim, err = dequeueTelegramChannelHeadWithScore(ctx, channelID)
	if err != nil || claim.JobID != job.Id || !claim.History {
		t.Fatalf("media job lost: %+v %v", claim, err)
	}
	if telegramChannelQueueNeedsMigration(claim.Score, telegramChannelEffectivePriority(job, false), claim.Urgent, claim.History) {
		t.Fatal("media wait migration loop")
	}
	// Requeue while still waiting, then promote after the readiness gate says
	// ready despite the unchanged persisted media error. Atomic enqueue must
	// remove the old history member and retain exactly one executable job.
	enqueue(job.Id, telegramChannelEffectivePriority(job, false))
	enqueue(job.Id, telegramChannelEffectivePriority(job, true))
	claim, err = dequeueTelegramChannelHeadWithScore(ctx, channelID)
	if err != nil || claim.JobID != job.Id || claim.History || claim.Urgent {
		t.Fatalf("ready job did not recover normal lane: %+v %v", claim, err)
	}
	if telegramChannelQueueNeedsMigration(claim.Score, telegramChannelEffectivePriority(job, true), claim.Urgent, claim.History) {
		t.Fatal("ready migration loop")
	}
	claim, err = dequeueTelegramChannelHeadWithScore(ctx, channelID)
	if err != nil || claim.JobID != 0 {
		t.Fatalf("duplicate job after migration: %+v %v", claim, err)
	}
}

func TestTelegramChannelDelayedEnqueueClaimsOnceWhenDue(t *testing.T) {
	server := miniredis.RunT(t)
	client, err := gredis.New(&gredis.Config{Address: server.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	shared := g.Redis()
	previous := shared.GetAdapter()
	shared.SetAdapter(client.GetAdapter())
	t.Cleanup(func() { shared.SetAdapter(previous); _ = client.Close(context.Background()) })
	ctx := context.Background()
	for i, priority := range []int{tgJobPriorityInteractive, tgJobPriorityDefault, tgJobPriorityHistory} {
		channelID, jobID := int64(9990700+i), int64(i+1)*telegramChannelQueueSequenceScale
		now := time.Now()
		due := telegramChannelDispatchDueAt(nil, 200*time.Millisecond, now)
		if err := enqueueTelegramChannelJob(ctx, channelID, jobID, priority, due); err != nil {
			t.Fatal(err)
		}
		claim, err := dequeueTelegramChannelHeadWithScore(ctx, channelID)
		if err != nil || claim.JobID != 0 {
			t.Fatalf("lane %d claimed early: %+v %v", priority, claim, err)
		}
		// Claim uses the client's wall clock, so advancing miniredis TTL time
		// alone cannot make this job due.
		time.Sleep(time.Until(due.Add(10 * time.Millisecond)))
		claim, err = dequeueTelegramChannelHeadWithScore(ctx, channelID)
		if err != nil || claim.JobID != jobID {
			t.Fatalf("lane %d missing due job: %+v %v", priority, claim, err)
		}
		claim, err = dequeueTelegramChannelHeadWithScore(ctx, channelID)
		if err != nil || claim.JobID != 0 {
			t.Fatalf("lane %d claimed twice: %+v %v", priority, claim, err)
		}
	}
}

func TestTelegramChannelClaimDueAllLanes(t *testing.T) {
	server := miniredis.RunT(t)
	client, err := gredis.New(&gredis.Config{Address: server.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	shared := g.Redis()
	previousRedis := shared.GetAdapter()
	shared.SetAdapter(client.GetAdapter())
	t.Cleanup(func() {
		shared.SetAdapter(previousRedis)
		_ = client.Close(context.Background())
	})
	ctx := context.Background()
	const channelID int64 = 9990601
	keys := []string{telegramChannelUrgentQueueKey(channelID), telegramChannelQueueKey(channelID), telegramChannelHistoryQueueKey(channelID)}
	for _, tt := range []struct {
		name     string
		due      [3]bool
		wantLane int
	}{
		{"all_empty", [3]bool{}, -1},
		{"urgent_first", [3]bool{true, true, true}, 0},
		{"future_urgent", [3]bool{false, true, true}, 1},
		{"future_urgent_normal", [3]bool{false, false, true}, 2},
		{"all_future", [3]bool{}, -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := shared.Do(ctx, "DEL", keys[0], keys[1], keys[2])
			if err != nil {
				t.Fatal(err)
			}
			scores := [3]int64{}
			if tt.name != "all_empty" {
				for i, key := range keys {
					at := time.Now().Add(time.Hour)
					if tt.due[i] {
						at = time.Now().Add(-time.Minute)
					}
					scores[i] = int64(i)*telegramChannelQueuePriorityScale + at.UnixMilli()
					if _, err := shared.Do(ctx, "ZADD", key, scores[i], i+1); err != nil {
						t.Fatal(err)
					}
				}
			}
			claim, err := dequeueTelegramChannelHeadWithScore(ctx, channelID)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantLane < 0 {
				if claim != (telegramChannelQueueClaim{}) {
					t.Fatalf("unexpected claim: %+v", claim)
				}
			} else if claim.JobID != int64(tt.wantLane+1) || claim.Score != scores[tt.wantLane] || claim.Urgent != (tt.wantLane == 0) || claim.History != (tt.wantLane == 2) {
				t.Fatalf("wrong claim: %+v lane=%d scores=%v", claim, tt.wantLane, scores)
			}
			for i, key := range keys {
				value, err := shared.Do(ctx, "ZCARD", key)
				want := 1
				if tt.name == "all_empty" || i == tt.wantLane {
					want = 0
				}
				if err != nil || value.Int() != want {
					t.Fatalf("lane %d cardinality=%v err=%v want=%d", i, value, err, want)
				}
			}
		})
	}
	// The single-key entry point keeps its score and lane metadata contract.
	for i, key := range keys {
		score := int64(i)*telegramChannelQueuePriorityScale + time.Now().Add(-time.Minute).UnixMilli()
		if _, err := shared.Do(ctx, "DEL", key); err != nil {
			t.Fatal(err)
		}
		if _, err := shared.Do(ctx, "ZADD", key, score, i+10); err != nil {
			t.Fatal(err)
		}
		claim, err := dequeueTelegramChannelHeadFromKey(ctx, key)
		if err != nil || claim.JobID != int64(i+10) || claim.Score != score || claim.Urgent != (i == 0) || claim.History != (i == 2) {
			t.Fatalf("single key lane %d: %+v err=%v", i, claim, err)
		}
	}
}

func TestTelegramChannelLaneFailureDowngrade(t *testing.T) {
	for _, tt := range []struct{ priority, before, after, want int }{
		{tgJobPriorityInteractive, 2, 3, 6},
		{tgJobPriorityDefault, 2, 3, 55},
		{55, 3, 3, 55},
		{55, 3, 4, 55},
		{55, 5, 6, 60},
		{94, 8, 9, tgJobPriorityHistory},
		{tgJobPriorityHistory, 8, 9, tgJobPriorityHistory},
		{999, 8, 9, tgJobPriorityHistory},
		{-5, 2, 3, tgJobPriorityHistory},
	} {
		got := telegramChannelPriorityAfterFailure(tt.priority, tt.before, tt.after)
		if got != tt.want {
			t.Fatalf("priority=%d retry=%d->%d got=%d want=%d", tt.priority, tt.before, tt.after, got, tt.want)
		}
	}
	// A failed bulk task must sort behind ready normal/bulk work, not jump
	// ahead into the normal lane as subtracting five used to do.
	at := time.Now()
	failed := telegramChannelPriorityAfterFailure(tgJobPriorityBulk, 2, 3)
	if telegramChannelQueueScore(failed, at, 1) <= telegramChannelQueueScore(tgJobPriorityBulk, at, 2) {
		t.Fatal("failure promoted the queue head")
	}
}

func TestTelegramChannelLaneMediaWaitReadyLifecycle(t *testing.T) {
	job := telegramJobRecord{Id: 71, Priority: tgJobPriorityDefault, Status: telegramJobStatusRetryWait, LastDispatchError: "媒体仍在异步处理中"}
	at := time.Now()
	priority := telegramChannelEffectivePriority(job, false)
	if priority != tgJobPriorityHistory {
		t.Fatalf("waiting media lane=%d", priority)
	}
	score := telegramChannelQueueScore(priority, at, job.Id)
	for i := 0; i < 10; i++ {
		if telegramChannelQueueNeedsMigration(score, telegramChannelEffectivePriority(job, false), false, true) {
			t.Fatal("media wait bounces back to the DB priority without sending")
		}
	}
	// Readiness is authoritative even while the old DB error is still present.
	priority = telegramChannelEffectivePriority(job, true)
	if priority != tgJobPriorityDefault || !telegramChannelQueueNeedsMigration(score, priority, false, true) {
		t.Fatal("ready media did not recover its original lane")
	}
	score = telegramChannelQueueScore(priority, at, job.Id)
	if telegramChannelQueueNeedsMigration(score, telegramChannelEffectivePriority(job, true), false, false) {
		t.Fatal("ready job keeps migrating instead of executing")
	}
	if job.Priority != tgJobPriorityDefault || job.Id != 71 {
		t.Fatal("temporary yielding changed the durable job")
	}
}

func TestTelegramChannelLaneMediaCannotMaskBarrier(t *testing.T) {
	for _, tt := range []struct{ status, reason string }{
		{telegramJobStatusSending, "媒体仍在异步处理中"},
		{telegramJobStatusConfirming, "媒体仍在异步处理中"},
		{telegramJobStatusRetryWait, "媒体仍在异步处理中 timeout"},
		{telegramJobStatusRetryWait, "媒体仍在异步处理中 awaiting_account_fallback"},
		{telegramJobStatusRetryWait, "媒体仍在异步处理中 账号降级"},
	} {
		job := telegramJobRecord{Priority: tgJobPriorityDefault, Status: JobStatus(tt.status), LastDispatchError: tt.reason}
		if decision := telegramChannelQueueDecisionFor(job); decision.YieldHead {
			t.Fatalf("barrier yielded: %+v", job)
		}
		if telegramChannelEffectivePriority(job) != tgJobPriorityDefault {
			t.Fatalf("barrier moved to history: %+v", job)
		}
	}
}
