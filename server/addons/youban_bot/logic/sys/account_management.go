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
	return true, bot.showAccountManagement(ctx, fc.BotId, fmt.Sprint(fc.Msg.Chat.ID), account)
}

func (s *sSysBot) showAccountManagement(ctx context.Context, botID int64, chatID string, account *botProfileAccount) error {
	name := fmt.Sprintf("账号 %d", account.AccountId)
	status := "未知"
	callCtx := quickPushMediaUploadContext(ctx, account.AccountId)
	if current, err := publishService.SysPublish().CurrentAccount(callCtx); err == nil && current != nil && current.Vip != nil {
		if current.Vip.IsVip {
			status = "VIP"
		} else {
			status = "非 VIP"
		}
		if current.Vip.ExpiredAt != nil {
			status += "，到期：" + current.Vip.ExpiredAt.String()
		}
	}
	text := fmt.Sprintf("<b>账户管理</b>\n\n当前账户：%s\n会员状态：%s\n\n请选择操作：", html.EscapeString(name), html.EscapeString(status))
	markup := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "当前账户", CallbackData: "acct:current"}, {Text: "会员续费", CallbackData: "acct:renew"}},
		{{Text: "重置密码", CallbackData: "acct:reset"}, {Text: "账号设置", CallbackData: "acct:setting"}},
		{{Text: "推送设置", CallbackData: "acct:notify"}, {Text: "邀请注册", CallbackData: "acct:invite"}},
		{{Text: "上架账户", CallbackData: "acct:accounts"}},
		{{Text: "创建上架账户", CallbackData: "acct:accounts:create"}},
		{{Text: "每日推送报告", CallbackData: "acct:report"}},
	}}
	return s.replyWithMarkup(ctx, botID, chatID, text, markup)
}

