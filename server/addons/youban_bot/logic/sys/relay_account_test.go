package sys

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/model/input/sysin"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestRelayTenantAccountIsolation(t *testing.T) {
	ctx, s, _ := relayInteractionFixture(t)
	if _, err := dao.YoubanBotBot.Ctx(ctx).Data(do.YoubanBotBot{Id: 2, BotToken: "456:other", Status: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).Data(do.YoubanBotRelayConfig{BotId: 2, TenantId: 2, AccountId: 99, SourceBotId: 2, OwnerId: 99, Enabled: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	s.relayAccountForUser = func(_ context.Context, id int64) (*botProfileAccount, error) {
		switch id {
		case 12:
			return &botProfileAccount{TenantId: 1, AccountId: 99, App: "api"}, nil
		case 13:
			return &botProfileAccount{TenantId: 2, AccountId: 99, App: "api"}, nil
		case 14:
			return &botProfileAccount{TenantId: 1, AccountId: 100, App: "api"}, nil
		}
		return nil, fmt.Errorf("unbound")
	}
	cfg, err := relayConfig(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id   int64
		want bool
	}{{12, true}, {13, false}, {14, false}} {
		allowed, err := s.relayIsAdmin(ctx, cfg, tc.id)
		if err != nil || allowed != tc.want {
			t.Fatalf("identity tenant/account crossed: %d %v %v", tc.id, allowed, err)
		}
	}
	actor := relayActorContext(ctx, 12)
	if err := s.relayCheckAccess(actor, 2); err == nil {
		t.Fatal("Bot ID guessed across tenants")
	}
	if _, _, err := s.AdminRelayBindingList(actor, &sysin.RelayListInp{BotId: 2}); err == nil {
		t.Fatal("binding list leaked")
	}
	if _, _, err := s.AdminRelayRuleList(actor, &sysin.RelayListInp{BotId: 2}); err == nil {
		t.Fatal("blacklist leaked")
	}
	if _, _, err := s.AdminRelayRecordList(actor, &sysin.RelayListInp{BotId: 2}); err == nil {
		t.Fatal("audit record leaked")
	}
	if err := s.AdminRelaySwitch(actor, &sysin.RelaySwitchInp{RelayBotInp: sysin.RelayBotInp{BotId: 2}, Enabled: 0}); err == nil {
		t.Fatal("cross tenant switch accepted")
	}
	text, err := s.relayManagementList(ctx, 12)
	if err != nil || !strings.Contains(text, "/relayadmin 1") || strings.Contains(text, "/relayadmin 2") {
		t.Fatalf("management list not scoped: %s %v", text, err)
	}
	if _, err := s.AdminRelaySettings(ctx, &sysin.RelayBotInp{BotId: 1}); err == nil {
		t.Fatal("global admin context obtained account settings")
	}
}

func TestRelayMutationAcceptsSourceBotID(t *testing.T) {
	ctx, s, _ := relayInteractionFixture(t)
	columns := dao.YoubanBotRelayConfig.Columns()
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).
		Where(columns.BotId, 1).
		Data(do.YoubanBotRelayConfig{SourceBotId: 20}).
		Update(); err != nil {
		t.Fatal(err)
	}

	config, err := relayConfigByIdentity(ctx, 20)
	if err != nil || config == nil || config.BotId != 1 || config.SourceBotId != 20 {
		t.Fatalf("source Bot ID did not resolve legacy config: %+v %v", config, err)
	}
	if mainConfig, err := s.relayIdentity(ctx, 20); err != nil || mainConfig != nil {
		t.Fatalf("source Bot ID leaked into main Bot identity: %+v %v", mainConfig, err)
	}
	requestedBotID := int64(0)
	s.relayOwnedBots = func(_ context.Context, tenantID, accountID, botID int64, lock bool) ([]*publishsysin.RelayOwnedBotModel, error) {
		requestedBotID = botID
		return []*publishsysin.RelayOwnedBotModel{{BotModel: publishsysin.BotModel{BotType: "two_way", Id: 20, TenantId: tenantID, Status: 1}, AccountId: accountID, CreatedBy: accountID}}, nil
	}
	if _, err = s.relaySourceBot(ctx, config); err != nil || requestedBotID != 20 {
		t.Fatalf("source lookup used config ID instead of source Bot ID: requested=%d err=%v", requestedBotID, err)
	}
	ensured, err := s.relayEnsureConfig(ctx, &botProfileAccount{TenantId: 1, AccountId: 99}, 20)
	if err != nil || ensured.BotId != 1 || ensured.SourceBotId != 20 {
		t.Fatalf("ensure migrated config identity online: %+v %v", ensured, err)
	}
	if _, err = dao.YoubanBotRelayBinding.Ctx(ctx).Data(do.YoubanBotRelayBinding{BotId: 1, PairKey: "12:13", LeftChatId: "12", RightChatId: "13", LeftName: "A", RightName: "B"}).Insert(); err != nil {
		t.Fatal(err)
	}
	list, total, err := s.AdminRelayBindingList(relayActorContext(ctx, 12), &sysin.RelayListInp{BotId: 20})
	if err != nil || total != 1 {
		t.Fatalf("source Bot ID did not resolve config child rows: total=%d rows=%d err=%v", total, len(list), err)
	}
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		return
	}
	called := false
	if err = relayMutate(ctx, 20, func(context.Context) error {
		called = true
		return nil
	}); err != nil || !called {
		t.Fatalf("source Bot ID mutation failed: called=%t err=%v", called, err)
	}
}

