package sys

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// TestLocalMultiAccountLoadRuntime 用五个独立账户、十五个频道验证账户级并发。
// 每个账户准备100份资料，发送入口仍走真实 submitProfilePublish，发送由Docker角色完成。
func TestLocalMultiAccountLoadRuntime(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_MULTI_ACCOUNT_LOAD") != "1" {
		t.Skip("requires local multi-account runtime")
	}
	ctx := localAccountRuntimeContext(t)
	profiles := localRuntimeProfiles(t, ctx, 0, 500)
	accounts := []int64{999, 999998, 999999, 1000000, 1000001}
	channelGroups := [][]int64{{1, 2, 3}, {2001, 2002, 2003}, {2011, 2012, 2013}, {2021, 2022, 2023}, {2031, 2032, 2033}}
	exec := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := g.DB().Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	// 克隆业务账户、账户设置和频道；频道 tg_account_id 必须指向业务账户，
	// 这样账户调度的 SQL 过滤和线上模型一致。
	for i, account := range accounts[1:] {
		exec(fmt.Sprintf("INSERT INTO %s SELECT (jsonb_populate_record(NULL::%s,to_jsonb(a)||jsonb_build_object('id',%d,'username','local_multi_%d','nickname','本地并发账户%d','status',1))).* FROM %s a WHERE a.id=999 ON CONFLICT(id) DO UPDATE SET status=1", publishAccountTable, publishAccountTable, account, account, i+2, publishAccountTable))
		exec(fmt.Sprintf("INSERT INTO %s SELECT (jsonb_populate_record(NULL::%s,to_jsonb(a)||jsonb_build_object('id',%d,'account_id',%d,'push_engine','unified_stream'))).* FROM %s a WHERE a.account_id=999 AND a.tenant_id=999 LIMIT 1 ON CONFLICT(id) DO UPDATE SET push_engine='unified_stream'", publishAccountSettingTable, publishAccountSettingTable, account, account, publishAccountSettingTable))
		for _, channel := range channelGroups[i+1] {
			exec(fmt.Sprintf("INSERT INTO %s SELECT (jsonb_populate_record(NULL::%s,to_jsonb(c)||jsonb_build_object('id',%d,'tg_account_id',%d,'target_chat_id','-100999%06d','channel_title','local-multi-%d','cycle_publish_enabled',0,'is_default_selected',1,'status',1))).* FROM %s c WHERE c.id=1 ON CONFLICT(id) DO UPDATE SET tg_account_id=EXCLUDED.tg_account_id,status=1", publishChannelTable, publishChannelTable, channel, account, channel, channel, publishChannelTable))
		}
	}
	for i, account := range accounts {
		if i > 0 {
			exec("UPDATE "+publishProfileStateTable+" SET account_id=? WHERE tenant_id=999 AND profile_id BETWEEN ? AND ?", account, profiles[i*100].ID, profiles[i*100+99].ID)
		}
		for n := 0; n < 100; n++ {
			channel := channelGroups[i][n%3]
			if i > 0 {
				exec("INSERT INTO "+publishProfileChannelTable+"(tenant_id,account_id,profile_id,channel_id,is_manual,created_at,updated_at) VALUES(999,?,?,?,0,NOW(),NOW()) ON CONFLICT DO NOTHING", account, profiles[i*100+n].ID, channel)
			}
		}
	}
	t.Cleanup(func() {
		for i, account := range accounts[1:] {
			exec("DELETE FROM "+publishProfileChannelTable+" WHERE account_id=? AND channel_id BETWEEN ? AND ?", account, channelGroups[i+1][0], channelGroups[i+1][2])
			exec("UPDATE "+publishProfileStateTable+" SET account_id=999 WHERE tenant_id=999 AND profile_id BETWEEN ? AND ?", profiles[(i+1)*100].ID, profiles[(i+1)*100+99].ID)
			exec("DELETE FROM "+publishChannelTable+" WHERE id IN (?,?,?)", channelGroups[i+1][0], channelGroups[i+1][1], channelGroups[i+1][2])
			exec("DELETE FROM "+publishAccountSettingTable+" WHERE account_id=?", account)
			exec("DELETE FROM "+publishAccountTable+" WHERE id=?", account)
		}
	})
	tag := fmt.Sprintf("profile:multi-account-load:%d", time.Now().UnixNano())
	s := NewSysPublish()
	type work struct{ account, channel, profile int64 }
	jobs := make(chan work, len(profiles))
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex
	for worker := 0; worker < 20; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				if err := s.submitProfilePublish(ctx, item.profile, 999, item.account, item.account, fmt.Sprintf("%s:%d", tag, item.profile), []int64{item.channel}, false); err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
				}
			}
		}()
	}
	for i, p := range profiles {
		jobs <- work{accounts[i/100], channelGroups[i/100][i%3], p.ID}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		t.Fatal(firstErr)
	}
	for _, account := range accounts {
		waitLocalRuntimeCount(t, ctx, "SELECT count(*) FROM "+publishTgJobTable+" WHERE tenant_id=999 AND account_id=? AND operation_no LIKE ? AND status='sent'", 100, account, tag+":%")
	}
	t.Logf("five-account concurrent load passed: 5 accounts, 15 channels, 500 profile jobs")
}
