package sys

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/addons/youban_bot/model/input/sysin"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"
	"github.com/gogf/gf/v2/database/gdb"
)

func relayTestDatabase(t *testing.T) context.Context {
	t.Helper()
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	database := filepath.Join(relaySQLiteDirectory, "relay.sqlite")
	if link := os.Getenv("YOUBAN_RELAY_TEST_PG"); link != "" {
		if !strings.Contains(link, "127.0.0.1") || !strings.HasSuffix(link, "/youban_relay_test") {
			t.Fatal("test database must be local and named youban_relay_test")
		}
		gdb.SetConfigGroup("default", gdb.ConfigGroup{{Link: link}})
		db, err := gdb.Instance()
		if err != nil {
			t.Fatal(err)
		}
		ddl, err := os.ReadFile("../../resource/sql/relay.pgsql.sql")
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"config", "admin", "binding", "endpoint", "rule", "audit_topic", "delivery"} {
			if _, err = db.Exec(context.Background(), "DROP TABLE IF EXISTS hg_youban_bot_relay_"+name); err != nil {
				t.Fatal(err)
			}
		}
		for _, statement := range strings.Split(string(ddl), ";") {
			if strings.TrimSpace(statement) != "" {
				if _, err = db.Exec(context.Background(), statement); err != nil {
					t.Fatal(err)
				}
			}
		}
		return context.Background()
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "sqlite", Name: database}})
	// GoFrame's frame-level DB singleton retains its initial SQLite schema path.
	// Reuse one process-scoped file, resetting tables between tests, so both the
	// frame singleton and gdb instances address the same temporary database.
	db, err := gdb.Instance()
	if err != nil {
		t.Fatal(err)
	}
	ddl, err := os.ReadFile("../../resource/sql/relay.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config", "admin", "binding", "endpoint", "rule", "audit_topic", "delivery"} {
		if _, err := db.Exec(context.Background(), "DROP TABLE IF EXISTS hg_youban_bot_relay_"+name); err != nil {
			t.Fatal(err)
		}
	}
	sql := strings.ReplaceAll(string(ddl), "bigserial PRIMARY KEY", "integer PRIMARY KEY AUTOINCREMENT")
	for _, statement := range strings.Split(sql, ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err = db.Exec(context.Background(), statement); err != nil {
			t.Fatal(err)
		}
	}
	return context.Background()
}

func TestRelayCommandsAndPairIdentity(t *testing.T) {
	cases := []struct{ input, name, args string }{{"绑定@alice-@bob", "绑定", "@alice-@bob"}, {"屏蔽@alice", "屏蔽", "@alice"}, {"删除屏蔽 联系 客服", "删除屏蔽", "联系 客服"}, {"解除屏蔽@alice", "解除屏蔽", "@alice"}, {"/help@mybot", "/help", ""}}
	for _, tc := range cases {
		got := parseRelayCommand(tc.input)
		if got.Name != tc.name || got.Args != tc.args {
			t.Errorf("%q: %+v", tc.input, got)
		}
	}
	for _, value := range []string{"@alice-@bob", "-1001234567890 @alice", "-1001234567890 -1009876543210"} {
		a, b, err := relayBindArguments(value)
		if err != nil || a == "" || b == "" {
			t.Fatalf("%q: %v", value, err)
		}
		if relayPairKey(a, b) != relayPairKey(b, a) {
			t.Fatal("pair is directional")
		}
	}
	if _, _, err := relayBindArguments("-100123-@alice"); err == nil {
		t.Fatal("ambiguous negative IDs accepted")
	}
}

func TestRelayFilterHiddenLinksAndGroupUsers(t *testing.T) {
	cfg := &entity.YoubanBotRelayConfig{BlockLinks: 1}
	for _, msg := range []*models.Message{{Text: "a@example.com"}, {Caption: "T.ME/example"}, {Text: "click", Entities: []models.MessageEntity{{Type: models.MessageEntityTypeTextLink, URL: "https://t.me/example"}}}, {Caption: "user", CaptionEntities: []models.MessageEntity{{Type: models.MessageEntityTypeTextMention}}}} {
		if !relayHasJump(msg) {
			t.Errorf("jump escaped: %+v", msg)
		}
	}
	msg := &models.Message{Text: "normal", From: &models.User{ID: 12}, Chat: models.Chat{ID: -100, Type: models.ChatTypeSupergroup}}
	rules := []*entity.YoubanBotRelayRule{{Kind: consts.RelayRuleUser, Value: "12"}}
	if relayFilterReason(cfg, rules, msg, "13") == "" {
		t.Fatal("blocked group sender forwarded")
	}
	rules = []*entity.YoubanBotRelayRule{{Kind: consts.RelayRuleKeyword, Value: "spam"}}
	msg.Caption = "SpAm"
	if relayFilterReason(cfg, rules, msg, "13") == "" {
		t.Fatal("caption keyword forwarded")
	}
}

