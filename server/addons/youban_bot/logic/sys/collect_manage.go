package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
)

func (s *sSysBot) collectManageAllowed(ctx context.Context, account *botProfileAccount) bool {
	if account == nil || account.AccountId <= 0 || account.TenantId <= 0 {
		return false
	}
	status, err := publishService.SysPublish().TenantVipStatus(ctx, account.TenantId)
	return err == nil && status != nil && status.IsVip
}

// handleCollectManageCallback provides the BOT entry point for collection-source
// management. Business mutations are delegated to existing publish services;
// this handler currently implements paginated browsing and safe action prompts.
func (s *sSysBot) handleCollectManageCallback(ctx context.Context, botId int64, chatId string, account *botProfileAccount, data string) (bool, error) {
	if account == nil || !s.collectManageAllowed(ctx, account) {
		return true, nil
	}
	parts := strings.Split(data, ":")
	action := "list"
	if len(parts) > 1 {
		action = parts[1]
	}
	switch action {
	case "list":
		page := 1
		if len(parts) > 2 {
			page, _ = strconv.Atoi(parts[2])
			if page < 1 {
				page = 1
			}
		}
		return true, s.showCollectSourceList(ctx, botId, chatId, account, page)
	case "view":
		if len(parts) < 3 {
			return true, nil
		}
		id, _ := strconv.ParseInt(parts[2], 10, 64)
		return true, s.showCollectSourceView(ctx, botId, chatId, account, id)
	case "config":
		// 采集规则包含租户级策略，仅租户管理员可以查看和修改。
		if account.AccountType != "admin" {
			return true, s.sendCollectManageNotice(ctx, botId, chatId, "仅租户管理员可以配置采集规则。")
		}
		if len(parts) < 3 {
			return true, nil
		}
		sourceID, _ := strconv.ParseInt(parts[2], 10, 64)
		return true, s.showCollectSourceConfig(ctx, botId, chatId, account, sourceID)
	case "rule":
		if account.AccountType != "admin" || len(parts) < 3 {
			return true, nil
		}
		ruleID, _ := strconv.ParseInt(parts[2], 10, 64)
		return true, s.showCollectRuleEditor(ctx, botId, chatId, account, ruleID)
	case "ruleswitch":
		if account.AccountType != "admin" || len(parts) < 4 {
			return true, nil
		}
		ruleID, _ := strconv.ParseInt(parts[2], 10, 64)
		return true, s.toggleCollectRuleField(ctx, botId, chatId, account, ruleID, parts[3])
	case "toggle":
		if len(parts) < 3 {
			return true, nil
		}
		sourceID, _ := strconv.ParseInt(parts[2], 10, 64)
		return true, s.toggleCollectSource(ctx, botId, chatId, account, sourceID)
	case "down":
		if len(parts) < 3 {
			return true, nil
		}
		sourceID, _ := strconv.ParseInt(parts[2], 10, 64)
		if account.AccountType != "admin" {
			return true, s.sendCollectManageNotice(ctx, botId, chatId, "仅租户管理员可以下架并删除采集资料。")
		}
		res, err := publishService.SysPublish().BotCollectSourceDown(ctx, sourceID, account.TenantId, account.AccountId, true)
		if err != nil {
			return true, err
		}
		return true, s.sendCollectManageNotice(ctx, botId, chatId, fmt.Sprintf("已提交下架删除任务，关联TG消息 %d 条。任务将在后台执行，失败会自动重试。", res.MessageCount))
	case "back":
		return true, s.showProfileMenuToChat(ctx, botId, chatId, "已返回资料管理，请选择操作：")
	}
	return true, nil
}

func (s *sSysBot) showCollectSourceList(ctx context.Context, botId int64, chatId string, account *botProfileAccount, page int) error {
	const perPage = 6
	var rows []publishsysin.CollectSourceModel
	mod := g.DB().Model("hg_youban_publish_collect_source").Ctx(ctx).Where("tenant_id", account.TenantId).Where("account_id", account.AccountId).WhereNull("deleted_at")
	if err := mod.Page(page, perPage).OrderDesc("id").Scan(&rows); err != nil {
		return err
	}
	buttons := make([][]models.InlineKeyboardButton, 0, len(rows)+2)
	for _, row := range rows {
		status := "运行中"
		if row.CollectEnabled != 1 {
			status = "已暂停"
		}
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: fmt.Sprintf("%s · %s", row.Title, status), CallbackData: fmt.Sprintf("cm:view:%d", row.Id)}})
	}
	nav := []models.InlineKeyboardButton{}
	if page > 1 {
		nav = append(nav, models.InlineKeyboardButton{Text: "上一页", CallbackData: fmt.Sprintf("cm:list:%d", page-1)})
	}
	if len(rows) == perPage {
		nav = append(nav, models.InlineKeyboardButton{Text: "下一页", CallbackData: fmt.Sprintf("cm:list:%d", page+1)})
	}
	if len(nav) > 0 {
		buttons = append(buttons, nav)
	}
	buttons = append(buttons, []models.InlineKeyboardButton{{Text: "返回", CallbackData: "cm:back"}})
	botRow, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	_, err = s.sendMessageWithMarkup(ctx, botRow.BotToken, chatId, fmt.Sprintf("采集源列表（第 %d 页）", page), "HTML", false, &models.InlineKeyboardMarkup{InlineKeyboard: buttons})
	return err
}

