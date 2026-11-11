package sys

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
)

type localRuntimeProfile struct {
	ID int64 `orm:"profile_id"`
}

// localRuntimeProfiles 从完整夹具读取资料，不依赖导入时分配的固定主键。
func localRuntimeProfiles(t *testing.T, ctx context.Context, offset, count int) []localRuntimeProfile {
	t.Helper()
	var profiles []localRuntimeProfile
	if err := g.DB().Model(publishProfileStateTable+" ps").Ctx(ctx).InnerJoin(dao.ContentProfile.Table()+" p", "p.id=ps.profile_id AND p.deleted_at IS NULL").
		Where("ps.tenant_id", 999).Where("ps.account_id", 999).Where("p.source_type", "local_regression").Fields("ps.profile_id").OrderAsc("ps.profile_id").Offset(offset).Limit(count).Scan(&profiles); err != nil {
		t.Fatal(err)
	}
	if len(profiles) != count {
		t.Fatal("missing complete local profile fixtures", len(profiles), count)
	}
	return profiles
}

// TestLocalAccountIntervalRuntime 两个账户并发提交真实任务，分别验证1秒与3秒间隔，不共用账户限速状态。
func TestLocalAccountIntervalRuntime(t *testing.T) {
	ctx := localAccountRuntimeContext(t)
	const secondAccount int64 = 999998
	primaryProfiles := localRuntimeProfiles(t, ctx, 5, 4)
	secondaryProfiles := localRuntimeProfiles(t, ctx, 105, 4)
	secondaryIDs := make([]int64, 0, len(secondaryProfiles))
	for _, profile := range secondaryProfiles {
		secondaryIDs = append(secondaryIDs, profile.ID)
	}
	// 克隆完整本地夹具，保留媒体/频道/Bot配置；不连接生产库。
	queries := []string{
		"INSERT INTO " + publishAccountTable + " SELECT (jsonb_populate_record(NULL::" + publishAccountTable + ",to_jsonb(a)||jsonb_build_object('id',999998,'username','local_interval_999998','nickname','本地间隔验证','status',1))).* FROM " + publishAccountTable + " a WHERE a.id=999 ON CONFLICT(id) DO UPDATE SET status=1",
		"INSERT INTO " + publishAccountSettingTable + " SELECT (jsonb_populate_record(NULL::" + publishAccountSettingTable + ",to_jsonb(a)||jsonb_build_object('id',999998,'account_id',999998,'publish_config_json','{\"sendIntervalSeconds\":3}'))).* FROM " + publishAccountSettingTable + " a WHERE a.account_id=999 AND a.tenant_id=999 ON CONFLICT(id) DO UPDATE SET publish_config_json=EXCLUDED.publish_config_json,push_engine=EXCLUDED.push_engine",
		"INSERT INTO " + publishChannelTable + " SELECT (jsonb_populate_record(NULL::" + publishChannelTable + ",to_jsonb(c)||jsonb_build_object('id',999998,'tg_account_id',999998,'target_chat_id','-100999999998','channel_username','local_interval_999998','cycle_publish_enabled',0,'is_default_selected',0,'status',1))).* FROM " + publishChannelTable + " c WHERE c.id=1 ON CONFLICT(id) DO UPDATE SET status=1",
	}
	for _, query := range queries {
		if _, err := g.DB().Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := g.DB().Model(publishProfileStateTable).Ctx(ctx).Where("tenant_id", 999).WhereIn("profile_id", secondaryIDs).Data(g.Map{"account_id": secondAccount, "publish_at": nil}).Update(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = g.DB().Exec(context.Background(), "UPDATE "+publishAccountTable+" SET status=0 WHERE id=999998")
		_, _ = g.DB().Exec(context.Background(), "UPDATE "+publishChannelTable+" SET status=0 WHERE id=999998")
		_, _ = g.DB().Model(publishProfileStateTable).Ctx(context.Background()).Where("tenant_id", 999).WhereIn("profile_id", secondaryIDs).Data(g.Map{"account_id": 999}).Update()
	})
	clearPushEngineCache(ctx, 999, secondAccount)
	clearTelegramPublishConfigCache(ctx, 999, secondAccount)
	clearTelegramPublishConfigCache(ctx, 999, 999)
	s := NewSysPublish()
	tag := fmt.Sprintf("profile:interval:%d", time.Now().UnixNano())
	for index, primary := range primaryProfiles {
		profile := primary.ID
		for _, target := range []struct{ account, channel int64 }{{999, 1}, {secondAccount, 999998}} {
			actualProfile := profile
			if target.account == secondAccount {
				actualProfile = secondaryProfiles[index].ID
			}
			if err := s.submitProfilePublish(ctx, actualProfile, 999, target.account, target.account, fmt.Sprintf("%s:%d:%d", tag, target.account, actualProfile), []int64{target.channel}, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE tenant_id=999 AND operation_no LIKE ? AND status='sent'", 8, tag+":%")
	for _, account := range []int64{999, secondAccount} {
		minimum := 1.0
		if account == secondAccount {
			minimum = 3
		}
		value, err := g.DB().GetValue(ctx, "WITH gaps AS (SELECT sent_at-lag(sent_at) OVER(ORDER BY id) gap FROM "+publishTgJobTable+" WHERE tenant_id=999 AND account_id=? AND operation_no LIKE ?) SELECT extract(epoch FROM min(gap)) FROM gaps", account, tag+":%")
		if err != nil || value.Float64() < minimum-.05 {
			t.Fatal("account interval violated", account, value, err)
		}
		t.Logf("account=%d configured=%fs minimum_actual=%fs", account, minimum, value.Float64())
	}
}

// localAccountRuntimeContext 只允许显式开启固定本地库的实际运行测试。
func localAccountRuntimeContext(t *testing.T) context.Context {
	t.Helper()
	if os.Getenv("YOUBAN_LOCAL_ACCOUNT_RUNTIME") != "1" {
		t.Skip("requires local Docker roles")
	}
	t.Setenv("YOUBAN_PUBLISH_PUSH_ENGINE", pushEngineUnifiedStream)
	if os.Getenv("YOUBAN_TELEGRAM_RUNTIME_API_URL") == "" {
		t.Setenv("YOUBAN_TELEGRAM_RUNTIME_API_URL", "http://127.0.0.1:18790")
	}
	if os.Getenv("YOUBAN_TELEGRAM_BOT_API_URL") == "" {
		t.Setenv("YOUBAN_TELEGRAM_BOT_API_URL", "http://127.0.0.1:18790")
	}
	t.Setenv("YOUBAN_TELEGRAM_LOCAL_BOT_ENABLED", "1")
	previousConfig := g.Cfg().GetAdapter()
	t.Cleanup(func() { g.Cfg().SetAdapter(previousConfig) })
	redisAddress := os.Getenv("YOUBAN_REDIS_ADDRESS")
	if redisAddress == "" {
		redisAddress = "127.0.0.1:16382"
	}
	redisDB := os.Getenv("YOUBAN_REDIS_DB")
	if redisDB == "" {
		redisDB = "15"
	}
	adapter, err := gcfg.NewAdapterContent(fmt.Sprintf(`{"cache":{"adapter":"redis"},"redis":{"default":{"address":"%s","db":%s}}}`, redisAddress, redisDB))
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	previousDB := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previousDB) })
	databaseName := os.Getenv("YOUBAN_LOCAL_DB_NAME")
	if databaseName == "" {
		databaseName = "youban_tgfake_test"
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "5432", User: "postgres", Pass: "postgres", Name: databaseName}})
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)
	cache.SetAdapter(ctx)
	return ctx
}

