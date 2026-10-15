package sys

import (
	"context"
	"testing"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
)

func relayIsolationUpdate() *models.Update {
	return &models.Update{Message: &models.Message{ID: 1, Text: "管理后台", From: &models.User{ID: 12}, Chat: models.Chat{ID: 12, Type: models.ChatTypePrivate}}}
}

func TestRelayMissingMigrationDoesNotDisableMainBot(t *testing.T) {
	ctx := relayTestDatabase(t)
	relayTestBaseTable(t, ctx, featureTable)
	if _, err := dao.YoubanBotFeature.Ctx(ctx).Data(do.YoubanBotFeature{FeatureKey: consts.RelayFeatureKey, Status: 1, ConfigJson: "{}"}).Insert(); err != nil {
		t.Fatal(err)
	}
	db, _ := gdb.Instance()
	if _, err := db.Exec(ctx, "DROP TABLE hg_youban_bot_relay_config"); err != nil {
		t.Fatal(err)
	}
	if err := db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	s := NewSysBot()
	handled, err := s.handleRelayUpdate(ctx, 1, relayIsolationUpdate())
	if handled || err != nil {
		t.Fatalf("optional migration failure hijacked main Bot: handled=%v err=%v", handled, err)
	}
	feature := &relayGatewayFeature{bot: s}
	menus, err := feature.Menus(ctx, gatewayservice.BotContext{Bindings: []gatewayservice.BotBinding{{Owner: "youban_bot", ReferenceID: 1}}})
	if err != nil || menus.Managed || len(menus.Items) != 0 {
		t.Fatalf("main Bot menu hijacked: %+v %v", menus, err)
	}
}

func TestRelayHelpOnlyBelongsToConfiguredBot(t *testing.T) {
	ctx := relayTestDatabase(t)
	relayTestBaseTable(t, ctx, featureTable)
	if _, err := dao.YoubanBotFeature.Ctx(ctx).Data(do.YoubanBotFeature{FeatureKey: consts.RelayFeatureKey, Status: 1, ConfigJson: "{}"}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).Data(do.YoubanBotRelayConfig{BotId: 2, OwnerId: 99, Enabled: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	s := NewSysBot()
	feature := &relayGatewayFeature{bot: s}
	for _, id := range []int64{1, 2} {
		menus, err := feature.Menus(ctx, gatewayservice.BotContext{Bindings: []gatewayservice.BotBinding{{Owner: "youban_bot", ReferenceID: id}}})
		if err != nil {
			t.Fatal(err)
		}
		if id == 1 && (menus.Managed || len(menus.Items) > 0) {
			t.Fatalf("help registered on main Bot: %+v", menus)
		}
		if id == 2 && (!menus.Managed || len(menus.Items) != 2 || menus.Items[0].Command != "help") {
			t.Fatalf("relay help absent: %+v", menus)
		}
	}
	handled, err := s.handleRelayUpdate(ctx, 1, relayIsolationUpdate())
	if handled || err != nil {
		t.Fatalf("unconfigured main Bot message claimed: %v %v", handled, err)
	}
	if s.featureMenuVisible(ctx, consts.RelayFeatureKey) {
		t.Fatal("relay leaked into global reply keyboard")
	}
}

func TestRelayDefaultsDoNotChangeGlobalCommands(t *testing.T) {
	if featureDefaultStatus(relayFeature{}) != 2 {
		t.Fatal("optional relay auto-enabled")
	}
	if featureDefaultStatus(startFeature{}) != 1 {
		t.Fatal("main start feature disabled")
	}
	if (relayFeature{}).Command() != "" {
		t.Fatal("relay registers a global slash command")
	}
	s := NewSysBot()
	if s.featureMenuVisible(context.Background(), consts.RelayFeatureKey) {
		t.Fatal("global menu includes relay")
	}
}

func TestRelayDisabledRuntimeDoesNotRequireMigration(t *testing.T) {
	ctx := relayTestDatabase(t)
	relayTestBaseTable(t, ctx, featureTable)
	if _, err := dao.YoubanBotFeature.Ctx(ctx).Data(do.YoubanBotFeature{FeatureKey: consts.RelayFeatureKey, Status: 2, ConfigJson: "{}"}).Insert(); err != nil {
		t.Fatal(err)
	}
	db, _ := gdb.Instance()
	if _, err := db.Exec(ctx, "DROP TABLE hg_youban_bot_relay_delivery"); err != nil {
		t.Fatal(err)
	}
	if err := NewSysBot().relayDeliveryTick(ctx); err != nil {
		t.Fatalf("disabled plugin required migration: %v", err)
	}
}
