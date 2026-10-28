package sys

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/model/input/sysin"
	gs "hotgo/addons/youban_tg_bot_gateway/service"
	"hotgo/internal/library/cache"
)

func featureCommandLocalPostgres(t *testing.T) context.Context {
	t.Helper()
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL at 127.0.0.1:35432")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	node := gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}
	root, err := gdb.New(node)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("bot_command_test_%d", time.Now().UnixNano())
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = root.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
	node.Namespace = schema
	gdb.SetConfigGroup("default", gdb.ConfigGroup{node})
	db := g.DB()
	t.Cleanup(func() { _ = db.GetCore().ClearTableFieldsAll(context.Background()) })
	ddl, err := os.ReadFile("../../resource/sql/install.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{featureTable, botTable} {
		marker := "CREATE TABLE IF NOT EXISTS " + table + " ("
		start := strings.Index(string(ddl), marker)
		if start < 0 {
			t.Fatal(table)
		}
		end := strings.Index(string(ddl)[start:], ";") + start
		if _, err = db.Exec(ctx, string(ddl)[start:end]); err != nil {
			t.Fatal(err)
		}
	}
	for _, feature := range botFeatures {
		if _, err = dao.YoubanBotFeature.Ctx(ctx).Data(do.YoubanBotFeature{FeatureKey: feature.Key(), Command: feature.Command(), Name: feature.Description(), ConfigJson: "{}", Status: 1}).Insert(); err != nil {
			t.Fatal(err)
		}
	}
	return ctx
}

