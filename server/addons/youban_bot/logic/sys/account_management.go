package sys

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"hotgo/internal/model/input/form"

	"hotgo/addons/youban_publish/library/dailyreport"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
)

type accountManagementFeature struct{}

func (accountManagementFeature) Key() string         { return "account_management" }
func (accountManagementFeature) Command() string     { return "account" }
func (accountManagementFeature) Description() string { return "账户管理" }
func (accountManagementFeature) Handle(ctx context.Context, bot *sSysBot, fc *botFeatureContext) (bool, error) {
	if fc.Msg == nil || fc.Msg.From == nil || fc.Msg.Chat.Type != models.ChatTypePrivate {
		return true, nil
	}
	account, err := bot.boundProfileAccountByUser(ctx, fc.Msg.From.ID)
	if err != nil {
		return true, bot.reply(ctx, fc.BotId, fmt.Sprint(fc.Msg.Chat.ID), "请先绑定管理员账号后再使用账户管理。")
	}
	if account == nil || account.AccountType != publishsysin.PublishAccountTypeAdmin {
		return true, bot.reply(ctx, fc.BotId, fmt.Sprint(fc.Msg.Chat.ID), "只有绑定的管理员账号可以使用账户管理。")
	}
	if err := bot.cancelAccountInput(ctx, fc.BotId, fmt.Sprint(fc.Msg.From.ID), fmt.Sprint(fc.Msg.Chat.ID)); err != nil {
		return true, err
	}
	return true, bot.showAccountManagement(ctx, fc.BotId, fmt.Sprint(fc.Msg.Chat.ID), account)
}

func (s *sSysBot) showAccountManagement(ctx context.Context, botID int64, chatID string, account *botProfileAccount) error {
	name := "当前账户"
	status := "未知"
	expires := "未设置"
	callCtx := quickPushMediaUploadContext(ctx, account.AccountId)
	if current, err := publishService.SysPublish().CurrentAccount(callCtx); err == nil && current != nil {
		name = firstNonEmpty(current.Nickname, current.Username, name)
		if current.Vip != nil {
			if current.Vip.IsVip {
				status = "VIP"
			} else {
				status = "非 VIP"
			}
			if current.Vip.ExpiredAt != nil {
				expires = current.Vip.ExpiredAt.String()
			}
		}
	}
	text := fmt.Sprintf("<b>账户管理</b>\n\n当前账户：<code>%s</code>\n\n会员状态：%s\n到期时间：%s\n\n请选择操作：", html.EscapeString(name), html.EscapeString(status), html.EscapeString(expires))
	markup := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "我的账户", CallbackData: "acct:mine"}},
		{{Text: "上架账户", CallbackData: "acct:accounts"}},
		{{Text: "每日推送报告", CallbackData: "acct:report"}},
	}}
	return s.accountMenuReply(ctx, botID, chatID, text, markup)
}

