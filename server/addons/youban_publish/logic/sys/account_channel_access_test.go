package sys

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/hibiken/asynq"
	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	botin "hotgo/addons/youban_bot/model/input/sysin"
	botservice "hotgo/addons/youban_bot/service"
	"hotgo/internal/library/cache"
)

func TestAccountChannelAccessNotificationText(t *testing.T) {
	text := accountChannelAccessText(&tgAccountAuthOwner{Id: 42, DisplayName: "七七<管理员>", TelegramUsername: "qqby66"}, &accountChannelAccessNotification{SourceID: 1357, SourceTitle: "刺客&频道", Event: collectorin.AccountChannelAccessEvent{ChannelID: 2736732070, Title: "刺客<频道>", Reason: collectorin.AccountChannelAccessForbidden}})
	for _, want := range []string{"TG号：七七&lt;管理员&gt;（@qqby66）", "TG账号编号：42", "2736732070", "编号1357", "无法访问", "其他频道不受影响", "历史消息不会自动补采"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "被踢") || strings.Contains(text, "已被移出") {
		t.Fatal("inaccessible must not claim kicked", text)
	}
}

func TestAccountChannelAccessPauseLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL at 127.0.0.1:35432")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	ctx := context.Background()
	if !cache.Initialized() {
		adapter := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
		original := g.Cfg().MustGet(ctx, "cache.adapter").Val()
		if err := adapter.Set("cache.adapter", "memory"); err != nil {
			t.Fatal(err)
		}
		cache.SetAdapter(ctx)
		if err := adapter.Set("cache.adapter", original); err != nil {
			t.Fatal(err)
		}
	}
	previousCache := cache.Instance().GetAdapter()
	cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	t.Cleanup(func() { cache.Instance().SetAdapter(previousCache) })
	previousBot := botservice.SysBot()
	fake := &channelAccessBotTest{ISysBot: previousBot, failOwner: true}
	botservice.RegisterSysBot(fake)
	t.Cleanup(func() { botservice.RegisterSysBot(previousBot) })
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_source(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,tg_account_id bigint,source_type text,source_chat_id text,collect_enabled int,status int,remark text,updated_at timestamp,deleted_at timestamp,title text DEFAULT '') ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account_setting(tenant_id bigint,account_id bigint,channel_alert_enabled int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_account(id bigint,tenant_id bigint,account_id bigint,display_name text,telegram_username text,status text,deleted_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_tg_account VALUES(42,38,512,'七七','qqby66','authorized',NULL)`,
			`INSERT INTO hg_youban_publish_collect_source VALUES
			(109,38,512,42,'account','3691274988',1,1,'原备注','2026-10-24 10:00:00.123456',NULL,'频道'),
			(1357,38,512,42,'account','2736732070',1,1,'','2026-10-24 10:00:00.123456',NULL,'频道')`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		n := &accountChannelAccessNotification{TenantID: 38, AccountID: 512, TGAccountID: 42, SourceID: 109, Version: "2026-10-24 10:00:00.123456", PauseVersion: "2026-10-24 12:00:00.654321", Event: collectorin.AccountChannelAccessEvent{ChannelID: 3691274988, Reason: collectorin.AccountChannelAccessRemoved}}
		event := n.Event
		event.Date = int(time.Date(2026, 10, 24, 12, 0, 0, 0, time.Local).Unix())
		sources, err := loadAccountChannelAccessSources(ctx, g.DB(), 38, 42, event)
		if err != nil {
			return err
		}
		if len(sources) != 1 || sources[0].Id != 109 || sources[0].Version != n.Version {
			t.Fatalf("incorrect source/version lookup: %+v", sources)
		}
		event.Date = int(time.Date(2026, 10, 23, 12, 0, 0, 0, time.Local).Unix())
		stale, err := loadAccountChannelAccessSources(ctx, g.DB(), 38, 42, event)
		if err != nil {
			return err
		}
		if len(stale) != 0 {
			t.Fatalf("stale event matched newer source: %+v", stale)
		}
		for _, field := range []string{"tenant", "owner", "protocol", "channel", "version"} {
			copy := *n
			switch field {
			case "tenant":
				copy.TenantID = 39
			case "owner":
				copy.AccountID = 999
			case "protocol":
				copy.TGAccountID = 43
			case "channel":
				copy.Event.ChannelID = 2736732070
			case "version":
				copy.Version = "2026-10-23 10:00:00"
			}
			paused, err := pauseAccountChannelSource(ctx, g.DB(), &copy)
			if err != nil {
				return err
			}
			if paused {
				t.Fatalf("cross-scope/stale pause: %s", field)
			}
		}
		body, err := json.Marshal(collectNotificationPayload{ID: "local-channel-access-test", ChannelAccess: n})
		if err != nil {
			return err
		}
		task := asynq.NewTask(tgTaskTypeCollectNotification, body)
		service := &sSysPublish{}
		if err := service.handleCollectNotificationTask(ctx, task); err == nil {
			t.Fatal("delivery failure was not returned for retry")
		}
		if fake.adminCalls != 1 || fake.ownerCalls != 1 {
			t.Fatalf("missing recipients: %+v", fake)
		}
		fake.failOwner = false
		for i := 0; i < 2; i++ {
			if err := service.handleCollectNotificationTask(ctx, task); err != nil {
				return err
			}
		}
		if fake.ownerCalls != 2 {
			t.Fatalf("owner duplicate delivery: %d", fake.ownerCalls)
		}
		for i := 0; i < 2; i++ {
			paused, err := pauseAccountChannelSource(ctx, g.DB(), n)
			if err != nil {
				return err
			}
			if !paused {
				t.Fatal("pause or notification retry failed")
			}
		}
		row, err := tx.Model("hg_youban_publish_collect_source").Where("id", 109).One()
		if err != nil {
			return err
		}
		if row["collect_enabled"].Int() != 0 || row["remark"].String() != "原备注" {
			t.Fatalf("invalid pause: %v", row)
		}
		other, err := tx.Model("hg_youban_publish_collect_source").Where("id", 1357).One()
		if err != nil {
			return err
		}
		if other["collect_enabled"].Int() != 1 {
			t.Fatal("unrelated source paused")
		}

		// Disabling owner alerts must neither prevent pausing the source nor
		// suppress the independently configured collection administrators.
		if _, err = tx.Exec(`INSERT INTO hg_youban_publish_account_setting VALUES(38,512,0,NULL)`); err != nil {
			return err
		}
		quiet := *n
		quiet.SourceID = 1357
		quiet.Event.ChannelID = 2736732070
		ownerCalls, adminCalls := fake.ownerCalls, fake.adminCalls
		if err = service.handleAccountChannelAccessNotification(ctx, "quiet-channel-test", &quiet); err != nil {
			return err
		}
		if fake.ownerCalls != ownerCalls || fake.adminCalls != adminCalls+1 {
			t.Fatalf("disabled owner alert changed recipients: %+v", fake)
		}
		quietRow, err := tx.Model("hg_youban_publish_collect_source").Where("id", 1357).One()
		if err != nil {
			return err
		}
		if quietRow["collect_enabled"].Int() != 0 {
			t.Fatal("disabled notifications prevented source pause")
		}
		// A user re-enable/edit wins against a delayed queue retry.
		if _, err = tx.Exec(`UPDATE hg_youban_publish_collect_source SET collect_enabled=1,updated_at='2026-10-24 17:00:00' WHERE id=109`); err != nil {
			return err
		}
		paused, err := pauseAccountChannelSource(ctx, g.DB(), n)
		if paused {
			t.Fatal("late retry paused manually restored source")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

type channelAccessBotTest struct {
	botservice.ISysBot
	adminCalls int
	ownerCalls int
	failOwner  bool
}

func (f *channelAccessBotTest) NotifyCollectEvent(_ context.Context, _ int64, _ string, _ string, text string, _ [][]botin.NotifyButton) error {
	f.adminCalls++
	if !strings.Contains(text, "TG号：七七（@qqby66）") {
		return errors.New("missing TG identity")
	}
	return nil
}
func (f *channelAccessBotTest) NotifyAccount(_ context.Context, in *botin.NotifyAccountInp) error {
	f.ownerCalls++
	if in.AccountId != 512 || !in.RequireDelivery || !in.IgnoreFeatureSwitch {
		return errors.New("wrong notification recipient/options")
	}
	if f.failOwner {
		return errors.New("simulated Telegram timeout")
	}
	return nil
}
