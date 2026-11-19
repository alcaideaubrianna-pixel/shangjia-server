package sys

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestTelegramChannelSameChatBarrierPostgres(t *testing.T) {
	port := os.Getenv("CHANNEL_ORDER_FIXTURE_PG_PORT")
	if port == "" {
		t.Skip("only run with standalone loopback fixture")
	}
	t.Setenv("YOUBAN_PUBLISH_CHANNEL_QUEUE_ENABLED", "true")
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: "channel_order_fixture", Name: "postgres"}})
	ctx := context.Background()
	db := g.DB()
	exec := func(sql string, args ...interface{}) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("CREATE TABLE " + publishTgJobTable + ` (id bigint PRIMARY KEY, tenant_id bigint, account_id bigint,
channel_id bigint, target_chat_id text, status text, send_phase text, timeout_cleanup_at timestamp)`)
	defer exec("DROP TABLE " + publishTgJobTable)
	exec("INSERT INTO " + publishTgJobTable + ` VALUES (3442309,78,615,231,'-1003763589169','unknown','display_confirmed',NULL),
(3442317,78,615,220,'-1003763589169','pending','',NULL)`)
	current := telegramJobRecord{Id: 3442316, TenantId: 78, AccountId: 615, ChannelId: 220, TargetChatId: "3763589169"}
	check := func(want bool) {
		t.Helper()
		blocked, err := new(sSysPublish).telegramChannelHasEarlierActiveJob(ctx, current)
		if err != nil || blocked != want {
			t.Fatalf("blocked=%v want=%v error=%v", blocked, want, err)
		}
	}
	check(true) // Real queue mode must no longer bypass a started alias.
	exec("UPDATE " + publishTgJobTable + " SET status='failed',send_phase='timeout_cancelled',timeout_cleanup_at=now()+interval '1 hour' WHERE id=3442309")
	check(true)
	exec("UPDATE " + publishTgJobTable + " SET status='superseded' WHERE id=3442309")
	check(true)
	exec("UPDATE " + publishTgJobTable + " SET timeout_cleanup_at=NULL WHERE id=3442309")
	check(false) // Untouched pending aliases never recreate a DB head gate.
	exec("UPDATE " + publishTgJobTable + " SET timeout_cleanup_at=now(),send_phase='' WHERE id=3442309")
	check(true) // Even a no-message timeout parent retains the cleanup fence.
	exec("UPDATE " + publishTgJobTable + " SET timeout_cleanup_at=NULL WHERE id=3442309")
	check(false)
	exec("UPDATE " + publishTgJobTable + " SET status='unknown',send_phase='' WHERE id=3442309")
	check(true)
	exec("UPDATE " + publishTgJobTable + " SET status='pending' WHERE id=3442309")
	check(false)
}

