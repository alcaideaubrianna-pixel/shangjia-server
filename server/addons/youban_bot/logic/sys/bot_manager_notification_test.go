package sys

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/util/guid"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
)

func TestBotManagerNotificationFailureIsolation(t *testing.T) {
	ctx, s, st := managerTestDB(t)
	relayTestBaseTable(t, ctx, botTable)
	if _, err := dao.YoubanBotBot.Ctx(ctx).Data(do.YoubanBotBot{Id: 1, BotToken: "123:fake", Status: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	var failed, delivered atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		w.Header().Set("Content-Type", "application/json")
		if r.FormValue("chat_id") == "12" {
			failed.Add(1)
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"temporary send failure"}`))
			return
		}
		delivered.Add(1)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":99}}`))
	}))
	t.Cleanup(server.Close)
	client, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	s.relayClientForToken = func(context.Context, string) (*tgbot.Bot, error) { return client, nil }
	batch := guid.S()
	var first int64
	for i := 0; i < 25; i++ {
		id, err := managerNewTask(ctx, st, batch, "attach", int64(i+1), 1, "{}")
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = id
		}
		if _, err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Data(do.YoubanBotManagerTask{Status: "success"}).Update(); err != nil {
			t.Fatal(err)
		}
	}
	other, err := s.managerState(ctx, 1, 23)
	if err != nil {
		t.Fatal(err)
	}
	id, err := managerNewTask(ctx, other, guid.S(), "attach", 1, 1, "{}")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Data(do.YoubanBotManagerTask{Status: "success"}).Update()
	// Dispatch maintenance must not perform any notification network I/O.
	if err := s.managerTick(ctx); err != nil {
		t.Fatal(err)
	}
	if failed.Load() != 0 || delivered.Load() != 0 {
		t.Fatal("dispatch still coupled to notifications")
	}
	if err := s.managerNotifyCompleted(ctx); err != nil {
		t.Fatal(err)
	}
	if failed.Load() != 1 || delivered.Load() != 1 {
		t.Fatalf("batch starvation or duplicate attempts: failed=%d delivered=%d", failed.Load(), delivered.Load())
	}
	for _, tc := range []struct {
		id       int64
		notified int
	}{{first, 0}, {id, 1}} {
		var row entity.YoubanBotManagerTask
		if err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(tc.id).Scan(&row); err != nil || row.Notified != tc.notified {
			t.Fatal("notification result was lost", row, err)
		}
	}
}

func TestBotManagerNotificationLookupFailureRemainsPending(t *testing.T) {
	ctx, s, st := managerTestDB(t)
	id, err := managerNewTask(ctx, st, guid.S(), "attach", 1, 1, "{}")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Data(do.YoubanBotManagerTask{Status: "success"}).Update()
	s.relayAccountForUser = func(context.Context, int64) (*botProfileAccount, error) {
		return nil, errors.New("temporary database outage")
	}
	var row entity.YoubanBotManagerTask
	_ = dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Scan(&row)
	if err := s.managerNotifyBatch(ctx, &row); err == nil {
		t.Fatal("lookup outage reported success")
	}
	if err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Scan(&row); err != nil || row.Notified != 0 {
		t.Fatal("transient lookup lost notification", row, err)
	}
}
