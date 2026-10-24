package sys

import (
	"context"
	"fmt"
	"html"
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
		return true, s.showManagedAccounts(ctx, botID, chatID, account)
	case "acct:current":
		return true, s.reply(ctx, botID, chatID, fmt.Sprintf("<b>当前账户</b>\n账号 ID：%d\n租户 ID：%d\n类型：管理员\n\n如需修改资料，请进入「账号设置」。", account.AccountId, account.TenantId))
	case "acct:renew":
		return true, s.reply(ctx, botID, chatID, "<b>会员续费</b>\n请在网页端选择套餐并完成支付。支付完成后会员状态会自动更新。")
	case "acct:setting":
		return true, s.reply(ctx, botID, chatID, "<b>账号设置</b>\n账号名称、标题标识、介绍费后缀等设置请在网页端修改。")
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
		return true, s.reply(ctx, botID, chatID, "<b>邀请注册</b>\n请在网页端生成授权链接，分享给需要绑定的管理员。")
	case "acct:report":
		return true, s.showDailyReportDates(ctx, botID, chatID, account)
	case "acct:report:back":
		return true, s.showAccountManagement(ctx, botID, chatID, account)
	default:
		if strings.HasPrefix(query.Data, "acct:report:date:") {
			return true, s.showDailyReport(ctx, botID, chatID, account, strings.TrimPrefix(query.Data, "acct:report:date:"))
		}
		return true, s.reply(ctx, botID, chatID, "该账户功能正在接入，请稍后重试。")
	}
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

func (s *sSysBot) showManagedAccounts(ctx context.Context, botID int64, chatID string, account *botProfileAccount) error {
	callCtx := quickPushMediaUploadContext(ctx, account.AccountId)
	list, _, err := publishService.SysPublish().AdminAccountList(callCtx, &publishsysin.AccountListInp{PageReq: form.PageReq{Page: 1, PerPage: 50, Pagination: true}, AccountType: publishsysin.PublishAccountTypeUploader})
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
	return s.replyWithMarkup(ctx, botID, chatID, strings.Join(lines, ""), &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "返回账户管理", CallbackData: "acct:report:back"}}}})
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
