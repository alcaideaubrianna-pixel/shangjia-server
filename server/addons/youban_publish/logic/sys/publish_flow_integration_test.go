package sys

import (
	"context"
	"fmt"
	"os"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestLocalFullAndCyclePublishFlow(t *testing.T) {
	if os.Getenv("YOUBAN_PUBLISH_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_PUBLISH_INTEGRATION=1 to run local PostgreSQL integration test")
	}
	ctx := context.Background()
	profile, channelId := integrationPublishFixture(t, ctx)
	service := &sSysPublish{}
	mediaCount, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		WhereNull("task_id").Where("profile_id", profile.ProfileId).WhereNull("deleted_at").Count()
	if err != nil || mediaCount <= 0 {
		t.Fatalf("profile media missing: profile=%d count=%d err=%v", profile.ProfileId, mediaCount, err)
	}

	now := gtime.Now()
	batchNo := newFullPushBatchNo(channelId, now.TimestampNano())
	batchId, err := g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).Data(g.Map{
		"batch_no": batchNo, "tenant_id": profile.TenantId, "channel_id": channelId,
		"status": fullPushBatchDispatching, "active_key": fmt.Sprintf("integration:%d", now.TimestampNano()),
		"created_at": now, "updated_at": now,
	}).InsertAndGetId()
	if err != nil {
		t.Fatalf("create full push batch: %v", err)
	}
	runId, err := g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id": profile.TenantId, "channel_id": channelId,
		"status": cycleRunStatusDispatching, "stage": "dispatching", "created_at": now, "updated_at": now,
	}).InsertAndGetId()
	if err != nil {
		t.Fatalf("create cycle run: %v", err)
	}
	operations := []string{
		fullPushProfileOperationNo(batchNo, profile.ProfileId),
		cyclePublishOperationNo(runId, profile.ProfileId, channelId),
	}
	t.Cleanup(func() { cleanupProfilePublishIntegration(ctx, profile.ProfileId, batchId, runId, operations) })

	source, err := service.profilePublishSource(ctx, profile.ProfileId, profile.TenantId, profile.AccountId, true)
	if err != nil {
		t.Fatalf("load profile publish source: %v", err)
	}
	channels, err := service.telegramJobChannels(ctx, source, []int64{channelId})
	if err != nil || len(channels) != 1 {
		t.Fatalf("load integration channel: count=%d err=%v", len(channels), err)
	}
	for _, operationNo := range operations {
		var jobId int64
		for attempt := 0; attempt < 2; attempt++ {
			if jobId, err = service.ensureTelegramProfileJob(ctx, source, channels[0], operationNo); err != nil {
				t.Fatalf("create profile job operation=%s attempt=%d: %v", operationNo, attempt+1, err)
			}
		}
		count, countErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			WhereNull("task_id").Where("profile_id", profile.ProfileId).
			Where("operation_no", operationNo).Where("channel_id", channelId).Count()
		if countErr != nil || count != 1 {
			t.Fatalf("expected one idempotent job operation=%s count=%d err=%v", operationNo, count, countErr)
		}
		job, readErr := service.telegramJobById(ctx, jobId)
		if readErr != nil || job.TaskId != 0 || job.ProfileId != profile.ProfileId {
			t.Fatalf("unexpected direct profile job: job=%+v err=%v", job, readErr)
		}
		caption, captionErr := service.telegramJobCaption(ctx, job)
		if captionErr != nil || caption == "" {
			t.Fatalf("read latest profile caption: caption=%q err=%v", caption, captionErr)
		}
		displayMedia, displayErr := service.telegramJobMedia(ctx, job, "display")
		verifyMedia, verifyErr := service.telegramJobMedia(ctx, job, "verify")
		if displayErr != nil || verifyErr != nil || len(displayMedia)+len(verifyMedia) == 0 {
			t.Fatalf("read latest profile media: display=%d verify=%d displayErr=%v verifyErr=%v", len(displayMedia), len(verifyMedia), displayErr, verifyErr)
		}
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			WhereNull("task_id").Where("profile_id", profile.ProfileId).Where("operation_no", operationNo).
			Data(g.Map{"status": "sent", "dispatch_status": tgDispatchStatusDone}).Update()
	}

	if err = service.finalizeFullPushBatch(ctx, fullPushBatchRecord{Id: batchId, BatchNo: batchNo, TenantId: profile.TenantId, ChannelId: channelId}); err != nil {
		t.Fatalf("finalize full push batch: %v", err)
	}
	if err = service.finalizeChannelCycleDelivery(ctx, cycleRunRecord{Id: runId, TenantId: profile.TenantId, ChannelId: channelId}); err != nil {
		t.Fatalf("finalize cycle run: %v", err)
	}
	assertIntegrationBatchStatus(t, ctx, publishFullPushBatchTable, batchId, fullPushBatchCompleted)
	assertIntegrationBatchStatus(t, ctx, publishCycleRunTable, runId, cycleRunStatusFinished)
}