func TestRelayDurableDedupAndAuditPair(t *testing.T) {
	ctx := relayTestDatabase(t)
	s := NewSysBot()
	cfg := &entity.YoubanBotRelayConfig{BotId: 1, AuditDatabase: 1, AuditGroupId: "-100999"}
	binding := &entity.YoubanBotRelayBinding{Id: 10, PairKey: relayPairKey("12", "13"), LeftChatId: "12", RightChatId: "13", LeftName: "Alice", RightName: "Bob"}
	a := &models.Message{ID: 2, Text: "hello", From: &models.User{ID: 12}, Chat: models.Chat{ID: 12, Type: models.ChatTypePrivate}}
	for i := 0; i < 2; i++ {
		if err := s.enqueueRelayMessage(ctx, cfg, "bot", a, binding, "13", "", false); err != nil {
			t.Fatal(err)
		}
	}
	b := &models.Message{ID: 3, Text: "reply", From: &models.User{ID: 13}, Chat: models.Chat{ID: 13, Type: models.ChatTypePrivate}}
	if err := s.enqueueRelayMessage(ctx, cfg, "bot", b, binding, "12", "", false); err != nil {
		t.Fatal(err)
	}
	var rows []*entity.YoubanBotRelayDelivery
	if err := dao.YoubanBotRelayDelivery.Ctx(ctx).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 6 {
		t.Fatalf("got %d tasks, want 6 (2 archives + 2 audits + 2 relays)", len(rows))
	}
	for _, r := range rows {
		if r.PairKey != binding.PairKey || r.TopicTitle != "Alice-Bob" {
			t.Fatalf("pair topic differs: %+v", r)
		}
		if r.Kind == consts.RelayKindArchive && r.Status != consts.RelayStored {
			t.Fatal("archive is transient")
		}
	}
}

func TestRelayFilteredAlbumCannotBypassCaption(t *testing.T) {
	ctx := relayTestDatabase(t)
	s := NewSysBot()
	cfg := &entity.YoubanBotRelayConfig{BotId: 1}
	binding := &entity.YoubanBotRelayBinding{Id: 10, PairKey: "12:13", LeftChatId: "12", RightChatId: "13", LeftName: "A", RightName: "B"}
	msg := &models.Message{ID: 1, Caption: "blocked", MediaGroupID: "album", From: &models.User{ID: 12}, Chat: models.Chat{ID: 12}, Photo: []models.PhotoSize{{FileID: "photo"}}}
	if err := s.enqueueRelayMessage(ctx, cfg, "bot", msg, binding, "13", "已拦截", false); err != nil {
		t.Fatal(err)
	}
	msg.ID = 2
	msg.Caption = ""
	if err := s.enqueueRelayMessage(ctx, cfg, "bot", msg, binding, "13", "", false); err != nil {
		t.Fatal(err)
	}
	var rows []*entity.YoubanBotRelayDelivery
	if err := dao.YoubanBotRelayDelivery.Ctx(ctx).Where(dao.YoubanBotRelayDelivery.Columns().Status, consts.RelayPending).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("pending=%d", len(rows))
	}
	reason, err := relayPendingFilter(ctx, cfg, rows)
	if err != nil || reason == "" {
		t.Fatalf("filtered album escaped: %q %v", reason, err)
	}
}

func TestRelayUnknownSendMustNotRetry(t *testing.T) {
	if relaySendErrorStatus(context.DeadlineExceeded, 1) != consts.RelayUncertain {
		t.Fatal("uncertain send retried")
	}
	if relaySendErrorStatus(&tgbot.TooManyRequestsError{RetryAfter: 30}, 1) != consts.RelayPending {
		t.Fatal("429 not retried")
	}
	if relaySendErrorStatus(tgbot.ErrorForbidden, 1) != consts.RelayFailed {
		t.Fatal("forbidden retried")
	}
	if relaySendErrorStatus(errors.New("EOF"), 1) != consts.RelayUncertain {
		t.Fatal("EOF retried")
	}
}

func TestRelayHelpAndSettingsValidation(t *testing.T) {
	for _, help := range []string{relayPublicHelp, relayAdminHelp} {
		if strings.Contains(help, "审计") || strings.Contains(help, "审核") {
			t.Fatal("audit exposed in help")
		}
	}
	if strings.Count(relayAdminHelp, "<code>") != strings.Count(relayAdminHelp, "</code>") {
		t.Fatal("invalid help markup")
	}
	in := &sysin.RelaySettingsInp{RelayBotInp: sysin.RelayBotInp{BotId: 1}, Enabled: 1, AuditDatabase: 1, AuditGroupId: "123"}
	if in.Filter() == nil {
		t.Fatal("private chat accepted as audit group")
	}
	raw, _ := json.Marshal(map[string]interface{}{"botId": 1, "enabled": 1, "auditDatabase": 1, "admins": "@alice\n123", "keywords": "广告\nSPAM"})
	got, words, err := relaySettingsFromFeature(string(raw))
	if err != nil || got.BotId != 1 || len(words) != 2 || len(got.Admins) != 2 {
		t.Fatalf("feature settings: %+v %+v %v", got, words, err)
	}
}

