package sys

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestTelegramJobOutboxRecordFromRecordAllowsMissingLegacyOutbox(t *testing.T) {
	if row := telegramJobOutboxRecordFromRecord(nil); row.Id != 0 {
		t.Fatalf("missing legacy outbox must remain empty, got id=%d", row.Id)
	}
	record := gdb.Record{
		"id":     gvar.New(int64(12)),
		"job_id": gvar.New(int64(34)),
	}
	row := telegramJobOutboxRecordFromRecord(record)
	if row.Id != 12 || row.JobId != 34 {
		t.Fatalf("unexpected outbox record: %+v", row)
	}
}

func TestTelegramDueOutboxPostgresSQLSelectsChannelHeads(t *testing.T) {
	query := telegramDueOutboxPostgresSQL()
	if strings.Count(query, "(") != strings.Count(query, ")") {
		t.Fatalf("channel-head query has unbalanced parentheses: %s", query)
	}
	for _, fragment := range []string{
		"WITH active_jobs AS MATERIALIZED", "channel_job_heads AS", "DISTINCT ON", "j.tenant_id", "j.channel_id", "target_chat_id",
		"j.status IN ('pending','failed_retry','confirming','sending')",
		"FROM active_jobs j", "available_bot.id=h.bot_id", "c.id=h.channel_id",
		"ORDER BY j.tenant_id", "j.created_at, j.id",
		"h.status IN ('pending','failed_retry','unknown')",
		"COALESCE(h.dispatch_status,'') IN ('','idle')",
		"h.next_retry_at IS NULL OR h.next_retry_at<=?",
		"o.status=? OR (o.status=? AND o.lease_expires_at<=?)",
		"ORDER BY o.available_at,o.id",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("channel-head query missing %q: %s", fragment, query)
		}
	}
	if got := strings.Count(query, "?"); got != 7 {
		t.Fatalf("channel-head query placeholder count=%d, want 7: %s", got, query)
	}
}

func TestTelegramOutboxTimeoutsAreBounded(t *testing.T) {
	if telegramOutboxQueryTimeout <= 0 || telegramOutboxQueryTimeout > 5*time.Second {
		t.Fatalf("unexpected outbox query timeout: %s", telegramOutboxQueryTimeout)
	}
	if telegramOutboxLeaseTTL <= telegramOutboxQueryTimeout || telegramOutboxLeaseTTL > time.Minute {
		t.Fatalf("unexpected outbox lease TTL: %s", telegramOutboxLeaseTTL)
	}
}

func TestTelegramOutboxRetryDelayIsBounded(t *testing.T) {
	if got := telegramOutboxRetryDelay(0); got != 3*time.Second {
		t.Fatalf("initial retry delay = %s", got)
	}
	if got := telegramOutboxRetryDelay(100); got != time.Minute {
		t.Fatalf("maximum retry delay = %s", got)
	}
}

func TestFairTelegramOutboxRowsSelectsOnePerChannel(t *testing.T) {
	candidates := []telegramJobOutboxRecord{
		{Id: 1, TenantId: 1, ChannelId: 10},
		{Id: 2, TenantId: 1, ChannelId: 10},
		{Id: 3, TenantId: 1, ChannelId: 20},
	}
	rows := fairTelegramOutboxRows(candidates, 10)
	if len(rows) != 2 || rows[0].Id != 1 || rows[1].Id != 3 {
		t.Fatalf("unexpected fair outbox rows: %+v", rows)
	}
}

func TestTelegramOutboxJobStillNeedsDispatch(t *testing.T) {
	tests := []struct {
		name           string
		status         string
		dispatchStatus string
		want           bool
	}{
		{name: "pending idle", status: "pending", dispatchStatus: tgDispatchStatusIdle, want: true},
		{name: "retry empty", status: "failed_retry", dispatchStatus: "", want: true},
		{name: "unknown idle", status: "unknown", dispatchStatus: tgDispatchStatusIdle, want: true},
		{name: "queued", status: "pending", dispatchStatus: tgDispatchStatusQueued, want: false},
		{name: "processing", status: "pending", dispatchStatus: tgDispatchStatusProcessing, want: false},
		{name: "sent", status: "sent", dispatchStatus: tgDispatchStatusDone, want: false},
		{name: "failed", status: "failed", dispatchStatus: tgDispatchStatusDone, want: false},
		{name: "superseded", status: "superseded", dispatchStatus: tgDispatchStatusDone, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := telegramOutboxJobStillNeedsDispatch(test.status, test.dispatchStatus); got != test.want {
				t.Fatalf("telegramOutboxJobStillNeedsDispatch(%q, %q)=%t, want %t", test.status, test.dispatchStatus, got, test.want)
			}
		})
	}
}

