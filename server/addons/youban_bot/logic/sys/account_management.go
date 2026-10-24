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
		setting, settingErr := publishService.SysPublish().AdminAccountSettingView(quickPushMediaUploadContext(ctx, account.AccountId), &publishsysin.AccountSettingViewInp{AccountId: account.AccountId})
		if settingErr != nil {
			return true, s.reply(ctx, botID, chatID, "读取账号设置失败，请稍后重试。")
		}
		return true, s.reply(ctx, botID, chatID, fmt.Sprintf("<b>账号设置</b>\n\n介绍费后缀：%s\n后缀内容：%s\n标题标识：%s\n\n详细修改请在网页端完成。", boolText(setting.EnableSuffix), html.EscapeString(setting.SuffixContent), boolText(setting.EnableTitleMark)))
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
		return true, s.reply(ctx, botID, chatID, "为保护账号安全，重置密码请在网页端完成验证后操作。")
	case "acct:invite":
		return (inviteFeature{}).Handle(quickPushMediaUploadContext(ctx, account.AccountId), s, &botFeatureContext{BotId: botID, Msg: query.Message.Message})
	case "acct:report":
		return true, s.showDailyReportDates(ctx, botID, chatID, account)
	case "acct:report:back":
		return true, s.showAccountManagement(ctx, botID, chatID, account)
	default:
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
	status := "已关闭"
	if setting.DailyReportEnabled == 1 {
		status = "已开启"
	}
	return s.replyWithMarkup(ctx, botID, chatID, fmt.Sprintf("<b>推送设置</b>\n\n每日推送报告：%s\n频道异常通知：%s\n任务失败通知：%s\n\n会员到期提醒始终开启。", status, boolText(setting.ChannelAlertEnabled), boolText(setting.TaskFailureEnabled)), &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "切换日报开关", CallbackData: "acct:notify:toggle"}}, {{Text: "返回账户管理", CallbackData: "acct:report:back"}}}})
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
	if len(list) == 0 {
		lines = append(lines, "\n暂无上架账户。")
	} else {
		for _, item := range list {
			if item == nil || item.Id == account.AccountId {
				continue
			}
			lines = append(lines, fmt.Sprintf("\n• %s（ID：%d）", html.EscapeString(firstNonEmpty(item.Nickname, item.Username)), item.Id))
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
	rows := [][]models.InlineKeyboardButton{}
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
				text += fmt.Sprintf("• %s：%d 组\n", html.EscapeString(source.SourceTitle), source.Count)
			}
		}
		text += fmt.Sprintf("\n✅ 成功推送：%d 组\n❌ 失败：%d 组\n⏳ 处理中：%d 组", item.PublishSuccess, item.PublishFailed, item.PublishProcessing)
		return s.replyWithMarkup(ctx, botID, chatID, text, &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "返回日期列表", CallbackData: "acct:report"}}}})
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
