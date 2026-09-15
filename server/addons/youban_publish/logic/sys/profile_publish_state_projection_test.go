package sys

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

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
	assertProfilePublishProjection(t, ctx, profileId, operationOne, sysin.PublishTaskStatusPending)
	if err := service.beginProfilePublishOperation(ctx, tenantId, accountId, profileId, operationTwo); err != nil {
		t.Fatalf("begin second operation: %v", err)
	}
	staleJob := telegramJobRecord{TenantId: tenantId, AccountId: accountId, ProfileId: profileId, OperationNo: operationOne}
	if err := service.updateProfilePublishOperationState(ctx, staleJob, sysin.PublishTaskStatusPublishing); err != nil {
		t.Fatalf("update stale operation: %v", err)
	}
	assertProfilePublishProjection(t, ctx, profileId, operationTwo, sysin.PublishTaskStatusPending)

	currentJob := telegramJobRecord{TenantId: tenantId, AccountId: accountId, ProfileId: profileId, OperationNo: operationTwo}
	if err := service.updateProfilePublishOperationState(ctx, currentJob, sysin.PublishTaskStatusPublishing); err != nil {
		t.Fatalf("update current operation: %v", err)
	}
	assertProfilePublishProjection(t, ctx, profileId, operationTwo, sysin.PublishTaskStatusPublishing)
	if err := service.clearProfilePublishOperationState(ctx, staleJob); err != nil {
		t.Fatalf("clear stale operation: %v", err)
	}
	assertProfilePublishProjection(t, ctx, profileId, operationTwo, sysin.PublishTaskStatusPublishing)
	if err := service.clearProfilePublishOperationState(ctx, currentJob); err != nil {
		t.Fatalf("clear current operation: %v", err)
	}
	assertProfilePublishProjection(t, ctx, profileId, operationTwo, "")
	if err := service.beginProfilePublishOperation(ctx, tenantId, accountId, profileId, operationTwo); err != nil {
		t.Fatalf("restart second operation: %v", err)
	}
	if err := service.updateProfilePublishOperationState(ctx, currentJob, sysin.PublishTaskStatusFailed); err != nil {
		t.Fatalf("fail current operation: %v", err)
	}
	operationThree := fmt.Sprintf("integration:three:%d", profileId)
	if err := service.beginProfilePublishOperation(ctx, tenantId, accountId, profileId, operationThree); err != nil {
		t.Fatalf("begin operation after failure: %v", err)
	}
	assertProfilePublishProjection(t, ctx, profileId, operationThree, sysin.PublishTaskStatusPending)
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
	assertProfilePublishProjection(t, ctx, profileId, operationFour, sysin.PublishTaskStatusPending)
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
	assertProfilePublishProjection(t, ctx, profileId, operationFour, sysin.PublishTaskStatusPublishing)
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
	operationNo := fmt.Sprintf("integration:recover-starvation:%d", baseId)

	for _, fixture := range []g.Map{
		{
			"tenant_id": baseId, "account_id": baseId, "profile_id": failedProfileId,
			"publish_operation_no": "terminal-failure", "publish_task_status": sysin.PublishTaskStatusFailed,
			"publish_task_updated_at": now.Add(-time.Hour), "created_at": now, "updated_at": now,
		},
		{
			"tenant_id": baseId, "account_id": baseId, "profile_id": activeProfileId,
			"publish_operation_no": operationNo, "publish_task_status": sysin.PublishTaskStatusPublishing,
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
		"task_status": sysin.PublishTaskStatusPublishing, "created_at": now, "updated_at": now,
	}).Insert(); err != nil {
		t.Fatalf("create note index fixture: %v", err)
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
		_, _ = g.DB().Model(publishNoteIndexTable).Safe().Ctx(ctx).Unscoped().Where("profile_id", activeProfileId).Delete()
		_, _ = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).Unscoped().WhereIn("profile_id", []int64{failedProfileId, activeProfileId}).Delete()
	})

	service := &sSysPublish{}
	if err = service.recoverProfilePublishOperationStates(ctx, 1); err != nil {
		t.Fatalf("recover active profile state: %v", err)
	}
	assertProfilePublishProjection(t, ctx, activeProfileId, operationNo, sysin.PublishTaskStatusFailed)
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
