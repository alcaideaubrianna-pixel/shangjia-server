package sys

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
)

type managerTestInvoker struct {
	invoke func(context.Context, bin.Encoder, bin.Decoder) error
}

func (m managerTestInvoker) Invoke(ctx context.Context, in bin.Encoder, out bin.Decoder) error {
	return m.invoke(ctx, in, out)
}
func managerTestResponse(out bin.Decoder, value bin.Encoder) error {
	b := &bin.Buffer{}
	if err := value.Encode(b); err != nil {
		return err
	}
	return out.Decode(b)
}

func TestBotManagerProtocolSyncAndPartialFailure(t *testing.T) {
	ctx, s, st := managerTestDB(t)
	managerTestChannel(t, ctx, 1, 1, 0, 0)
	var ch entity.YoubanBotManagedChannel
	if err := dao.YoubanBotManagedChannel.Ctx(ctx).WherePri(1).Scan(&ch); err != nil {
		t.Fatal(err)
	}
	full := &tg.ChannelFull{ID: 1, About: "", ChatPhoto: &tg.PhotoEmpty{}, NotifySettings: tg.PeerNotifySettings{}, Pts: 1}
	full.SetAdminsCount(3)
	full.SetExportedInvite(&tg.ChatInviteExported{Link: "https://t.me/+test", AdminID: 7, Date: 1})
	calls := 0
	fail := false
	client := tg.NewClient(managerTestInvoker{invoke: func(ctx context.Context, in bin.Encoder, out bin.Decoder) error {
		switch request := in.(type) {
		case *tg.ChannelsGetFullChannelRequest:
			return managerTestResponse(out, &tg.MessagesChatFull{FullChat: full, Chats: []tg.ChatClass{&tg.Channel{ID: 1, Creator: true, Title: "test", Photo: &tg.ChatPhotoEmpty{}, Date: 1}}})
		case *tg.HelpGetAppConfigRequest:
			return errors.New("optional configuration unavailable")
		case *tg.ChannelsGetParticipantsRequest:
			calls++
			if fail {
				return errors.New("page download interrupted")
			}
			if _, ok := request.Filter.(*tg.ChannelParticipantsAdmins); ok {
				return managerTestResponse(out, &tg.ChannelsChannelParticipants{Count: 1, Participants: []tg.ChannelParticipantClass{&tg.ChannelParticipantAdmin{UserID: 1001, PromotedBy: 7, Date: 1, AdminRights: tg.ChatAdminRights{PostMessages: true, EditMessages: true, DeleteMessages: true}}}, Users: []tg.UserClass{&tg.User{ID: 1001, Bot: true, FirstName: "Bot", Username: "example_bot"}}})
			}
			return managerTestResponse(out, &tg.ChannelsChannelParticipants{Count: 0})
		default:
			return fmt.Errorf("unexpected RPC %T", in)
		}
	}})
	if err := s.managerSyncChannel(ctx, client, st, &ch, &tg.InputChannel{ChannelID: 1, AccessHash: 123}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("admin and bot pages were not both read", calls)
	}
	if err := dao.YoubanBotManagedChannel.Ctx(ctx).WherePri(1).Scan(&ch); err != nil {
		t.Fatal(err)
	}
	if ch.AdminCount != 3 || ch.AdminLimit != 0 || ch.CanManage != 1 || ch.BotCount != 1 || ch.VisitUrl != "https://t.me/+test" {
		t.Fatalf("incorrect sync projection: %+v", ch)
	}
	var bots []*entity.YoubanBotManaged
	if err := dao.YoubanBotManaged.Ctx(ctx).Scan(&bots); err != nil || len(bots) != 1 {
		t.Fatal(bots, err)
	}
	if bots[0].BotToken != "" {
		t.Fatal("discovered Bot must not invent credentials")
	}
	var members []*entity.YoubanBotManagedMember
	if err := dao.YoubanBotManagedMember.Ctx(ctx).Scan(&members); err != nil || len(members) != 1 || members[0].MemberStatus != "admin" {
		t.Fatal(members, err)
	}
	fail = true
	if err := s.managerSyncChannel(ctx, client, st, &ch, &tg.InputChannel{ChannelID: 1, AccessHash: 123}); err == nil {
		t.Fatal("partial sync succeeded")
	}
	if err := dao.YoubanBotManagedMember.Ctx(ctx).WherePri(members[0].Id).Scan(&members[0]); err != nil || members[0].MemberStatus != "admin" {
		t.Fatal("partial sync erased valid membership", err)
	}
	// A complete empty sync can remove a stale relationship; a failed sync cannot.
	_, err := dao.YoubanBotManaged.Ctx(ctx).Data(do.YoubanBotManaged{TenantId: 2, AccountId: 100, TelegramBotId: 1001, Status: 1}).Insert()
	if err != nil {
		t.Fatal("discovery must support multiple user views", err)
	}
}

func TestBotManagerTokenClaimUniqueness(t *testing.T) {
	ctx, _, _ := managerTestDB(t)
	for _, id := range []int64{1, 2} {
		_, err := dao.YoubanBotManaged.Ctx(ctx).Data(do.YoubanBotManaged{TenantId: 1, AccountId: id, TelegramBotId: 1001, Status: 1}).Insert()
		if err != nil {
			t.Fatal(err)
		}
	}
	c := dao.YoubanBotManaged.Columns()
	if _, err := dao.YoubanBotManaged.Ctx(ctx).Where(c.AccountId, 1).Data(do.YoubanBotManaged{BotToken: "1001:original"}).Update(); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotManaged.Ctx(ctx).Where(c.AccountId, 2).Data(do.YoubanBotManaged{BotToken: "1001:rotated"}).Update(); err == nil {
		t.Fatal("two accounts claimed same Bot with different tokens")
	}
}
