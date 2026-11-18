package sys

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

func TestProfilePublishStateRecoveryBudget(t *testing.T) {
	if !profilePublishStateRecoveryHasBudget(context.Background(), time.Second) {
		t.Fatal("a context without a deadline must keep its existing behavior")
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Hour))
	defer cancel()
	if !profilePublishStateRecoveryHasBudget(ctx, time.Second) {
		t.Fatal("available budget was rejected")
	}
	if profilePublishStateRecoveryHasBudget(ctx, 2*time.Hour) {
		t.Fatal("recovery must yield before consuming its reserve")
	}
	cancel()
	if profilePublishStateRecoveryHasBudget(ctx, 0) {
		t.Fatal("a canceled context must not start recovery I/O")
	}
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	if profilePublishStateRecoveryHasBudget(expired, 0) {
		t.Fatal("an expired context must not start recovery I/O")
	}
}

func TestProfilePublishRecoveryUnchangedSnapshot(t *testing.T) {
	state := profilePublishOperationState{
		TenantId: 1, AccountId: 2, ProfileId: 3,
		PublishOperationNo: "operation:one", PublishTaskStatus: sysin.PublishTaskStatusPending,
	}
	job := telegramJobRecord{TenantId: 1, AccountId: 2, ProfileId: 3, OperationNo: "operation:one"}
	for _, status := range []sysin.PublishTaskStatus{sysin.PublishTaskStatusPending, sysin.PublishTaskStatusPublishing} {
		snapshot := state
		snapshot.PublishTaskStatus = status
		if !profilePublishRecoveryStateUnchanged(snapshot, job, status) {
			t.Fatalf("matching active snapshot should skip its write: %s", status)
		}
	}
	for _, tc := range []struct {
		name   string
		change func(*profilePublishOperationState, *telegramJobRecord)
		status sysin.PublishTaskStatus
	}{
		{"pending to publishing", func(_ *profilePublishOperationState, _ *telegramJobRecord) {}, sysin.PublishTaskStatusPublishing},
		{"failed must retain fence", func(_ *profilePublishOperationState, _ *telegramJobRecord) {}, sysin.PublishTaskStatusFailed},
		{"canceled must retain fence", func(_ *profilePublishOperationState, _ *telegramJobRecord) {}, sysin.PublishTaskStatusCanceled},
		{"canceling snapshot", func(s *profilePublishOperationState, _ *telegramJobRecord) {
			s.PublishTaskStatus = sysin.PublishTaskStatusCanceling
		}, sysin.PublishTaskStatusPending},
		{"new operation", func(_ *profilePublishOperationState, j *telegramJobRecord) { j.OperationNo = "operation:two" }, sysin.PublishTaskStatusPending},
		{"empty operation", func(s *profilePublishOperationState, j *telegramJobRecord) {
			s.PublishOperationNo, j.OperationNo = "", ""
		}, sysin.PublishTaskStatusPending},
		{"other tenant", func(_ *profilePublishOperationState, j *telegramJobRecord) { j.TenantId++ }, sysin.PublishTaskStatusPending},
		{"other account", func(_ *profilePublishOperationState, j *telegramJobRecord) { j.AccountId++ }, sysin.PublishTaskStatusPending},
		{"other profile", func(_ *profilePublishOperationState, j *telegramJobRecord) { j.ProfileId++ }, sysin.PublishTaskStatusPending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, current := state, job
			tc.change(&snapshot, &current)
			if profilePublishRecoveryStateUnchanged(snapshot, current, tc.status) {
				t.Fatal("changed scope, operation or state incorrectly skipped its fence")
			}
		})
	}
}

func TestProfilePublishNoteIndexStatusCache(t *testing.T) {
	ctx := context.Background()
	cache.SetAdapter(ctx)
	previous := cache.Instance().GetAdapter()
	cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	t.Cleanup(func() { cache.Instance().SetAdapter(previous) })

	if noteIndexStatusRecentlyWritten(ctx, 1, 2, 3, string(sysin.PublishTaskStatusPublishing)) {
		t.Fatal("status must not be cached before the write marker is set")
	}
	markNoteIndexStatusWritten(ctx, 1, 2, 3, string(sysin.PublishTaskStatusPublishing))
	if !noteIndexStatusRecentlyWritten(ctx, 1, 2, 3, string(sysin.PublishTaskStatusPublishing)) {
		t.Fatal("status write marker was not found")
	}
	if noteIndexStatusRecentlyWritten(ctx, 1, 2, 3, string(sysin.PublishTaskStatusFailed)) {
		t.Fatal("different task status must not share the write marker")
	}
}

