package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/os/gtime"
	ci "hotgo/addons/telegram_collector/model/input/sysin"
	cs "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
)

type managerTestPublish struct{ ps.ISysPublish }

func (managerTestPublish) BotManagerUpdateCredentials(context.Context, *pi.BotManagerProjection) (int64, error) {
	return 1, nil
}
func (managerTestPublish) BotManagerRefreshCredentials(context.Context) {}

func (managerTestPublish) BotManagerValidateToken(context.Context, int64, int64, string) error {
	return nil
}
func managerPreviousPublish() (previous ps.ISysPublish) {
	defer func() { _ = recover() }()
	return ps.SysPublish()
}

func TestBotManagerTokenInputAndReAdd(t *testing.T) {
	ctx, s, st := managerTestDB(t)
	relayTestBaseTable(t, ctx, botTable)
	if _, err := dao.YoubanBotBot.Ctx(ctx).Data(do.YoubanBotBot{Id: 1, BotToken: "123:fake", Status: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	previous := managerPreviousPublish()
	ps.RegisterSysPublish(managerTestPublish{})
	t.Cleanup(func() { ps.RegisterSysPublish(previous) })
	s.features = map[string]*botFeatureRow{"bot_management": {FeatureKey: "bot_management", Status: 1, ConfigJson: "{}"}}
	s.featureAt = time.Now()
	s.featureDefaultsAt = time.Now()
	var deletes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":10001,"is_bot":true,"first_name":"Test","username":"test_bot"}}`))
		case strings.HasSuffix(r.URL.Path, "/deleteMessage"):
			deletes.Add(1)
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":99}}`))
		}
	}))
	t.Cleanup(server.Close)
	client, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	s.relayClientForToken = func(context.Context, string) (*tgbot.Bot, error) { return client, nil }
	message := &models.Message{ID: 1, From: &models.User{ID: 12}, Chat: models.Chat{ID: 12, Type: models.ChatTypePrivate}, Text: "10001:abcdefghijklmnopqrstuvxyz"}
	var original int64
	for i := 0; i < 2; i++ {
		if _, err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{InputMode: "token"}).Update(); err != nil {
			t.Fatal(err)
		}
		handled, err := s.handleBotManagerInput(ctx, 1, message)
		if err != nil || !handled {
			t.Fatal(handled, err)
		}
		var b *entity.YoubanBotManaged
		if err := dao.YoubanBotManaged.Ctx(ctx).Where(dao.YoubanBotManaged.Columns().TelegramBotId, 10001).Scan(&b); err != nil || b == nil || b.BotToken != message.Text {
			t.Fatal(b, err)
		}
		if i == 0 {
			original = b.Id
			if _, err := dao.YoubanBotManaged.Ctx(ctx).WherePri(b.Id).Delete(); err != nil {
				t.Fatal(err)
			}
		} else if b.Id != original {
			t.Fatal("re-add changed canonical Bot ID")
		}
	}
	message.Text = "Token: " + message.Text
	if handled, err := s.handleBotManagerInput(ctx, 1, message); err != nil || !handled {
		t.Fatal("embedded credential escaped interception", handled, err)
	}
	if deletes.Load() != 3 {
		t.Fatal("sensitive input was not deleted", deletes.Load())
	}
	if _, err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{InputMode: "token"}).Update(); err != nil {
		t.Fatal(err)
	}
	message.Text = "/help"
	if handled, err := s.handleBotManagerInput(ctx, 1, message); err != nil || handled {
		t.Fatal("input mode trapped navigation", handled, err)
	}
	current, err := s.managerState(ctx, 1, 12)
	if err != nil || current.InputMode != "" {
		t.Fatal("navigation did not clear input mode", current, err)
	}
}

func managerTestHTTP(t *testing.T, ctx context.Context, s *sSysBot) <-chan string {
	t.Helper()
	relayTestBaseTable(t, ctx, botTable)
	_, err := dao.YoubanBotBot.Ctx(ctx).Data(do.YoubanBotBot{Id: 1, BotToken: "123:fake", Status: 1}).Insert()
	if err != nil {
		t.Fatal(err)
	}
	requests := make(chan string, 100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		requests <- r.FormValue("text") + "\n" + r.FormValue("reply_markup")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":99}}`))
	}))
	t.Cleanup(server.Close)
	client, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	s.relayClientForToken = func(context.Context, string) (*tgbot.Bot, error) { return client, nil }
	return requests
}