func (s *sSysBot) showCollectSourceView(ctx context.Context, botId int64, chatId string, account *botProfileAccount, id int64) error {
	var row publishsysin.CollectSourceModel
	if err := g.DB().Model("hg_youban_publish_collect_source").Ctx(ctx).Where("id", id).Where("tenant_id", account.TenantId).Where("account_id", account.AccountId).WhereNull("deleted_at").Scan(&row); err != nil {
		return err
	}
	text := fmt.Sprintf("采集源：%s\n编号：%d\n状态：%s", row.Title, row.Id, map[bool]string{true: "运行中", false: "已暂停"}[row.CollectEnabled == 1])
	markup := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "采集配置", CallbackData: fmt.Sprintf("cm:config:%d", id)}},
		{{Text: map[bool]string{true: "暂停采集", false: "恢复采集"}[row.CollectEnabled == 1], CallbackData: fmt.Sprintf("cm:toggle:%d", id)}},
		{{Text: "下架并删除", CallbackData: fmt.Sprintf("cm:down:%d", id)}},
		{{Text: "返回列表", CallbackData: "cm:list:1"}},
	}}
	botRow, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	_, err = s.sendMessageWithMarkup(ctx, botRow.BotToken, chatId, text, "HTML", false, markup)
	return err
}

func (s *sSysBot) toggleCollectSource(ctx context.Context, botId int64, chatId string, account *botProfileAccount, id int64) error {
	var row publishsysin.CollectSourceModel
	if err := g.DB().Model("hg_youban_publish_collect_source").Ctx(ctx).Where("id", id).Where("tenant_id", account.TenantId).Where("account_id", account.AccountId).WhereNull("deleted_at").Scan(&row); err != nil {
		return err
	}
	if row.Id <= 0 {
		return s.sendCollectManageNotice(ctx, botId, chatId, "采集源不存在或已失效。")
	}
	enabled := 0
	if row.CollectEnabled != 1 {
		enabled = 1
	}
	if _, err := g.DB().Model("hg_youban_publish_collect_source").Ctx(ctx).Where("id", id).Where("tenant_id", account.TenantId).Where("account_id", account.AccountId).Data(g.Map{"collect_enabled": enabled, "updated_at": gtime.Now()}).Update(); err != nil {
		return err
	}
	return s.showCollectSourceView(ctx, botId, chatId, account, id)
}

func (s *sSysBot) sendCollectManageNotice(ctx context.Context, botId int64, chatId, text string) error {
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, text, "HTML", false, nil)
	return err
}

// showCollectSourceConfig renders the existing rule configuration. Mutations are
// intentionally restricted to tenant administrators and delegated to the
// publish service in the admin flow, keeping BOT and HTTP paths consistent.
func (s *sSysBot) showCollectSourceConfig(ctx context.Context, botId int64, chatId string, account *botProfileAccount, sourceID int64) error {
	var src publishsysin.CollectSourceModel
	if err := g.DB().Model("hg_youban_publish_collect_source").Ctx(ctx).Where("id", sourceID).Where("tenant_id", account.TenantId).Where("account_id", account.AccountId).WhereNull("deleted_at").Scan(&src); err != nil {
		return err
	}
	var bindings []struct {
		RuleId int64 `json:"ruleId"`
	}
	if err := g.DB().Model("hg_youban_publish_collect_source_rule").Ctx(ctx).Where("source_id", sourceID).Where("tenant_id", account.TenantId).Where("status", 1).OrderAsc("sort").Scan(&bindings); err != nil {
		return err
	}
	for _, b := range bindings {
		src.RuleIds = append(src.RuleIds, b.RuleId)
	}
	if len(src.RuleIds) == 0 {
		var global []struct {
			Id int64 `json:"id"`
		}
		if err := g.DB().Model("hg_youban_publish_collect_rule").Ctx(ctx).Where("tenant_id", account.TenantId).Where("account_id", account.AccountId).Where("global_enabled", 1).Where("status", 1).WhereNull("deleted_at").OrderAsc("sort").Scan(&global); err != nil {
			return err
		}
		for _, r := range global {
			src.RuleIds = append(src.RuleIds, r.Id)
		}
	}
	text := fmt.Sprintf("采集配置：%s\n规则数量：%d", src.Title, len(src.RuleIds))
	buttons := make([][]models.InlineKeyboardButton, 0, len(src.RuleIds)+1)
	for _, rid := range src.RuleIds {
		rule, err := publishService.SysPublish().BotCollectRuleView(ctx, rid, account.TenantId, account.AccountId)
		if err != nil || rule == nil {
			continue
		}
		text += fmt.Sprintf("\n\n规则：%s\n删除整行：%d 条\n删除文本：%d 条\n替换：%d 条\n费用清理：%s\n前置文案：%s\n后置文案：%s", rule.Name, len(rule.DeleteLineTexts), len(rule.DeleteTexts), len(rule.Replacements), map[bool]string{true: "开启", false: "关闭"}[rule.TruncateIntroFeeEnabled == 1], map[bool]string{true: "开启", false: "关闭"}[rule.HeaderEnabled == 1], map[bool]string{true: "开启", false: "关闭"}[rule.FooterEnabled == 1])
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: "编辑 · " + rule.Name, CallbackData: fmt.Sprintf("cm:rule:%d", rule.Id)}})
	}
	buttons = append(buttons, []models.InlineKeyboardButton{{Text: "返回采集源", CallbackData: fmt.Sprintf("cm:view:%d", sourceID)}})
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, text, "HTML", false, &models.InlineKeyboardMarkup{InlineKeyboard: buttons})
	return err
}