func TestTelegramOutboxDispatchDecisionRequiresDueIdleHead(t *testing.T) {
	now := gtime.New(time.Now())
	past := gtime.New(now.Add(-time.Second))
	future := gtime.New(now.Add(time.Second))
	tests := []struct {
		name           string
		status         string
		dispatchStatus string
		nextRetryAt    *gtime.Time
		hasEarlier     bool
		want           string
	}{
		{name: "pending channel head", status: telegramJobStatusPending, dispatchStatus: tgDispatchStatusIdle, want: telegramOutboxDecisionDispatch},
		{name: "retry channel head due", status: telegramJobStatusRetryWait, dispatchStatus: "", nextRetryAt: past, want: telegramOutboxDecisionDispatch},
		{name: "unknown channel head", status: telegramJobStatusConfirming, dispatchStatus: tgDispatchStatusIdle, want: telegramOutboxDecisionDispatch},
		{name: "later outbox stays pending", status: telegramJobStatusPending, dispatchStatus: tgDispatchStatusIdle, hasEarlier: true, want: telegramOutboxDecisionWait},
		{name: "retry not due", status: telegramJobStatusRetryWait, dispatchStatus: tgDispatchStatusIdle, nextRetryAt: future, want: telegramOutboxDecisionWait},
		{name: "already queued completes outbox", status: telegramJobStatusPending, dispatchStatus: tgDispatchStatusQueued, want: telegramOutboxDecisionComplete},
		{name: "already processing completes outbox", status: telegramJobStatusPending, dispatchStatus: tgDispatchStatusProcessing, want: telegramOutboxDecisionComplete},
		{name: "terminal sent completes outbox", status: telegramJobStatusSent, dispatchStatus: tgDispatchStatusDone, want: telegramOutboxDecisionComplete},
		{name: "terminal failed completes outbox", status: telegramJobStatusFailed, dispatchStatus: tgDispatchStatusDone, want: telegramOutboxDecisionComplete},
		{name: "superseded completes outbox", status: telegramJobStatusSuperseded, dispatchStatus: tgDispatchStatusDone, want: telegramOutboxDecisionComplete},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := telegramOutboxDispatchDecision(test.status, test.dispatchStatus, test.nextRetryAt, now, test.hasEarlier)
			if got != test.want {
				t.Fatalf("telegramOutboxDispatchDecision()=%q, want %q", got, test.want)
			}
		})
	}
}

func TestTelegramOutboxDispatchDecisionPostgresWallClock(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	now := gtime.NewFromTime(time.Date(2026, 10, 16, 15, 20, 0, 0, location))
	for _, test := range []struct {
		name string
		hour int
		want string
	}{
		{"overdue_database_timestamp", 14, telegramOutboxDecisionDispatch},
		{"future_database_timestamp", 16, telegramOutboxDecisionWait},
	} {
		t.Run(test.name, func(t *testing.T) {
			// PostgreSQL timestamp without time zone carries UTC from lib/pq,
			// but its stored calendar fields represent application local time.
			retryAt := gtime.NewFromTime(time.Date(2026, 10, 16, test.hour, 54, 54, 409178000, time.UTC))
			got := telegramOutboxDispatchDecision("pending", "idle", retryAt, now, false)
			if got != test.want {
				t.Fatalf("decision=%s want=%s retry=%s now=%s", got, test.want, retryAt, now)
			}
		})
	}
}

