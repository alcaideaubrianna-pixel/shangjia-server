package sys

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"hotgo/addons/telegram_collector/domain"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/internal/library/cache"
)

func TestSystemNoticeIngressLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_COLLECTOR_BATCH_TEST_PORT") != "55489" {
		t.Skip("requires dedicated loopback PostgreSQL 55489")
	}
	if !collectorBatchPostgresChild(t) {
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55489", User: "postgres", Name: "postgres"}})
	r := miniredis.RunT(t)
	adapter, err := gcfg.NewAdapterContent("telegramCollector:\n  enabled: true\ncache:\n  adapter: redis\nredis:\n  default:\n    address: '" + r.Addr() + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	ctx := context.Background()
	cache.SetAdapter(ctx)
	schema, err := os.ReadFile("../../resource/sql/install.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	ddl := string(schema)
	start := strings.Index(ddl, `CREATE TABLE IF NOT EXISTS "hg_tg_collector_event"`)
	if start < 0 {
		t.Fatal("missing raw event schema")
	}
	ddl = ddl[start:]
	ddl = ddl[:strings.Index(ddl, ");")+2]
	ddl = strings.Replace(ddl, "CREATE TABLE IF NOT EXISTS", "CREATE TABLE", 1)
	defer g.DB().Exec(ctx, `DROP TABLE IF EXISTS hg_tg_collector_event`)
	defer g.DB().Exec(ctx, `DROP TABLE IF EXISTS hg_youban_publish_relay_channel`)
	if _, err := g.DB().Exec(ctx, ddl); err != nil {
		t.Fatal(err)
	}
	if _, err := g.DB().Exec(ctx, `CREATE TABLE hg_youban_publish_relay_channel(id bigint,tenant_id bigint,target_chat_id text,deleted_at timestamp)`); err != nil {
		t.Fatal(err)
	}
	s := &sCollector{}
	defer s.StopRuntime()
	u := &models.Update{ID: 101, ChannelPost: &models.Message{ID: 10, Chat: models.Chat{ID: -1004411799859}, Text: "✅提交成功"}}
	bot := collectorservice.BotContext{Key: "local-test-bot", Binding: collectorservice.BotBinding{TenantID: 78, SourceID: 351}}
	for range 2 {
		if err := s.IngestBotUpdate(ctx, bot, u); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := g.DB().GetAll(ctx, `SELECT id,status,raw_update,error_message FROM hg_tg_collector_event`)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if rows[0]["status"].String() != sysin.EventStatusIgnored || !domain.EventIgnored.Valid() {
		t.Fatal("receipt terminal state missing")
	}
	var stored models.Update
	if err := json.Unmarshal([]byte(rows[0]["raw_update"].String()), &stored); err != nil || stored.ChannelPost.Text != u.ChannelPost.Text {
		t.Fatal("raw update was not preserved")
	}
	if s.queueClient != nil || s.deliveryServer != nil {
		t.Fatal("receipt unexpectedly created an execution queue")
	}
	id := rows[0]["id"].Int64()
	if _, err := g.DB().Exec(ctx, `UPDATE hg_tg_collector_event SET status='received',processed_at=NULL WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	// There is deliberately no Delivery table. A legacy task must finish before
	// touching Delivery persistence or calling a business handler.
	if err := s.processEvent(ctx, id); err != nil {
		t.Fatal(err)
	}
	state, err := g.DB().GetValue(ctx, `SELECT status FROM hg_tg_collector_event WHERE id=?`, id)
	if err != nil || state.String() != sysin.EventStatusIgnored {
		t.Fatalf("legacy state=%v err=%v", state, err)
	}
	if _, err := g.DB().Exec(ctx, `UPDATE hg_tg_collector_event SET status='processing',lease_owner='new-owner',attempt_count=2 WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if err := ignoreClaimedSystemNotice(ctx, &entity.TgCollectorEvent{Id: id, TenantId: 78, SourceId: 351, LeaseOwner: "old-owner", AttemptCount: 1}); err == nil {
		t.Fatal("stale owner overwrote newer execution")
	}
	state, err = g.DB().GetValue(ctx, `SELECT status FROM hg_tg_collector_event WHERE id=?`, id)
	if err != nil || state.String() != sysin.EventStatusProcessing {
		t.Fatalf("new owner changed: %v %v", state, err)
	}
}