// waitLocalRuntimeCount 等待真实角色产生数据库结果；不直接推进任务状态。
func waitLocalRuntimeCount(t *testing.T, ctx context.Context, query string, expected int, args ...interface{}) {
	t.Helper()
	for {
		value, err := g.DB().GetValue(ctx, query, args...)
		if err != nil {
			t.Fatal(err)
		}
		if value.Int() == expected {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("runtime count timeout expected=%d actual=%s query=%s", expected, value.String(), query)
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// TestLocalAccountCycleRuntime 验证实际scheduler触发多频道循环、配置修改及批次游标，TG只经过本地Hook。
func TestLocalAccountCycleRuntime(t *testing.T) {
	ctx := localAccountRuntimeContext(t)
	// 会员权限使用真实表和既有加载链路，不跳过批次入口的权限检查。
	if _, err := g.DB().Exec(ctx, "INSERT INTO hg_youban_publish_tenant_vip(tenant_id,level,status,expired_at) VALUES(999,1,1,NOW()+interval '1 day') ON CONFLICT(tenant_id) WHERE deleted_at IS NULL DO UPDATE SET level=1,status=1,expired_at=EXCLUDED.expired_at"); err != nil {
		t.Fatal(err)
	}
	_, _ = cache.Instance().Remove(ctx, tenantVipCacheKey(999))
	// 仅修改专用夹具频道，结束后关闭，避免后续场景相互污染。
	t.Cleanup(func() {
		_, _ = g.DB().Exec(context.Background(), "UPDATE "+publishChannelTable+" SET cycle_publish_enabled=0,cycle_publish_mode='time',cycle_active_run_id=0 WHERE tenant_id=999 AND id IN (1,2,3)")
	})
	_, err := g.DB().Exec(ctx, "UPDATE "+publishChannelTable+" SET cycle_publish_enabled=1,cycle_publish_mode='time',cycle_publish_days=3,cycle_publish_time='23:50' WHERE tenant_id=999 AND id IN (1,2,3)")
	if err != nil {
		t.Fatal(err)
	}
	// 循环必须基于上一轮已发送记录计算；不能选择尚未建立 last_job_id 的空资料。
	var cycleProfile struct {
		ID int64 `orm:"profile_id"`
	}
	if err := g.DB().Model(publishProfileStateTable+" ps").Ctx(ctx).
		Where("ps.tenant_id", 999).Where("ps.account_id", 999).
		Where("EXISTS (SELECT 1 FROM " + publishChannelProfileTable + " cp WHERE cp.tenant_id=ps.tenant_id AND cp.account_id=ps.account_id AND cp.profile_id=ps.profile_id AND cp.channel_id IN (1,2,3) AND cp.last_job_id>0)").
		Fields("ps.profile_id").OrderAsc("ps.profile_id").Limit(1).Scan(&cycleProfile); err != nil {
		t.Fatal(err)
	}
	if cycleProfile.ID <= 0 {
		t.Fatal("missing cycle baseline profile")
	}
	profileID := cycleProfile.ID
	// 统一把基线任务标记为可循环，避免前序手工/编辑场景留下的 cycle_enabled=0 影响本用例。
	if _, err := g.DB().Exec(ctx, "UPDATE "+publishTgJobTable+" j SET cycle_enabled=1,cycle_days=3,cycle_publish_time='23:50',next_cycle_at=NOW()-interval '1 second' WHERE j.tenant_id=999 AND j.account_id=999 AND j.profile_id=? AND j.id IN (SELECT cp.last_job_id FROM "+publishChannelProfileTable+" cp WHERE cp.profile_id=? AND cp.channel_id IN (1,2,3))", profileID, profileID); err != nil {
		t.Fatal(err)
	}
	var baseline int64
	v, err := g.DB().GetValue(ctx, "SELECT COALESCE(max(id),0) FROM "+publishTgJobTable)
	if err != nil {
		t.Fatal(err)
	}
	baseline = v.Int64()
	_, err = g.DB().Exec(ctx, "UPDATE "+publishChannelProfileTable+" cp SET next_cycle_at=NOW()-interval '1 second' FROM "+publishTgJobTable+" j WHERE cp.last_job_id=j.id AND cp.tenant_id=999 AND cp.account_id=999 AND cp.profile_id=? AND cp.channel_id IN (1,2,3)", profileID)
	if err != nil {
		t.Fatal(err)
	}
	query := "SELECT count(*) FROM " + publishTgJobTable + " WHERE tenant_id=999 AND account_id=999 AND profile_id=? AND id>? AND status='sent' AND operation_no LIKE 'profile_cycle:account:%'"
	waitLocalRuntimeCount(t, ctx, query, 3, profileID, baseline)
	value, err := g.DB().GetValue(ctx, "SELECT count(DISTINCT operation_no) FROM "+publishTgJobTable+" WHERE tenant_id=999 AND account_id=999 AND profile_id=? AND id>?", profileID, baseline)
	if err != nil || value.Int() != 1 {
		t.Fatal("cycle did not share one operation", value, err)
	}
	// 本轮已逾期任务正常发送，后续计划必须采用新配置的三天和23:50。
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE tenant_id=999 AND account_id=999 AND profile_id=? AND id>? AND cycle_enabled=1 AND next_cycle_at>NOW()+interval '2 days' AND cycle_publish_time='23:50'", 3, profileID, baseline)
	v, err = g.DB().GetValue(ctx, "SELECT max(id) FROM "+publishTgJobTable)
	if err != nil {
		t.Fatal(err)
	}
	baseline = v.Int64()
	if _, err = g.DB().Exec(ctx, "UPDATE "+publishChannelTable+" SET cycle_publish_enabled=0 WHERE tenant_id=999 AND id=2"); err != nil {
		t.Fatal(err)
	}
	if _, err = g.DB().Exec(ctx, "UPDATE "+publishChannelTable+" SET cycle_publish_time='22:40' WHERE tenant_id=999 AND id IN (1,3)"); err != nil {
		t.Fatal(err)
	}
	s := NewSysPublish()
	for _, channel := range []int64{1, 2, 3} {
		if err := s.syncChannelCycleAfterSave(ctx, 999, channel, 1, 3, "22:40"); err != nil {
			t.Fatal(err)
		}
	}
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" j JOIN "+publishChannelProfileTable+" cp ON cp.last_job_id=j.id WHERE cp.account_id=999 AND cp.profile_id=? AND cp.channel_id IN (1,3) AND j.cycle_publish_time='22:40'", 2, profileID)
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" j JOIN "+publishChannelProfileTable+" cp ON cp.last_job_id=j.id WHERE cp.account_id=999 AND cp.profile_id=? AND cp.channel_id=2 AND j.cycle_enabled=0", 1, profileID)
	if _, err = g.DB().Exec(ctx, "UPDATE "+publishChannelProfileTable+" cp SET next_cycle_at=NOW()-interval '1 second' FROM "+publishTgJobTable+" j WHERE cp.last_job_id=j.id AND cp.account_id=999 AND cp.profile_id=? AND cp.channel_id IN (1,2,3)", profileID); err != nil {
		t.Fatal(err)
	}
	waitLocalRuntimeCount(t, ctx, query, 2, profileID, baseline)
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE account_id=999 AND profile_id=? AND id>? AND channel_id=2", 0, profileID, baseline)
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE account_id=999 AND profile_id=? AND id>? AND cycle_publish_time='22:40' AND next_cycle_at>NOW()+interval '2 days'", 2, profileID, baseline)
	// 旧配置的0天和空mode按既有默认规则处理，不能被新计划器直接漏掉。
	defaultBaseline, err := g.DB().GetValue(ctx, "SELECT max(id) FROM "+publishTgJobTable)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.DB().Exec(ctx, "UPDATE "+publishChannelTable+" SET cycle_publish_days=0,cycle_publish_mode='' WHERE tenant_id=999 AND id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.DB().Exec(ctx, "UPDATE "+publishChannelProfileTable+" cp SET next_cycle_at=NOW()-interval '1 second' FROM "+publishTgJobTable+" j WHERE cp.last_job_id=j.id AND cp.account_id=999 AND cp.profile_id=? AND cp.channel_id=1", profileID); err != nil {
		t.Fatal(err)
	}
	waitLocalRuntimeCount(t, ctx, query, 1, profileID, defaultBaseline.Int64())
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE account_id=999 AND profile_id=? AND id>? AND cycle_days=? AND next_cycle_at>NOW()+interval '3 days'", 1, profileID, defaultBaseline.Int64(), defaultCycleDays(0))
	batchBaseline, err := g.DB().GetValue(ctx, "SELECT COALESCE(max(id),0) FROM "+publishCycleRunTable)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = g.DB().Exec(ctx, "UPDATE "+publishChannelTable+" SET cycle_publish_mode='batch',cycle_publish_enabled=1,cycle_batch_size=2,cycle_batch_time='23:59',cycle_batch_cursor=0,cycle_batch_cursor_kind='profile',cycle_active_run_id=0,cycle_next_run_at=NOW() WHERE tenant_id=999 AND id=3"); err != nil {
		t.Fatal(err)
	}
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishCycleRunTable+" WHERE tenant_id=999 AND channel_id=3 AND id>? AND status='finished' AND total_count=2 AND queued_count=2", 1, batchBaseline.Int64())
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishChannelTable+" WHERE tenant_id=999 AND id=3 AND cycle_active_run_id=0 AND cycle_batch_cursor>0 AND cycle_next_run_at>NOW()", 1)
	t.Log("actual time cycles, changed configuration and batch cursor completed")
}
