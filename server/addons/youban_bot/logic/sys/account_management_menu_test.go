package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
	gs "hotgo/addons/youban_tg_bot_gateway/service"
	"hotgo/internal/library/contexts"
)

type accountMenuTestService struct {
	ps.ISysPublish
	capabilityType string
	currentMissing bool
}

func (s *accountMenuTestService) AccountCapability(_ context.Context, app string, id int64) (*pi.AccountCapabilityModel, error) {
	if app != "api" || id != 99 {
		return nil, fmt.Errorf("invalid binding")
	}
	kind := s.capabilityType
	if kind == "" {
		kind = pi.PublishAccountTypeAdmin
	}
	return &pi.AccountCapabilityModel{TenantId: 1, AccountId: 99, AccountType: kind, TelegramBindingEnabled: 1}, nil
}

func (s *accountMenuTestService) CurrentAccount(ctx context.Context) (*pi.CurrentAccountModel, error) {
	if contexts.GetUserId(ctx) != 99 {
		return nil, fmt.Errorf("missing acting administrator")
	}
	if s.currentMissing {
		return nil, nil
	}
	return &pi.CurrentAccountModel{Id: 99, Nickname: "<管理员>", Vip: &pi.TenantVipStatusModel{IsVip: true, ExpiredAt: gtime.New("2026-11-24 00:00:00")}}, nil
}
func (s *accountMenuTestService) AdminAccountList(ctx context.Context, in *pi.AccountListInp) ([]*pi.AccountModel, int, error) {
	if contexts.GetUserId(ctx) != 99 || in.AccountType != pi.PublishAccountTypeUploader || in.PerPage != 10 {
		return nil, 0, fmt.Errorf("invalid account list scope")
	}
	return []*pi.AccountModel{{Id: 100, Username: "child", Nickname: "<上架账户>"}}, 21, nil
}
func (s *accountMenuTestService) AdminAccountSettingView(ctx context.Context, in *pi.AccountSettingViewInp) (*pi.AccountSettingModel, error) {
	if contexts.GetUserId(ctx) != 99 || (in.AccountId != 99 && in.AccountId != 100) {
		return nil, fmt.Errorf("invalid setting scope")
	}
	return &pi.AccountSettingModel{AccountId: in.AccountId, SuffixContent: "<后缀>", DailyReportEnabled: 1, ChannelAlertEnabled: 1, AccountEventEnabled: 1, TaskFailureEnabled: 1}, nil
}
func (s *accountMenuTestService) PatchAccountSetting(ctx context.Context, in *pi.AccountSettingPatchInp) (*pi.AccountSettingModel, error) {
	if contexts.GetUserId(ctx) != 99 || (in.AccountId != 99 && in.AccountId != 100) {
		return nil, fmt.Errorf("invalid patch scope")
	}
	if err := in.Filter(ctx); err != nil {
		return nil, err
	}
	return &pi.AccountSettingModel{AccountId: in.AccountId}, nil
}

func (s *accountMenuTestService) AdminAccountOptions(ctx context.Context, in *pi.AccountOptionsInp) ([]*pi.AccountOptionModel, error) {
	if contexts.GetUserId(ctx) != 99 || in.Scope != pi.AccountOptionsScopeMine {
		return nil, fmt.Errorf("invalid account name scope")
	}
	return []*pi.AccountOptionModel{{Value: 100, Label: "<上架账户>"}}, nil
}

func (s *accountMenuTestService) DailyReports(_ context.Context, in *pi.DailyReportListInp) ([]*pi.DailyReportModel, error) {
	if in.TenantID != 1 || in.AccountID != 99 || in.Days != 7 {
		return nil, fmt.Errorf("invalid report scope")
	}
	return []*pi.DailyReportModel{{TenantID: 1, AccountID: 99, ReportDate: "2026-10-23", CollectedTotal: 128, PublishSuccess: 96, PublishFailed: 18, PublishProcessing: 14, Sources: []*pi.DailyReportSourceRow{{SourceTitle: "<来源频道>", Count: 128}}}}, nil
}

