package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
	gs "hotgo/addons/youban_tg_bot_gateway/service"
)

type batchProfileCreateStub struct {
	publishService.ISysPublish
	t     *testing.T
	calls int
}

func (s *batchProfileCreateStub) BotProfileCreate(_ context.Context, in *pi.BotProfileCreateInp) (*pi.ProfileSaveModel, error) {
	s.calls++
	wantDisplay := fmt.Sprintf("display-%02d", s.calls)
	wantVerify := fmt.Sprintf("verify-%02d", s.calls)
	if in.PlainText != wantDisplay || in.VerifyText != wantVerify {
		s.t.Fatalf("profile %d mismatched content: display=%q verify=%q", s.calls, in.PlainText, in.VerifyText)
	}
	if len(in.DisplayMedia) != 1 || in.DisplayMedia[0] != nil {
		s.t.Fatalf("profile %d display media crossed batch boundary: %#v", s.calls, in.DisplayMedia)
	}
	return &pi.ProfileSaveModel{Id: int64(900000 + s.calls), ProfileNo: fmt.Sprintf("B%05d", s.calls)}, nil
}

func TestProfileVerificationReceivedText(t *testing.T) {
	for _, tc := range []struct {
		name  string
		draft *profileCreateDraft
		want  string
	}{
		{"video", &profileCreateDraft{VerifyMedia: []*pi.MessageTemplateMediaInp{{MediaType: "video"}}}, "已收到验证视频"},
		{"image", &profileCreateDraft{VerifyMedia: []*pi.MessageTemplateMediaInp{{MediaType: "image"}}}, "已收到验证资料"},
		{"text", &profileCreateDraft{VerifyText: "验证"}, "已收到验证资料"},
		{"skip", &profileCreateDraft{}, "已跳过验证资料"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.HasPrefix(profileVerificationReceivedText(tc.draft), tc.want) {
				t.Fatal(profileVerificationReceivedText(tc.draft))
			}
		})
	}
}