func TestRelayGlobalPluginCannotOverwriteAccountConfig(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("requires PostgreSQL row locks")
	}
	ctx, s, _ := relayInteractionFixture(t)
	c := dao.YoubanBotFeature.Columns()
	payload := `{"botId":1,"enabled":0,"admins":"777","bindings":"12 13","auditGroupId":"-1009"}`
	in := &sysin.FeatureSaveInp{FeatureKey: consts.RelayFeatureKey, Status: 1, ConfigJson: payload}
	if err := s.persistRelayFeature(ctx, in, g.Map{c.Status: 1, c.ConfigJson: payload, c.Command: "help"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := relayConfig(ctx, 1)
	if err != nil || cfg.Enabled != 1 || cfg.AccountId != 99 || cfg.TenantId != 1 || cfg.AuditGroupId != "" {
		t.Fatalf("global form overwrote account: %+v %v", cfg, err)
	}
	var feature *botFeatureRow
	if err := dao.YoubanBotFeature.Ctx(ctx).Where(c.FeatureKey, consts.RelayFeatureKey).Scan(&feature); err != nil {
		t.Fatal(err)
	}
	if feature.ConfigJson != "{}" || feature.Command != "" {
		t.Fatal("global feature retained business config", feature)
	}
}

func TestRelayLegacyConfigurationCannotBeClaimed(t *testing.T) {
	ctx, s, _ := relayInteractionFixture(t)
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).Where(dao.YoubanBotRelayConfig.Columns().BotId, 1).Data(do.YoubanBotRelayConfig{TenantId: 0, AccountId: 0, Enabled: 1}).Update(); err != nil {
		t.Fatal(err)
	}
	cfg, err := s.relayIdentity(ctx, 1)
	if err != nil || cfg == nil || relayOwned(cfg) {
		t.Fatal("legacy Bot lost quarantine identity", err)
	}
	active, _, err := s.relayRuntimeState(ctx, cfg)
	if err != nil || active {
		t.Fatal("unowned configuration entered runtime", err)
	}
	old, err := relayConfig(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if allowed, err := s.relayIsAdmin(ctx, old, 12); err != nil || allowed {
		t.Fatal("legacy owner admin ID implicitly granted tenant access", allowed, err)
	}
	if err := s.relayCheckAccess(relayActorContext(ctx, 12), 1); err == nil {
		t.Fatal("legacy config claimed")
	}
}

func TestRelayBackendBindingUniqueAndRollback(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("requires PostgreSQL row locks")
	}
	ctx, s, _ := relayInteractionFixture(t)
	db, _ := gdb.Instance()
	if _, err := db.Exec(ctx, "SELECT setval(pg_get_serial_sequence('hg_youban_bot_bot','id'), (SELECT COALESCE(MAX(id),1) FROM hg_youban_bot_bot))"); err != nil {
		t.Fatal(err)
	}
	source := &publishsysin.RelayOwnedBotModel{BotModel: publishsysin.BotModel{BotType: "two_way", Id: 50, TenantId: 3, BotToken: "789:tenant", BotName: "Tenant", Status: 1}, AccountId: 300, CreatedBy: 300}
	account := &botProfileAccount{TenantId: 3, AccountId: 300, App: "api"}
	normal := *source
	normal.BotType = "normal"
	if _, err := s.relayAttachBackendBot(ctx, account, &normal); err == nil {
		t.Fatal("ordinary Bot accepted as dedicated two-way Bot")
	}

	id, err := s.relayAttachBackendBot(ctx, account, source)
	if err != nil || id <= 0 {
		t.Fatal("account binding failed", id, err)
	}
	cfg, err := relayConfig(ctx, id)
	if err != nil || cfg.TenantId != 3 || cfg.AccountId != 300 || cfg.SourceBotId != 50 || cfg.Enabled != 0 {
		t.Fatal("binding lost ownership or defaulted enabled", cfg, err)
	}
	if _, err := s.relayAttachBackendBot(ctx, account, source); err == nil {
		t.Fatal("same account created second configuration")
	}
	if _, err := s.relayAttachBackendBot(ctx, &botProfileAccount{TenantId: 4, AccountId: 400}, source); err == nil {
		t.Fatal("other tenant's backend Bot accepted")
	}
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for i := int64(5); i <= 6; i++ {
		wg.Add(1)
		go func(i int64) {
			defer wg.Done()
			_, err := s.relayAttachBackendBot(ctx, &botProfileAccount{TenantId: i, AccountId: i * 100}, &publishsysin.RelayOwnedBotModel{BotModel: publishsysin.BotModel{BotType: "two_way", Id: 51, TenantId: i, BotToken: "790:race", Status: 1}, AccountId: i * 100, CreatedBy: i * 100})
			errors <- err
		}(i)
	}
	wg.Wait()
	close(errors)
	wins := 0
	for err := range errors {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatal("same backend Bot acquired by multiple accounts", wins)
	}

}

