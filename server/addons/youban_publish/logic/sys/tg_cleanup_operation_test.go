package sys

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestTelegramCleanupUsesCurrentUnpublishOperation(t *testing.T) {
	if os.Getenv("YOUBAN_PUBLISH_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_PUBLISH_INTEGRATION=1 to run local PostgreSQL integration test")
	}

	ctx := context.Background()
	profileID := time.Now().UnixNano()
	tenantID := profileID
	currentOperationNo := fmt.Sprintf("unpublish:test:%d", profileID)
	staleOperationNo := fmt.Sprintf("profile:test:%d", profileID)
	now := gtime.Now()

	if _, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id":               tenantID,
		"account_id":              tenantID,
		"profile_id":              profileID,
		"publish_operation_no":    currentOperationNo,
		"publish_task_status":     sysin.PublishTaskStatusUnpublishing.String(),
		"publish_task_updated_at": now,
		"created_at":              now,
		"updated_at":              now,
	}).Insert(); err != nil {
		t.Fatalf("create profile state fixture: %v", err)
	}
	if _, err := g.DB().Model(publishNoteIndexTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id":   tenantID,
		"account_id":  tenantID,
		"profile_id":  profileID,
		"uuid":        fmt.Sprintf("cleanup-operation-%d", profileID),
		"profile_no":  fmt.Sprintf("T%d", profileID),
		"task_status": sysin.PublishTaskStatusUnpublishing.String(),
		"created_at":  now,
		"updated_at":  now,
	}).Insert(); err != nil {
		t.Fatalf("create note index fixture: %v", err)
	}
	t.Cleanup(func() {
		_, _ = g.DB().Model(publishNoteIndexTable).Safe().Ctx(ctx).Unscoped().Where("profile_id", profileID).Delete()
		_, _ = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).Unscoped().Where("profile_id", profileID).Delete()
	})

	service := &sSysPublish{}
	gotOperationNo, err := service.currentTelegramCleanupOperationNo(ctx, tenantID, profileID)
	if err != nil {
		t.Fatalf("resolve current cleanup operation: %v", err)
	}
	if gotOperationNo != currentOperationNo {
		t.Fatalf("current cleanup operation = %q, want %q", gotOperationNo, currentOperationNo)
	}

	if err := service.clearProfilesUnpublishing(ctx, []int64{profileID}, tenantID, staleOperationNo); err == nil {
		t.Fatal("stale operation must not clear the current unpublishing state")
	}
	state, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Fields("publish_operation_no,publish_task_status").
		Where("tenant_id", tenantID).Where("profile_id", profileID).One()
	if err != nil {
		t.Fatalf("read state after stale cleanup: %v", err)
	}
	if state["publish_operation_no"].String() != currentOperationNo ||
		state["publish_task_status"].String() != sysin.PublishTaskStatusUnpublishing.String() {
		t.Fatalf("stale cleanup changed state: operation=%q status=%q", state["publish_operation_no"].String(), state["publish_task_status"].String())
	}

	if err := service.clearProfilesUnpublishing(ctx, []int64{profileID}, tenantID, currentOperationNo); err != nil {
		t.Fatalf("clear current unpublishing state: %v", err)
	}
	state, err = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Fields("publish_operation_no,publish_task_status").
		Where("tenant_id", tenantID).Where("profile_id", profileID).One()
	if err != nil {
		t.Fatalf("read state after current cleanup: %v", err)
	}
	if state["publish_operation_no"].String() != "" || state["publish_task_status"].String() != "" {
		t.Fatalf("current cleanup did not clear state: operation=%q status=%q", state["publish_operation_no"].String(), state["publish_task_status"].String())
	}
}
