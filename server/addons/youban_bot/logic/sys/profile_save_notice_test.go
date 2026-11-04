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

	tgbot "github.com/go-telegram/bot"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	gs "hotgo/addons/youban_tg_bot_gateway/service"
)

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
