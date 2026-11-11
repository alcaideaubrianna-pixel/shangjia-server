package sys

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"hotgo/internal/library/cache"
)

// TestLocalAccountChannelFailureRuntime 用真实错误Bot配置验证创建阶段隔离、部分失败和同操作重试。
func TestLocalAccountChannelFailureRuntime(t *testing.T) {
	ctx := localAccountRuntimeContext(t)
	s := NewSysPublish()
	profile := localRuntimeProfiles(t, ctx, 20, 1)[0].ID
	original, err := g.DB().GetValue(ctx, "SELECT bot_id_json FROM "+publishChannelTable+" WHERE tenant_id=999 AND id=2")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = g.DB().Exec(context.Background(), "UPDATE "+publishChannelTable+" SET bot_id_json=? WHERE tenant_id=999 AND id=2", original.String())
	})
	if _, err := g.DB().Exec(ctx, "UPDATE "+publishChannelTable+" SET bot_id_json='[]' WHERE tenant_id=999 AND id=2"); err != nil {
		t.Fatal(err)
	}
	op := fmt.Sprintf("profile:channel-failure:%d", time.Now().UnixNano())
	channels := []int64{1, 2, 3}
	if err := s.submitProfilePublish(ctx, profile, 999, 999, 999, op, channels, false); err != nil {
		t.Fatal(err)
	}
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE account_id=999 AND operation_no=? AND status='sent'", 2, op)
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE account_id=999 AND operation_no=? AND status='failed' AND channel_id=2 AND error_message<>''", 1, op)
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishProfileStateTable+" WHERE account_id=999 AND profile_id=? AND publish_operation_no=? AND publish_task_status='failed'", 1, profile, op)
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgAttemptTable+" a JOIN "+publishTgJobTable+" j ON j.id=a.job_id WHERE j.operation_no=? AND j.channel_id=2", 0, op)
	// 完成后再次提交同一操作，不能新增发送，也不能把成功记录改回pending。
	if err := s.submitProfilePublish(ctx, profile, 999, 999, 999, op, channels, false); err != nil {
		t.Fatal(err)
	}
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE account_id=999 AND operation_no=?", 3, op)
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE account_id=999 AND operation_no=? AND status='sent'", 2, op)
	// 修复配置后以新的操作发布，旧失败任务不占用频道，也不污染新操作状态。
	if _, err := g.DB().Exec(ctx, "UPDATE "+publishChannelTable+" SET bot_id_json=? WHERE tenant_id=999 AND id=2", original.String()); err != nil {
		t.Fatal(err)
	}
	retry := op + ":fixed"
	if err := s.submitProfilePublish(ctx, profile, 999, 999, 999, retry, channels, false); err != nil {
		t.Fatal(err)
	}
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE account_id=999 AND operation_no=? AND status='sent'", 3, retry)
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishProfileStateTable+" WHERE account_id=999 AND profile_id=? AND publish_operation_no=? AND COALESCE(publish_task_status,'')=''", 1, profile, retry)
	// 循环入口同样必须隔离坏频道，并将run收口为partial，而非永久dispatched。
	cycleProfile := localRuntimeProfiles(t, ctx, 3, 1)[0].ID
	baselineOp := op + ":cycle-baseline"
	if err := s.submitProfilePublish(ctx, cycleProfile, 999, 999, 999, baselineOp, channels, false); err != nil {
		t.Fatal(err)
	}
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE account_id=999 AND operation_no=? AND status='sent'", 3, baselineOp)
	if _, err := g.DB().Exec(ctx, "INSERT INTO hg_youban_publish_tenant_vip(tenant_id,level,status,expired_at) VALUES(999,1,1,NOW()+interval '1 day') ON CONFLICT(tenant_id) WHERE deleted_at IS NULL DO UPDATE SET level=1,status=1,expired_at=EXCLUDED.expired_at"); err != nil {
		t.Fatal(err)
	}
	_, _ = cache.Instance().Remove(ctx, tenantVipCacheKey(999))
	t.Cleanup(func() {
		_, _ = g.DB().Exec(context.Background(), "UPDATE "+publishChannelTable+" SET cycle_publish_enabled=0 WHERE tenant_id=999 AND id IN (1,2,3)")
	})
	if _, err := g.DB().Exec(ctx, "UPDATE "+publishChannelTable+" SET cycle_publish_enabled=1,cycle_publish_mode='time',cycle_publish_days=3 WHERE tenant_id=999 AND id IN (1,2,3)"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.DB().Exec(ctx, "UPDATE "+publishChannelTable+" SET bot_id_json='[]' WHERE tenant_id=999 AND id=2"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.DB().Exec(ctx, "UPDATE "+publishChannelProfileTable+" cp SET next_cycle_at=NOW()-interval '1 second' FROM "+publishTgJobTable+" j WHERE cp.last_job_id=j.id AND cp.account_id=999 AND j.operation_no=?", baselineOp); err != nil {
		t.Fatal(err)
	}
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishProfileCycleRunTable+" r WHERE r.account_id=999 AND r.profile_id=? AND r.status='partial' AND EXISTS(SELECT 1 FROM "+publishProfileCycleRunChannelTable+" rc JOIN "+publishTgJobTable+" j ON j.id=rc.job_id WHERE rc.run_id=r.id AND j.id>(SELECT max(id) FROM "+publishTgJobTable+" WHERE account_id=999 AND operation_no=?))", 1, cycleProfile, baselineOp)
	t.Log("bad Bot channel failed independently; healthy channels sent; duplicate operation and corrected configuration passed")
}
