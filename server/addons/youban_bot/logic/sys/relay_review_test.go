package sys

import (
	"os"
	"testing"
	"time"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"

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
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).Data(do.YoubanBotRelayConfig{BotId: 1, TenantId: 1, AccountId: 99, SourceBotId: 1, OwnerId: 99}).Insert(); err != nil {
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