func (s *sSysBot) handleAccountManagementCallback(ctx context.Context, botID int64, query *models.CallbackQuery) (bool, error) {
	if query == nil || !strings.HasPrefix(query.Data, "acct:") || query.Message.Message == nil {
		return false, nil
	}
	chatID := fmt.Sprint(query.Message.Message.Chat.ID)
	if _, enabled := s.featureConfig(ctx, accountManagementFeature{}.Key()); !enabled {
		return true, s.reply(ctx, botID, chatID, "账户管理当前未启用。")
	}
	if query.Message.Message.Chat.Type != models.ChatTypePrivate {
		return true, s.reply(ctx, botID, chatID, "账户管理只能在私聊中使用。")
	}
	account, err := s.boundProfileAccountByUser(ctx, query.From.ID)
	if err != nil || account == nil || account.AccountType != publishsysin.PublishAccountTypeAdmin {
		return true, s.reply(ctx, botID, chatID, "只有绑定的管理员账号可以使用账户管理。")
	}
	previousSession := s.activeProfileSessionByIds(ctx, botID, fmt.Sprint(query.From.ID), chatID)
	// Navigation exits only account text-entry sessions. Confirmation sessions
	// remain active until consumed by their existing transactional handlers.
	if err := s.cancelAccountInputForNavigation(ctx, botID, fmt.Sprint(query.From.ID), chatID, query.Data); err != nil {
		return true, err
	}
	queryCopy := *query
	query = &queryCopy
	page := 1
	if index := strings.LastIndex(query.Data, "~p"); index >= 0 {
		parsed, parseErr := strconv.Atoi(query.Data[index+2:])
		if parseErr != nil || parsed < 1 {
			return true, s.reply(ctx, botID, chatID, "菜单已失效，请重新打开账户管理。")
		}
		page = parsed
		query.Data = query.Data[:index]
	}
	ctx = context.WithValue(ctx, accountMenuContextKey{}, accountMenuContext{MessageID: query.Message.Message.ID, Page: page})
	tgBot, botErr := s.telegramBot(ctx, s.replyBotToken(ctx, botID))
	if botErr == nil {
		callCtx, cancel := telegramAPICtx()
		defer cancel()
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
	}
	switch query.Data {
	case "acct:share":
		return true, s.showAccountAuthorizationLink(ctx, botID, fmt.Sprint(query.From.ID), chatID, account)
	case "acct:share:revoke":
		if err := s.revokeAccountAuthorizationShares(ctx, botID, fmt.Sprint(query.From.ID), account); err != nil {
			return true, err
		}
		return true, s.reply(ctx, botID, chatID, "授权链接已撤销。")
	case "acct:mine":
		return true, s.accountMenuReply(ctx, botID, chatID, "<b>我的账户</b>\n请选择操作：", &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "当前账户", CallbackData: "acct:current"}},
			{{Text: "会员续费", CallbackData: "acct:renew"}},
			{{Text: "重置密码", CallbackData: "acct:reset"}},
			{{Text: "账号设置", CallbackData: "acct:setting"}},
			{{Text: "推送设置", CallbackData: "acct:notify"}},
			{{Text: "邀请注册", CallbackData: "acct:invite"}},
			{{Text: "返回账户管理", CallbackData: "acct:report:back"}},
		}})
	case "acct:accounts:create":
		return true, s.startProfileSessionByIds(ctx, botID, fmt.Sprint(query.From.ID), chatID, account, "account_create", "waiting_username", 0, "", nil, "请输入新上架账户的登录用户名（3～32位字母、数字或下划线）。发送 /cancel 取消。")
	case "acct:accounts":
		return true, s.showManagedAccounts(ctx, botID, chatID, account, 1)
	case "acct:current":
		current, loadErr := publishService.SysPublish().CurrentAccount(quickPushMediaUploadContext(ctx, account.AccountId))
		if loadErr != nil {
			return true, loadErr
		}
		if current == nil {
			return true, s.reply(ctx, botID, chatID, "当前账户已不可用，请重新绑定后再试。")
		}
		return true, s.accountMenuReply(ctx, botID, chatID, fmt.Sprintf("<b>当前账户</b>\n\n名称：<code>%s</code>\n用户名：<code>%s</code>\n类型：管理员", html.EscapeString(firstNonEmpty(current.Nickname, current.Username, "未命名账户")), html.EscapeString(current.Username)), &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "返回我的账户", CallbackData: "acct:mine"}}}})
	case "acct:renew":
		return true, s.showVipPlans(ctx, botID, chatID)
	case "acct:setting":
		return true, s.showEditableAccountSetting(ctx, botID, chatID, account, account.AccountId)
	case "acct:notify":
		return true, s.showNotificationSetting(ctx, botID, chatID, account)
	case "acct:notify:toggle":
		callCtx := quickPushMediaUploadContext(ctx, account.AccountId)
		setting, settingErr := publishService.SysPublish().AdminAccountSettingView(callCtx, &publishsysin.AccountSettingViewInp{AccountId: account.AccountId})
		if settingErr != nil {
			return true, s.reply(ctx, botID, chatID, "读取推送设置失败，请稍后重试。")
		}
		enabled := 1
		if setting.DailyReportEnabled == 1 {
			enabled = 0
		}
		if settingErr = publishService.SysPublish().SaveAccountNotificationSetting(callCtx, &publishsysin.AccountNotificationSettingInp{AccountId: account.AccountId, Kind: "daily", Enabled: enabled}); settingErr != nil {
			return true, s.reply(ctx, botID, chatID, "保存推送设置失败，请稍后重试。")
		}
		return true, s.showNotificationSetting(ctx, botID, chatID, account)
	case "acct:reset":
		if err := s.startProfileSessionByIds(ctx, botID, fmt.Sprint(query.From.ID), chatID, account, "account_reset", "confirm", 0, "", nil, ""); err != nil {
			return true, err
		}
		session := s.activeProfileSessionByIds(ctx, botID, fmt.Sprint(query.From.ID), chatID)
		if session == nil {
			return true, s.reply(ctx, botID, chatID, "创建确认失败，请重试。")
		}
		return true, s.accountMenuReply(ctx, botID, chatID, "确定重置当前管理员的登录密码？重置后请使用新密码登录。", &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "确认重置", CallbackData: fmt.Sprintf("acct:reset_confirm:%d", session.Id)}}, {{Text: "取消", CallbackData: "acct:reset_cancel"}}}})
	case "acct:reset_cancel":
		if err := s.cancelAccountSessions(ctx, botID, fmt.Sprint(query.From.ID), chatID); err != nil {
			return true, err
		}
		if previousSession != nil {
			switch previousSession.Scene {
			case "account_setting":
				return true, s.showAccountSettingSection(ctx, botID, chatID, account, previousSession.ProfileId, accountSettingSectionForField(previousSession.Step))
			case "account_create":
				return true, s.showManagedAccounts(ctx, botID, chatID, account, accountMenuPage(ctx))
			case "account_delete", "account_managed_reset":
				return true, s.showManagedAccount(ctx, botID, chatID, account, previousSession.ProfileId, accountMenuPage(ctx))
			}
		}
		return true, s.showAccountManagement(ctx, botID, chatID, account)
	case "acct:invite":
		return (inviteFeature{}).Handle(quickPushMediaUploadContext(ctx, account.AccountId), s, &botFeatureContext{BotId: botID, Msg: query.Message.Message})
	case "acct:report":
		return true, s.showDailyReportDates(ctx, botID, chatID, account)
	case "acct:report:back":
		return true, s.showAccountManagement(ctx, botID, chatID, account)
	default:
		if strings.HasPrefix(query.Data, "acct:reset_confirm:") {
			id, err := strconv.ParseInt(strings.TrimPrefix(query.Data, "acct:reset_confirm:"), 10, 64)
			if err != nil {
				return true, err
			}
			session := s.activeProfileSessionByIds(ctx, botID, fmt.Sprint(query.From.ID), chatID)
			if session == nil || session.Id != id || (session.Scene != "account_reset" && session.Scene != "account_managed_reset") || session.AccountId != account.AccountId || session.TenantId != account.TenantId {
				return true, s.reply(ctx, botID, chatID, "确认已失效，请重新操作。")
			}
			result, err := s.resetOwnAccountFromSession(ctx, session, account)
			if err != nil {
				return true, s.reply(ctx, botID, chatID, "重置失败，请稍后重试。")
			}
			if result == nil {
				return true, nil
			}
			return true, s.reply(ctx, botID, chatID, "新密码：<code>"+html.EscapeString(result.Password)+"</code>\n请妥善保存。")
		}
		if strings.HasPrefix(query.Data, "acct:set:") {
			parts := strings.Split(query.Data, ":")
			if len(parts) != 5 {
				return true, s.reply(ctx, botID, chatID, "设置参数无效。")
			}
			id, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				return true, err
			}
			_, err = publishService.SysPublish().PatchAccountSetting(quickPushMediaUploadContext(ctx, account.AccountId), &publishsysin.AccountSettingPatchInp{AccountId: id, Field: parts[3], Value: parts[4]})
			if err != nil {
				return true, err
			}
			return true, s.showAccountSettingSection(context.WithValue(ctx, accountMenuSavedKey{}, true), botID, chatID, account, id, accountSettingSectionForField(parts[3]))
		}
		if strings.HasPrefix(query.Data, "acct:notice:") {
			parts := strings.Split(query.Data, ":")
			if len(parts) != 4 {
				return true, s.reply(ctx, botID, chatID, "设置参数无效。")
			}
			enabled, err := strconv.Atoi(parts[3])
			if err != nil {
				return true, err
			}
			err = publishService.SysPublish().SaveAccountNotificationSetting(quickPushMediaUploadContext(ctx, account.AccountId), &publishsysin.AccountNotificationSettingInp{AccountId: account.AccountId, Kind: parts[2], Enabled: enabled})
			if err != nil {
				return true, err
			}
			return true, s.showNotificationSetting(ctx, botID, chatID, account)
		}
		if strings.HasPrefix(query.Data, "acct:section:") {
			parts := strings.Split(query.Data, ":")
			if len(parts) != 4 {
				return true, s.reply(ctx, botID, chatID, "设置参数无效。")
			}
			id, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil || id <= 0 {
				return true, s.reply(ctx, botID, chatID, "账户参数无效。")
			}
			return true, s.showAccountSettingSection(ctx, botID, chatID, account, id, parts[3])
		}
		if strings.HasPrefix(query.Data, "acct:accounts:view:") {
			parts := strings.Split(query.Data, ":")
			if len(parts) != 5 {
				return true, s.reply(ctx, botID, chatID, "账户参数无效。")
			}
			id, err := strconv.ParseInt(parts[3], 10, 64)
			page, pageErr := strconv.Atoi(parts[4])
			if err != nil || id <= 0 || pageErr != nil || page < 1 {
				return true, s.reply(ctx, botID, chatID, "账户参数无效。")
			}
			return true, s.showManagedAccount(ctx, botID, chatID, account, id, page)
		}
		if strings.HasPrefix(query.Data, "acct:edit:") {
			parts := strings.Split(query.Data, ":")
			if len(parts) != 4 {
				return true, s.reply(ctx, botID, chatID, "设置参数无效。")
			}
			id, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil || id <= 0 {
				return true, s.reply(ctx, botID, chatID, "账号参数无效。")
			}
			if parts[3] == "view" {
				return true, s.showEditableAccountSetting(ctx, botID, chatID, account, id)
			}
			if parts[3] != "suffixContent" && parts[3] != "customMarkText" {
				return true, s.reply(ctx, botID, chatID, "设置参数无效。")
			}
			setting, name, err := s.accountManagementSetting(ctx, account, id)
			if err != nil {
				return true, err
			}
			label, value := "发送后缀", setting.SuffixContent
			if parts[3] == "customMarkText" {
				label, value = "自定义标识", setting.CustomMarkText
			}
			prompt := fmt.Sprintf("修改 <code>%s</code> 的%s\n\n当前内容：\n%s\n\n请输入新内容；发送 - 清空，发送 /cancel 取消。", html.EscapeString(name), label, html.EscapeString(firstNonEmpty(value, "尚未设置")))
			return true, s.startProfileSessionByIds(ctx, botID, fmt.Sprint(query.From.ID), chatID, account, "account_setting", parts[3], id, "", accountMenuContext{Page: accountMenuPage(ctx)}, prompt)
		}

		if strings.HasPrefix(query.Data, "acct:accounts:delete:") {
			id, parseErr := strconv.ParseInt(strings.TrimPrefix(query.Data, "acct:accounts:delete:"), 10, 64)
			if parseErr != nil || id <= 0 {
				return true, s.reply(ctx, botID, chatID, "账号参数无效。")
			}
			if id == account.AccountId {
				return true, s.reply(ctx, botID, chatID, "不能删除当前管理员账户。")
			}
			name, err := s.accountManagementName(ctx, account, id)
			if err != nil {
				return true, err
			}
			if err := s.startProfileSessionByIds(ctx, botID, fmt.Sprint(query.From.ID), chatID, account, "account_delete", "confirm", id, "", nil, ""); err != nil {
				return true, err
			}
			session := s.activeProfileSessionByIds(ctx, botID, fmt.Sprint(query.From.ID), chatID)
			if session == nil {
				return true, s.reply(ctx, botID, chatID, "无法创建确认，请稍后重试。")
			}
			return true, s.accountMenuReply(ctx, botID, chatID, fmt.Sprintf("确定删除上架账户 <code>%s</code> 吗？\n\n删除后该账户将无法登录，此操作不可撤销。", html.EscapeString(name)), &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "确认删除", CallbackData: fmt.Sprintf("acct:accounts:delete_confirm:%d", session.Id)}}, {{Text: "取消", CallbackData: "acct:reset_cancel"}}}})
		}
		if strings.HasPrefix(query.Data, "acct:accounts:delete_confirm:") {
			id, parseErr := strconv.ParseInt(strings.TrimPrefix(query.Data, "acct:accounts:delete_confirm:"), 10, 64)
			if parseErr != nil || id <= 0 {
				return true, s.reply(ctx, botID, chatID, "账号参数无效。")
			}
			session := s.activeProfileSessionByIds(ctx, botID, fmt.Sprint(query.From.ID), chatID)
			if session == nil || session.Id != id || session.Scene != "account_delete" || session.AccountId != account.AccountId || session.TenantId != account.TenantId {
				return true, s.reply(ctx, botID, chatID, "删除确认已失效，请重新操作。")
			}
			if deleteErr := s.deleteManagedAccountFromSession(ctx, session, account); deleteErr != nil {
				return true, s.reply(ctx, botID, chatID, "删除账号失败："+html.EscapeString(deleteErr.Error()))
			}
			return true, s.showManagedAccounts(ctx, botID, chatID, account, accountMenuPage(ctx))
		}
		if strings.HasPrefix(query.Data, "acct:accounts:reset:") {
			id, parseErr := strconv.ParseInt(strings.TrimPrefix(query.Data, "acct:accounts:reset:"), 10, 64)
			if parseErr != nil || id <= 0 {
				return true, s.reply(ctx, botID, chatID, "账号参数无效。")
			}
			name, err := s.accountManagementName(ctx, account, id)
			if err != nil {
				return true, err
			}
			if err := s.startProfileSessionByIds(ctx, botID, fmt.Sprint(query.From.ID), chatID, account, "account_managed_reset", "confirm", id, "", nil, ""); err != nil {
				return true, err
			}
			session := s.activeProfileSessionByIds(ctx, botID, fmt.Sprint(query.From.ID), chatID)
			if session == nil {
				return true, s.reply(ctx, botID, chatID, "无法创建确认，请稍后重试。")
			}
			return true, s.accountMenuReply(ctx, botID, chatID, fmt.Sprintf("确定重置 <code>%s</code> 的密码？\n\n旧密码将立即失效。", html.EscapeString(name)), &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "确认重置", CallbackData: fmt.Sprintf("acct:reset_confirm:%d", session.Id)}}, {{Text: "取消", CallbackData: "acct:reset_cancel"}}}})
		}
		if strings.HasPrefix(query.Data, "acct:accounts:page:") {
			page, parseErr := strconv.Atoi(strings.TrimPrefix(query.Data, "acct:accounts:page:"))
			if parseErr != nil || page < 1 {
				page = 1
			}
			return true, s.showManagedAccounts(ctx, botID, chatID, account, page)
		}
		if strings.HasPrefix(query.Data, "acct:renew_confirm:") {
			return true, s.confirmAccountRenewal(ctx, botID, chatID, query, account)
		}
		if strings.HasPrefix(query.Data, "acct:renew:") {
			return true, s.startAccountRenewal(ctx, botID, fmt.Sprint(query.From.ID), chatID, account, strings.TrimPrefix(query.Data, "acct:renew:"))
		}
		if strings.HasPrefix(query.Data, "acct:report:date:") {
			return true, s.showDailyReport(ctx, botID, chatID, account, strings.TrimPrefix(query.Data, "acct:report:date:"))
		}
		return true, s.accountMenuReply(ctx, botID, chatID, "这个操作已失效，请返回账户管理重新选择。", &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "返回账户管理", CallbackData: "acct:report:back"}}}})
	}
}