func (s *sSysBot) handleAccountManagementCallback(ctx context.Context, botID int64, query *models.CallbackQuery) (bool, error) {
	if query == nil || !strings.HasPrefix(query.Data, "acct:") || query.Message.Message == nil {
		return false, nil
	}
	chatID := fmt.Sprint(query.Message.Message.Chat.ID)
	if query.Message.Message.Chat.Type != models.ChatTypePrivate {
		return true, s.reply(ctx, botID, chatID, "账户管理只能在私聊中使用。")
	}
	account, err := s.boundProfileAccountByUser(ctx, query.From.ID)
	if err != nil || account == nil || account.AccountType != publishsysin.PublishAccountTypeAdmin {
		return true, s.reply(ctx, botID, chatID, "只有绑定的管理员账号可以使用账户管理。")
	}
	tgBot, botErr := s.telegramBot(ctx, s.replyBotToken(ctx, botID))
	if botErr == nil {
		callCtx, cancel := telegramAPICtx()
		defer cancel()
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
	}
	switch query.Data {
	case "acct:accounts:create":
		return true, s.startProfileSessionByIds(ctx, botID, fmt.Sprint(query.From.ID), chatID, account, "account_create", "waiting_username", 0, "", nil, "请输入新上架账户的登录用户名（3～32位字母、数字或下划线）。发送 /cancel 取消。")
	case "acct:accounts":
		return true, s.showManagedAccounts(ctx, botID, chatID, account, 1)
	case "acct:current":
		current, loadErr := publishService.SysPublish().CurrentAccount(quickPushMediaUploadContext(ctx, account.AccountId))
		if loadErr != nil {
			return true, loadErr
		}
		return true, s.reply(ctx, botID, chatID, fmt.Sprintf("<b>当前账户</b>\n名称：%s\n用户名：%s\n账号 ID：%d\n类型：管理员", html.EscapeString(current.Nickname), html.EscapeString(current.Username), current.Id))
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
		return true, s.replyWithMarkup(ctx, botID, chatID, "确定重置当前管理员的登录密码？重置后请使用新密码登录。", &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "确认重置", CallbackData: fmt.Sprintf("acct:reset_confirm:%d", session.Id)}}, {{Text: "取消", CallbackData: "acct:reset_cancel"}}}})
	case "acct:reset_cancel":
		return true, s.cancelProfileSessionByIds(ctx, botID, fmt.Sprint(query.From.ID), chatID)
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
			if session == nil || session.Id != id || session.Scene != "account_reset" || session.AccountId != account.AccountId || session.TenantId != account.TenantId {
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
			return true, s.showEditableAccountSetting(ctx, botID, chatID, account, id)
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
		if strings.HasPrefix(query.Data, "acct:edit:") {
			parts := strings.Split(query.Data, ":")
			if len(parts) != 4 {
				return true, s.reply(ctx, botID, chatID, "设置参数无效。")
			}
			id, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil || id <= 0 {
				return true, s.reply(ctx, botID, chatID, "账号参数无效。")
			}
			if _, err = publishService.SysPublish().AdminAccountSettingView(quickPushMediaUploadContext(ctx, account.AccountId), &publishsysin.AccountSettingViewInp{AccountId: id}); err != nil {
				return true, err
			}
			if parts[3] == "view" {
				return true, s.showEditableAccountSetting(ctx, botID, chatID, account, id)
			}
			if parts[3] != "suffixContent" && parts[3] != "customMarkText" {
				return true, s.reply(ctx, botID, chatID, "设置参数无效。")
			}
			return true, s.startProfileSessionByIds(ctx, botID, fmt.Sprint(query.From.ID), chatID, account, "account_setting", parts[3], id, "", nil, "请输入新内容；发送 - 清空，发送 /cancel 取消。")
		}

		if strings.HasPrefix(query.Data, "acct:accounts:delete:") {
			id, parseErr := strconv.ParseInt(strings.TrimPrefix(query.Data, "acct:accounts:delete:"), 10, 64)
			if parseErr != nil || id <= 0 {
				return true, s.reply(ctx, botID, chatID, "账号参数无效。")
			}
			return true, s.replyWithMarkup(ctx, botID, chatID, fmt.Sprintf("确定删除上架账户 %d 吗？删除后该账号将无法登录。", id), &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "确认删除", CallbackData: fmt.Sprintf("acct:accounts:delete_confirm:%d", id)}}, {{Text: "取消", CallbackData: "acct:accounts"}}}})
		}
		if strings.HasPrefix(query.Data, "acct:accounts:delete_confirm:") {
			id, parseErr := strconv.ParseInt(strings.TrimPrefix(query.Data, "acct:accounts:delete_confirm:"), 10, 64)
			if parseErr != nil || id <= 0 {
				return true, s.reply(ctx, botID, chatID, "账号参数无效。")
			}
			if deleteErr := publishService.SysPublish().AdminAccountDelete(quickPushMediaUploadContext(ctx, account.AccountId), &publishsysin.AccountDeleteInp{Ids: []int64{id}}); deleteErr != nil {
				return true, s.reply(ctx, botID, chatID, "删除账号失败："+html.EscapeString(deleteErr.Error()))
			}
			return true, s.showManagedAccounts(ctx, botID, chatID, account, 1)
		}
		if strings.HasPrefix(query.Data, "acct:accounts:reset:") {
			id, parseErr := strconv.ParseInt(strings.TrimPrefix(query.Data, "acct:accounts:reset:"), 10, 64)
			if parseErr != nil || id <= 0 {
				return true, s.reply(ctx, botID, chatID, "账号参数无效。")
			}
			result, resetErr := publishService.SysPublish().AdminAccountResetPassword(quickPushMediaUploadContext(ctx, account.AccountId), &publishsysin.AccountResetPasswordInp{Id: id})
			if resetErr != nil {
				return true, s.reply(ctx, botID, chatID, "重置密码失败："+html.EscapeString(resetErr.Error()))
			}
			return true, s.reply(ctx, botID, chatID, fmt.Sprintf("账号 %d 的新密码：<code>%s</code>\n请及时转交并修改。", id, html.EscapeString(result.Password)))
		}
		if strings.HasPrefix(query.Data, "acct:accounts:page:") {
			page, parseErr := strconv.Atoi(strings.TrimPrefix(query.Data, "acct:accounts:page:"))
			if parseErr != nil || page < 1 {
				page = 1
			}
			return true, s.showManagedAccounts(ctx, botID, chatID, account, page)
		}
		if strings.HasPrefix(query.Data, "acct:renew:") {
			planCode := strings.TrimPrefix(query.Data, "acct:renew:")
			order, orderErr := publishService.SysPublish().TenantVipOrderCreate(quickPushMediaUploadContext(ctx, account.AccountId), &publishsysin.TenantVipOrderCreateInp{PlanCode: planCode, ProductType: "vip", Quantity: 1})
			if orderErr != nil {
				return true, s.reply(ctx, botID, chatID, "创建续费订单失败："+html.EscapeString(orderErr.Error()))
			}
			if order.PayUrl == "" {
				return true, s.reply(ctx, botID, chatID, "订单已创建，请在网页端完成支付。")
			}
			return true, s.replyWithMarkup(ctx, botID, chatID, "<b>续费订单已创建</b>\n请点击下方按钮完成支付。", &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "打开支付页面", URL: order.PayUrl}}}})
		}
		if strings.HasPrefix(query.Data, "acct:report:date:") {
			return true, s.showDailyReport(ctx, botID, chatID, account, strings.TrimPrefix(query.Data, "acct:report:date:"))
		}
		return true, s.reply(ctx, botID, chatID, "该账户功能正在接入，请稍后重试。")
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
	return s.replyWithMarkup(ctx, botID, chatID, strings.Join(lines, ""), &models.InlineKeyboardMarkup{InlineKeyboard: rows})
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
		{"account", "账户操作", setting.AccountEventEnabled}, {"task", "任务失败", setting.TaskFailureEnabled},
	} {
		rows = append(rows, []models.InlineKeyboardButton{{Text: item.label + "：" + boolText(item.value), CallbackData: fmt.Sprintf("acct:notice:%s:%d", item.kind, 1-item.value)}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "返回账户管理", CallbackData: "acct:report:back"}})
	return s.replyWithMarkup(ctx, botID, chatID, "<b>推送设置</b>\n点击按钮切换。会员到期提醒始终开启。", &models.InlineKeyboardMarkup{InlineKeyboard: rows})
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
	lines := []string{"<b>上架账户</b>"}
	rows := [][]models.InlineKeyboardButton{}
	if len(list) == 0 {
		lines = append(lines, "\n暂无上架账户。")
	} else {
		for _, item := range list {
			if item == nil || item.Id == account.AccountId {
				continue
			}
			lines = append(lines, fmt.Sprintf("\n• %s（ID：%d）", html.EscapeString(firstNonEmpty(item.Nickname, item.Username)), item.Id))
			rows = append(rows, []models.InlineKeyboardButton{{Text: "账号设置", CallbackData: fmt.Sprintf("acct:edit:%d:view", item.Id)}, {Text: fmt.Sprintf("重置 %d 密码", item.Id), CallbackData: fmt.Sprintf("acct:accounts:reset:%d", item.Id)}, {Text: "删除", CallbackData: fmt.Sprintf("acct:accounts:delete:%d", item.Id)}})
		}
	}
	lines = append(lines, "\n可在网页端创建、删除或修改上架账户。")
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
	return s.replyWithMarkup(ctx, botID, chatID, strings.Join(lines, ""), &models.InlineKeyboardMarkup{InlineKeyboard: rows})
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
		text := fmt.Sprintf("<b>📊 %s 推送报告</b>\n\n📥 采集资料：%d 组\n\n<b>来源频道：</b>\n", item.ReportDate, item.CollectedTotal)
		for _, source := range item.Sources {
			if source != nil {
				text += fmt.Sprintf("• %s：%d 组\n", accountReportTitle(source.SourceTitle), source.Count)
			}
		}
		text += fmt.Sprintf("\n<b>采集推送</b>\n✅ 成功：%d 组\n❌ 失败：%d 组\n⏳ 处理中：%d 组\n\n<b>循环上架</b>\n✅ 成功：%d 组\n❌ 失败：%d 组\n⏳ 处理中：%d 组\n\n推送按频道投递次数统计，同一资料发往多个频道会分别计数。", item.PublishSuccess, item.PublishFailed, item.PublishProcessing, item.CycleSuccess, item.CycleFailed, item.CycleProcessing)
		for _, channel := range item.Channels {
			if channel == nil {
				continue
			}
			kind := "采集推送"
			if channel.Action == "cycle_publish" {
				kind = "循环上架"
			}
			text += fmt.Sprintf("\n• [%s] %s：成功%d，失败%d，处理中%d", accountReportTitle(channel.Title), kind, channel.Success, channel.Failed, channel.Processing)
		}
		parts := accountReportMessageParts(text)
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
	return s.replyWithMarkup(ctx, botID, chatID, "<b>每日推送报告</b>\n请选择最近 7 天的日期：", &models.InlineKeyboardMarkup{InlineKeyboard: rows})
}