func TestTelegramChannelOrderedRetry(t *testing.T) {
	reliableChannelTestRedis(t, false)
	ctx := context.Background()
	const channel, oldJob, nextJob = int64(231), int64(3442309), int64(3442316)
	enqueue := func(id int64, priority int, at time.Time) {
		t.Helper()
		if err := enqueueTelegramChannelJob(ctx, channel, id, priority, at); err != nil {
			t.Fatal(err)
		}
	}
	pick := func(want int64) telegramChannelQueueClaim {
		t.Helper()
		claim, err := claimTelegramChannelJob(ctx, channel)
		if err != nil || claim.JobID != want {
			t.Fatalf("next claim=%+v error=%v want=%d", claim, err, want)
		}
		return claim
	}
	enqueue(oldJob, tgJobPriorityDefault, time.Now().Add(-time.Second))
	first := pick(oldJob)
	unknown := telegramJobRecord{Status: "unknown", SendPhase: telegramSendPhaseDisplayConfirmed}
	first.PreserveOrder = telegramChannelJobOwnsOrder(unknown)
	if !first.PreserveOrder {
		t.Fatal("unknown displayed job lost ordering")
	}
	displayed := telegramJobRecord{Status: telegramJobStatusRetryWait, SendPhase: telegramSendPhaseDisplayConfirmed, Priority: tgJobPriorityDefault, LastDispatchError: "媒体仍在异步处理中"}
	if decision := telegramChannelQueueDecisionFor(displayed); decision.YieldHead || decision.BlockReason == telegramChannelBlockNone {
		t.Fatal("stale media error yielded a displayed operation")
	}
	if priority := telegramChannelEffectivePriority(displayed); priority != tgJobPriorityDefault {
		t.Fatalf("displayed operation downgraded to history priority %d", priority)
	}
	enqueue(nextJob, tgJobPriorityInteractive, time.Now().Add(-time.Second))
	if ok, err := settleTelegramChannelClaim(ctx, first, true, tgJobPriorityDefault, time.Now().Add(time.Hour)); err != nil || !ok {
		t.Fatalf("ordered requeue %v %v", ok, err)
	}
	pick(0) // An urgent next job cannot overtake the future-due owner.
	enqueue(oldJob, tgJobPriorityDefault, time.Now().Add(-time.Second))
	retry := pick(oldJob)
	if ok, err := settleTelegramChannelClaim(ctx, first, false, 0, time.Time{}); err != nil || ok {
		t.Fatalf("stale token must not release owner: %v %v", ok, err)
	}
	retry.PreserveOrder = true
	if ok, err := settleTelegramChannelClaim(ctx, retry, true, tgJobPriorityDefault, time.Now().Add(-time.Second)); err != nil || !ok {
		t.Fatalf("retry settle %v %v", ok, err)
	}
	var wg sync.WaitGroup
	winners := make(chan telegramChannelQueueClaim, 32)
	failures := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := claimTelegramChannelJob(ctx, channel)
			if err != nil {
				failures <- err
			} else if c.JobID > 0 {
				winners <- c
			}
		}()
	}
	wg.Wait()
	close(winners)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	if len(winners) != 1 {
		t.Fatalf("concurrent claims=%d want=1", len(winners))
	}
	current := <-winners
	if current.JobID != oldJob {
		t.Fatalf("concurrent claim bypassed owner: %+v", current)
	}
	expired := current
	if _, err := telegramChannelRedisDo(ctx, "HSET", telegramChannelProcessingKey(channel), "deadline", 0); err != nil {
		t.Fatal(err)
	}
	if ok, err := recoverExpiredTelegramChannelClaim(ctx, channel); err != nil || !ok {
		t.Fatalf("ordered owner crash recovery %v %v", ok, err)
	}
	current = pick(oldJob)
	if ok, err := settleTelegramChannelClaim(ctx, expired, false, 0, time.Time{}); err != nil || ok {
		t.Fatalf("expired owner released ordering %v %v", ok, err)
	}
	cleanup := telegramJobRecord{Status: telegramJobStatusSuperseded, SendPhase: telegramSendPhaseTimeoutCancelled, TimeoutCleanupAt: gtime.Now()}
	if isTelegramChannelJobTerminal(cleanup) || !telegramChannelJobOwnsOrder(cleanup) {
		t.Fatal("terminal timeout parent released before cleanup")
	}
	current.PreserveOrder = telegramChannelJobOwnsOrder(cleanup)
	if ok, err := settleTelegramChannelClaim(ctx, current, true, tgJobPriorityHistory, time.Now().Add(time.Hour)); err != nil || !ok {
		t.Fatalf("cleanup requeue %v %v", ok, err)
	}
	pick(0)
	// Removing a queue notification must not strand or bypass its durable owner.
	if err := removeTelegramChannelJob(ctx, channel, oldJob); err != nil {
		t.Fatal(err)
	}
	if err := removeActiveTelegramChannel(ctx, channel); err != nil {
		t.Fatal(err)
	}
	current = pick(oldJob)
	cleanup.TimeoutCleanupAt = nil
	if !isTelegramChannelJobTerminal(cleanup) || telegramChannelJobOwnsOrder(cleanup) {
		t.Fatal("successful cleanup did not release terminal parent")
	}
	if ok, err := settleTelegramChannelClaim(ctx, current, false, 0, time.Time{}); err != nil || !ok {
		t.Fatalf("completed cleanup ack %v %v", ok, err)
	}
	next := pick(nextJob)
	if ok, err := settleTelegramChannelClaim(ctx, next, false, 0, time.Time{}); err != nil || !ok {
		t.Fatalf("next ack %v %v", ok, err)
	}
}

