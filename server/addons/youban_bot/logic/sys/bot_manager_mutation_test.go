package sys

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
)

type managerMutationPublish struct {
	ps.ISysPublish
	apply  func(*pi.BotManagerProjection) (int64, error)
	remove func(*pi.BotManagerProjection, func() error) error
}

func (p managerMutationPublish) BotManagerRemoveFromChannel(_ context.Context, in *pi.BotManagerProjection, remove func() error) error {
	return p.remove(in, remove)
}

func TestBotManagerRemoveDoesNotReplayOnRefreshFailure(t *testing.T) {
	ctx, s, st := managerTestDB(t)
	managerTestChannel(t, ctx, 1, 1, 10, 1)
	managerTestDraft(t, ctx, st, 1)
	_, _ = dao.YoubanBotManaged.Ctx(ctx).WherePri(1).Data(do.YoubanBotManaged{BotUsername: "example_bot", PublishBotId: 1}).Update()
	var ch entity.YoubanBotManagedChannel
	_ = dao.YoubanBotManagedChannel.Ctx(ctx).WherePri(1).Scan(&ch)
	previous := managerPreviousPublish()
	t.Cleanup(func() { ps.RegisterSysPublish(previous) })
	ps.RegisterSysPublish(managerMutationPublish{remove: func(in *pi.BotManagerProjection, remove func() error) error {
		if !in.Remove {
			t.Fatal("remove lost unbinding intent")
		}
		return remove()
	}})
	absent, kicks := false, 0
	client := tg.NewClient(managerTestInvoker{invoke: func(_ context.Context, in bin.Encoder, out bin.Decoder) error {
		switch in.(type) {
		case *tg.ContactsResolveUsernameRequest:
			u := &tg.User{ID: 1001, Bot: true, FirstName: "Bot", Username: "example_bot"}
			u.SetAccessHash(123)
			return managerTestResponse(out, &tg.ContactsResolvedPeer{Peer: &tg.PeerUser{UserID: 1001}, Users: []tg.UserClass{u}})
		case *tg.ChannelsGetParticipantRequest:
			if absent {
				return tgerr.New(400, "USER_NOT_PARTICIPANT")
			}
			return managerTestResponse(out, &tg.ChannelsChannelParticipant{Participant: &tg.ChannelParticipantAdmin{UserID: 1001, PromotedBy: 7, Date: 1}})
		case *tg.ChannelsEditBannedRequest:
			kicks++
			absent = true
			return managerTestResponse(out, &tg.Updates{})
		case *tg.ChannelsGetFullChannelRequest:
			return errors.New("refresh failed after successful kick")
		default:
			return fmt.Errorf("unexpected RPC %T", in)
		}
	}})
	for attempt := 0; attempt < 2; attempt++ {
		if err := s.managerMutateChannel(ctx, client, st, &entity.YoubanBotManagerTask{Action: "remove", ManagedBotId: 1, RightsJson: "{}"}, &ch, &tg.InputChannel{ChannelID: 1, AccessHash: 123}); err != nil {
			t.Fatal(err)
		}
	}
	if kicks != 1 {
		t.Fatalf("already absent Bot was kicked again: %d", kicks)
	}
}

func (p managerMutationPublish) BotManagerApplyBinding(_ context.Context, in *pi.BotManagerProjection) (int64, error) {
	return p.apply(in)
}

func TestBotManagerRightsRevocationAndRefreshFailure(t *testing.T) {
	for _, post := range []bool{false, true} {
		for _, rpcFailure := range []bool{false, true} {
			t.Run(fmt.Sprintf("post=%t/rpcFailure=%t", post, rpcFailure), func(t *testing.T) {
				ctx, s, st := managerTestDB(t)
				managerTestChannel(t, ctx, 1, 1, 10, 1)
				managerTestDraft(t, ctx, st, 1)
				var ch entity.YoubanBotManagedChannel
				var b entity.YoubanBotManaged
				_ = dao.YoubanBotManagedChannel.Ctx(ctx).WherePri(1).Scan(&ch)
				_ = dao.YoubanBotManaged.Ctx(ctx).WherePri(1).Scan(&b)
				previous := managerPreviousPublish()
				t.Cleanup(func() { ps.RegisterSysPublish(previous) })
				bound, applied := true, 0
				ps.RegisterSysPublish(managerMutationPublish{apply: func(in *pi.BotManagerProjection) (int64, error) {
					bound = !in.Remove
					applied++
					return 1, nil
				}})
				client := tg.NewClient(managerTestInvoker{invoke: func(_ context.Context, in bin.Encoder, out bin.Decoder) error {
					switch in.(type) {
					case *tg.ChannelsGetParticipantRequest:
						return managerTestResponse(out, &tg.ChannelsChannelParticipant{Participant: &tg.ChannelParticipantAdmin{UserID: 1001, PromotedBy: 7, Date: 1, AdminRights: tg.ChatAdminRights{PostMessages: post, EditMessages: true, DeleteMessages: true}}})
					case *tg.ChannelsEditAdminRequest:
						if !post && bound {
							t.Fatal("remote posting permission revoked before unbinding")
						}
						if rpcFailure {
							return errors.New("remote timeout")
						}
						return managerTestResponse(out, &tg.Updates{})
					case *tg.ChannelsGetFullChannelRequest:
						return errors.New("refresh unavailable after mutation")
					default:
						return fmt.Errorf("unexpected RPC %T", in)
					}
				}})
				err := s.managerSetRights(ctx, client, st, &b, &ch, &tg.InputChannel{ChannelID: 1, AccessHash: 123}, &tg.InputUser{UserID: 1001, AccessHash: 123}, managerRights{post, true, true}, &pi.BotManagerProjection{BotId: 1})
				if (err != nil) != rpcFailure {
					t.Fatalf("optional refresh changed mutation result: %v", err)
				}
				if !post && bound || post && !rpcFailure && (!bound || applied != 1) {
					t.Fatalf("unsafe projection bound=%t calls=%d", bound, applied)
				}
				if post && rpcFailure && applied != 0 {
					t.Fatal("failed grant enabled publishing")
				}
			})
		}
	}
}

func TestBotManagerMutationsWaitForAutomaticRetries(t *testing.T) {
	ctx, _, st := managerTestDB(t)
	first, err := managerNewTask(ctx, st, "first", "rights", 1, 1, "{}")
	if err != nil {
		t.Fatal(err)
	}
	second, err := managerNewTask(ctx, st, "second", "remove", 1, 1, "{}")
	if err != nil {
		t.Fatal(err)
	}
	var row entity.YoubanBotManagerTask
	if err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(second).Scan(&row); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"pending", "submitted", "success", "failed"} {
		if _, err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(first).Data(do.YoubanBotManagerTask{Status: status}).Update(); err != nil {
			t.Fatal(err)
		}
		waiting, err := managerHasPredecessor(ctx, &row)
		if err != nil || waiting != (status == "pending" || status == "submitted") {
			t.Fatalf("status=%s waiting=%t err=%v", status, waiting, err)
		}
	}
	row.ChannelId = 2
	_, _ = dao.YoubanBotManagerTask.Ctx(ctx).WherePri(first).Data(do.YoubanBotManagerTask{Status: "submitted"}).Update()
	if waiting, err := managerHasPredecessor(ctx, &row); err != nil || waiting {
		t.Fatal("unrelated channel blocked", err)
	}
}