func TestAccountManagementMenusLocalTelegram(t *testing.T) {
	ctx := relayTestDatabase(t)
	previous := managerPreviousPublish()
	fake := &accountMenuTestService{ISysPublish: previous}
	ps.RegisterSysPublish(fake)
	t.Cleanup(func() { ps.RegisterSysPublish(previous) })
	bot := &sSysBot{}
	requests := managerTestHTTP(t, ctx, bot)
	var previousGateway gs.IGateway
	func() { defer func() { _ = recover() }(); previousGateway = gs.Gateway() }()
	gs.RegisterGateway(accountRenewalTestGateway{client: bot.relayClientForToken})
	t.Cleanup(func() { gs.RegisterGateway(previousGateway) })
	account := &botProfileAccount{AccountId: 99, TenantId: 1, AccountType: pi.PublishAccountTypeAdmin, App: "api"}
	cases := []struct {
		name         string
		run          func() error
		want, absent []string
	}{
		{"home", func() error { return bot.showAccountManagement(ctx, 1, "11", account) }, []string{"当前账户：<code>&lt;管理员&gt;</code>\n\n会员状态：VIP\n到期时间：2026-11-24", "我的账户", "上架账户", "每日推送报告"}, []string{"账号 ID", "账号 99"}},
		{"accounts", func() error { return bot.showManagedAccounts(ctx, 1, "11", account, 2) }, []string{"&lt;上架账户&gt;", "acct:accounts:page:1", "acct:accounts:page:3", "创建账户", "分享授权链接", "acct:accounts:view:100:2"}, []string{"（ID：", "重置 100", "acct:accounts:delete:100"}},
		{"own settings", func() error { return bot.showEditableAccountSetting(ctx, 1, "11", account, 99) }, []string{"账户：<code>&lt;管理员&gt;</code>", "发送后缀", "标题标识", "acct:section:99:suffix", "返回账户"}, []string{"groupPushEnabled", "循环上架", "acct:set:", "账号：99"}},
		{"uploader settings", func() error { return bot.showEditableAccountSetting(ctx, 1, "11", account, 100) }, []string{"账户：<code>&lt;上架账户&gt;</code>", "账户权限", "acct:section:100:permissions"}, []string{"循环上架", "acct:set:", "账号：100"}},
		{"account detail", func() error { return bot.showManagedAccount(ctx, 1, "11", account, 100, 2) }, []string{"账户：<code>&lt;上架账户&gt;</code>", "重置密码", "删除账户", "acct:accounts:page:2"}, []string{"重置 100", "账号 ID", "（ID："}},
		{"suffix settings", func() error { return bot.showAccountSettingSection(ctx, 1, "11", account, 99, "suffix") }, []string{"&lt;后缀&gt;", "修改后缀内容", "acct:set:99:enableSuffix:1", "返回账号设置"}, []string{"groupPushEnabled", "markPosition"}},
		{"title settings", func() error { return bot.showAccountSettingSection(ctx, 1, "11", account, 100, "title") }, []string{"✅ 昵称标识", "✅ 顺序编号", "✅ 顶部", "修改自定义标识", "acct:set:100:markPosition:feeLine"}, []string{"groupPushEnabled", "enableSuffix"}},
		{"permissions", func() error { return bot.showAccountSettingSection(ctx, 1, "11", account, 100, "permissions") }, []string{"groupPushEnabled", "telegramBindingEnabled", "profileDetailViewEnabled", "sharedResourceEnabled", "返回账号设置"}, []string{"循环上架", "enableSuffix"}},
		{"notification settings", func() error { return bot.showNotificationSetting(ctx, 1, "11", account) }, []string{"会员到期提醒始终开启", "资料上架失败", "acct:notice:daily:0", "acct:notice:channel:0", "acct:notice:account:0", "acct:notice:task:0"}, []string{"acct:notice:vip"}},
		{"dates", func() error { return bot.showDailyReportDates(ctx, 1, "11", account) }, []string{"acct:report:date:2026-10-23", "返回账户管理"}, nil},
		{"report", func() error { return bot.showDailyReport(ctx, 1, "11", account, "2026-10-23") }, []string{"128", "96", "18", "14", "&lt;来源频道&gt;", "返回日期列表"}, nil},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); err != nil {
				t.Fatal(err)
			}
			var text string
			select {
			case text = <-requests:
			case <-time.After(3 * time.Second):
				t.Fatal("no Telegram reply")
			}
			if split := strings.LastIndex(text, "\n{"); split >= 0 {
				body := text[:split]
				for _, leaked := range []string{"账号 ID：", "（ID：", "账号：99", "账号：100", "重置 100 密码"} {
					if strings.Contains(body, leaked) {
						t.Fatalf("internal ID leaked in message: %s", body)
					}
				}
				var markup models.InlineKeyboardMarkup
				if err := json.Unmarshal([]byte(text[split+1:]), &markup); err != nil {
					t.Fatal(err)
				}
				for _, row := range markup.InlineKeyboard {
					for _, button := range row {
						if len(button.CallbackData) > 64 {
							t.Fatalf("oversized callback %q", button.CallbackData)
						}
						if strings.HasPrefix(button.CallbackData, "acct:accounts:view:") && test.name == "accounts" && len(row) != 1 {
							t.Fatal("account selection must have one account per row")
						}
					}
				}
			}
			for _, want := range test.want {
				if !strings.Contains(text, want) {
					t.Fatalf("missing %q in %s", want, text)
				}
			}
			for _, absent := range test.absent {
				if strings.Contains(text, absent) {
					t.Fatalf("unexpected %q in %s", absent, text)
				}
			}
		})
	}
	t.Run("settings reject inaccessible accounts and invalid sections", func(t *testing.T) {
		for _, id := range []int64{0, -1, 123456} {
			if err := bot.showAccountSettingSection(ctx, 1, "11", account, id, "suffix"); err == nil {
				t.Fatalf("allowed inaccessible account %d", id)
			}
		}
		if err := bot.showAccountSettingSection(ctx, 1, "11", account, 99, "permissions"); err == nil {
			t.Fatal("offered uploader permission controls for the administrator")
		}
		if err := bot.showAccountSettingSection(ctx, 1, "11", account, 100, "invalid"); err == nil {
			t.Fatal("allowed obsolete setting section")
		}
		if err := bot.showManagedAccount(ctx, 1, "11", account, 99, 1); err == nil {
			t.Fatal("offered administrator deletion through the uploader menu")
		}
		select {
		case text := <-requests:
			t.Fatalf("unexpected account details sent: %s", text)
		default:
		}
	})
	// Exercise the real callback gate, including callbacks from old messages
	// after the user's binding capability or feature configuration has changed.
	db, _ := gdb.Instance()
	// Earlier session fixtures intentionally use a reduced binding schema.
	// Recreate this table rather than inheriting whichever test ran first.
	if _, err := db.Exec(ctx, "DROP TABLE IF EXISTS "+accountBindTbl); err != nil {
		t.Fatal(err)
	}
	relayTestBaseTable(t, ctx, accountBindTbl)
	if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_account_bind(app,account_id,telegram_user_id,bot_id,status) VALUES('api',99,'11',1,1)`); err != nil {
		t.Fatal(err)
	}
	bot.features = map[string]*botFeatureRow{"account_management": {FeatureKey: "account_management", Status: 1}}
	bot.featureAt = time.Now()
	bot.featureDefaultsAt = time.Now()
	for _, test := range []struct {
		name             string
		user             int64
		kind             models.ChatType
		data, role, want string
		disabled         bool
		missing          bool
	}{
		{name: "bound administrator", user: 11, kind: models.ChatTypePrivate, data: "acct:mine", want: "我的账户"},
		{name: "unbound recipient", user: 12, kind: models.ChatTypePrivate, data: "acct:accounts", want: "只有绑定的管理员"},
		{name: "revoked admin capability", user: 11, kind: models.ChatTypePrivate, data: "acct:accounts", role: pi.PublishAccountTypeUploader, want: "只有绑定的管理员"},
		{name: "group chat", user: 11, kind: models.ChatTypeGroup, data: "acct:accounts", want: "只能在私聊"},
		{name: "disabled feature", user: 11, kind: models.ChatTypePrivate, data: "acct:accounts", want: "当前未启用", disabled: true},
		{name: "suffix toggle returns to suffix settings", user: 11, kind: models.ChatTypePrivate, data: "acct:set:100:enableSuffix:1", want: "修改后缀内容"},
		{name: "title toggle returns to title settings", user: 11, kind: models.ChatTypePrivate, data: "acct:set:100:markPosition:bottom", want: "修改自定义标识"},
		{name: "permission toggle returns to permission settings", user: 11, kind: models.ChatTypePrivate, data: "acct:set:100:groupPushEnabled:1", want: "只影响当前选中的上架账户"},
		{name: "legacy settings button", user: 11, kind: models.ChatTypePrivate, data: "acct:edit:100:view", want: "请选择要修改的设置"},
		{name: "account setting section", user: 11, kind: models.ChatTypePrivate, data: "acct:section:100:suffix", want: "修改后缀内容"},
		{name: "managed account navigation", user: 11, kind: models.ChatTypePrivate, data: "acct:accounts:view:100:2", want: "重置密码"},
		{name: "invalid section target", user: 11, kind: models.ChatTypePrivate, data: "acct:section:bad:suffix", want: "账户参数无效"},
		{name: "current account navigation", user: 11, kind: models.ChatTypePrivate, data: "acct:current", want: "返回我的账户"},
		{name: "account removed after permission check", user: 11, kind: models.ChatTypePrivate, data: "acct:current", want: "当前账户已不可用", missing: true},
		{name: "obsolete action", user: 11, kind: models.ChatTypePrivate, data: "acct:obsolete", want: "操作已失效"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake.capabilityType = test.role
			fake.currentMissing = test.missing
			bot.features["account_management"].Status = 1
			if test.disabled {
				bot.features["account_management"].Status = 2
			}
			query := &models.CallbackQuery{ID: "local", From: models.User{ID: test.user}, Data: test.data, Message: models.MaybeInaccessibleMessage{Message: &models.Message{Chat: models.Chat{ID: test.user, Type: test.kind}}}}
			handled, err := bot.handleAccountManagementCallback(ctx, 1, query)
			if err != nil || !handled {
				t.Fatalf("callback: handled=%v err=%v", handled, err)
			}
			deadline := time.After(3 * time.Second)
			for {
				select {
				case text := <-requests:
					if strings.TrimSpace(text) == "" {
						continue
					} // callback acknowledgement
					if !strings.Contains(text, test.want) {
						t.Fatalf("missing %q in %s", test.want, text)
					}
					return
				case <-deadline:
					t.Fatal("missing callback reply")
				}
			}
		})
	}

	t.Run("authorization opening and username", func(t *testing.T) {
		fake.capabilityType = ""
		bot.features["account_management"].Status = 1
		if _, err := db.Exec(ctx, "DROP TABLE IF EXISTS "+profileSessionTable); err != nil {
			t.Fatal(err)
		}
		// Keep the SQLite session shape consistent with the other interaction
		// fixtures. Production PostgreSQL definitions are tested separately.
		if _, err := db.Exec(ctx, `CREATE TABLE hg_youban_bot_profile_session(id integer primary key autoincrement,bot_id bigint,telegram_user_id text,chat_id text,app text,account_id bigint,tenant_id bigint,scene text,step text,profile_id bigint,profile_no text,status text,payload_json text,expires_at timestamp,created_at timestamp,updated_at timestamp)`); err != nil {
			t.Fatal(err)
		}
		if err := db.GetCore().ClearTableFieldsAll(ctx); err != nil {
			t.Fatal(err)
		}
		token := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" // valid 24-byte bearer fixture
		if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_profile_session(bot_id,telegram_user_id,chat_id,app,account_id,tenant_id,scene,profile_no,status,expires_at) VALUES(1,'11','11','api',99,1,'account_share',?,'available',?)`, accountAuthorizationDigest(token), gtime.Now().Add(30*time.Minute)); err != nil {
			t.Fatal(err)
		}
		readReply := func(want string) {
			t.Helper()
			select {
			case text := <-requests:
				if !strings.Contains(text, want) {
					t.Fatalf("missing %q in %s", want, text)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("missing authorization reply")
			}
		}
		msg := &models.Message{From: &models.User{ID: 12}, Chat: models.Chat{ID: 12, Type: models.ChatTypePrivate}}
		if err := bot.beginAccountAuthorization(ctx, &botFeatureContext{BotId: 1, Msg: msg, Args: accountAuthorizationPrefix + token}); err != nil {
			t.Fatal(err)
		}
		readReply("请输入上架账户用户名")
		msg.Text = "recipient_name"
		if handled, err := bot.handleAccountManagementInput(ctx, 1, msg); err != nil || !handled {
			t.Fatalf("username input: %v %v", handled, err)
		}
		readReply("确认创建并绑定")
		session := bot.activeProfileSessionByIds(ctx, 1, "12", "12")
		if session == nil || session.Step != "confirm" {
			t.Fatalf("missing username confirmation: %+v", session)
		}
		var draft accountAuthorizationDraft
		if err := json.Unmarshal([]byte(session.PayloadJson), &draft); err != nil {
			t.Fatal(err)
		}
		if draft.Username != "recipient_name" || draft.ShareID <= 0 {
			t.Fatalf("wrong authorization draft: %+v", draft)
		}
		msg = &models.Message{From: &models.User{ID: 13, Username: "tg_recipient"}, Chat: models.Chat{ID: 13, Type: models.ChatTypePrivate}}
		if err := bot.beginAccountAuthorization(ctx, &botFeatureContext{BotId: 1, Msg: msg, Args: accountAuthorizationPrefix + token}); err != nil {
			t.Fatal(err)
		}
		readReply("tg_recipient")
		msg.From.ID = 11
		msg.Chat.ID = 11
		if err := bot.beginAccountAuthorization(ctx, &botFeatureContext{BotId: 1, Msg: msg, Args: accountAuthorizationPrefix + token}); err != nil {
			t.Fatal(err)
		}
		readReply("已绑定管理员或上架账户")
		if err := bot.beginAccountAuthorization(ctx, &botFeatureContext{BotId: 1, Msg: msg, Args: accountAuthorizationPrefix + "invalid"}); err != nil {
			t.Fatal(err)
		}
		readReply("授权链接无效")
		count, err := db.GetCount(ctx, `SELECT COUNT(*) FROM hg_youban_bot_profile_session WHERE scene='account_share' AND status='available'`)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatal("opening a share consumed its single-use authorization")
		}
	})
}