func TestProfileSaveNoticeBatchAndSingleTelegramRequests(t *testing.T) {
	ctx := relayTestDatabase(t)
	relayTestBaseTable(t, ctx, botTable)
	if _, err := dao.YoubanBotBot.Ctx(ctx).Data(do.YoubanBotBot{Id: 1, BotToken: "123:fake", Status: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	type request struct{ text, markup string }
	requests := make(chan request, 10)
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		requests <- request{r.FormValue("text"), r.FormValue("reply_markup")}
		w.Header().Set("Content-Type", "application/json")
		if fail.Load() {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"ok":false,"error_code":400,"description":"injected failure"}`)
			return
		}
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":99}}`)
	}))
	defer server.Close()
	client, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	var previous gs.IGateway
	func() { defer func() { _ = recover() }(); previous = gs.Gateway() }()
	gs.RegisterGateway(accountRenewalTestGateway{client: func(context.Context, string) (*tgbot.Bot, error) { return client, nil }})
	t.Cleanup(func() { gs.RegisterGateway(previous) })
	s := NewSysBot()
	saved := &pi.ProfileSaveModel{Id: 727045, ProfileNo: "M68532"}
	if err := s.sendProfileSaveNotice(ctx, 1, "123", profileVerificationReceivedText(&profileCreateDraft{VerifyMedia: []*pi.MessageTemplateMediaInp{{MediaType: "video"}}}), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.sendProfileCreateSuccess(ctx, 1, "123", saved, true); err != nil {
		t.Fatal(err)
	}
	ack, success := <-requests, <-requests
	if !strings.Contains(ack.text, "已收到验证视频") || !strings.Contains(success.text, "当前资料已保存") || !strings.Contains(success.text, "M68532") || !strings.Contains(success.text, "下一份展示资料") || strings.Contains(success.text, "请发送【展示资料】") {
		t.Fatalf("incorrect batch messages: %+v %+v", ack, success)
	}
	var markup map[string]interface{}
	if err := json.Unmarshal([]byte(success.markup), &markup); err != nil || !strings.Contains(success.markup, "pf:up:M68532") {
		t.Fatalf("missing up action: %s err=%v", success.markup, err)
	}
	if len(requests) != 0 {
		t.Fatal("sent separate next-display prompt")
	}
	if err := s.sendProfileCreateSuccess(ctx, 1, "123", saved); err != nil {
		t.Fatal(err)
	}
	if single := <-requests; strings.Contains(single.text, "下一份") {
		t.Fatal("single create invited another profile")
	}
	fail.Store(true)
	if err := s.sendProfileCreateSuccess(ctx, 1, "123", saved, true); err == nil {
		t.Fatal("Telegram failure was swallowed")
	}
	<-requests
	if len(requests) != 0 {
		t.Fatal("failure emitted another prompt")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.sendProfileSaveNotice(canceled, 1, "123", "已收到验证视频", nil); err == nil {
		t.Fatal("canceled notice ignored caller context")
	}
	if len(requests) != 0 {
		t.Fatal("canceled notice still reached Telegram")
	}
}

func TestBatchProfileCreateKeepsTwentyDisplayVerifyPairs(t *testing.T) {
	ctx := relayTestDatabase(t)
	relayTestBaseTable(t, ctx, botTable)
	relayTestBaseTable(t, ctx, profileSessionTable)
	if _, err := dao.YoubanBotBot.Ctx(ctx).Data(do.YoubanBotBot{Id: 1, BotToken: "123:fake", Status: 1}).Insert(); err != nil {
		t.Fatal(err)
	}
	resultMessages := make(chan string, 128)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		resultMessages <- r.FormValue("text")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":99}}`)
	}))
	defer server.Close()
	client, err := tgbot.New("123:fake", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	var previousGateway gs.IGateway
	func() { defer func() { _ = recover() }(); previousGateway = gs.Gateway() }()
	gs.RegisterGateway(accountRenewalTestGateway{client: func(context.Context, string) (*tgbot.Bot, error) { return client, nil }})
	t.Cleanup(func() { gs.RegisterGateway(previousGateway) })

	var previousPublish publishService.ISysPublish
	func() { defer func() { _ = recover() }(); previousPublish = publishService.SysPublish() }()
	stub := &batchProfileCreateStub{ISysPublish: previousPublish, t: t}
	publishService.RegisterSysPublish(stub)
	t.Cleanup(func() { publishService.RegisterSysPublish(previousPublish) })

	now := time.Now()
	result, err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Data(g.Map{
		"bot_id": 1, "telegram_user_id": "123", "chat_id": "123", "app": "api",
		"account_id": 9, "tenant_id": 5, "scene": "batch_create", "step": "waiting_display",
		"payload_json": "", "expires_at": now.Add(time.Hour), "status": profileSessionStatusActive,
		"created_at": now, "updated_at": now,
	}).Insert()
	if err != nil {
		t.Fatal(err)
	}
	sessionId, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	account := &botProfileAccount{TenantId: 5, AccountId: 9, App: "api"}
	s := NewSysBot()
	loadSession := func() *profileSessionRow {
		t.Helper()
		var row profileSessionRow
		if err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Where("id", sessionId).Scan(&row); err != nil {
			t.Fatal(err)
		}
		return &row
	}
	for index := 1; index <= 20; index++ {
		displayText := fmt.Sprintf("display-%02d", index)
		verifyText := fmt.Sprintf("verify-%02d", index)
		if err := s.consumeProfileCreatePart(ctx, 1, "123", account, loadSession(), displayText, []*pi.MessageTemplateMediaInp{nil}); err != nil {
			t.Fatalf("display %d: %v", index, err)
		}
		if got := loadSession(); got.Step != "waiting_verify" {
			t.Fatalf("after display %d step=%q, want waiting_verify", index, got.Step)
		}
		if err := s.consumeProfileCreatePart(ctx, 1, "123", account, loadSession(), verifyText, nil); err != nil {
			t.Fatalf("verification %d: %v", index, err)
		}
		got := loadSession()
		if got.Step != "waiting_display" || got.Status != profileSessionStatusActive {
			t.Fatalf("after profile %d session=(%s,%s), want (waiting_display,active)", index, got.Step, got.Status)
		}
		if draft := decodeProfileCreateDraft(got.PayloadJson); draft.DisplayText != "" || draft.VerifyText != "" || len(draft.DisplayMedia) != 0 || len(draft.VerifyMedia) != 0 {
			t.Fatalf("profile %d draft leaked into next entry: %+v", index, draft)
		}
	}
	if stub.calls != 20 {
		t.Fatalf("BotProfileCreate calls=%d, want 20", stub.calls)
	}
	successNotifications := 0
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for successNotifications < 20 {
		select {
		case text := <-resultMessages:
			if strings.Contains(text, "当前资料已保存") {
				successNotifications++
			}
		case <-deadline.C:
			t.Fatalf("batch success notifications=%d, want 20", successNotifications)
		}
	}
}