func (s *sSysBot) showCollectRuleEditor(ctx context.Context, botId int64, chatId string, account *botProfileAccount, ruleID int64) error {
	r, err := publishService.SysPublish().BotCollectRuleView(ctx, ruleID, account.TenantId, account.AccountId)
	if err != nil {
		return err
	}
	text := fmt.Sprintf("规则：%s\n\n删除整行：%d 条\n删除文本：%d 条\n替换：%d 条\n费用清理：%s\n前置文案：%s\n后置文案：%s", r.Name, len(r.DeleteLineTexts), len(r.DeleteTexts), len(r.Replacements), onOff(r.TruncateIntroFeeEnabled), onOff(r.HeaderEnabled), onOff(r.FooterEnabled))
	buttons := [][]models.InlineKeyboardButton{
		{{Text: "费用清理 · " + onOff(r.TruncateIntroFeeEnabled), CallbackData: fmt.Sprintf("cm:ruleswitch:%d:fee", ruleID)}},
		{{Text: "前置文案 · " + onOff(r.HeaderEnabled), CallbackData: fmt.Sprintf("cm:ruleswitch:%d:header", ruleID)}},
		{{Text: "后置文案 · " + onOff(r.FooterEnabled), CallbackData: fmt.Sprintf("cm:ruleswitch:%d:footer", ruleID)}},
		{{Text: "返回采集配置", CallbackData: "cm:list:1"}},
		{{Text: "返回资料管理", CallbackData: "cm:back"}},
	}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, text, "HTML", false, &models.InlineKeyboardMarkup{InlineKeyboard: buttons})
	return err
}

func onOff(v int) string {
	if v == 1 {
		return "开启"
	}
	return "关闭"
}

func (s *sSysBot) toggleCollectRuleField(ctx context.Context, botId int64, chatId string, account *botProfileAccount, ruleID int64, field string) error {
	r, err := publishService.SysPublish().BotCollectRuleView(ctx, ruleID, account.TenantId, account.AccountId)
	if err != nil {
		return err
	}
	in := &publishsysin.CollectRuleSaveInp{Id: r.Id, Name: r.Name, GlobalEnabled: r.GlobalEnabled, TargetChannelIds: r.TargetChannelIds, ReviewEnabled: r.ReviewEnabled, DedupeEnabled: r.DedupeEnabled, DedupeDays: r.DedupeDays, FullMatchEnabled: r.FullMatchEnabled, Keywords: r.Keywords, Tags: r.Tags, Replacements: r.Replacements, DeleteLineTexts: r.DeleteLineTexts, DeleteTexts: r.DeleteTexts, TruncateIntroFeeEnabled: r.TruncateIntroFeeEnabled, IntroFeeSuffixEnabled: r.IntroFeeSuffixEnabled, IntroFeeSuffix: r.IntroFeeSuffix, BlockTexts: r.BlockTexts, BlockLink: r.BlockLink, BlockUsername: r.BlockUsername, BlockPlainText: r.BlockPlainText, HeaderEnabled: r.HeaderEnabled, HeaderMarkdown: r.HeaderMarkdown, FooterEnabled: r.FooterEnabled, FooterMarkdown: r.FooterMarkdown, Sort: r.Sort, Status: r.Status}
	switch field {
	case "fee":
		in.TruncateIntroFeeEnabled = 1 - in.TruncateIntroFeeEnabled
	case "header":
		in.HeaderEnabled = 1 - in.HeaderEnabled
	case "footer":
		in.FooterEnabled = 1 - in.FooterEnabled
	default:
		return nil
	}
	if _, err = publishService.SysPublish().BotCollectRuleSave(ctx, in, account.TenantId, account.AccountId); err != nil {
		return err
	}
	return s.showCollectRuleEditor(ctx, botId, chatId, account, ruleID)
}