type integrationProfileFixture struct {
	TenantId  int64 `orm:"tenant_id"`
	AccountId int64 `orm:"account_id"`
	ProfileId int64 `orm:"profile_id"`
}

func integrationPublishFixture(t *testing.T, ctx context.Context) (integrationProfileFixture, int64) {
	t.Helper()
	var profile integrationProfileFixture
	err := fullPushOnlineProfileBaseModel(ctx, 2).
		Fields("ps.tenant_id,ps.account_id,p.id AS profile_id").
		Where("EXISTS (SELECT 1 FROM " + publishMediaTable + " m WHERE m.profile_id=p.id AND m.task_id IS NULL AND m.deleted_at IS NULL)").
		OrderAsc("p.id").Limit(1).Scan(&profile)
	if err != nil || profile.ProfileId <= 0 {
		t.Fatalf("load integration profile: profile=%+v err=%v", profile, err)
	}
	channel, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id").Where("tenant_id", profile.TenantId).Where("publish_direction", "up").
		Where("status", 1).WhereNull("deleted_at").OrderAsc("id").One()
	if err != nil || channel["id"].Int64() <= 0 {
		t.Fatalf("load integration channel: channel=%v err=%v", channel, err)
	}
	return profile, channel["id"].Int64()
}

func assertIntegrationBatchStatus(t *testing.T, ctx context.Context, table string, id int64, expected string) {
	t.Helper()
	row, err := g.DB().Model(table).Safe().Ctx(ctx).Fields("status").Where("id", id).One()
	if err != nil || row["status"].String() != expected {
		t.Fatalf("unexpected batch status table=%s id=%d status=%s err=%v", table, id, row["status"].String(), err)
	}
}

func cleanupProfilePublishIntegration(ctx context.Context, profileId, batchId, runId int64, operations []string) {
	var jobs []struct {
		Id int64 `orm:"id"`
	}
	_ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Fields("id").
		WhereNull("task_id").Where("profile_id", profileId).WhereIn("operation_no", operations).Scan(&jobs)
	jobIds := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		if job.Id > 0 {
			jobIds = append(jobIds, job.Id)
		}
	}
	if len(jobIds) > 0 {
		_, _ = g.DB().Model(publishSuccessRecordTable).Safe().Ctx(ctx).Unscoped().WhereIn("job_id", jobIds).Delete()
		_, _ = g.DB().Model(publishTgJobLogTable).Safe().Ctx(ctx).Unscoped().WhereIn("job_id", jobIds).Delete()
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Unscoped().WhereIn("id", jobIds).Delete()
	}
	_, _ = g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).Unscoped().Where("id", batchId).Delete()
	_, _ = g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).Unscoped().Where("id", runId).Delete()
}

func TestLocalCreateFullPushBatch(t *testing.T) {
	if os.Getenv("YOUBAN_PUBLISH_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_PUBLISH_INTEGRATION=1 to run local PostgreSQL integration test")
	}
	ctx := context.Background()
	channel, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,tenant_id").Where("id", 6).WhereNull("deleted_at").One()
	if err != nil || channel["id"].Int64() <= 0 {
		t.Fatalf("load full push channel: channel=%v err=%v", channel, err)
	}
	batch, err := (&sSysPublish{}).createFullPushBatch(ctx, channel["tenant_id"].Int64(), channel["id"].Int64(), 0)
	if err != nil {
		t.Fatalf("create full push batch: %+v", err)
	}
	t.Cleanup(func() {
		_, _ = g.DB().Model(publishFullPushBatchTable).Safe().Ctx(ctx).Unscoped().Where("id", batch.Id).Delete()
	})
}
