package sys

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/addons/youban_bot/model/input/sysin"
	coreconsts "hotgo/internal/consts"
	"hotgo/internal/library/contexts"
	"hotgo/internal/model"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestRelayHistoricalUsernameDoesNotGrantAdmin(t *testing.T) {
	ctx := relayTestDatabase(t)
	relayTestBaseTable(t, ctx, userTable)
	if _, err := dao.YoubanBotUser.Ctx(ctx).Data(do.YoubanBotUser{BotId: 1, TelegramUserId: "12", TelegramUsername: "alice", Status: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	admins, err := NewSysBot().resolveRelayAdmins(ctx, 1, []string{"@alice"})
	if err != nil || len(admins) != 1 || admins[0].UserId != "" {
		t.Fatalf("historical username granted admin: %+v %v", admins, err)
	}
}

func TestRelayLateAlbumSkipsClearedSentPayload(t *testing.T) {
	ctx := relayTestDatabase(t)
	cfg := &entity.YoubanBotRelayConfig{BotId: 1}
	for _, status := range []string{consts.RelaySent, consts.RelayCancelled} {
		if _, err := dao.YoubanBotRelayDelivery.Ctx(ctx).Data(do.YoubanBotRelayDelivery{BotId: 1, SourceChatId: "12", SourceMessageId: 1, MediaGroupId: "album", TargetChatId: "13", Kind: consts.RelayKindChat, Status: status, RawJson: "", ErrorMessage: ""}).Insert(); err != nil {
			t.Fatal(err)
		}
		rows := []*entity.YoubanBotRelayDelivery{{BotId: 1, SourceChatId: "12", MediaGroupId: "album", TargetChatId: "13", Kind: consts.RelayKindChat, RawJson: `{"chat":{"id":12},"text":"hello"}`}}
		reason, err := relayPendingFilter(ctx, cfg, rows)
		if err != nil || reason != "" {
			t.Fatalf("completed album member poisoned late fragment: %q %v", reason, err)
		}
		if _, err := dao.YoubanBotRelayDelivery.Ctx(ctx).Where(dao.YoubanBotRelayDelivery.Columns().BotId, 1).Delete(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := dao.YoubanBotRelayDelivery.Ctx(ctx).Data(do.YoubanBotRelayDelivery{BotId: 1, SourceChatId: "12", SourceMessageId: 2, MediaGroupId: "album", TargetChatId: "13", Kind: consts.RelayKindChat, Status: consts.RelayCancelled, RawJson: "", ErrorMessage: "blocked"}).Insert(); err != nil {
		t.Fatal(err)
	}
	reason, err := relayPendingFilter(ctx, cfg, []*entity.YoubanBotRelayDelivery{{BotId: 1, SourceChatId: "12", MediaGroupId: "album", TargetChatId: "13", Kind: consts.RelayKindChat, RawJson: `{}`}})
	if err != nil || reason == "" {
		t.Fatal("blocked member no longer blocks late fragment", err)
	}
}

func TestRelayClaimRespectsConversationBarrier(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("requires PostgreSQL row locks")
	}
	ctx := relayTestDatabase(t)
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).Data(do.YoubanBotRelayConfig{BotId: 1, OwnerId: 99}).Insert(); err != nil {
		t.Fatal(err)
	}
	insert := func(pair string, delay time.Duration) *entity.YoubanBotRelayDelivery {
		t.Helper()
		id, err := dao.YoubanBotRelayDelivery.Ctx(ctx).Data(do.YoubanBotRelayDelivery{BotId: 1, PairKey: pair, SourceChatId: "12", SourceMessageId: time.Now().UnixNano(), TargetChatId: "13", Kind: consts.RelayKindChat, Status: consts.RelayPending, RawJson: "{}", AvailableAt: gtime.Now().Add(delay)}).InsertAndGetId()
		if err != nil {
			t.Fatal(err)
		}
		var row *entity.YoubanBotRelayDelivery
		if err := dao.YoubanBotRelayDelivery.Ctx(ctx).WherePri(id).Scan(&row); err != nil {
			t.Fatal(err)
		}
		return row
	}
	first := insert("12:13", time.Hour)
	second := insert("12:13", 0)
	other := insert("14:15", 0)
	rows, _, err := claimRelayDeliveries(ctx, second)
	if err != nil || len(rows) != 0 {
		t.Fatal("retrying predecessor overtaken", err)
	}
	var candidates []*entity.YoubanBotRelayDelivery
	c := dao.YoubanBotRelayDelivery.Columns()
	err = dao.YoubanBotRelayDelivery.Ctx(ctx).As("candidate").Where("candidate."+c.Status, consts.RelayPending).WhereLTE("candidate."+c.AvailableAt, gtime.Now()).Where(relayNoPredecessorSQL(), consts.RelayPending, consts.RelaySending).Scan(&candidates)
	if err != nil || len(candidates) != 1 || candidates[0].Id != other.Id {
		t.Fatalf("blocked conversation starved other: %+v %v", candidates, err)
	}
	rows, _, err = claimRelayDeliveries(ctx, other)
	if err != nil || len(rows) != 1 {
		t.Fatal("other session blocked", err)
	}
	if _, err = dao.YoubanBotRelayDelivery.Ctx(ctx).WherePri(first.Id).Data(do.YoubanBotRelayDelivery{AvailableAt: gtime.Now()}).Update(); err != nil {
		t.Fatal(err)
	}
	rows, _, err = claimRelayDeliveries(ctx, first)
	if err != nil || len(rows) != 1 {
		t.Fatal("first message not claimed", err)
	}
	rows, _, err = claimRelayDeliveries(ctx, second)
	if err != nil || len(rows) != 0 {
		t.Fatal("sending predecessor overtaken", err)
	}
}

func TestRelayWrongBotRevisionIsRejected(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("requires PostgreSQL row locks")
	}
	ctx := relayTestDatabase(t)
	relayTestBaseTable(t, ctx, botTable)
	relayTestBaseTable(t, ctx, featureTable)
	db, _ := gdb.Instance()
	if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_bot(id,bot_name,status)VALUES(1,'A',1),(2,'B',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotFeature.Ctx(ctx).Data(do.YoubanBotFeature{FeatureKey: consts.RelayFeatureKey, Status: 1, ConfigJson: `{"botId":1}`}).Insert(); err != nil {
		t.Fatal(err)
	}
	s := NewSysBot()
	m := &sysin.FeatureModel{ConfigSchema: featureConfigSchema(consts.RelayFeatureKey), ConfigValues: featureConfigValues(normalizeFeatureConfigJson(consts.RelayFeatureKey, `{"botId":1}`))}
	if err := s.relayFeatureSchema(ctx, m); err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, coreconsts.ContextHTTPKey, &model.Context{})
	contexts.SetUser(ctx, &model.Identity{Id: 99, App: coreconsts.AppAdmin})
	c := dao.YoubanBotFeature.Columns()
	valid, _ := json.Marshal(m.ConfigValues)
	validIn := &sysin.FeatureSaveInp{FeatureKey: consts.RelayFeatureKey, Status: 1, ConfigJson: string(valid)}
	if err := s.persistRelayFeature(ctx, validIn, g.Map{c.Status: 1, c.ConfigJson: string(valid)}); err != nil {
		t.Fatal("current Bot snapshot rejected", err)
	}
	if _, err := dao.YoubanBotRelayRule.Ctx(ctx).Data(do.YoubanBotRelayRule{BotId: 1, Kind: consts.RelayRuleKeyword, Value: "new", Label: "new"}).Insert(); err != nil {
		t.Fatal(err)
	}
	if err := s.persistRelayFeature(ctx, validIn, g.Map{c.Status: 1, c.ConfigJson: string(valid)}); err == nil {
		t.Fatal("concurrent command edit overwritten by stale form")
	}
	m.ConfigValues["botId"] = 2
	raw, _ := json.Marshal(m.ConfigValues)
	in := &sysin.FeatureSaveInp{FeatureKey: consts.RelayFeatureKey, Status: 1, ConfigJson: string(raw)}
	if err := s.persistRelayFeature(ctx, in, g.Map{c.Status: 1, c.ConfigJson: string(raw)}); err == nil {
		t.Fatal("Bot A snapshot accepted for Bot B")
	}
	// Status-only disable must work even when the optional migration is missing.
	if _, err := db.Exec(ctx, "DROP TABLE hg_youban_bot_relay_config"); err != nil {
		t.Fatal(err)
	}
	in.StatusOnly = true
	in.Status = 2
	if err := s.persistRelayFeature(ctx, in, g.Map{c.Status: 2}); err != nil {
		t.Fatal("kill switch depends on relay schema", err)
	}
	var row *botFeatureRow
	if err := dao.YoubanBotFeature.Ctx(ctx).Where(c.FeatureKey, consts.RelayFeatureKey).Scan(&row); err != nil || row.Status != 2 {
		t.Fatal("plugin not disabled", err)
	}
}