func (s *sSysBot) showVipPlans(ctx context.Context, botID int64, chatID string) error {
	plans, err := publishService.SysPublish().TenantVipPlans(ctx)
	if err != nil {
		return s.reply(ctx, botID, chatID, "读取会员套餐失败，请稍后重试。")
	}
	rows := make([][]models.InlineKeyboardButton, 0, len(plans))
	lines := []string{"<b>会员续费</b>\n请选择套餐："}
	for _, plan := range plans {
		if plan == nil || plan.Level <= 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("\n%s：%.2f %s，%d 天", html.EscapeString(plan.Name), plan.Price, html.EscapeString(plan.Currency), plan.Days))
		rows = append(rows, []models.InlineKeyboardButton{{Text: plan.Name, CallbackData: "acct:renew:" + plan.Code}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "返回账户管理", CallbackData: "acct:report:back"}})
	return s.accountMenuReply(ctx, botID, chatID, strings.Join(lines, ""), &models.InlineKeyboardMarkup{InlineKeyboard: rows})
}

func (s *sSysBot) showNotificationSetting(ctx context.Context, botID int64, chatID string, account *botProfileAccount) error {
	callCtx := quickPushMediaUploadContext(ctx, account.AccountId)
	setting, err := publishService.SysPublish().AdminAccountSettingView(callCtx, &publishsysin.AccountSettingViewInp{AccountId: account.AccountId})
	if err != nil {
		return s.reply(ctx, botID, chatID, "读取推送设置失败，请稍后重试。")
	}
	rows := [][]models.InlineKeyboardButton{}
	for _, item := range []struct {
		kind, label string
		value       int
	}{
		{"daily", "每日报告", setting.DailyReportEnabled}, {"channel", "频道异常", setting.ChannelAlertEnabled},
		{"account", "账户操作", setting.AccountEventEnabled}, {"task", "资料上架失败", setting.TaskFailureEnabled},
	} {
		rows = append(rows, []models.InlineKeyboardButton{{Text: item.label + "：" + boolText(item.value), CallbackData: fmt.Sprintf("acct:notice:%s:%d", item.kind, 1-item.value)}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "返回我的账户", CallbackData: "acct:mine"}})
	return s.accountMenuReply(ctx, botID, chatID, "<b>推送设置</b>\n点击按钮切换。会员到期提醒始终开启。", &models.InlineKeyboardMarkup{InlineKeyboard: rows})
}

