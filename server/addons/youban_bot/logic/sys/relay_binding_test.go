package sys

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/model/input/sysin"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/database/gdb"
)

func TestRelayFourKindsAndConcurrentBindingConflict(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("requires PostgreSQL row locks")
	}
	ctx := relayTestDatabase(t)
	ctx = relayActorContext(ctx, 99)
	relayTestBaseTable(t, ctx, "hg_youban_bot_account_bind")
	if _, err := dao.YoubanBotRelayAdmin.Ctx(ctx).Data(do.YoubanBotRelayAdmin{BotId: 1, UserId: "99", Username: "99"}).Insert(); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{botTable, userTable, channelCacheTable} {
		relayTestBaseTable(t, ctx, table)
	}
	db, _ := gdb.Instance()
	if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_registry(id,bot_token,bot_name,status)VALUES(1,'123:fake','Test',1)`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{12, 13, 14} {
		if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_user(bot_id,telegram_user_id,telegram_first_name,status)VALUES(1,?,?,1)`, fmt.Sprint(id), fmt.Sprint(id)); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"-1001", "-1002"} {
		if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_channel_cache(bot_id,chat_id,chat_type,chat_title)VALUES(1,?,'supergroup',?)`, id, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).Data(do.YoubanBotRelayConfig{BotId: 1, TenantId: 1, AccountId: 99, Enabled: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		id := r.FormValue("chat_id")
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "getChat") {
			kind := "private"
			if strings.HasPrefix(id, "-") {
				kind = "supergroup"
			}
			_, _ = fmt.Fprintf(w, `{"ok":true,"result":{"id":%s,"type":"%s","title":"test"}}`, id, kind)
		} else {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"status":"administrator","user":{"id":123,"is_bot":true,"first_name":"bot"},"can_manage_topics":true}}`))
		}
	}))
	defer server.Close()
	client, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	s := NewSysBot()
	s.relayOwnedBots = relayTestOwnedBackendBots
	s.relayClientForToken = func(context.Context, string) (*tgbot.Bot, error) { return client, nil }
	base := sysin.RelayBotInp{BotId: 1}
	for _, pair := range [][2]string{{"12", "13"}, {"12", "-1001"}, {"-1001", "12"}, {"-1001", "-1002"}} {
		if err := s.AdminRelayBind(ctx, &sysin.RelayBindingInp{RelayBotInp: base, Left: pair[0], Right: pair[1]}); err != nil {
			t.Fatalf("bind %v: %v", pair, err)
		}
		binding, err := relayBindingByChat(ctx, 1, pair[0])
		if err != nil || binding == nil {
			t.Fatalf("missing binding %v", err)
		}
		if err := s.AdminRelayUnbind(ctx, &sysin.RelayDeleteInp{RelayBotInp: base, Id: binding.Id}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, right := range []string{"13", "14"} {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			<-start
			results <- s.AdminRelayBind(ctx, &sysin.RelayBindingInp{RelayBotInp: base, Left: "12", Right: target})
		}(right)
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent conflicting bindings accepted=%d", success)
	}
	count, err := dao.YoubanBotRelayEndpoint.Ctx(ctx).Where(do.YoubanBotRelayEndpoint{BotId: 1, ChatId: "12"}).Count()
	if err != nil || count != 1 {
		t.Fatalf("endpoint uniqueness failed %d %v", count, err)
	}
}