func TestLocalProfilePublishStateProjection(t *testing.T) {
	if os.Getenv("YOUBAN_PUBLISH_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_PUBLISH_INTEGRATION=1 to run local PostgreSQL integration test")
	}
	ctx := context.Background()
	now := gtime.Now()
	profileId := time.Now().UnixNano()
	tenantId := profileId
	accountId := profileId
	if _, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id": tenantId, "account_id": accountId, "profile_id": profileId,
		"publish_operation_no": "", "publish_task_status": "", "created_at": now, "updated_at": now,
	}).Insert(); err != nil {
		t.Fatalf("create profile state fixture: %v", err)
	}
	if _, err := g.DB().Model(publishNoteIndexTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id": tenantId, "account_id": accountId, "profile_id": profileId,
		"uuid": fmt.Sprintf("publish-state-%d", profileId), "profile_no": fmt.Sprintf("T%d", profileId),
		"task_status": "", "created_at": now, "updated_at": now,
	}).Insert(); err != nil {
		t.Fatalf("create note index fixture: %v", err)
	}
	t.Cleanup(func() {
		_, _ = g.DB().Model(publishNoteIndexTable).Safe().Ctx(ctx).Unscoped().Where("profile_id", profileId).Delete()
		_, _ = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).Unscoped().Where("profile_id", profileId).Delete()
	})

	service := &sSysPublish{}
	operationOne := fmt.Sprintf("integration:one:%d", profileId)
	operationTwo := fmt.Sprintf("integration:two:%d", profileId)
	if err := service.beginProfilePublishOperation(ctx, tenantId, accountId, profileId, operationOne); err != nil {
		t.Fatalf("begin first operation: %v", err)
	}
	assertProfilePublishProjection(t, ctx, profileId, operationOne, string(sysin.PublishTaskStatusPending))
	if err := service.beginProfilePublishOperation(ctx, tenantId, accountId, profileId, operationTwo); err != nil {
		t.Fatalf("begin second operation: %v", err)
	}
	staleJob := telegramJobRecord{TenantId: tenantId, AccountId: accountId, ProfileId: profileId, OperationNo: operationOne}
	if err := service.updateProfilePublishOperationState(ctx, staleJob, string(sysin.PublishTaskStatusPublishing)); err != nil {
		t.Fatalf("update stale operation: %v", err)
	}
	assertProfilePublishProjection(t, ctx, profileId, operationTwo, string(sysin.PublishTaskStatusPending))

	currentJob := telegramJobRecord{TenantId: tenantId, AccountId: accountId, ProfileId: profileId, OperationNo: operationTwo}
	if err := service.updateProfilePublishOperationState(ctx, currentJob, string(sysin.PublishTaskStatusPublishing)); err != nil {
		t.Fatalf("update current operation: %v", err)
	}
	assertProfilePublishProjection(t, ctx, profileId, operationTwo, string(sysin.PublishTaskStatusPublishing))
	if err := service.clearProfilePublishOperationState(ctx, staleJob); err != nil {
		t.Fatalf("clear stale operation: %v", err)
	}
	assertProfilePublishProjection(t, ctx, profileId, operationTwo, string(sysin.PublishTaskStatusPublishing))
	if err := service.clearProfilePublishOperationState(ctx, currentJob); err != nil {
		t.Fatalf("clear current operation: %v", err)
	}
	assertProfilePublishProjection(t, ctx, profileId, operationTwo, "")
	if err := service.beginProfilePublishOperation(ctx, tenantId, accountId, profileId, operationTwo); err != nil {
		t.Fatalf("restart second operation: %v", err)
	}
	if err := service.updateProfilePublishOperationState(ctx, currentJob, string(sysin.PublishTaskStatusFailed)); err != nil {
		t.Fatalf("fail current operation: %v", err)
	}
	operationThree := fmt.Sprintf("integration:three:%d", profileId)
	if err := service.beginProfilePublishOperation(ctx, tenantId, accountId, profileId, operationThree); err != nil {
		t.Fatalf("begin operation after failure: %v", err)
	}
	assertProfilePublishProjection(t, ctx, profileId, operationThree, string(sysin.PublishTaskStatusPending))
	if err := service.cancelProfilePublishOperation(ctx, profileId); err != nil {
		t.Fatalf("cancel current operation: %v", err)
	}
	assertProfilePublishProjection(t, ctx, profileId, operationThree, "")

	operationFour := fmt.Sprintf("integration:recover:%d", profileId)
	jobId, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
		"operation_no": operationFour, "tenant_id": tenantId, "merchant_id": tenantId,
		"account_id": accountId, "profile_id": profileId, "channel_id": profileId,
		"status": "pending", "dispatch_status": tgDispatchStatusIdle, "created_at": now, "updated_at": now,
	}).InsertAndGetId()
	if err != nil {
		t.Fatalf("create legacy active job fixture: %v", err)
	}
	t.Cleanup(func() {
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Unscoped().Where("id", jobId).Delete()
	})
	if err = service.recoverMissingProfilePublishOperationStates(ctx, 100); err != nil {
		t.Fatalf("recover missing pending projection: %v", err)
	}
	assertProfilePublishProjection(t, ctx, profileId, operationFour, string(sysin.PublishTaskStatusPending))
	if err = service.cancelProfilePublishOperation(ctx, profileId); err != nil {
		t.Fatalf("clear recovered pending projection: %v", err)
	}
	if _, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobId).
		Data(g.Map{"status": "sending", "updated_at": gtime.Now()}).Update(); err != nil {
		t.Fatalf("mark legacy job sending: %v", err)
	}
	if err = service.recoverMissingProfilePublishOperationStates(ctx, 100); err != nil {
		t.Fatalf("recover missing sending projection: %v", err)
	}
	assertProfilePublishProjection(t, ctx, profileId, operationFour, string(sysin.PublishTaskStatusPublishing))
}