func boolText(value int) string {
	if value == 1 {
		return "已开启"
	}
	return "已关闭"
}

func (s *sSysBot) showManagedAccounts(ctx context.Context, botID int64, chatID string, account *botProfileAccount, page int) error {
	callCtx := quickPushMediaUploadContext(ctx, account.AccountId)
	if page < 1 {
		page = 1
	}
	list, total, err := publishService.SysPublish().AdminAccountList(callCtx, &publishsysin.AccountListInp{PageReq: form.PageReq{Page: page, PerPage: 10, Pagination: true}, AccountType: publishsysin.PublishAccountTypeUploader})
	if err != nil {
		return s.reply(ctx, botID, chatID, "读取上架账户失败，请稍后重试。")
	}
	lastPage := (total + 9) / 10
	if lastPage < 1 {
		lastPage = 1
	}
	if page > lastPage {
		return s.showManagedAccounts(ctx, botID, chatID, account, lastPage)
	}
	ctx = accountMenuWithPage(ctx, page)
	lines := []string{fmt.Sprintf("<b>上架账户</b>\n\n共 %d 个账户 · 第 %d 页\n点击账户名称，进入设置、重置密码或删除。", total, page)}
	rows := [][]models.InlineKeyboardButton{}
	if len(list) == 0 {
		lines = append(lines, "\n暂无上架账户。")
	} else {
		for _, item := range list {
			if item == nil || item.Id == account.AccountId {
				continue
			}
			name := firstNonEmpty(item.Nickname, item.Username, "未命名账户")
			lines = append(lines, "\n• <code>"+html.EscapeString(name)+"</code>")
			rows = append(rows, []models.InlineKeyboardButton{{Text: name, CallbackData: fmt.Sprintf("acct:accounts:view:%d:%d", item.Id, page)}})
		}
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "创建账户", CallbackData: "acct:accounts:create"}})
	rows = append(rows, []models.InlineKeyboardButton{{Text: "分享授权链接", CallbackData: "acct:share"}})
	buttons := []models.InlineKeyboardButton{}
	if page > 1 {
		buttons = append(buttons, models.InlineKeyboardButton{Text: "上一页", CallbackData: fmt.Sprintf("acct:accounts:page:%d", page-1)})
	}
	if page*10 < total {
		buttons = append(buttons, models.InlineKeyboardButton{Text: "下一页", CallbackData: fmt.Sprintf("acct:accounts:page:%d", page+1)})
	}
	if len(buttons) > 0 {
		rows = append(rows, buttons)
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "返回账户管理", CallbackData: "acct:report:back"}})
	return s.accountMenuReply(ctx, botID, chatID, strings.Join(lines, ""), &models.InlineKeyboardMarkup{InlineKeyboard: rows})
}