func TestRelayAuditTopicReusedInBothDirections(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("requires local PostgreSQL row locks; set YOUBAN_RELAY_TEST_PG")
	}
	ctx := relayTestDatabase(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_thread_id":77,"name":"Alice-Bob","icon_color":7322096}}`))
	}))
	defer server.Close()
	bot, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	s := NewSysBot()
	for _, pair := range [][2]string{{"12", "13"}, {"13", "12"}} {
		id, err := s.relayAuditThread(ctx, bot, &entity.YoubanBotRelayDelivery{BotId: 1, TargetChatId: "-100999", PairKey: relayPairKey(pair[0], pair[1]), TopicTitle: "Alice-Bob"})
		if err != nil || id != 77 {
			t.Fatalf("topic=%d err=%v", id, err)
		}
	}
	if calls != 1 {
		t.Fatalf("created %d topics for same pair", calls)
	}
}

func TestRelaySenderPreservesMediaAndAuditAttribution(t *testing.T) {
	ctx := relayTestDatabase(t)
	db, _ := gdb.Instance()

	baseDDL, err := os.ReadFile("../../resource/sql/install.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	statement := strings.Split(string(baseDDL), ";")[0]
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		statement = strings.ReplaceAll(statement, "bigserial PRIMARY KEY", "integer PRIMARY KEY AUTOINCREMENT")
	}
	if _, err := db.Exec(ctx, statement); err != nil {
		t.Fatal(err)
	}
	if err := db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(ctx, `DELETE FROM hg_youban_bot_bot WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_bot (id,bot_token,bot_name,status) VALUES (1,'123:fake','Test',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotRelayConfig.Ctx(ctx).Data(do.YoubanBotRelayConfig{BotId: 1, OwnerId: 1, Enabled: 1, AuditGroupId: "-100999"}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotRelayBinding.Ctx(ctx).Data(do.YoubanBotRelayBinding{Id: 10, BotId: 1, PairKey: "12:13", LeftChatId: "12", RightChatId: "13", LeftName: "A", RightName: "B"}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotRelayAuditTopic.Ctx(ctx).Data(do.YoubanBotRelayAuditTopic{BotId: 1, GroupId: "-100999", PairKey: "12:13", ThreadId: 77}).Insert(); err != nil {
		t.Fatal(err)
	}
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		var payload struct {
			MessageIDs []int `json:"message_ids"`
			ThreadID   int   `json:"message_thread_id"`
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		if err := json.Unmarshal([]byte(r.FormValue("message_ids")), &payload.MessageIDs); err != nil {
			t.Error(err)
		}
		payload.ThreadID, _ = strconv.Atoi(r.FormValue("message_thread_id"))
		if len(payload.MessageIDs) != 1 || payload.MessageIDs[0] != 2 {
			t.Errorf("source IDs lost: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":[{"message_id":100}]}`))
	}))
	defer server.Close()
	bot, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	s := NewSysBot()
	s.relayClientForToken = func(context.Context, string) (*tgbot.Bot, error) { return bot, nil }
	msg := models.Message{ID: 2, Text: "hello", Chat: models.Chat{ID: 12}, From: &models.User{ID: 12}}
	raw, _ := json.Marshal(msg)
	row := &entity.YoubanBotRelayDelivery{BotId: 1, BindingId: 10, SourceChatId: "12", SourceMessageId: 2, TargetChatId: "13", Kind: consts.RelayKindChat, PairKey: "12:13", RawJson: string(raw)}
	ids, status, err := s.sendRelayDeliveries(ctx, []*entity.YoubanBotRelayDelivery{row})
	if err != nil || status != consts.RelaySent || len(ids) != 1 || ids[0] != 100 {
		t.Fatalf("copy failed %v %s %v", ids, status, err)
	}
	if !strings.HasSuffix(calls[0], "/copyMessages") {
		t.Fatalf("unexpected method %v", calls)
	}
	if os.Getenv("YOUBAN_RELAY_TEST_PG") != "" {
		row.Kind = consts.RelayKindAudit
		row.TargetChatId = "-100999"
		_, status, err = s.sendRelayDeliveries(ctx, []*entity.YoubanBotRelayDelivery{row})
		if err != nil || status != consts.RelaySent {
			t.Fatalf("audit forward failed %s %v", status, err)
		}
		if !strings.HasSuffix(calls[1], "/forwardMessages") {
			t.Fatalf("audit source identity lost %v", calls)
		}
	}
}