func TestBotManagerChatSelectionAndDatabaseSearch(t *testing.T) {
	ctx, s, st := managerTestDB(t)
	requests := managerTestHTTP(t, ctx, s)
	managerTestDraft(t, ctx, st, 1)
	st.DraftBotId = 1
	for id := int64(1); id <= 23; id++ {
		managerTestChannel(t, ctx, id, 3, 0, 0)
	}
	if err := s.managerSelection(ctx, st, 0, managerCommand{Action: "select", ID: 1, Page: 2, Revision: st.Revision}); err != nil {
		t.Fatal(err)
	}
	text := <-requests
	parts := strings.SplitN(text, "\n{", 2)
	if len(parts) != 2 {
		t.Fatal("missing inline markup", text)
	}
	var markup models.InlineKeyboardMarkup
	if err := json.Unmarshal([]byte("{"+parts[1]), &markup); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			if strings.HasPrefix(button.Text, "✅ ") {
				count++
			}
			if len(button.CallbackData) > 64 {
				t.Fatal("oversize callback")
			}
		}
	}
	if count != 10 || !strings.Contains(text, "管理员 3/未知") || !strings.Contains(text, "已选 23 个") {
		t.Fatal("wrong selection page", text)
	}
	if err := s.managerSelection(ctx, st, 0, managerCommand{Action: "toggle", ID: 12, Page: 2, Revision: 0}); err != nil {
		t.Fatal(err)
	}
	<-requests
	restored, err := s.managerState(ctx, 1, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(managerIDs(restored.SelectedChannelIds)) != 22 {
		t.Fatal("cross-page selection was not saved")
	}
	if err := s.managerSelection(ctx, restored, 0, managerCommand{Action: "toggle", ID: 13, Page: 2, Revision: 0}); err == nil {
		t.Fatal("stale toggle changed selection")
	}
	_, err = dao.YoubanBotManaged.Ctx(ctx).WherePri(1).Data(do.YoubanBotManaged{BotName: "City Bot", BotUsername: "city_bot"}).Update()
	if err != nil {
		t.Fatal(err)
	}
	restored.Keyword = "@city_bot"
	if err := s.managerList(ctx, restored, 0, 1); err != nil {
		t.Fatal(err)
	}
	if text := <-requests; !strings.Contains(text, "city_bot") {
		t.Fatal("local DB search missing Bot", text)
	}
}

type managerTestTasks struct {
	cs.IAccountTasks
	submitted int
	state     string
}

func (m *managerTestTasks) Submit(_ context.Context, in *ci.AccountTaskSubmit) (int64, error) {
	if in.Priority != -200 || in.MaxAttempts != 3 || in.TaskType != managerTaskType {
		return 0, fmt.Errorf("unsafe task policy")
	}
	m.submitted++
	return int64(1000 + m.submitted), nil
}
func (m *managerTestTasks) Get(_ context.Context, id int64) (*ci.AccountTask, error) {
	return &ci.AccountTask{ID: id, Status: m.state, ErrorMessage: "account unavailable"}, nil
}
func managerPreviousTasks() (previous cs.IAccountTasks) {
	defer func() { _ = recover() }()
	return cs.AccountTasks()
}

