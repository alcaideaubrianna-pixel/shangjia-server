package sys

import (
	"context"
	"fmt"
	"github.com/gogf/gf/v2/os/gcfg"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"hotgo/internal/bootstrap/envconfig"
	"hotgo/internal/library/cache"
)

// TestLocalCycleProtectionRuntime 使用真实Hook失权响应验证频道隔离，再验证免费天数与闲置保护。
func TestLocalCycleProtectionRuntime(t *testing.T) {
	ctx := localAccountRuntimeContext(t)
	s := NewSysPublish()
	exec := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := g.DB().Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	setHook := func(body string) error {
		request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://127.0.0.1:19100/__test/scenario", strings.NewReader(body))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 5 * time.Second}
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			return fmt.Errorf("hook scenario status %d", response.StatusCode)
		}
		return nil
	}
	t.Cleanup(func() {
		_ = setHook(`{"default":{"action":"success","delayMs":5}}`)
		_, _ = g.DB().Exec(context.Background(), "UPDATE "+publishChannelTable+" SET status=0,cycle_publish_enabled=0 WHERE id=999996")
	})
	exec("INSERT INTO " + publishChannelTable + " SELECT (jsonb_populate_record(NULL::" + publishChannelTable + ",to_jsonb(c)||jsonb_build_object('id',999996,'target_chat_id','-100999999996','channel_username','local_permission_guard','is_default_selected',0,'cycle_publish_enabled',1,'cycle_publish_mode','time','cycle_active_run_id',0))).* FROM " + publishChannelTable + " c WHERE c.id=1 ON CONFLICT(id) DO UPDATE SET status=1,cycle_publish_enabled=1")
	exec("DELETE FROM " + channelPermissionCircuitTable + " WHERE tenant_id=999 AND channel_id=999996")
	profiles := localRuntimeProfiles(t, ctx, 310, 3)
	tag := fmt.Sprintf("profile:permission:%d", time.Now().UnixNano())
	if err := s.submitProfilePublish(ctx, profiles[0].ID, 999, 999, 999, tag+":baseline", []int64{999996}, false); err != nil {
		t.Fatal(err)
	}
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE operation_no=? AND status='sent'", 1, tag+":baseline")
	// 持有旧资料生命周期锁时，新循环准备仍应完成，证明兄弟频道不再争用该Redis锁。
	proofOp := "profile_cycle:lock-proof:" + tag
	if err := s.withProfileLifecycleLock(ctx, 999, profiles[0].ID, func() error {
		bounded, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		return s.submitProfilePublish(bounded, profiles[0].ID, 999, 999, 999, proofOp, []int64{999996}, true)
	}); err != nil {
		t.Fatalf("new cycle still waits on old profile lifecycle lock: %v", err)
	}
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE operation_no=? AND status='sent'", 1, proofOp)
	var rules []string
	for _, method := range []string{"sendMessage", "sendPhoto", "sendMediaGroup", "sendVideo"} {
		rules = append(rules, fmt.Sprintf(`{"method":%q,"chatId":"-100999999996","action":"error","code":403,"message":"Forbidden: bot was kicked from the channel chat"}`, method))
	}
	if err := setHook(`{"default":{"action":"success","delayMs":5},"rules":[` + strings.Join(rules, ",") + `]}`); err != nil {
		t.Fatal(err)
	}
	if err := s.submitProfilePublish(ctx, profiles[1].ID, 999, 999, 999, tag+":lost", []int64{1, 999996}, false); err != nil {
		t.Fatal(err)
	}
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+channelPermissionCircuitTable+" WHERE tenant_id=999 AND channel_id=999996 AND paused=1", 1)
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE operation_no=? AND channel_id=1 AND status='sent'", 1, tag+":lost")
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishChannelTable+" WHERE id=999996 AND bot_permission_status_json LIKE '%kicked%'", 1)
	if err := s.submitProfilePublish(ctx, profiles[2].ID, 999, 999, 999, tag+":paused", []int64{1, 999996}, false); err != nil {
		t.Fatal(err)
	}
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE operation_no=? AND channel_id=1 AND status='sent'", 1, tag+":paused")
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgAttemptTable+" a JOIN "+publishTgJobTable+" j ON j.id=a.job_id WHERE j.operation_no=? AND j.channel_id=999996", 0, tag+":paused")
	if err := setHook(`{"default":{"action":"success","delayMs":5}}`); err != nil {
		t.Fatal(err)
	}
	// 保存并恢复本地租户配置；不改生产，不把后台自动活动当作用户操作。
	vip, err := g.DB().Model("hg_youban_publish_tenant_vip").Ctx(ctx).Where("tenant_id", 999).One()
	if err != nil {
		t.Fatal(err)
	}
	activity, err := g.DB().Model(publishAccountTable).Ctx(ctx).Where("id", 999).Fields("last_active_at").Value()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = g.DB().Exec(context.Background(), "UPDATE hg_youban_publish_tenant_vip SET expired_at=? WHERE tenant_id=999", vip["expired_at"].Interface())
		_, _ = g.DB().Exec(context.Background(), "UPDATE "+publishAccountTable+" SET last_active_at=? WHERE id=999", activity.Interface())
		_, _ = cache.Instance().Remove(context.Background(), tenantVipCacheKey(999))
	})
	exec("UPDATE hg_youban_publish_tenant_vip SET expired_at=NOW()-interval '1 day' WHERE tenant_id=999")
	_, _ = cache.Instance().Remove(ctx, tenantVipCacheKey(999))
	t.Setenv("YOUBAN_PUBLISH_CYCLE_FREE_INTERVAL_DAYS", "2")
	t.Setenv("YOUBAN_PUBLISH_CYCLE_FREE_INACTIVE_DAYS", "2")
	// ENV启动覆盖使用与服务相同的AdapterFile；普通测试AdapterContent不执行该启动映射。
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"cache":{"adapter":"redis"},"redis":{"default":{"address":"127.0.0.1:16382","db":15}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	adapter, err := gcfg.NewAdapterFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	envconfig.Apply(ctx)
	if days := s.effectiveChannelCycleDays(ctx, 999, 1); days != 2 {
		t.Fatalf("free days=%d", days)
	}
	exec("UPDATE " + publishAccountTable + " SET last_active_at=NOW()-interval '3 days' WHERE id=999")
	if err := s.pauseInactiveFreeChannelCycles(ctx, publishAccountScope{TenantID: 999, AccountID: 999}); err != nil {
		t.Fatal(err)
	}
	waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishChannelTable+" WHERE id=999996 AND cycle_publish_enabled=0", 1)
	allowed, err := cycleChannelAllowed(ctx, 999, 999996, "profile_cycle:stopped")
	if err != nil || allowed {
		t.Fatalf("inactive cycle escaped guard: %v %v", allowed, err)
	}
	allowed, err = cycleChannelAllowed(ctx, 999, 999996, "profile:manual")
	if err != nil || !allowed {
		t.Fatalf("manual incorrectly blocked by cycle guard: %v %v", allowed, err)
	}
	t.Log("single permission failure paused only target channel; later attempts blocked; free interval and inactivity protection reused")
}
