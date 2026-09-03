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
)

func (s *sSysBot) collectManageAllowed(ctx context.Context, account *botProfileAccount) bool {
	if account == nil || account.AccountId <= 0 || account.TenantId <= 0 {
		return false
	}
	count, err := g.DB().Model("hg_youban_publish_tenant_vip").Ctx(ctx).
		Where("tenant_id", account.TenantId).Where("status", 1).Where("level", ">", 0).
		Where("(expired_at IS NULL OR expired_at > ?)", gtime.Now()).WhereNull("deleted_at").Count()
	return err == nil && count > 0
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
		{{Text: "返回列表", CallbackData: "cm:list:1"}},
	}}
	botRow, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	_, err = s.sendMessageWithMarkup(ctx, botRow.BotToken, chatId, text, "HTML", false, markup)
	return err
}