func TestTelegramDueOutboxChannelHeadAndLeaseIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_PUBLISH_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_PUBLISH_INTEGRATION=1 to run local PostgreSQL integration test")
	}
	ctx := context.Background()
	seed := time.Now().UnixNano()
	now := gtime.Now()
	old := now.Add(-24 * time.Hour)

	insertJob := func(channelOffset int64, status, dispatchStatus, sendPhase string, createdAt *gtime.Time) int64 {
		t.Helper()
		id, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
			"operation_no": fmt.Sprintf("outbox-integration-%d-%d", seed, channelOffset),
			"tenant_id":    seed, "channel_id": seed + channelOffset,
			"target_chat_id": fmt.Sprintf("-100%d", seed+channelOffset),
			"status":         status, "dispatch_status": dispatchStatus, "send_phase": sendPhase,
			"created_at": createdAt, "updated_at": now,
		}).InsertAndGetId()
		if err != nil {
			t.Fatalf("insert job: %v", err)
		}
		return id
	}
	insertOutbox := func(jobId int64, status string, leaseExpiresAt *gtime.Time, attemptCount int) int64 {
		t.Helper()
		id, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).Data(g.Map{
			"job_id": jobId, "event_type": telegramOutboxEventPublish,
			"dedupe_key": fmt.Sprintf("outbox-integration-%d-%d", seed, jobId),
			"status":     status, "attempt_count": attemptCount,
			"available_at": old, "lease_expires_at": leaseExpiresAt,
			"created_at": now, "updated_at": now,
		}).InsertAndGetId()
		if err != nil {
			t.Fatalf("insert outbox: %v", err)
		}
		return id
	}

	queuedHead := insertJob(1, telegramJobStatusPending, tgDispatchStatusQueued, "", old)
	laterSameChannel := insertJob(1, telegramJobStatusPending, tgDispatchStatusIdle, "", now)
	expiredLeaseHead := insertJob(2, telegramJobStatusPending, tgDispatchStatusIdle, "", old)
	activeLeaseHead := insertJob(3, telegramJobStatusPending, tgDispatchStatusIdle, "", old)
	failedDisplayHead := insertJob(4, telegramJobStatusFailed, tgDispatchStatusDone, telegramSendPhaseDisplayConfirmed, old)
	laterAfterFailedDisplay := insertJob(4, telegramJobStatusPending, tgDispatchStatusIdle, "", now)
	unknownHead := insertJob(5, telegramJobStatusConfirming, tgDispatchStatusIdle, "", old)
	invalidFailedDisplayHead := insertJob(6, telegramJobStatusFailed, tgDispatchStatusDone, telegramSendPhaseDisplayConfirmed, old)
	laterAfterInvalidFailedDisplay := insertJob(6, telegramJobStatusPending, tgDispatchStatusIdle, "", now)
	deliveredIdleHead := insertJob(7, telegramJobStatusPending, tgDispatchStatusIdle, "", old)
	if _, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).Data(g.Map{
		"job_id": failedDisplayHead, "tenant_id": seed, "target_chat_id": fmt.Sprintf("-100%d", seed+4),
		"tg_message_id": seed, "purpose": "display", "status": "sent", "created_at": now, "updated_at": now,
	}).Insert(); err != nil {
		t.Fatalf("insert confirmed display evidence: %v", err)
	}

	jobIDs := []int64{queuedHead, laterSameChannel, expiredLeaseHead, activeLeaseHead, failedDisplayHead, laterAfterFailedDisplay, unknownHead, invalidFailedDisplayHead, laterAfterInvalidFailedDisplay, deliveredIdleHead}
	outboxIDs := []int64{
		insertOutbox(queuedHead, telegramOutboxPending, nil, 0),
		insertOutbox(laterSameChannel, telegramOutboxPending, nil, 7),
		insertOutbox(expiredLeaseHead, telegramOutboxProcessing, now.Add(-time.Second), 1),
		insertOutbox(activeLeaseHead, telegramOutboxProcessing, now.Add(time.Minute), 2),
		insertOutbox(failedDisplayHead, telegramOutboxDelivered, nil, 0),
		insertOutbox(laterAfterFailedDisplay, telegramOutboxPending, nil, 3),
		insertOutbox(unknownHead, telegramOutboxPending, nil, 0),
		insertOutbox(invalidFailedDisplayHead, telegramOutboxDelivered, nil, 0),
		insertOutbox(laterAfterInvalidFailedDisplay, telegramOutboxPending, nil, 0),
		insertOutbox(deliveredIdleHead, telegramOutboxDelivered, nil, 0),
	}
	t.Cleanup(func() {
		_, _ = g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).WhereIn("id", outboxIDs).Delete()
		_, _ = g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).Unscoped().Where("job_id", failedDisplayHead).Delete()
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Unscoped().WhereIn("id", jobIDs).Delete()
	})

	rows, err := new(sSysPublish).dueTelegramJobOutboxRows(ctx, 100, now)
	if err != nil {
		t.Fatalf("read due outbox rows: %v", err)
	}
	selected := make(map[int64]bool, len(rows))
	for _, row := range rows {
		selected[row.JobId] = true
	}
	if !selected[expiredLeaseHead] || !selected[unknownHead] || !selected[laterAfterInvalidFailedDisplay] {
		t.Fatalf("expired lease, unknown and successor of invalid failed display must be selected: %+v", selected)
	}
	for _, blocked := range []int64{queuedHead, laterSameChannel, activeLeaseHead, failedDisplayHead, laterAfterFailedDisplay} {
		if selected[blocked] {
			t.Fatalf("blocked job %d must not be selected: %+v", blocked, selected)
		}
	}

	decision, err := new(sSysPublish).telegramOutboxJobDispatchDecision(ctx, laterSameChannel, now)
	if err != nil || decision != telegramOutboxDecisionWait {
		t.Fatalf("later channel job decision=%q err=%v", decision, err)
	}
	attemptCount, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("job_id", laterSameChannel).Value("attempt_count")
	if err != nil || attemptCount.Int() != 7 {
		t.Fatalf("waiting outbox attempt_count=%d err=%v, want 7", attemptCount.Int(), err)
	}

	if err = new(sSysPublish).rearmTelegramJobOutbox(ctx, deliveredIdleHead, 0); err != nil {
		t.Fatalf("rearm delivered idle Outbox: %v", err)
	}
	status, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("job_id", deliveredIdleHead).Value("status")
	if err != nil || status.String() != telegramOutboxPending {
		t.Fatalf("rearmed Outbox status=%q err=%v, want %q", status.String(), err, telegramOutboxPending)
	}
}
