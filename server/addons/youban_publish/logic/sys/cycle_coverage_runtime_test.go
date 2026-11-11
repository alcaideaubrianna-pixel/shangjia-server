package sys

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/cache"
)

// TestCyclePermissionImmediateClassification 永久失权立即拦截，模糊错误沿用原熔断阈值。
func TestCyclePermissionImmediateClassification(t *testing.T) {
	for _, item := range []struct {
		message string
		want    bool
	}{
		{"Forbidden: bot was kicked from the channel chat", true},
		{"not enough rights to send text messages", true},
		{"Bad Request: chat not found", false},
		{"Too Many Requests: retry after 30", false},
		{"context deadline exceeded", false},
		{"wrong file identifier", false},
	} {
		if got := isDefinitiveTelegramPermissionError(errors.New(item.message)); got != item.want {
			t.Fatalf("%s: %v", item.message, got)
		}
	}
}

// TestLocalCycleCoverageRuntime 实际角色执行三轮时间循环和有限批次；只压缩到期时间，不伪造发送成功。
func TestLocalCycleCoverageRuntime(t *testing.T) {
	ctx := localAccountRuntimeContext(t)
	s := NewSysPublish()
	profiles := localRuntimeProfiles(t, ctx, 210, 9)
	ids := make([]int64, 0, len(profiles))
	for _, p := range profiles {
		ids = append(ids, p.ID)
	}
	exec := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := g.DB().Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO hg_youban_publish_tenant_vip(tenant_id,level,status,expired_at) VALUES(999,1,1,NOW()+interval '1 day') ON CONFLICT(tenant_id) WHERE deleted_at IS NULL DO UPDATE SET level=1,status=1,expired_at=EXCLUDED.expired_at")
	_, _ = cache.Instance().Remove(ctx, tenantVipCacheKey(999))
	exec("UPDATE " + publishChannelTable + " SET cycle_publish_enabled=1,cycle_publish_mode='time',cycle_publish_days=1,cycle_publish_time='' WHERE tenant_id=999 AND id IN(1,2)")
	t.Cleanup(func() {
		_, _ = g.DB().Exec(context.Background(), "UPDATE "+publishChannelTable+" SET cycle_publish_enabled=0 WHERE tenant_id=999 AND id IN(1,2,999997)")
	})
	tag := fmt.Sprintf("profile:coverage:%d", time.Now().UnixNano())
	for round := 0; round < 3; round++ {
		// 每轮加入两条之前未参与本测试的新资料，模拟持续采集/新上架。
		for _, p := range profiles[round*2 : round*2+2] {
			if err := s.submitProfilePublish(ctx, p.ID, 999, 999, 999, fmt.Sprintf("%s:%d", tag, p.ID), []int64{1, 2}, false); err != nil {
				t.Fatal(err)
			}
		}
		waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE operation_no LIKE ? AND status IN ('sent','superseded')", (round+1)*4, tag+":%")
		waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishChannelProfileTable+" cp JOIN "+publishTgJobTable+" j ON j.id=cp.last_job_id WHERE cp.tenant_id=999 AND cp.profile_id IN(?) AND cp.channel_id IN(1,2) AND j.operation_no LIKE ? AND cp.next_cycle_at>NOW()", 4, ids[round*2:round*2+2], tag+":%")
		if round == 0 {
			rollback := errors.New("rollback local mapping invariant")
			err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
				var job telegramJobRecord
				if err := tx.Model(publishTgJobTable+" j").InnerJoin(publishChannelProfileTable+" cp", "cp.last_job_id=j.id").Where("cp.tenant_id", 999).Where("cp.profile_id", ids[0]).Where("cp.channel_id", 1).Fields("j.*").Scan(&job); err != nil {
					return err
				}
				if _, err := tx.Model(publishChannelProfileTable).Where("channel_id", 1).Where("profile_id", ids[0]).Data(g.Map{"next_cycle_at": gdb.Raw("NOW()-interval '1 day'")}).Update(); err != nil {
					return err
				}
				if err := s.upsertChannelProfileFromJob(ctx, job); err != nil {
					return err
				}
				count, err := tx.Model(publishChannelProfileTable).Where("channel_id", 1).Where("profile_id", ids[0]).WhereNull("next_cycle_at").Count()
				if err != nil {
					return err
				}
				if count != 1 {
					return fmt.Errorf("mapping retained stale due deadline after success replacement")
				}
				return rollback
			})
			if !errors.Is(err, rollback) {
				t.Fatal(err)
			}
		}
		current := ids[:round*2+2]
		// 等真实成功回调完成计划投影，再模拟下一天到期。
		waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishChannelProfileTable+" WHERE tenant_id=999 AND account_id=999 AND profile_id IN(?) AND channel_id IN(1,2) AND status='active' AND next_cycle_at>NOW()", len(current)*2, current)
		baseline, err := g.DB().GetValue(ctx, "SELECT max(id) FROM "+publishTgJobTable)
		if err != nil {
			t.Fatal(err)
		}
		exec("UPDATE "+publishChannelProfileTable+" SET next_cycle_at=NOW()-interval '1 second' WHERE tenant_id=999 AND account_id=999 AND profile_id IN(?) AND channel_id IN(1,2)", current)
		waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE tenant_id=999 AND account_id=999 AND id>? AND profile_id IN(?) AND channel_id IN(1,2) AND operation_no LIKE 'profile_cycle:%' AND status='sent'", len(current)*2, baseline.Int64(), current)
		// 以资料和频道的集合验收，计数相同但漏一个/重复另一个也必须失败。
		waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM (SELECT profile_id,channel_id FROM "+publishTgJobTable+" WHERE tenant_id=999 AND account_id=999 AND id>? AND profile_id IN(?) AND channel_id IN(1,2) AND operation_no LIKE 'profile_cycle:%' AND status='sent' GROUP BY profile_id,channel_id HAVING count(*)=1) covered", len(current)*2, baseline.Int64(), current)
		t.Logf("time round=%d expected profile/channel pairs=%d, coverage complete", round+1, len(current)*2)
	}
	// 删除一条真实成功映射，并清空另一条计划，验证真实账户补偿能恢复而不伪造成功时间。
	repairFrom, err := g.DB().GetValue(ctx, "SELECT min(last_job_id)-1 FROM "+publishChannelProfileTable+" WHERE tenant_id=999 AND account_id=999 AND channel_id=1 AND profile_id IN(?)", ids[:2])
	if err != nil {
		t.Fatal(err)
	}
	// 设置补偿游标到待验证页，模拟历史分页已经完成；成功、映射和计划仍由真实角色恢复。
	if err := cache.Instance().Set(ctx, fmt.Sprintf("%s999:999", publishconsts.AccountCycleRepairPrefix), map[string]interface{}{"Cursor": repairFrom.Int64(), "NextAt": 0}, time.Hour); err != nil {
		t.Fatal(err)
	}
	exec("DELETE FROM "+publishChannelProfileTable+" WHERE tenant_id=999 AND account_id=999 AND channel_id=1 AND profile_id=?", ids[0])
	exec("UPDATE "+publishChannelProfileTable+" SET next_cycle_at=NULL WHERE tenant_id=999 AND account_id=999 AND channel_id=1 AND profile_id=?", ids[1])
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishChannelProfileTable+" WHERE tenant_id=999 AND account_id=999 AND channel_id=1 AND profile_id IN(?) AND status='active' AND next_cycle_at>NOW()", 2, ids[:2])
	// 显式inactive关系不能被历史成功记录复活。
	exec("UPDATE "+publishChannelProfileTable+" SET status='inactive',next_cycle_at=NULL WHERE tenant_id=999 AND account_id=999 AND channel_id=2 AND profile_id=?", ids[0])
	// 单独频道只绑定六条初始资料，避免其他夹具影响轮次集合。
	exec("INSERT INTO " + publishChannelTable + " SELECT (jsonb_populate_record(NULL::" + publishChannelTable + ",to_jsonb(c)||jsonb_build_object('id',999997,'target_chat_id','-100999999997','channel_username','local_cycle_coverage','is_default_selected',0,'cycle_publish_enabled',0,'cycle_publish_mode','batch','cycle_batch_size',2,'cycle_batch_cursor',0,'cycle_batch_cursor_kind','profile','cycle_round_end_id',0,'cycle_active_run_id',0,'cycle_next_run_at',NULL))).* FROM " + publishChannelTable + " c WHERE c.id=1 ON CONFLICT(id) DO UPDATE SET status=1,cycle_publish_enabled=0,cycle_publish_mode='batch',cycle_batch_size=2,cycle_batch_cursor=0,cycle_round_end_id=0,cycle_active_run_id=0,is_default_selected=0")
	t.Cleanup(func() {
		_, _ = g.DB().Exec(context.Background(), "DELETE FROM "+publishProfileChannelTable+" WHERE tenant_id=999 AND channel_id=999997")
	})
	bind := func(profile int64) {
		exec("INSERT INTO "+publishProfileChannelTable+"(tenant_id,account_id,profile_id,channel_id,is_manual) VALUES(999,999,?,999997,1) ON CONFLICT DO NOTHING", profile)
	}
	for _, p := range ids[:6] {
		bind(p)
	}
	exec("DELETE FROM "+publishProfileChannelTable+" WHERE tenant_id=999 AND channel_id=999997 AND profile_id NOT IN(?)", ids[:6])
	var runIDs []int64
	for batch := 0; batch < 4; batch++ {
		if batch == 1 {
			// 本轮开始后持续加入更大ID的资料，不能挤走本轮尚未轮到的旧资料。
			for _, p := range ids[6:] {
				bind(p)
			}
		}
		base, err := g.DB().GetValue(ctx, "SELECT COALESCE(max(id),0) FROM "+publishCycleRunTable)
		if err != nil {
			t.Fatal(err)
		}
		exec("UPDATE " + publishChannelTable + " SET cycle_publish_enabled=1,cycle_batch_time='23:59',cycle_next_run_at=NOW() WHERE id=999997")
		waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishCycleRunTable+" WHERE channel_id=999997 AND id>? AND status='finished'", 1, base.Int64())
		v, err := g.DB().GetValue(ctx, "SELECT max(id) FROM "+publishCycleRunTable+" WHERE channel_id=999997")
		if err != nil {
			t.Fatal(err)
		}
		runIDs = append(runIDs, v.Int64())
		waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE channel_id=999997 AND operation_no LIKE ? AND status='sent'", 2, fmt.Sprintf("cycle_batch:%d:%%", v.Int64()))
		if batch == 0 {
			waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishChannelTable+" WHERE id=999997 AND cycle_round_end_id=?", 1, ids[5])
		}
	}
	for _, p := range ids[:6] {
		waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE channel_id=999997 AND profile_id=? AND operation_no LIKE 'cycle_batch:%' AND id IN(SELECT j.id FROM "+publishTgJobTable+" j WHERE j.operation_no LIKE ? OR j.operation_no LIKE ? OR j.operation_no LIKE ?) AND status IN ('sent','superseded')", 1, p, fmt.Sprintf("cycle_batch:%d:%%", runIDs[0]), fmt.Sprintf("cycle_batch:%d:%%", runIDs[1]), fmt.Sprintf("cycle_batch:%d:%%", runIDs[2]))
	}
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE channel_id=999997 AND profile_id IN(?) AND operation_no LIKE ? AND status='sent'", 2, ids[:2], fmt.Sprintf("cycle_batch:%d:%%", runIDs[3]))
	// 本轮游标之后的资料全部取消绑定，下一次应直接回绕，不能空过一个调度日。
	exec("DELETE FROM "+publishProfileChannelTable+" WHERE tenant_id=999 AND channel_id=999997 AND profile_id IN(?)", ids[2:])
	tailBase, err := g.DB().GetValue(ctx, "SELECT max(id) FROM "+publishCycleRunTable)
	if err != nil {
		t.Fatal(err)
	}
	exec("UPDATE " + publishChannelTable + " SET cycle_next_run_at=NOW() WHERE id=999997")
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishCycleRunTable+" WHERE channel_id=999997 AND id>? AND status='finished' AND total_count=2", 1, tailBase.Int64())
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishChannelProfileTable+" WHERE tenant_id=999 AND account_id=999 AND channel_id=2 AND profile_id=? AND status='inactive'", 1, ids[0])
	t.Log("batch completed finite old-profile round despite new arrivals; next round restarted at oldest profiles; missing plans repaired without reviving inactive mapping")
}