func TestTelegramChannelOrderOwnerWake(t *testing.T) {
	reliableChannelTestRedis(t, false)
	t.Setenv("YOUBAN_PUBLISH_CHANNEL_QUEUE_ENABLED", "true")
	ctx := context.Background()
	const channel, oldJob, nextJob = int64(231), int64(3442309), int64(3442316)
	enqueue := func(channelID, id int64, priority int) {
		t.Helper()
		if err := enqueueTelegramChannelJob(ctx, channelID, id, priority, time.Now().Add(-time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	pick := func(want int64) telegramChannelQueueClaim {
		t.Helper()
		claim, err := claimTelegramChannelJob(ctx, channel)
		if err != nil || claim.JobID != want {
			t.Fatalf("claim=%+v err=%v want=%d", claim, err, want)
		}
		return claim
	}
	enqueue(channel, oldJob, tgJobPriorityDefault)
	old := pick(oldJob)
	old.PreserveOrder = true
	if ok, err := settleTelegramChannelClaim(ctx, old, true, tgJobPriorityHistory, time.Now().Add(2*time.Minute)); err != nil || !ok {
		t.Fatalf("cleanup lease backoff %v %v", ok, err)
	}
	enqueue(channel, nextJob, tgJobPriorityInteractive)
	pick(0)
	if ok, err := wakeTelegramChannelOrderOwner(ctx, telegramJobRecord{Id: nextJob, ChannelId: channel}); err != nil || ok {
		t.Fatalf("non-owner wake accepted %v %v", ok, err)
	}
	pick(0)
	finishedCtx, cancel := context.WithCancel(ctx)
	cancel()
	if ok, err := wakeTelegramChannelOrderOwner(finishedCtx, telegramJobRecord{Id: oldJob, ChannelId: channel}); err != nil || !ok {
		t.Fatalf("completed cleanup owner wake %v %v", ok, err)
	}
	// Immediate claim: no sleep, clock advancement or re-enqueue of the owner.
	old = pick(oldJob)
	if !old.History {
		t.Fatal("wake moved owner out of its existing history lane")
	}
	// Cleanup may finish after claim but before a stale worker settles with the
	// old future cleanup lease. Its wake must survive that requeue atomically.
	if ok, err := wakeTelegramChannelOrderOwner(finishedCtx, telegramJobRecord{Id: oldJob, ChannelId: channel}); err != nil || !ok {
		t.Fatalf("inflight owner wake %v %v", ok, err)
	}
	if ok, err := settleTelegramChannelClaim(ctx, old, true, tgJobPriorityHistory, time.Now().Add(2*time.Minute)); err != nil || !ok {
		t.Fatalf("stale post-cleanup settle %v %v", ok, err)
	}
	old = pick(oldJob)
	if ok, err := settleTelegramChannelClaim(ctx, old, false, 0, time.Time{}); err != nil || !ok {
		t.Fatalf("cleaned owner ACK %v %v", ok, err)
	}
	next := pick(nextJob)
	if ok, err := settleTelegramChannelClaim(ctx, next, false, 0, time.Time{}); err != nil || !ok {
		t.Fatalf("next ACK %v %v", ok, err)
	}
	// A normal queued task with no owner must not become a new owner.
	enqueue(channel, nextJob, tgJobPriorityInteractive)
	if ok, err := wakeTelegramChannelOrderOwner(ctx, telegramJobRecord{Id: nextJob, ChannelId: channel}); err != nil || ok {
		t.Fatalf("wake created a new owner %v %v", ok, err)
	}
	owner, err := telegramChannelRedisDo(ctx, "GET", telegramChannelOrderOwnerKey(channel))
	if err != nil || !owner.IsNil() {
		t.Fatalf("unexpected new owner %v %v", owner, err)
	}
	// First cleanup can finish before the first ordered settlement creates an
	// owner key. Tag the existing claim, without manufacturing an owner/member.
	first := pick(nextJob)
	if ok, err := wakeTelegramChannelOrderOwner(finishedCtx, telegramJobRecord{Id: nextJob, ChannelId: channel}); err != nil || !ok {
		t.Fatalf("first inflight cleanup wake %v %v", ok, err)
	}
	owner, err = telegramChannelRedisDo(ctx, "GET", telegramChannelOrderOwnerKey(channel))
	if err != nil || !owner.IsNil() {
		t.Fatalf("inflight wake manufactured an owner %v %v", owner, err)
	}
	first.PreserveOrder = true
	if ok, err := settleTelegramChannelClaim(ctx, first, true, tgJobPriorityDefault, time.Now().Add(2*time.Minute)); err != nil || !ok {
		t.Fatalf("first post-cleanup settle %v %v", ok, err)
	}
	first = pick(nextJob)
	if ok, err := settleTelegramChannelClaim(ctx, first, false, 0, time.Time{}); err != nil || !ok {
		t.Fatalf("first cleaned owner ACK %v %v", ok, err)
	}
}
