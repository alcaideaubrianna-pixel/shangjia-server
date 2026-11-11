package sys

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"hotgo/internal/library/cache"
)

// logLocalCycleGaps 记录真实发送时间的频道内间隔，只统计本轮已确认发送，不把Hook吞吐当成TG容量。
func logLocalCycleGaps(t *testing.T, ctx context.Context, base int64, prefix string) {
	t.Helper()
	value, err := g.DB().GetValue(ctx, "WITH gaps AS(SELECT extract(epoch FROM sent_at-lag(sent_at) OVER(PARTITION BY channel_id ORDER BY sent_at,id)) AS gap FROM "+publishTgJobTable+" WHERE tenant_id=999 AND account_id=999 AND id>? AND operation_no LIKE ? AND sent_at IS NOT NULL) SELECT json_build_object('p50_seconds',percentile_cont(.5) WITHIN GROUP(ORDER BY gap),'p95_seconds',percentile_cont(.95) WITHIN GROUP(ORDER BY gap))::text FROM gaps", base, prefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s actual channel gaps: %s", prefix, value.String())
}

// TestLocalCycleLoadRuntime 在真实角色中生成1000份资料的首次发布和到期循环，按资料集合核验覆盖。
func TestLocalCycleLoadRuntime(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_CYCLE_LOAD") != "1" {
		t.Skip("explicit local 1000-profile load run")
	}
	_ = localAccountRuntimeContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	s := NewSysPublish()
	profiles := localRuntimeProfiles(t, ctx, 0, 1000)
	exec := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := g.DB().Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO hg_youban_publish_tenant_vip(tenant_id,level,status,expired_at) VALUES(999,1,1,NOW()+interval '1 day') ON CONFLICT(tenant_id) WHERE deleted_at IS NULL DO UPDATE SET level=1,status=1,expired_at=EXCLUDED.expired_at")
	_, _ = cache.Instance().Remove(ctx, tenantVipCacheKey(999))
	exec("UPDATE " + publishChannelTable + " SET cycle_publish_enabled=1,cycle_publish_mode='time',cycle_publish_days=30,cycle_publish_time='' WHERE tenant_id=999 AND tg_account_id=999 AND id BETWEEN 1 AND 100")
	t.Cleanup(func() {
		_, _ = g.DB().Exec(context.Background(), "UPDATE "+publishChannelTable+" SET cycle_publish_enabled=0 WHERE tenant_id=999 AND id BETWEEN 1 AND 100")
	})
	tag := fmt.Sprintf("profile:cycle-load:%d", time.Now().UnixNano())
	started := time.Now()
	ids := make([]int64, 0, 1000)
	for _, p := range profiles {
		ids = append(ids, p.ID)
	}
	// 注入阶段只负责创建持久化任务；使用有限 worker 并发提交，避免
	// 测试本身的逐条调用把真实角色吞吐掩盖成串行耗时。发送与循环仍由
	// Docker 中的真实 scheduler/publish/background 角色完成并核验。
	const submitWorkers = 16
	tasks := make(chan int)
	errs := make(chan error, submitWorkers)
	var wg sync.WaitGroup
	for worker := 0; worker < submitWorkers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range tasks {
				if err := s.submitProfilePublish(ctx, profiles[i].ID, 999, 999, 999, fmt.Sprintf("%s:%d", tag, profiles[i].ID), []int64{int64(i%100 + 1)}, false); err != nil {
					select {
					case errs <- err:
					default:
					}
				}
			}
		}()
	}
	for i := range profiles {
		tasks <- i
	}
	close(tasks)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE tenant_id=999 AND operation_no LIKE ? AND status='sent'", 1000, tag+":%")
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishChannelProfileTable+" cp JOIN "+publishTgJobTable+" j ON j.id=cp.last_job_id WHERE cp.tenant_id=999 AND cp.next_cycle_at>NOW() AND j.operation_no LIKE ?", 1000, tag+":%")
	t.Logf("1000 actual initial publishes completed in %s", time.Since(started))
	base, err := g.DB().GetValue(ctx, "SELECT max(id) FROM "+publishTgJobTable)
	if err != nil {
		t.Fatal(err)
	}
	exec("UPDATE " + publishChannelTable + " SET cycle_publish_enabled=1 WHERE tenant_id=999 AND tg_account_id=999 AND id BETWEEN 1 AND 100")
	started = time.Now()
	exec("UPDATE "+publishChannelProfileTable+" cp SET next_cycle_at=NOW()-interval '1 second' FROM "+publishTgJobTable+" j WHERE j.id=cp.last_job_id AND cp.tenant_id=999 AND cp.account_id=999 AND j.operation_no LIKE ?", tag+":%")
	waitLocalRuntimeCount(t, ctx, "SELECT count(DISTINCT profile_id) FROM "+publishTgJobTable+" WHERE tenant_id=999 AND account_id=999 AND id>? AND profile_id IN(?) AND operation_no LIKE 'profile_cycle:%' AND status='sent'", 1000, base.Int64(), ids)
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM (SELECT profile_id FROM "+publishTgJobTable+" WHERE tenant_id=999 AND account_id=999 AND id>? AND profile_id IN(?) AND operation_no LIKE 'profile_cycle:%' AND status='sent' GROUP BY profile_id HAVING count(*)=1) covered", 1000, base.Int64(), ids)
	t.Logf("1000 actual cycles across 100 channels completed in %s, all profiles covered exactly once", time.Since(started))
	logLocalCycleGaps(t, ctx, base.Int64(), "profile_cycle:%")
	// 再验证100频道同时批次翻新同一批旧资料，覆盖热点资料状态竞争。
	type pair struct {
		ProfileID int64 `orm:"profile_id"`
		ChannelID int64 `orm:"channel_id"`
	}
	expected := make(map[string]bool)
	for channel := int64(1); channel <= 100; channel++ {
		var candidates []struct {
			ID int64 `orm:"id"`
		}
		if err := s.batchCycleEligibleProfileModel(ctx, 999, channel).Fields("p.id").OrderAsc("p.id").Limit(10).Scan(&candidates); err != nil {
			t.Fatal(err)
		}
		for _, p := range candidates {
			expected[fmt.Sprintf("%d:%d", p.ID, channel)] = true
		}
	}
	if len(expected) != 1000 {
		t.Fatalf("batch fixtures missing: %d", len(expected))
	}
	runBase, err := g.DB().GetValue(ctx, "SELECT max(id) FROM "+publishCycleRunTable)
	if err != nil {
		t.Fatal(err)
	}
	jobBase, err := g.DB().GetValue(ctx, "SELECT max(id) FROM "+publishTgJobTable)
	if err != nil {
		t.Fatal(err)
	}
	started = time.Now()
	exec("UPDATE " + publishChannelTable + " SET cycle_publish_enabled=1,cycle_publish_mode='batch',cycle_batch_size=10,cycle_batch_time='23:59',cycle_batch_cursor=0,cycle_batch_cursor_kind='profile',cycle_round_end_id=0,cycle_active_run_id=0,cycle_next_run_at=NOW() WHERE tenant_id=999 AND tg_account_id=999 AND id BETWEEN 1 AND 100")
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishCycleRunTable+" WHERE tenant_id=999 AND channel_id BETWEEN 1 AND 100 AND id>? AND status='finished'", 100, runBase.Int64())
	var actual []pair
	if err := g.DB().Model(publishTgJobTable).Ctx(ctx).Where("tenant_id", 999).Where("account_id", 999).WhereGT("id", jobBase.Int64()).Where("operation_no LIKE 'cycle_batch:%'").Where("status", "sent").Fields("profile_id,channel_id").Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if len(actual) != len(expected) {
		t.Fatalf("batch coverage count: got %d expected %d", len(actual), len(expected))
	}
	for _, item := range actual {
		key := fmt.Sprintf("%d:%d", item.ProfileID, item.ChannelID)
		if !expected[key] {
			t.Fatalf("unexpected or duplicate batch pair %s", key)
		}
		delete(expected, key)
	}
	if len(expected) != 0 {
		t.Fatalf("missing batch pairs: %v", expected)
	}
	t.Logf("1000 actual batch sends across 100 channels completed in %s; exact expected profile/channel set covered", time.Since(started))
	logLocalCycleGaps(t, ctx, jobBase.Int64(), "cycle_batch:%")
}