func TestFeatureCommandSaveLocalPostgres(t *testing.T) {
	ctx := featureCommandLocalPostgres(t)
	c := dao.YoubanBotFeature.Columns()
	var rows []*botFeatureRow
	if err := dao.YoubanBotFeature.Ctx(ctx).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	initial := map[string]*botFeatureRow{}
	for _, row := range rows {
		initial[row.FeatureKey] = row
	}
	// Separate service objects keep stale snapshots, as separate SAE instances do.
	instances := []*sSysBot{
		{features: initial, featureAt: time.Now(), featureDefaultsAt: time.Now()},
		{features: initial, featureAt: time.Now(), featureDefaultsAt: time.Now()},
	}
	inputs := []*sysin.FeatureSaveInp{
		{Id: initial["info"].Id, FeatureKey: "info", Command: "shared_review_command", ConfigJson: "{}", Status: 1},
		{Id: initial["account_bind"].Id, FeatureKey: "account_bind", Command: "shared_review_command", ConfigJson: "{}", Status: 1},
	}
	ready := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := range instances {
		wg.Add(1)
		go func(i int) { defer wg.Done(); <-ready; results <- instances[i].persistFeature(ctx, inputs[i]) }(i)
	}
	close(ready)
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if strings.Contains(err.Error(), "命令已被其他插件使用") {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	count, err := dao.YoubanBotFeature.Ctx(ctx).Where(c.Command, "shared_review_command").Count()
	if err != nil || count != 1 {
		t.Fatalf("duplicate command persisted: %d %v", count, err)
	}
	// A later save on an instance with a stale cache must see the committed owner.
	err = instances[0].persistFeature(ctx, &sysin.FeatureSaveInp{Id: initial["account_login"].Id, FeatureKey: "account_login", Command: "shared_review_command", ConfigJson: "{}", Status: 1})
	if err == nil || !strings.Contains(err.Error(), "命令已被其他插件使用") {
		t.Fatalf("stale cache bypassed validation: %v", err)
	}

	// Clearing a command must also reserve its effective built-in fallback.
	if _, err = dao.YoubanBotFeature.Ctx(ctx).Where(c.FeatureKey, "info").Data(do.YoubanBotFeature{Command: "custominfo"}).Update(); err != nil {
		t.Fatal(err)
	}
	if _, err = dao.YoubanBotFeature.Ctx(ctx).Where(c.FeatureKey, "account_bind").Data(do.YoubanBotFeature{Command: "info"}).Update(); err != nil {
		t.Fatal(err)
	}
	err = instances[0].persistFeature(ctx, &sysin.FeatureSaveInp{Id: initial["info"].Id, FeatureKey: "info", Command: "", Name: "must rollback", ConfigJson: "{}", Status: 1})
	if err == nil {
		t.Fatal("empty command conflict saved")
	}
	var after *botFeatureRow
	if err = dao.YoubanBotFeature.Ctx(ctx).WherePri(initial["info"].Id).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after.Command != "custominfo" || after.Name == "must rollback" {
		t.Fatalf("rejected save changed row: %+v", after)
	}
	// Validation errors release the lock, and a valid save still succeeds.
	if err = instances[0].persistFeature(ctx, &sysin.FeatureSaveInp{Id: initial["info"].Id, FeatureKey: "info", Command: "another_info", ConfigJson: "{}", Status: 1}); err != nil {
		t.Fatal(err)
	}
	// The existing relay status-only save remains atomic inside the outer lock.
	relayKey := (relayFeature{}).Key()
	if err = instances[0].persistFeature(ctx, &sysin.FeatureSaveInp{Id: initial[relayKey].Id, FeatureKey: relayKey, Command: "", Status: 2, StatusOnly: true}); err != nil {
		t.Fatal(err)
	}
	var relay *botFeatureRow
	if err = dao.YoubanBotFeature.Ctx(ctx).WherePri(initial[relayKey].Id).Scan(&relay); err != nil {
		t.Fatal(err)
	}
	if relay.Status != 2 || relay.Command != "" || relay.ConfigJson != "{}" {
		t.Fatalf("relay status save regressed: %+v", relay)
	}

}

func TestEmptyKeyboardDeliveryLocalPostgres(t *testing.T) {
	ctx := featureCommandLocalPostgres(t)
	ctx = context.WithValue(ctx, telegramUserIdCtxKey{}, "keyboard-regression")
	if !cache.Initialized() {
		adapter, err := gcfg.NewAdapterContent(`{"cache":{"adapter":"memory"}}`)
		if err != nil {
			t.Fatal(err)
		}
		previousAdapter := g.Cfg().GetAdapter()
		g.Cfg().SetAdapter(adapter)
		cache.SetAdapter(ctx)
		g.Cfg().SetAdapter(previousAdapter)
	}
	key := replyKeyboardUserVersionKey(1, telegramUserIdFromCtx(ctx))
	_, _ = cache.Instance().Remove(ctx, key)
	t.Cleanup(func() { _, _ = cache.Instance().Remove(context.Background(), key) })
	if _, err := dao.YoubanBotBot.Ctx(ctx).Data(do.YoubanBotBot{Id: 1, BotToken: "123:fake", Status: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	rows := map[string]*botFeatureRow{}
	for _, feature := range botFeatures {
		rows[feature.Key()] = &botFeatureRow{Status: 2}
	}
	rows["start"] = &botFeatureRow{Status: 1, ConfigJson: `{"menuVisible":0}`}
	bot := &sSysBot{features: rows, featureAt: time.Now(), featureDefaultsAt: time.Now()}
	var sends, deletes atomic.Int64
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/deleteMessage") {
			deletes.Add(1)
			fmt.Fprint(w, `{"ok":true,"result":true}`)
			return
		}
		sends.Add(1)
		if !strings.Contains(r.FormValue("reply_markup"), `"remove_keyboard":true`) {
			t.Errorf("missing removal: %s", r.FormValue("reply_markup"))
		}
		if fail.Load() {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"ok":false,"error_code":400,"description":"temporary send failure"}`)
			return
		}
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":99}}`)
	}))
	defer server.Close()
	client, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	var previous gs.IGateway
	func() { defer func() { _ = recover() }(); previous = gs.Gateway() }()
	gs.RegisterGateway(accountRenewalTestGateway{client: func(context.Context, string) (*tgbot.Bot, error) { return client, nil }})
	t.Cleanup(func() { gs.RegisterGateway(previous) })
	for i := 0; i < 2; i++ {
		if err = bot.reply(ctx, 1, "11", "normal response"); err != nil {
			t.Fatal(err)
		}
		if err = bot.ensureReplyKeyboardCurrent(ctx, 1, "11"); err != nil {
			t.Fatal(err)
		}
	}
	if sends.Load() != 2 || deletes.Load() != 0 {
		t.Fatalf("redundant refresh sends=%d deletes=%d", sends.Load(), deletes.Load())
	}
	// Failed deliveries must not acknowledge a new removal version.
	previousMarkup := &models.ReplyKeyboardMarkup{Keyboard: [][]models.KeyboardButton{{{Text: "old menu"}}}}
	bot.markReplyKeyboardDelivered(ctx, 1, previousMarkup)
	fail.Store(true)
	if err = bot.reply(ctx, 1, "11", "failed response"); err == nil {
		t.Fatal("expected send failure")
	}
	version, err := cache.Instance().Get(ctx, key)
	if err != nil || version.String() != replyKeyboardDeliveryVersion(previousMarkup) {
		t.Fatalf("failed send changed delivery version: %v %v", version, err)
	}
}
