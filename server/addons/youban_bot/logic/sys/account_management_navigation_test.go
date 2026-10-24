package sys

import (
	"context"
	"fmt"
	tgbot "github.com/go-telegram/bot"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	gs "hotgo/addons/youban_tg_bot_gateway/service"
)

func TestAccountManagementNavigationIsolation(t *testing.T) {
	ctx := relayTestDatabase(t)
	db, _ := gdb.Instance()
	if _, err := db.Exec(ctx, "DROP TABLE IF EXISTS "+profileSessionTable); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "CREATE TABLE "+profileSessionTable+" (id integer primary key, bot_id bigint, telegram_user_id text, chat_id text, scene text, status text, updated_at timestamp)"); err != nil {
		t.Fatal(err)
	}
	if err := db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id, bot     int
		user, scene string
	}{
		{1, 1, "11", "account_setting"}, {2, 1, "11", "edit_text"}, {3, 2, "11", "account_setting"}, {4, 1, "12", "account_setting"}, {5, 1, "11", "account_renewal"}, {6, 1, "11", "account_delete"},
	} {
		if _, err := db.Exec(ctx, "INSERT INTO "+profileSessionTable+"(id,bot_id,telegram_user_id,chat_id,scene,status) VALUES(?,?,?,?,?,'active')", row.id, row.bot, row.user, "11", row.scene); err != nil {
			t.Fatal(err)
		}
	}
	bot := &sSysBot{}
	if err := bot.cancelAccountInputForNavigation(ctx, 1, "11", "11", "acct:renew_confirm:5~p2"); err != nil {
		t.Fatal(err)
	}
	value, err := db.Model(profileSessionTable).Ctx(ctx).Where("id", 5).Value("status")
	if err != nil || value.String() != "active" {
		t.Fatalf("renewal confirmation canceled: %v %v", value, err)
	}
	if err := bot.cancelAccountInputForNavigation(ctx, 1, "11", "11", "acct:mine"); err != nil {
		t.Fatal(err)
	}
	for id := 1; id <= 6; id++ {
		value, err := db.Model(profileSessionTable).Ctx(ctx).Where("id", id).Value("status")
		want := "active"
		if id == 1 || id == 5 {
			want = "canceled"
		}
		if err != nil || value.String() != want {
			t.Fatalf("session %d: %v %v want %s", id, value, err, want)
		}
	}
}
func TestAccountManagementNavigationPageMarkup(t *testing.T) {
	original := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "设置", CallbackData: "acct:section:100:suffix"}}}}
	ctx := accountMenuWithPage(context.Background(), 3)
	got := accountMenuMarkup(ctx, original).(*models.InlineKeyboardMarkup)
	if got.InlineKeyboard[0][0].CallbackData != "acct:section:100:suffix~p3" {
		t.Fatal(got)
	}
	if strings.Contains(original.InlineKeyboard[0][0].CallbackData, "~p") {
		t.Fatal("mutated shared keyboard")
	}
	again := accountMenuMarkup(ctx, got).(*models.InlineKeyboardMarkup)
	if again.InlineKeyboard[0][0].CallbackData != got.InlineKeyboard[0][0].CallbackData {
		t.Fatal("duplicated page")
	}
}

func TestAccountManagementMenuEditTransport(t *testing.T) {
	for _, test := range []struct {
		name, failure string
		sends         int
	}{
		{"edit", "", 0},
		{"unchanged", "Bad Request: message is not modified", 0},
		{"deleted", "Bad Request: message to edit not found", 1},
		{"unknown outcome", "upstream timeout", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := relayTestDatabase(t)
			bot := &sSysBot{}
			_ = managerTestHTTP(t, ctx, bot)
			sends, edits := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseMultipartForm(1 << 20)
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/editMessageText") {
					edits++
					if r.FormValue("message_id") != "55" {
						t.Errorf("wrong edited message %s", r.FormValue("message_id"))
					}
					if test.failure != "" {
						w.WriteHeader(400)
						fmt.Fprintf(w, `{"ok":false,"error_code":400,"description":%q}`, test.failure)
						return
					}
				} else {
					sends++
				}
				fmt.Fprint(w, `{"ok":true,"result":{"message_id":55}}`)
			}))
			defer server.Close()
			client, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
			if err != nil {
				t.Fatal(err)
			}
			bot.relayClientForToken = func(context.Context, string) (*tgbot.Bot, error) { return client, nil }
			var previous gs.IGateway
			func() { defer func() { _ = recover() }(); previous = gs.Gateway() }()
			gs.RegisterGateway(accountRenewalTestGateway{client: bot.relayClientForToken})
			t.Cleanup(func() { gs.RegisterGateway(previous) })
			ctx = context.WithValue(ctx, accountMenuContextKey{}, accountMenuContext{MessageID: 55, Page: 2})
			err = bot.accountMenuReply(ctx, 1, "11", "menu", &models.InlineKeyboardMarkup{})
			if test.name == "unknown outcome" && err == nil {
				t.Fatal("unknown edit outcome swallowed")
			}
			if test.name != "unknown outcome" && err != nil {
				t.Fatal(err)
			}
			if edits != 1 || sends != test.sends {
				t.Fatalf("edits=%d sends=%d want sends=%d", edits, sends, test.sends)
			}
		})
	}
}
