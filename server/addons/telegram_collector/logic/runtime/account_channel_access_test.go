package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestAccountChannelAccessEvents(t *testing.T) {
	for _, tc := range []struct {
		name   string
		self   int64
		update tg.UpdatesClass
		want   string
	}{
		{"self kicked", 42, &tg.UpdateShort{Update: &tg.UpdateChannelParticipant{ChannelID: 109, UserID: 42, Date: 100, NewParticipant: &tg.ChannelParticipantBanned{BannedRights: tg.ChatBannedRights{ViewMessages: true}}}}, sysin.AccountChannelAccessRemoved},
		{"other user kicked", 42, &tg.UpdateShort{Update: &tg.UpdateChannelParticipant{ChannelID: 109, UserID: 99, NewParticipant: &tg.ChannelParticipantBanned{Left: true}}}, ""},
		{"self unknown", 0, &tg.UpdateShort{Update: &tg.UpdateChannelParticipant{ChannelID: 109, UserID: 42, NewParticipant: &tg.ChannelParticipantLeft{}}}, ""},
		{"self left", 42, &tg.UpdateShort{Update: &tg.UpdateChannelParticipant{ChannelID: 109, UserID: 42, NewParticipant: &tg.ChannelParticipantLeft{}}}, sysin.AccountChannelAccessLeft},
		{"send restricted only", 42, &tg.UpdateShort{Update: &tg.UpdateChannelParticipant{ChannelID: 109, UserID: 42, NewParticipant: &tg.ChannelParticipantBanned{BannedRights: tg.ChatBannedRights{SendMessages: true}}}}, ""},
		{"missing status", 42, &tg.UpdateShort{Update: &tg.UpdateChannelParticipant{ChannelID: 109, UserID: 42}}, ""},
		{"promoted", 42, &tg.UpdateShort{Update: &tg.UpdateChannelParticipant{ChannelID: 109, UserID: 42, NewParticipant: &tg.ChannelParticipantAdmin{}}}, ""},
		{"channel inaccessible", 42, &tg.Updates{Date: 100, Updates: []tg.UpdateClass{&tg.UpdateChannel{ChannelID: 109}}, Chats: []tg.ChatClass{&tg.ChannelForbidden{ID: 109, Title: "嘉祐"}}}, sysin.AccountChannelAccessForbidden},
		{"channel left", 42, &tg.UpdatesCombined{Date: 100, Updates: []tg.UpdateClass{&tg.UpdateChannel{ChannelID: 109}}, Chats: []tg.ChatClass{&tg.Channel{ID: 109, Title: "嘉祐", Left: true}}}, sysin.AccountChannelAccessLeft},
		{"channel renamed", 42, &tg.Updates{Updates: []tg.UpdateClass{&tg.UpdateChannel{ChannelID: 109}}, Chats: []tg.ChatClass{&tg.Channel{ID: 109, Title: "新名字"}}}, ""},
		{"partial channel", 42, &tg.Updates{Updates: []tg.UpdateClass{&tg.UpdateChannel{ChannelID: 109}}, Chats: []tg.ChatClass{&tg.Channel{ID: 109, Min: true, Left: true}}}, ""},
		{"unrelated forbidden entity", 42, &tg.Updates{Chats: []tg.ChatClass{&tg.ChannelForbidden{ID: 109}}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := accountChannelAccessEvents(tc.self, tc.update)
			if tc.want == "" {
				if len(events) != 0 {
					t.Fatalf("false alarm: %+v", events)
				}
				return
			}
			if len(events) != 1 || events[0].Reason != tc.want || events[0].ChannelID != 109 || events[0].UserID != 42 {
				t.Fatalf("unexpected events: %+v", events)
			}
		})
	}
}

type channelAccessTestSession struct {
	events []sysin.AccountChannelAccessEvent
}

func (*channelAccessTestSession) UpdateAccountRuntime(*sysin.AccountRuntimeBinding) bool {
	return false
}
func (*channelAccessTestSession) NewAccountRuntimeClient(context.Context, telegram.UpdateHandler) (*telegram.Client, error) {
	return nil, nil
}
func (*channelAccessTestSession) StartAccountRuntime(context.Context, *telegram.Client) {}
func (*channelAccessTestSession) StopAccountRuntime()                                   {}
func (*channelAccessTestSession) HandleAccountRuntimeError(context.Context, error)      {}
func (s *channelAccessTestSession) HandleAccountRuntimeChannelAccess(_ context.Context, event sysin.AccountChannelAccessEvent) error {
	s.events = append(s.events, event)
	return errors.New("queue unavailable")
}

func TestAccountChannelAccessFailurePreservesMessages(t *testing.T) {
	session := &channelAccessTestSession{}
	w := &accountWorker{session: session}
	w.selfUserID.Store(42)
	d := tg.NewUpdateDispatcher()
	messages := 0
	d.OnNewChannelMessage(func(context.Context, tg.Entities, *tg.UpdateNewChannelMessage) error { messages++; return nil })
	update := &tg.Updates{Date: 100, Chats: []tg.ChatClass{&tg.ChannelForbidden{ID: 109, Title: "嘉祐"}}, Updates: []tg.UpdateClass{
		&tg.UpdateChannel{ChannelID: 109},
		&tg.UpdateChannelParticipant{ChannelID: 109, UserID: 42, Date: 100, NewParticipant: &tg.ChannelParticipantBanned{Left: true}},
		&tg.UpdateNewChannelMessage{Message: &tg.Message{ID: 1}},
	}}
	if err := w.channelAccessUpdateHandler(d).Handle(context.Background(), update); err != nil {
		t.Fatal(err)
	}
	if messages != 1 || len(session.events) != 1 || session.events[0].Reason != sysin.AccountChannelAccessRemoved {
		t.Fatalf("messages=%d events=%+v", messages, session.events)
	}
}