func (s *sSysBot) accountReportDate(_ context.Context, value string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", value, time.FixedZone("CST", 8*3600))
}

func (s *sSysBot) showEditableAccountSetting(ctx context.Context, botID int64, chatID string, account *botProfileAccount, id int64) error {
	setting, err := publishService.SysPublish().AdminAccountSettingView(quickPushMediaUploadContext(ctx, account.AccountId), &publishsysin.AccountSettingViewInp{AccountId: id})
	if err != nil {
		return err
	}
	text := fmt.Sprintf("<b>账号设置</b>\n账号：%d\n\n发送后缀：%s\n后缀内容：%s\n标题标识：%s\n自定义标识：%s", id, boolText(setting.EnableSuffix), html.EscapeString(setting.SuffixContent), boolText(setting.EnableTitleMark), html.EscapeString(setting.CustomMarkText))
	rows := [][]models.InlineKeyboardButton{
		{{Text: "切换后缀开关", CallbackData: fmt.Sprintf("acct:set:%d:enableSuffix:%d", id, 1-setting.EnableSuffix)}, {Text: "切换标题标识", CallbackData: fmt.Sprintf("acct:set:%d:enableTitleMark:%d", id, 1-setting.EnableTitleMark)}},
		{{Text: "昵称标识", CallbackData: fmt.Sprintf("acct:set:%d:markMode:nickname", id)}, {Text: "自定义标识", CallbackData: fmt.Sprintf("acct:set:%d:markMode:custom", id)}},
		{{Text: "顺序编号", CallbackData: fmt.Sprintf("acct:set:%d:numberSource:sequence", id)}, {Text: "随机编号", CallbackData: fmt.Sprintf("acct:set:%d:numberSource:random", id)}},
		{{Text: "标识在顶部", CallbackData: fmt.Sprintf("acct:set:%d:markPosition:top", id)}, {Text: "底部", CallbackData: fmt.Sprintf("acct:set:%d:markPosition:bottom", id)}, {Text: "介绍费行", CallbackData: fmt.Sprintf("acct:set:%d:markPosition:feeLine", id)}},
		{{Text: "修改后缀", CallbackData: fmt.Sprintf("acct:edit:%d:suffixContent", id)}, {Text: "修改自定义标识", CallbackData: fmt.Sprintf("acct:edit:%d:customMarkText", id)}},
		{{Text: "返回账户管理", CallbackData: "acct:report:back"}},
	}
	return s.replyWithMarkup(ctx, botID, chatID, text, &models.InlineKeyboardMarkup{InlineKeyboard: rows})
}

// Split on complete lines so HTML entities and tags are not cut between
// messages. Titles are bounded separately before building report lines.
func accountReportMessageParts(text string) []string {
	const limit = 3000
	parts := []string{}
	current := ""
	for _, line := range strings.Split(text, "\n") {
		if len([]rune(current))+len([]rune(line))+1 > limit && current != "" {
			parts = append(parts, current)
			current = ""
		}
		if current != "" {
			current += "\n"
		}
		current += line
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func accountReportTitle(title string) string {
	runes := []rune(title)
	if len(runes) > 200 {
		title = string(runes[:200]) + "…"
	}
	return html.EscapeString(title)
}