func (s *sSysBot) showDailyReport(ctx context.Context, botID int64, chatID string, account *botProfileAccount, date string) error {
	if _, err := s.accountReportDate(ctx, date); err != nil {
		return s.reply(ctx, botID, chatID, "日报日期无效。")
	}
	list, err := publishService.SysPublish().DailyReports(ctx, &publishsysin.DailyReportListInp{TenantID: account.TenantId, AccountID: account.AccountId, Days: 7})
	if err != nil {
		return s.reply(ctx, botID, chatID, "读取日报失败，请稍后重试。")
	}
	for _, item := range list {
		if item == nil || item.ReportDate != date {
			continue
		}
		parts := dailyreport.Messages(item)
		for index, part := range parts {
			var markup models.ReplyMarkup
			if index == len(parts)-1 {
				markup = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "返回日期列表", CallbackData: "acct:report"}}}}
			}
			if err := s.replyWithMarkup(ctx, botID, chatID, part, markup); err != nil {
				return err
			}
		}
		return nil
	}
	return s.reply(ctx, botID, chatID, "这一天暂无日报记录。")
}

func (s *sSysBot) showDailyReportDates(ctx context.Context, botID int64, chatID string, account *botProfileAccount) error {
	list, err := publishService.SysPublish().DailyReports(ctx, &publishsysin.DailyReportListInp{TenantID: account.TenantId, AccountID: account.AccountId, Days: 7})
	if err != nil {
		return s.reply(ctx, botID, chatID, "读取日报失败，请稍后重试。")
	}
	rows := [][]models.InlineKeyboardButton{}
	for _, item := range list {
		if item == nil {
			continue
		}
		rows = append(rows, []models.InlineKeyboardButton{{Text: item.ReportDate, CallbackData: "acct:report:date:" + item.ReportDate}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "返回账户管理", CallbackData: "acct:report:back"}})
	return s.accountMenuReply(ctx, botID, chatID, "<b>每日推送报告</b>\n请选择最近 7 天的日期：", &models.InlineKeyboardMarkup{InlineKeyboard: rows})
}

func (s *sSysBot) accountReportDate(_ context.Context, value string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", value, time.FixedZone("CST", 8*3600))
}

func (s *sSysBot) showEditableAccountSetting(ctx context.Context, botID int64, chatID string, account *botProfileAccount, id int64) error {
	return s.showAccountSettingSection(ctx, botID, chatID, account, id, "menu")
}

// Split on complete lines so HTML entities and tags are not cut between
// messages. Titles are bounded separately before building report lines.
func accountReportMessageParts(text string) []string { return dailyreport.Parts(text) }
func accountReportTitle(title string) string         { return dailyreport.Title(title) }