func TestBotManagerDispatcherRotatesBlockedAccountsAndExpires(t *testing.T) {
	ctx, s, st := managerTestDB(t)
	managerTestChannel(t, ctx, 1, 3, 0, 0)
	previous := managerPreviousTasks()
	fake := &managerTestTasks{state: ci.AccountTaskStatusProcessing}
	cs.RegisterAccountTasks(fake)
	t.Cleanup(func() { cs.RegisterAccountTasks(previous) })
	for i := 0; i < 21; i++ {
		id, err := managerNewTask(ctx, st, fmt.Sprintf("waiting-%d", i), "attach", int64(i+1), 1, "{}")
		if err != nil {
			t.Fatal(err)
		}
		if i < 20 {
			_, err = dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Data(do.YoubanBotManagerTask{Status: "submitted", AccountTaskId: 100 + int64(i)}).Update()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	for i := 0; i < 3 && fake.submitted == 0; i++ {
		if err := s.managerTick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if fake.submitted != 1 {
		t.Fatal("waiting tasks starved another account", fake.submitted)
	}
	id, err := managerNewTask(ctx, st, "expired", "attach", 100, 1, "{}")
	if err != nil {
		t.Fatal(err)
	}
	// Explicitly age a fixture; production time fields are ORM-managed.
	db := dao.YoubanBotManagerTask.DB()
	if _, err := db.Exec(ctx, "UPDATE hg_youban_bot_manager_task SET created_at=?,updated_at=? WHERE id=?", gtime.New(time.Now().Add(-time.Hour)), gtime.New(time.Now().Add(-time.Hour)), id); err != nil {
		t.Fatal(err)
	}
	// Avoid actual notifications in this scheduling test by leaving the same batch active.
	if _, err := managerNewTask(ctx, st, "expired", "attach", 101, 1, "{}"); err != nil {
		t.Fatal(err)
	}
	if err := s.managerTick(ctx); err != nil {
		t.Fatal(err)
	}
	var row entity.YoubanBotManagerTask
	if err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Scan(&row); err != nil || row.Status != "failed" {
		t.Fatal("expired pending task did not terminate", row, err)
	}
}

func TestBotManagerNotificationDoesNotStarveCompletedBatch(t *testing.T) {
	ctx, s, st := managerTestDB(t)
	requests := managerTestHTTP(t, ctx, s)
	for i := 0; i < 21; i++ {
		id, err := managerNewTask(ctx, st, "incomplete", "attach", int64(i), 1, "{}")
		if err != nil {
			t.Fatal(err)
		}
		if i < 20 {
			if _, err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Data(do.YoubanBotManagerTask{Status: "success"}).Update(); err != nil {
				t.Fatal(err)
			}
		}
	}
	id, err := managerNewTask(ctx, st, "complete", "attach", 100, 1, "{}")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Data(do.YoubanBotManagerTask{Status: "failed", ErrorMessage: "<bad>"}).Update(); err != nil {
		t.Fatal(err)
	}
	if err := s.managerNotifyCompleted(ctx); err != nil {
		t.Fatal(err)
	}
	if text := <-requests; !strings.Contains(text, "&lt;bad&gt;") {
		t.Fatal("notification failed or HTML was not escaped", text)
	}
	var row entity.YoubanBotManagerTask
	if err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Scan(&row); err != nil || row.Notified != 1 {
		t.Fatal(row, err)
	}
}

func TestBotManagerRetryCannotReplayOldOperation(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("requires PostgreSQL row locks")
	}
	ctx, _, st := managerTestDB(t)
	managerTestDraft(t, ctx, st, 1)
	id, err := managerNewTask(ctx, st, "failed", "attach", 1, 1, "{}")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Data(do.YoubanBotManagerTask{Status: "failed"}).Update(); err != nil {
		t.Fatal(err)
	}
	if _, err := managerRetry(ctx, st, id); err != nil {
		t.Fatal(err)
	}
	if _, err := managerRetry(ctx, st, id); err == nil {
		t.Fatal("old retry replayed")
	}
	other := *st
	other.AccountId = 100
	if _, err := managerRetry(ctx, &other, id); err == nil {
		t.Fatal("cross-account retry")
	}
}

func TestBotManagerRightsDraftDoesNotChangeSearch(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("requires PostgreSQL row locks")
	}
	ctx, s, st := managerTestDB(t)
	requests := managerTestHTTP(t, ctx, s)
	managerTestDraft(t, ctx, st, 1)
	managerTestChannel(t, ctx, 1, 3, 0, 0)
	if _, err := dao.YoubanBotManagerState.Ctx(ctx).WherePri(st.Id).Data(do.YoubanBotManagerState{Keyword: "city"}).Update(); err != nil {
		t.Fatal(err)
	}
	id, err := dao.YoubanBotManagedMember.Ctx(ctx).Data(do.YoubanBotManagedMember{TenantId: 1, AccountId: 99, ManagedBotId: 1, ChannelId: 1, MemberStatus: "admin", RightsJson: managerJSON(managerDefaultRights())}).InsertAndGetId()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.managerMemberAction(ctx, st, 0, managerCommand{Action: "rights", ID: id}); err != nil {
		t.Fatal(err)
	}
	<-requests
	current, err := s.managerState(ctx, 1, 12)
	if err != nil {
		t.Fatal(err)
	}
	if current.Keyword != "city" || current.DraftRightsJson != managerJSON(managerDefaultRights()) {
		t.Fatal("rights editing polluted search", current)
	}
	if err := s.managerMemberAction(ctx, current, 0, managerCommand{Action: "saveRights", ID: id, Revision: current.Revision}); err != nil {
		t.Fatal(err)
	}
	<-requests
	if err := s.managerMemberAction(ctx, current, 0, managerCommand{Action: "saveRights", ID: id, Revision: current.Revision}); err == nil {
		t.Fatal("duplicate rights save accepted")
	}
}