func TestRelayAccountMigrationIsIdempotent(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("requires PostgreSQL migration")
	}
	ctx := relayTestDatabase(t)
	db, _ := gdb.Instance()
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).Data(do.YoubanBotRelayConfig{BotId: 10, OwnerId: 99, Enabled: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).Data(do.YoubanBotRelayConfig{BotId: 11, TenantId: 1, AccountId: 99, SourceBotId: 1, OwnerId: 99, Enabled: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	sql, err := os.ReadFile("../../resource/sql/relay.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		for _, statement := range strings.Split(string(sql), ";") {
			if strings.TrimSpace(statement) != "" {
				if _, err := db.Exec(ctx, statement); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	old, err := relayConfig(ctx, 10)
	if err != nil || old.Enabled != 0 || old.OwnerId != 99 {
		t.Fatal("legacy migration destroyed data or kept enabled", old, err)
	}
	owned, err := relayConfig(ctx, 11)
	if err != nil || owned.Enabled != 1 {
		t.Fatal("migration disabled valid owned config", owned, err)
	}
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).Data(do.YoubanBotRelayConfig{BotId: 12, TenantId: 1, AccountId: 99, SourceBotId: 1, OwnerId: 99}).Insert(); err == nil {
		t.Fatal("DB accepted duplicate tenant account config")
	}
}

func TestRelayOnlyOwnBackendBotsAreSelectable(t *testing.T) {
	ctx, s, _ := relayInteractionFixture(t)
	requested := false
	s.relayOwnedBots = func(_ context.Context, tenantID, accountID, botID int64, lock bool) ([]*publishsysin.RelayOwnedBotModel, error) {
		if tenantID != 1 || accountID != 99 {
			t.Fatal("backend lookup omitted tenant/account")
		}
		requested = true
		return []*publishsysin.RelayOwnedBotModel{}, nil
	}
	text, err := s.relayBackendBotChoices(ctx, &botProfileAccount{TenantId: 1, AccountId: 99})
	if err != nil || !requested || !strings.Contains(text, "请先在后台创建") || strings.Contains(text, "创建 BOT_TOKEN") {
		t.Fatal("missing Bot guidance wrong", text, err)
	}
	s.relayOwnedBots = func(_ context.Context, tenantID, accountID, botID int64, lock bool) ([]*publishsysin.RelayOwnedBotModel, error) {
		return []*publishsysin.RelayOwnedBotModel{{BotModel: publishsysin.BotModel{BotType: "two_way", Id: 8, TenantId: 1, BotName: "Own", BotUsername: "own_bot", Status: 1}, AccountId: 99, CreatedBy: 99}}, nil
	}
	text, err = s.relayBackendBotChoices(ctx, &botProfileAccount{TenantId: 1, AccountId: 99})
	if err != nil || !strings.Contains(text, "请选择列表中的双向机器人按钮") {
		t.Fatal("own backend Bot not selectable", text, err)
	}
	// Source ownership/status changes immediately stop delivery and management.
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).Where(dao.YoubanBotRelayConfig.Columns().BotId, 1).Data(do.YoubanBotRelayConfig{SourceBotId: 8}).Update(); err != nil {
		t.Fatal(err)
	}
	s.relayOwnedBots = func(context.Context, int64, int64, int64, bool) ([]*publishsysin.RelayOwnedBotModel, error) {
		return nil, nil
	}
	cfg, err := relayConfig(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	active, state, err := s.relayRuntimeState(ctx, cfg)
	if err != nil || active || !strings.Contains(state, "双向机器人") {
		t.Fatal("source deletion did not stop runtime", active, state, err)
	}
	if err := s.relayCheckAccess(relayActorContext(ctx, 12), 1); err == nil {
		t.Fatal("source ownership change left mutation access")
	}
}
