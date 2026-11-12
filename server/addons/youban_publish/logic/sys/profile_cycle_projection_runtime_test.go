package sys

import (
	"fmt"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// TestLocalProfileCycleProjectionReconcileRuntime 用真实本地数据库验证：
// Job 已经终态而频道投影仍 pending 时，账户调度可以收口 run，避免下一轮被永久阻塞。
func TestLocalProfileCycleProjectionReconcileRuntime(t *testing.T) {
	ctx := localAccountRuntimeContext(t)
	profile := localRuntimeProfiles(t, ctx, 0, 1)[0].ID
	jobValue, err := g.DB().Model(publishTgJobTable).Ctx(ctx).
		Where("tenant_id", 999).Where("account_id", 999).Where("profile_id", profile).
		Where("channel_id", 1).WhereIn("status", []string{"sent", "superseded"}).
		Fields("id").OrderDesc("id").Limit(1).Value()
	if err != nil {
		t.Fatal(err)
	}
	jobID := jobValue.Int64()
	if jobID <= 0 {
		t.Skip("local fixture has no terminal Telegram job")
	}
	key := fmt.Sprintf("runtime-reconcile:%d", time.Now().UnixNano())
	runID, err := g.DB().Model(publishProfileCycleRunTable).Ctx(ctx).Data(g.Map{
		"tenant_id": 999, "account_id": 999, "profile_id": profile,
		"run_key": key, "status": "dispatched", "due_at": time.Now(),
	}).InsertAndGetId()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = g.DB().Model(publishProfileCycleRunChannelTable).Ctx(ctx).Where("run_id", runID).Delete()
		_, _ = g.DB().Model(publishProfileCycleRunTable).Ctx(ctx).Where("id", runID).Delete()
	})
	if _, err = g.DB().Model(publishProfileCycleRunChannelTable).Ctx(ctx).Data(g.Map{
		"run_id": runID, "tenant_id": 999, "account_id": 999,
		"profile_id": profile, "channel_id": 1, "job_id": jobID, "status": "pending",
	}).Insert(); err != nil {
		t.Fatal(err)
	}
	if err = reconcileProfileCycleRunChannels(ctx, publishAccountScope{TenantID: 999, AccountID: 999}, 200); err != nil {
		t.Fatal(err)
	}
	var state struct {
		ChannelStatus string `orm:"channel_status"`
		RunStatus     string `orm:"run_status"`
	}
	if err = g.DB().Model(publishProfileCycleRunChannelTable+" rc").Ctx(ctx).
		InnerJoin(publishProfileCycleRunTable+" r", "r.id=rc.run_id").
		Fields("rc.status AS channel_status,r.status AS run_status").Where("rc.run_id", runID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state.ChannelStatus != "sent" || state.RunStatus != "completed" {
		t.Fatalf("projection not reconciled: channel=%q run=%q", state.ChannelStatus, state.RunStatus)
	}
}
