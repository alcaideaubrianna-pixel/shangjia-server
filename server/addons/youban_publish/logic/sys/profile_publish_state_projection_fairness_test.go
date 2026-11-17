package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/library/cache"
)

func TestProfilePublishRecoveryCursorWhere(t *testing.T) {
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Link: "pgsql:test:test@tcp(127.0.0.1:5432)/test"})
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	where, args := db.Model(publishProfileStateTable).Builder().
		Where("publish_task_updated_at >= ?", stamp).
		Where("(publish_task_updated_at > ? OR (publish_task_updated_at = ? AND profile_id > ?))", stamp, stamp, int64(123)).Build()
	if len(args) != 4 || strings.Count(where, "?") != 4 || !strings.Contains(where, "publish_task_updated_at > ? OR") || args[3] != int64(123) {
		t.Fatalf("unexpected GoFrame cursor predicate: %s, args=%v", where, args)
	}
}

// This test requires an explicitly configured isolated local PostgreSQL DB.
func TestLocalProfilePublishRecoveryFairness(t *testing.T) {
	if os.Getenv("YOUBAN_PROFILE_PROJECTION_FAIRNESS_TEST") != "1" {
		t.Skip("set YOUBAN_PROFILE_PROJECTION_FAIRNESS_TEST=1 for isolated local PostgreSQL")
	}
	ctx := context.Background()
	address, err := g.DB().GetValue(ctx, "SELECT COALESCE(inet_server_addr()::text, 'local-socket')")
	if err != nil || (address.String() != "127.0.0.1" && address.String() != "::1" && address.String() != "local-socket") {
		t.Fatalf("test requires a local PostgreSQL server: %v", err)
	}
	cache.SetAdapter(ctx)
	previous := cache.Instance().GetAdapter()
	cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	t.Cleanup(func() { cache.Instance().SetAdapter(previous) })

	base := time.Now().UnixNano()
	stamp := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	cursor, _ := json.Marshal(profilePublishRecoveryCursor{Status: "pending", UpdatedAt: stamp.Add(-time.Second), ProfileId: base})
	if err := cache.Instance().Set(ctx, profilePublishStateRecoveryCursorKey, string(cursor), profilePublishStateRecoveryCursorTTL); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Unscoped().Where("tenant_id", base).Delete()
		_, _ = g.DB().Model(publishNoteIndexTable).Safe().Ctx(ctx).Unscoped().Where("tenant_id", base).Delete()
		_, _ = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).Unscoped().Where("tenant_id", base).Delete()
	})
	for i := int64(0); i < 3; i++ {
		id := base + i
		op := fmt.Sprintf("fairness:%d", id)
		if _, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).Data(g.Map{
			"tenant_id": base, "account_id": base, "profile_id": id, "publish_operation_no": op,
			"publish_task_status": "pending", "publish_task_updated_at": stamp,
			"created_at": gtime.Now(), "updated_at": gtime.Now(),
		}).Insert(); err != nil {
			t.Fatal(err)
		}
		if _, err := g.DB().Model(publishNoteIndexTable).Safe().Ctx(ctx).Data(g.Map{
			"tenant_id": base, "account_id": base, "profile_id": id, "uuid": op,
			"profile_no": fmt.Sprintf("F%d", id), "task_status": "pending", "created_at": gtime.Now(), "updated_at": gtime.Now(),
		}).Insert(); err != nil {
			t.Fatal(err)
		}
		status, phase := "pending", ""
		if i == 1 {
			status, phase = "failed", "timeout_wait_cleanup"
		} else if i == 2 {
			status = "failed"
		}
		if _, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
			"tenant_id": base, "merchant_id": base, "account_id": base, "profile_id": id,
			"channel_id": id, "operation_no": op, "status": status, "send_phase": phase,
			"dispatch_status": tgDispatchStatusIdle, "created_at": gtime.Now(), "updated_at": gtime.Now(),
		}).Insert(); err != nil {
			t.Fatal(err)
		}
	}
	s := &sSysPublish{}
	for i := 0; i < 5; i++ {
		if err := s.recoverProfilePublishOperationStates(ctx, 1); err != nil {
			t.Fatal(err)
		}
	}
	for i, status := range []string{"pending", "pending", "failed"} {
		id := base + int64(i)
		assertProfilePublishProjection(t, ctx, id, fmt.Sprintf("fairness:%d", id), status)
	}

	// Another channel sending takes precedence over a tail waiting for cleanup.
	waitID := base + 1
	waitOp := fmt.Sprintf("fairness:%d", waitID)
	if _, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id": base, "merchant_id": base, "account_id": base, "profile_id": waitID,
		"channel_id": base + 10, "operation_no": waitOp, "status": "sending",
		"dispatch_status": tgDispatchStatusIdle, "created_at": gtime.Now(), "updated_at": gtime.Now(),
	}).Insert(); err != nil {
		t.Fatal(err)
	}
	state := profilePublishOperationState{TenantId: base, AccountId: base, ProfileId: waitID, PublishOperationNo: waitOp, PublishTaskStatus: "pending"}
	if err := s.recoverProfilePublishOperationState(ctx, state); err != nil {
		t.Fatal(err)
	}
	assertProfilePublishProjection(t, ctx, waitID, waitOp, "publishing")
	if _, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", base).Where("profile_id", waitID).Where("status", "sending").
		Data(g.Map{"status": "superseded"}).Update(); err != nil {
		t.Fatal(err)
	}
	state.PublishTaskStatus = "publishing"
	if err := s.recoverProfilePublishOperationState(ctx, state); err != nil {
		t.Fatal(err)
	}
	assertProfilePublishProjection(t, ctx, waitID, waitOp, "pending")
}