func TestLocalRecoverProfilePublishStateSkipsTerminalRows(t *testing.T) {
	if os.Getenv("YOUBAN_PUBLISH_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_PUBLISH_INTEGRATION=1 to run local PostgreSQL integration test")
	}
	ctx := context.Background()
	now := gtime.Now()
	baseId := time.Now().UnixNano()
	failedProfileId := baseId
	activeProfileId := baseId + 1
	freshProfileId := baseId + 2
	operationNo := fmt.Sprintf("integration:recover-starvation:%d", baseId)

	for _, fixture := range []g.Map{
		{
			"tenant_id": baseId, "account_id": baseId, "profile_id": failedProfileId,
			"publish_operation_no": "terminal-failure", "publish_task_status": string(sysin.PublishTaskStatusFailed),
			"publish_task_updated_at": now.Add(-time.Hour), "created_at": now, "updated_at": now,
		},
		{
			"tenant_id": baseId, "account_id": baseId, "profile_id": activeProfileId,
			"publish_operation_no": operationNo, "publish_task_status": string(sysin.PublishTaskStatusPublishing),
			"publish_task_updated_at": now.Add(-time.Hour), "created_at": now, "updated_at": now,
		},
		{
			"tenant_id": baseId, "account_id": baseId, "profile_id": freshProfileId,
			"publish_operation_no": operationNo + ":fresh", "publish_task_status": string(sysin.PublishTaskStatusPublishing),
			"publish_task_updated_at": now, "created_at": now, "updated_at": now,
		},
	} {
		if _, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).Data(fixture).Insert(); err != nil {
			t.Fatalf("create profile state fixture: %v", err)
		}
	}
	if _, err := g.DB().Model(publishNoteIndexTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id": baseId, "account_id": baseId, "profile_id": activeProfileId,
		"uuid": fmt.Sprintf("publish-state-starvation-%d", baseId), "profile_no": fmt.Sprintf("T%d", baseId),
		"task_status": string(sysin.PublishTaskStatusPublishing), "created_at": now, "updated_at": now,
	}).Insert(); err != nil {
		t.Fatalf("create note index fixture: %v", err)
	}
	if _, err := g.DB().Model(publishNoteIndexTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id": baseId, "account_id": baseId, "profile_id": freshProfileId,
		"uuid": fmt.Sprintf("publish-state-fresh-%d", baseId), "profile_no": fmt.Sprintf("TF%d", baseId),
		"task_status": string(sysin.PublishTaskStatusPublishing), "created_at": now, "updated_at": now,
	}).Insert(); err != nil {
		t.Fatalf("create fresh note index fixture: %v", err)
	}
	jobId, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
		"operation_no": operationNo, "tenant_id": baseId, "merchant_id": baseId,
		"account_id": baseId, "profile_id": activeProfileId, "channel_id": baseId,
		"status": "failed", "dispatch_status": tgDispatchStatusIdle, "created_at": now, "updated_at": now,
	}).InsertAndGetId()
	if err != nil {
		t.Fatalf("create failed job fixture: %v", err)
	}
	t.Cleanup(func() {
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Unscoped().Where("id", jobId).Delete()
		_, _ = g.DB().Model(publishNoteIndexTable).Safe().Ctx(ctx).Unscoped().WhereIn("profile_id", []int64{activeProfileId, freshProfileId}).Delete()
		_, _ = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).Unscoped().WhereIn("profile_id", []int64{failedProfileId, activeProfileId, freshProfileId}).Delete()
	})

	service := &sSysPublish{}
	if err = service.recoverProfilePublishOperationStates(ctx, 1); err != nil {
		t.Fatalf("recover active profile state: %v", err)
	}
	assertProfilePublishProjection(t, ctx, activeProfileId, operationNo, string(sysin.PublishTaskStatusFailed))
	assertProfilePublishProjection(t, ctx, freshProfileId, operationNo+":fresh", string(sysin.PublishTaskStatusPublishing))
}

func assertProfilePublishProjection(t *testing.T, ctx context.Context, profileId int64, operationNo, status string) {
	t.Helper()
	state, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Fields("publish_operation_no,publish_task_status").Where("profile_id", profileId).One()
	if err != nil {
		t.Fatalf("read profile state: %v", err)
	}
	if state["publish_operation_no"].String() != operationNo || state["publish_task_status"].String() != status {
		t.Fatalf("unexpected profile state operation=%q status=%q", state["publish_operation_no"].String(), state["publish_task_status"].String())
	}
	index, err := g.DB().Model(publishNoteIndexTable).Safe().Ctx(ctx).
		Fields("task_status").Where("profile_id", profileId).One()
	if err != nil {
		t.Fatalf("read note index state: %v", err)
	}
	if index["task_status"].String() != status {
		t.Fatalf("unexpected note index task status=%q want=%q", index["task_status"].String(), status)
	}
}
