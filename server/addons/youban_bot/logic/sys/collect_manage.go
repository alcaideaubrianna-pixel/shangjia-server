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
	text := fmt.Sprintf("采集配置：%s\n规则数量：%d\n\n请在后台采集规则页面修改删除关键词、替换关键词、费用清理及前后置文案。BOT 配置编辑即将开放。", src.Title, len(src.RuleIds))
	return s.sendCollectManageNotice(ctx, botId, chatId, text)
}
