package sys

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gtime"
	ci "hotgo/addons/telegram_collector/model/input/sysin"
	cs "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
	"hotgo/internal/library/cache"
)

func TestBotManagerProgressEditsSingleMessage(t *testing.T) {
	ctx, s, st := managerTestDB(t)
	adapter, err := gcfg.NewAdapterContent(`{"cache":{"adapter":"memory"}}`)
	if err != nil {
		t.Fatal(err)
	}
	previousAdapter := g.Cfg().GetAdapter()
	g.Cfg().SetAdapter(adapter)
	cache.SetAdapter(ctx)
	g.Cfg().SetAdapter(previousAdapter)
	relayTestBaseTable(t, ctx, botTable)
	_, _ = dao.YoubanBotBot.Ctx(ctx).Data(do.YoubanBotBot{Id: 1, BotToken: "123:fake", Status: 1}).Insert()
	type request struct{ path, messageID, text string }
	requests := make(chan request, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		requests <- request{r.URL.Path, r.FormValue("message_id"), r.FormValue("text")}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":77}}`))
	}))
	t.Cleanup(server.Close)
	client, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	s.relayClientForToken = func(context.Context, string) (*tgbot.Bot, error) { return client, nil }
	var root *entity.YoubanBotManagerTask
	for i := int64(1); i <= 3; i++ {
		managerTestChannel(t, ctx, i, 2, 10, 1)
		id, err := managerNewTask(ctx, st, "progress-batch", "sync", 0, i, "{}")
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			if err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Scan(&root); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := s.managerStartProgress(ctx, st, root); err != nil {
		t.Fatal(err)
	}
	if r := <-requests; !strings.HasSuffix(r.path, "/sendMessage") || !strings.Contains(r.text, "0/3") {
		t.Fatal("initial progress missing", r)
	}
	_, _ = dao.YoubanBotManagerTask.Ctx(ctx).WherePri(root.Id).Data(do.YoubanBotManagerTask{Status: "success"}).Update()
	if err := s.managerUpdateSyncProgress(ctx); err != nil {
		t.Fatal(err)
	}
	if r := <-requests; !strings.HasSuffix(r.path, "/editMessageText") || r.messageID != "77" || !strings.Contains(r.text, "33%") {
		t.Fatal("progress did not edit original message", r)
	}
	if err := s.managerUpdateSyncProgress(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-requests:
		t.Fatal("unchanged progress generated a duplicate Telegram request", r)
	default:
	}
	// Simulate a new runtime using the same DB and shared progress pointer.
	restarted := NewSysBot()
	restarted.relayClientForToken = s.relayClientForToken
	restarted.relayAccountForUser = s.relayAccountForUser
	_, _ = dao.YoubanBotManagerTask.Ctx(ctx).WherePri(root.Id + 1).Data(do.YoubanBotManagerTask{Status: "failed", ErrorMessage: "协议号没有权限"}).Update()
	if err := restarted.managerUpdateSyncProgress(ctx); err != nil {
		t.Fatal(err)
	}
	if r := <-requests; r.messageID != "77" || !strings.Contains(r.text, "66%") || !strings.Contains(r.text, "协议号没有权限") {
		t.Fatal("restart lost progress or failure reason", r)
	}
	_, _ = dao.YoubanBotManagerTask.Ctx(ctx).WherePri(root.Id + 2).Data(do.YoubanBotManagerTask{Status: "success"}).Update()
	if err := restarted.managerNotifyBatch(ctx, root); err != nil {
		t.Fatal(err)
	}
	if r := <-requests; r.messageID != "77" || !strings.Contains(r.text, "100%") || !strings.Contains(r.text, "失败 1") {
		t.Fatal("completion did not finish the same progress message", r)
	}
	<-requests // Final Bot list is a separate interactive menu.
	current, err := s.managerState(ctx, 1, 12)
	if err != nil || current.SyncedAt != nil {
		t.Fatal("partial failure incorrectly marked full sync successful", current, err)
	}
}

type managerOfflineCatalog struct{ ps.ISysPublish }

func (managerOfflineCatalog) BotManagerCatalog(context.Context, int64, int64) ([]*pi.BotManagerChannel, []*pi.BotManagerBot, error) {
	return []*pi.BotManagerChannel{{Id: 1, TgAccountId: 7, TargetChatId: "-100123", AccessHash: "456", AccountError: "协议号未登录，请先登录"}}, nil, nil
}

func TestBotManagerOfflineAccountFailsBeforeSubmission(t *testing.T) {
	if os.Getenv("YOUBAN_RELAY_TEST_PG") == "" {
		t.Skip("requires local PostgreSQL")
	}
	ctx, s, st := managerTestDB(t)
	previous := managerPreviousPublish()
	ps.RegisterSysPublish(managerOfflineCatalog{})
	t.Cleanup(func() { ps.RegisterSysPublish(previous) })
	if err := s.managerSync(ctx, st); err != nil {
		t.Fatal(err)
	}
	var root *entity.YoubanBotManagerTask
	if err := dao.YoubanBotManagerTask.Ctx(ctx).Scan(&root); err != nil || root == nil || root.Status != "failed" || root.AccountTaskId != 0 {
		t.Fatal("offline account was queued instead of failed", root, err)
	}
	text, err := managerSyncProgressText(ctx, root)
	if err != nil || !strings.Contains(text, "协议号未登录") || !strings.Contains(text, "100%") {
		t.Fatal("offline error missing from immediate progress", text, err)
	}
}

func TestBotManagerSyncErrorClassification(t *testing.T) {
	for _, code := range []string{"AUTH_KEY_UNREGISTERED", "SESSION_REVOKED", "CHAT_ADMIN_REQUIRED", "CHANNEL_PRIVATE"} {
		if reason, permanent := managerPermanentSyncError(errors.New(code)); !permanent || reason == code {
			t.Fatal("confirmed login/permission error remains retryable", code, reason)
		}
	}
	if _, permanent := managerPermanentSyncError(context.DeadlineExceeded); permanent {
		t.Fatal("network timeout incorrectly classified as no permission")
	}
}

func TestBotManagerSyncTimeoutTerminatesProcessingTask(t *testing.T) {
	ctx, s, st := managerTestDB(t)
	managerTestChannel(t, ctx, 1, 1, 10, 0)
	id, _ := managerNewTask(ctx, st, "timeout-progress", "sync", 0, 1, "{}")
	_, _ = dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Data(do.YoubanBotManagerTask{Status: "submitted", AccountTaskId: 100}).Update()
	_, _ = dao.YoubanBotManagerTask.DB().Exec(ctx, "UPDATE hg_youban_bot_manager_task SET created_at=? WHERE id=?", gtime.New(time.Now().Add(-3*time.Minute)), id)
	previous := managerPreviousTasks()
	cs.RegisterAccountTasks(&managerTestTasks{state: ci.AccountTaskStatusProcessing})
	t.Cleanup(func() { cs.RegisterAccountTasks(previous) })
	if err := s.managerTick(ctx); err != nil {
		t.Fatal(err)
	}
	var row entity.YoubanBotManagerTask
	if err := dao.YoubanBotManagerTask.Ctx(ctx).WherePri(id).Scan(&row); err != nil || row.Status != "failed" || !strings.Contains(row.ErrorMessage, "两分钟") {
		t.Fatal("sync processing task can wait forever", row, err)
	}
}
