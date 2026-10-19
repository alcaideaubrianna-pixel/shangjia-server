package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/grand"

	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
)

type collectChannelSuffixPayload struct {
	ConfirmationToken string `json:"confirmationToken"`
	RuleId            int64  `json:"ruleId"`
	BotId             int64  `json:"botId"`
	AccountId         int64  `json:"accountId"`
	Id                int64  `json:"id"`
	ChannelId         string `json:"channelId"`
	ChannelTitle      string `json:"channelTitle"`
	ChannelLink       string `json:"channelLink"`
	DefaultSuffix     string `json:"defaultSuffix"`
	Draft             string `json:"draft"`
}

func (s *sSysBot) collectManageAllowed(ctx context.Context, account *botProfileAccount) bool {
	if account == nil || account.AccountId <= 0 || account.TenantId <= 0 {
		return false
	}
	status, err := publishService.SysPublish().TenantVipStatus(ctx, account.TenantId)
	return err == nil && status != nil && status.IsVip
}

func (s *sSysBot) collectRuleOwnerAccount(ctx context.Context, account *botProfileAccount, ruleID int64) *botProfileAccount {
	if account == nil || account.AccountType != "admin" || ruleID <= 0 {
		return account
	}
	var owner struct {
		AccountId int64 `json:"accountId"`
	}
	if err := g.DB().Model("hg_youban_publish_collect_rule").Ctx(ctx).Fields("account_id").Where("id", ruleID).Where("tenant_id", account.TenantId).WhereNull("deleted_at").Scan(&owner); err != nil || owner.AccountId <= 0 || owner.AccountId == account.AccountId {
		return account
	}
	resolved := *account
	resolved.AccountId = owner.AccountId
	return &resolved
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
	case "channelactioncancel":
		return true, s.sendCollectManageNotice(ctx, botId, chatId, "已取消操作，频道采集状态和后缀保持不变。")
	case "channelapprove", "channelreject", "channelrejectconfirm":
		if account.AccountType != "admin" || len(parts) < 3 {
			return true, nil
		}
		relationID, _ := strconv.ParseInt(parts[2], 10, 64)
		if action == "channelreject" {
			channel, err := publishService.SysPublish().BotCollectChannelView(ctx, relationID, account.TenantId, account.AccountId)
			if err != nil {
				return true, err
			}
			text := fmt.Sprintf("确认禁用频道采集？\n频道：%s\n频道 ID：%s%s\n\n确认后不再采集该频道的新消息。已有资料、已推送消息和后缀配置保留。", html.EscapeString(firstNonEmpty(channel.ChannelTitle, channel.ChannelId)), html.EscapeString(channel.ChannelId), collectManageChannelLinkLine(channel.ChannelLink))
			return true, s.sendCollectManageMarkup(ctx, botId, chatId, text, [][]models.InlineKeyboardButton{{{Text: "确认禁用", CallbackData: fmt.Sprintf("cm:channelrejectconfirm:%d", relationID)}, {Text: "取消", CallbackData: "cm:channelactioncancel"}}})
		}
		approval, err := publishService.SysPublish().BotCollectChannelApproval(ctx, relationID, account.TenantId, account.AccountId, action == "channelapprove")
		if err != nil {
			return true, err
		}
		if action == "channelapprove" {
			return true, s.startApprovedChannelSuffixSession(ctx, botId, chatId, account, approval)
		}
		return true, s.sendCollectManageNotice(ctx, botId, chatId, fmt.Sprintf("频道采集已禁用\n频道：%s\n频道 ID：%s%s\n操作原因：管理员主动禁用采集\n后续新消息将不再采集；已有资料和已推送消息保留。", html.EscapeString(firstNonEmpty(approval.ChannelTitle, approval.ChannelId)), html.EscapeString(approval.ChannelId), collectManageChannelLinkLine(approval.ChannelLink)))
	case "approve", "reject":
		if account.AccountType != "admin" || len(parts) < 3 {
			return true, nil
		}
		sourceID, _ := strconv.ParseInt(parts[2], 10, 64)
		if err := publishService.SysPublish().BotCollectSourceApproval(ctx, sourceID, account.TenantId, account.AccountId, action == "approve"); err != nil {
			return true, err
		}
		if action == "approve" {
			buttons := [][]models.InlineKeyboardButton{
				{{Text: "配置介绍费后缀", CallbackData: fmt.Sprintf("cm:config:%d", sourceID)}},
				{{Text: "暂不设置", CallbackData: "cm:list:1"}},
			}
			return true, s.sendCollectManageMarkup(ctx, botId, chatId, "已允许采集该频道。是否现在配置介绍费后缀？", buttons)
		}
		return true, s.sendCollectManageNotice(ctx, botId, chatId, "已禁用该频道采集。后续新消息将不再采集；已有资料和已推送消息保留。")
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
		return true, s.showCollectRuleEditor(ctx, botId, chatId, s.collectRuleOwnerAccount(ctx, account, ruleID), ruleID)
	case "channels":
		if account.AccountType != "admin" || len(parts) < 3 {
			return true, nil
		}
		ruleID, _ := strconv.ParseInt(parts[2], 10, 64)
		page := 1
		if len(parts) > 3 {
			page, _ = strconv.Atoi(parts[3])
		}
		return true, s.showCollectRuleChannels(ctx, botId, chatId, s.collectRuleOwnerAccount(ctx, account, ruleID), ruleID, page)
	case "channel":
		if account.AccountType != "admin" || len(parts) < 5 {
			return true, nil
		}
		ruleID, _ := strconv.ParseInt(parts[2], 10, 64)
		rowID, _ := strconv.ParseInt(parts[3], 10, 64)
		return true, s.showCollectRuleChannel(ctx, botId, chatId, s.collectRuleOwnerAccount(ctx, account, ruleID), ruleID, rowID, parts[4])
	case "channelstatus", "channelstatusconfirm", "channeldelete", "channeldeleteconfirm":
		if account.AccountType != "admin" || len(parts) < 6 {
			return true, nil
		}
		ruleID, _ := strconv.ParseInt(parts[2], 10, 64)
		rowID, _ := strconv.ParseInt(parts[3], 10, 64)
		status, _ := strconv.Atoi(parts[5])
		owner := s.collectRuleOwnerAccount(ctx, account, ruleID)
		if action == "channeldelete" {
			item, err := s.collectRuleChannelItem(ctx, botId, owner, ruleID, rowID, parts[4])
			if err != nil {
				return true, err
			}
			if item == nil {
				return true, s.sendCollectManageNotice(ctx, botId, chatId, "频道不存在或已被清理。")
			}
			preview, err := publishService.SysPublish().BotIntroFeeChannelPurge(ctx, &publishsysin.IntroFeeChannelPurgeInp{Id: rowID, RuleId: ruleID, BotId: botId, ChannelId: item.ChannelId, Preview: true}, owner.TenantId, owner.AccountId)
			if err != nil {
				return true, err
			}
			return true, s.sendCollectManageMarkup(ctx, botId, chatId,
				collectChannelDeleteDetails(item, preview)+"\n\n确认后将禁用该频道采集、删除以上代理资料及其已推送消息，并清理频道后缀和计数。此操作不可恢复。",
				[][]models.InlineKeyboardButton{{
					{Text: "确认删除并下架", CallbackData: fmt.Sprintf("cm:channeldeleteconfirm:%d:%d:%s:0", ruleID, rowID, item.ChannelId)},
					{Text: "取消", CallbackData: fmt.Sprintf("cm:channel:%d:%d:%s", ruleID, rowID, item.ChannelId)},
				}})
		}
		if action == "channeldeleteconfirm" {
			item, err := s.collectRuleChannelItem(ctx, botId, owner, ruleID, rowID, parts[4])
			if err != nil {
				return true, err
			}
			if item == nil {
				return true, s.sendCollectManageNotice(ctx, botId, chatId, "频道不存在或已被清理。")
			}
			res, err := publishService.SysPublish().BotIntroFeeChannelPurge(ctx, &publishsysin.IntroFeeChannelPurgeInp{Id: rowID, RuleId: ruleID, BotId: botId, ChannelId: item.ChannelId, NotifyChatId: chatId}, owner.TenantId, owner.AccountId)
			if err != nil {
				return true, err
			}
			return true, s.sendCollectManageNotice(ctx, botId, chatId, collectChannelDeleteNotice(item, res))
		}
		item, err := s.collectRuleChannelItem(ctx, botId, owner, ruleID, rowID, parts[4])
		if err != nil {
			return true, err
		}
		if item == nil {
			return true, s.sendCollectManageNotice(ctx, botId, chatId, "频道不存在或已被清理。")
		}
		if action == "channelstatus" && status == 2 {
			text := fmt.Sprintf("确认禁用频道采集？\n频道：%s\n频道 ID：%s%s\n\n确认后不再采集该频道的新消息。已有资料、已推送消息和后缀配置保留。", html.EscapeString(firstNonEmpty(item.ChannelTitle, item.ChannelId)), html.EscapeString(item.ChannelId), collectManageChannelLinkLine(item.ChannelLink))
			return true, s.sendCollectManageMarkup(ctx, botId, chatId, text, [][]models.InlineKeyboardButton{{{Text: "确认禁用", CallbackData: fmt.Sprintf("cm:channelstatusconfirm:%d:%d:%s:2", ruleID, rowID, item.ChannelId)}, {Text: "取消", CallbackData: "cm:channelactioncancel"}}})
		}
		_, err = publishService.SysPublish().BotIntroFeeChannelRuleSave(ctx, &publishsysin.IntroFeeChannelRuleSaveInp{Id: rowID, RuleId: ruleID, BotId: botId, ChannelId: item.ChannelId, Suffix: item.Suffix, Status: status}, owner.TenantId, owner.AccountId)
		if err != nil {
			return true, err
		}
		return true, s.showCollectRuleChannel(ctx, botId, chatId, owner, ruleID, rowID, parts[4])
	case "addchannel":
		if account.AccountType != "admin" || len(parts) < 3 {
			return true, nil
		}
		ruleID, _ := strconv.ParseInt(parts[2], 10, 64)
		owner := s.collectRuleOwnerAccount(ctx, account, ruleID)
		return true, s.startProfileSessionByIds(ctx, botId, telegramUserIdFromCtx(ctx), chatId, account, "collect_channel_add", "waiting_value", ruleID, "", map[string]interface{}{"ruleId": ruleID, "botId": botId, "accountId": owner.AccountId}, "请输入：频道ID 后缀\n例如：-1001234567890 新后缀")
	case "channeledit", "channelinput":
		if account.AccountType != "admin" || len(parts) < 4 {
			return true, nil
		}
		ruleID, _ := strconv.ParseInt(parts[2], 10, 64)
		channelID := strings.TrimSpace(parts[3])
		if ruleID <= 0 || channelID == "" {
			return true, nil
		}
		rowID := int64(0)
		if len(parts) > 4 {
			rowID, _ = strconv.ParseInt(parts[4], 10, 64)
		}
		owner := s.collectRuleOwnerAccount(ctx, account, ruleID)
		item, err := s.collectRuleChannelItem(ctx, botId, owner, ruleID, rowID, channelID)
		if err != nil {
			return true, err
		}
		if item == nil {
			return true, s.sendCollectManageNotice(ctx, botId, chatId, "频道不存在或已失效，请刷新频道列表后重试。")
		}
		if strings.TrimSpace(item.Suffix) == "" {
			rule, err := publishService.SysPublish().BotCollectRuleView(ctx, ruleID, owner.TenantId, owner.AccountId)
			if err != nil {
				return true, err
			}
			if rule != nil {
				item.Suffix = rule.IntroFeeSuffix
			}
		}
		if action == "channeledit" {
			text := fmt.Sprintf("确认修改频道后缀？\n频道：%s\n频道 ID：%s%s\n当前后缀：%s\n\n继续后请输入新后缀，预览并再次确认保存后才会生效。", html.EscapeString(firstNonEmpty(item.ChannelTitle, item.ChannelId)), html.EscapeString(item.ChannelId), collectManageChannelLinkLine(item.ChannelLink), html.EscapeString(firstNonEmpty(item.Suffix, "未配置")))
			return true, s.sendCollectManageMarkup(ctx, botId, chatId, text, [][]models.InlineKeyboardButton{{{Text: "确认修改", CallbackData: fmt.Sprintf("cm:channelinput:%d:%s:%d", ruleID, item.ChannelId, item.Id)}, {Text: "取消", CallbackData: "cm:channelactioncancel"}}})
		}
		payload := collectChannelSuffixPayload{RuleId: ruleID, BotId: botId, AccountId: owner.AccountId, ChannelId: item.ChannelId, ChannelTitle: item.ChannelTitle, ChannelLink: item.ChannelLink, Id: item.Id, DefaultSuffix: item.Suffix}
		prompt := fmt.Sprintf("修改频道后缀\n频道：%s\n频道 ID：%s%s\n当前后缀：%s\n\n请发送新的频道后缀，例如：北京代理。\n发送后会显示预览，确认保存后才生效。\n发送 /cancel 取消修改。", html.EscapeString(firstNonEmpty(item.ChannelTitle, item.ChannelId)), html.EscapeString(item.ChannelId), collectManageChannelLinkLine(item.ChannelLink), html.EscapeString(firstNonEmpty(item.Suffix, "未配置")))
		return true, s.startProfileSessionByIds(ctx, botId, telegramUserIdFromCtx(ctx), chatId, account, "collect_channel_add", "waiting_value", ruleID, "", payload, prompt)
	case "ruleswitch":
		if account.AccountType != "admin" || len(parts) < 4 {
			return true, nil
		}
		ruleID, _ := strconv.ParseInt(parts[2], 10, 64)
		return true, s.toggleCollectRuleField(ctx, botId, chatId, s.collectRuleOwnerAccount(ctx, account, ruleID), ruleID, parts[3])
	case "edit":
		if account.AccountType != "admin" || len(parts) < 4 {
			return true, nil
		}
		ruleID, _ := strconv.ParseInt(parts[2], 10, 64)
		return true, s.showCollectRuleField(ctx, botId, chatId, s.collectRuleOwnerAccount(ctx, account, ruleID), ruleID, parts[3])
	case "editstart":
		if account.AccountType != "admin" || len(parts) < 4 {
			return true, nil
		}
		ruleID, _ := strconv.ParseInt(parts[2], 10, 64)
		return true, s.startCollectRuleEdit(ctx, botId, chatId, s.collectRuleOwnerAccount(ctx, account, ruleID), ruleID, parts[3])
	case "editconfirm":
		session := s.activeProfileSessionByIds(ctx, botId, telegramUserIdFromCtx(ctx), chatId)
		if session == nil || session.Scene != "collect_rule_edit" {
			return true, nil
		}
		var payload struct {
			Field string `json:"field"`
			Draft string `json:"draft"`
		}
		if json.Unmarshal([]byte(session.PayloadJson), &payload) != nil {
			return true, nil
		}
		session.Step = "confirming"
		return true, s.handleCollectRuleEditSession(ctx, botId, chatId, account, session, payload.Draft)
	case "editcancel":
		session := s.activeProfileSessionByIds(ctx, botId, telegramUserIdFromCtx(ctx), chatId)
		if session == nil {
			return true, nil
		}
		_ = s.completeProfileSession(ctx, session.Id)
		return true, s.showCollectRuleField(ctx, botId, chatId, account, session.ProfileId, session.Step)
	case "channelsuffixconfirm":
		session := s.activeProfileSessionByIds(ctx, botId, telegramUserIdFromCtx(ctx), chatId)
		if !collectChannelSuffixSessionMatches(session, parts, action == "channelsuffixconfirm") {
			return true, s.sendCollectManageNotice(ctx, botId, chatId, "确认已失效，请使用最新的后缀预览重新操作。")
		}
		return true, s.confirmCollectChannelSuffix(ctx, botId, chatId, account, session)
	case "channelsuffixcancel":
		session := s.activeProfileSessionByIds(ctx, botId, telegramUserIdFromCtx(ctx), chatId)
		if !collectChannelSuffixSessionMatches(session, parts, false) {
			return true, s.sendCollectManageNotice(ctx, botId, chatId, "该按钮已失效，当前操作未取消。")
		}
		result, err := g.DB().Model(profileSessionTable).Ctx(ctx).Where("id", session.Id).Where("status", profileSessionStatusActive).
			Where("payload_json", session.PayloadJson).Data(g.Map{"status": profileSessionStatusCanceled}).Update()
		if err != nil {
			return true, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return true, err
		}
		if n == 0 {
			return true, s.sendCollectManageNotice(ctx, botId, chatId, "该按钮已失效，当前操作未取消。")
		}
		return true, s.sendCollectManageNotice(ctx, botId, chatId, "已取消修改，频道采集状态和当前后缀保持不变。")
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

func (s *sSysBot) startApprovedChannelSuffixSession(ctx context.Context, botId int64, chatId string, account *botProfileAccount, approval *publishsysin.CollectChannelApprovalModel) error {
	if approval == nil || approval.RuleId <= 0 || strings.TrimSpace(approval.ChannelId) == "" {
		return s.sendCollectManageNotice(ctx, botId, chatId, "已允许采集该频道。当前采集规则不可用，可稍后在频道后缀配置中设置后缀。")
	}
	payload := collectChannelSuffixPayload{
		ConfirmationToken: grand.S(12), RuleId: approval.RuleId, BotId: approval.BotId, AccountId: approval.AccountId,
		ChannelId: approval.ChannelId, ChannelTitle: approval.ChannelTitle, ChannelLink: approval.ChannelLink, DefaultSuffix: approval.DefaultSuffix,
	}
	if err := s.startProfileSessionByIds(ctx, botId, telegramUserIdFromCtx(ctx), chatId, account, "collect_channel_add", "waiting_value", approval.RuleId, "", payload, ""); err != nil {
		return err
	}
	defaultSuffix := strings.TrimSpace(approval.DefaultSuffix)
	if defaultSuffix == "" {
		defaultSuffix = "未设置"
	}
	name := strings.TrimSpace(approval.ChannelTitle)
	if name == "" {
		name = approval.ChannelId
	}
	text := fmt.Sprintf("修改频道后缀\n频道：%s%s\n当前规则默认后缀：%s\n\n请发送新的频道后缀，例如：北京代理。\n发送后会显示预览，确认保存后才生效。取消修改不影响当前配置。", html.EscapeString(name), collectManageChannelLinkLine(approval.ChannelLink), html.EscapeString(defaultSuffix))
	session := s.activeProfileSessionByIds(ctx, botId, telegramUserIdFromCtx(ctx), chatId)
	if session == nil {
		return gerror.New("后缀修改会话创建失败，请重试")
	}
	buttons := [][]models.InlineKeyboardButton{{{Text: "取消修改", CallbackData: fmt.Sprintf("cm:channelsuffixcancel:%d:%s", session.Id, payload.ConfirmationToken)}}}
	return s.sendCollectManageMarkup(ctx, botId, chatId, text, buttons)
}

func (s *sSysBot) confirmCollectChannelSuffix(ctx context.Context, botId int64, chatId string, account *botProfileAccount, session *profileSessionRow) error {
	var payload collectChannelSuffixPayload
	if json.Unmarshal([]byte(session.PayloadJson), &payload) != nil || payload.RuleId <= 0 || strings.TrimSpace(payload.ChannelId) == "" || strings.TrimSpace(payload.Draft) == "" {
		return s.sendCollectManageNotice(ctx, botId, chatId, "频道后缀设置已失效，请重新操作。")
	}
	ownerAccountID := payload.AccountId
	if ownerAccountID <= 0 {
		ownerAccountID = account.AccountId
	}
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var locked profileSessionRow
		if err := g.DB().Model(profileSessionTable).Ctx(ctx).Where("id", session.Id).LockUpdate().Scan(&locked); err != nil {
			return err
		}
		if locked.Status != profileSessionStatusActive || locked.Step != "confirming" || locked.PayloadJson != session.PayloadJson {
			return gerror.New("确认已失效，请使用最新的后缀预览重新操作")
		}
		if _, err := publishService.SysPublish().BotIntroFeeChannelRuleSave(ctx, &publishsysin.IntroFeeChannelRuleSaveInp{
			Id: payload.Id, RuleId: payload.RuleId, BotId: payload.BotId, ChannelId: payload.ChannelId, Suffix: strings.TrimSpace(payload.Draft), Status: 1,
		}, account.TenantId, ownerAccountID); err != nil {
			return err
		}
		return s.completeProfileSession(ctx, session.Id)
	})
	if err != nil {
		return err
	}
	return s.sendCollectManageNotice(ctx, botId, chatId, fmt.Sprintf("频道后缀已保存\n频道：%s\n频道 ID：%s%s\n新后缀：%s\n采集状态：已开启\n后续新采集资料将使用此后缀。", html.EscapeString(firstNonEmpty(payload.ChannelTitle, payload.ChannelId)), html.EscapeString(payload.ChannelId), collectManageChannelLinkLine(payload.ChannelLink), html.EscapeString(strings.TrimSpace(payload.Draft))))
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
	mod := g.DB().Model("hg_youban_publish_collect_source").Ctx(ctx).Where("id", sourceID).Where("tenant_id", account.TenantId).WhereNull("deleted_at")
	if account.AccountType != "admin" {
		mod = mod.Where("account_id", account.AccountId)
	}
	if err := mod.Scan(&src); err != nil {
		return err
	}
	var bindings []struct {
		RuleId int64 `json:"ruleId"`
	}
	if err := g.DB().Model("hg_youban_publish_collect_source_rule").Ctx(ctx).Where("source_id", sourceID).Where("tenant_id", account.TenantId).OrderAsc("sort").Scan(&bindings); err != nil {
		return err
	}
	for _, b := range bindings {
		src.RuleIds = append(src.RuleIds, b.RuleId)
	}
	if len(src.RuleIds) == 0 {
		return s.sendCollectManageNotice(ctx, botId, chatId, "当前采集源未绑定专属规则，请先在后台完成配置。")
	}
	if len(src.RuleIds) == 1 {
		return s.showCollectRuleEditor(ctx, botId, chatId, s.collectRuleOwnerAccount(ctx, account, src.RuleIds[0]), src.RuleIds[0])
	}
	text := fmt.Sprintf("采集配置：%s\n该采集源绑定了 %d 条规则，请选择要配置的规则。", html.EscapeString(src.Title), len(src.RuleIds))
	buttons := make([][]models.InlineKeyboardButton, 0, len(src.RuleIds)+1)
	for _, rid := range src.RuleIds {
		owner := s.collectRuleOwnerAccount(ctx, account, rid)
		rule, err := publishService.SysPublish().BotCollectRuleView(ctx, rid, owner.TenantId, owner.AccountId)
		if err != nil || rule == nil {
			continue
		}
		text += fmt.Sprintf("\n\n规则：%s", html.EscapeString(rule.Name))
		label := "修改配置"
		if len(src.RuleIds) > 1 {
			label += " · " + rule.Name
		}
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: label, CallbackData: fmt.Sprintf("cm:rule:%d", rule.Id)}})
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
	text := fmt.Sprintf("采集配置\n规则：%s\n请选择要修改的配置项：", html.EscapeString(r.Name))
	buttons := [][]models.InlineKeyboardButton{
		{{Text: fmt.Sprintf("删除整行（%d条）", len(r.DeleteLineTexts)), CallbackData: fmt.Sprintf("cm:edit:%d:delete_line", ruleID)}},
		{{Text: fmt.Sprintf("删除文本（%d条）", len(r.DeleteTexts)), CallbackData: fmt.Sprintf("cm:edit:%d:delete_text", ruleID)}},
		{{Text: fmt.Sprintf("文本替换（%d条）", len(r.Replacements)), CallbackData: fmt.Sprintf("cm:edit:%d:replace", ruleID)}},
		{{Text: "费用清理 · " + onOff(r.TruncateIntroFeeEnabled), CallbackData: fmt.Sprintf("cm:ruleswitch:%d:fee", ruleID)}},
		{{Text: "介绍费后缀 · " + onOff(r.IntroFeeSuffixEnabled), CallbackData: fmt.Sprintf("cm:channels:%d:1", ruleID)}},
		{{Text: "前置文案 · " + onOff(r.HeaderEnabled), CallbackData: fmt.Sprintf("cm:edit:%d:header", ruleID)}},
		{{Text: "后置文案 · " + onOff(r.FooterEnabled), CallbackData: fmt.Sprintf("cm:edit:%d:footer", ruleID)}},
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

func (s *sSysBot) showCollectRuleChannels(ctx context.Context, botId int64, chatId string, account *botProfileAccount, ruleID int64, page int) error {
	if page < 1 {
		page = 1
	}
	in := &publishsysin.IntroFeeChannelRuleListInp{RuleId: ruleID, BotId: botId, IncludeCandidates: true}
	in.Page = page
	in.PerPage = 8
	rows, total, err := publishService.SysPublish().BotIntroFeeChannelRuleList(ctx, in, account.TenantId, account.AccountId)
	if err != nil {
		return err
	}
	pages := (total + in.PerPage - 1) / in.PerPage
	lines := []string{fmt.Sprintf("介绍费后缀\n频道列表 · 第 %d/%d 页 · 共 %d 个", page, max(1, pages), total)}
	buttons := make([][]models.InlineKeyboardButton, 0, len(rows)+4)
	for _, row := range rows {
		if row == nil {
			continue
		}
		name := row.ChannelTitle
		if name == "" {
			name = row.ChannelId
		}
		if row.ChannelLink != "" {
			name += " · " + row.ChannelLink
		}
		suffix := row.Suffix
		status := "已启用"
		if row.Status != 1 {
			status = "已禁用"
		}
		lines = append(lines, fmt.Sprintf("%s（%s）\n后缀：%s · 状态：%s", html.EscapeString(name), html.EscapeString(row.ChannelId), html.EscapeString(suffix), status))
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: name + " · " + status, CallbackData: fmt.Sprintf("cm:channel:%d:%d:%s", ruleID, row.Id, row.ChannelId)}})
	}
	nav := make([]models.InlineKeyboardButton, 0, 2)
	if page > 1 {
		nav = append(nav, models.InlineKeyboardButton{Text: "上一页", CallbackData: fmt.Sprintf("cm:channels:%d:%d", ruleID, page-1)})
	}
	if page < pages {
		nav = append(nav, models.InlineKeyboardButton{Text: "下一页", CallbackData: fmt.Sprintf("cm:channels:%d:%d", ruleID, page+1)})
	}
	if len(nav) > 0 {
		buttons = append(buttons, nav)
	}
	buttons = append(buttons, []models.InlineKeyboardButton{{Text: "添加频道", CallbackData: fmt.Sprintf("cm:addchannel:%d", ruleID)}})
	buttons = append(buttons, []models.InlineKeyboardButton{{Text: "返回规则", CallbackData: fmt.Sprintf("cm:rule:%d", ruleID)}})
	return s.sendCollectManageMarkup(ctx, botId, chatId, strings.Join(lines, "\n"), buttons)
}

func (s *sSysBot) showCollectRuleChannel(ctx context.Context, botId int64, chatId string, account *botProfileAccount, ruleID, rowID int64, channelID string) error {
	item, err := s.collectRuleChannelItem(ctx, botId, account, ruleID, rowID, channelID)
	if err != nil {
		return err
	}
	if item == nil {
		return s.sendCollectManageNotice(ctx, botId, chatId, "频道不存在或已被删除。")
	}
	name := item.ChannelTitle
	if name == "" {
		name = item.ChannelId
	}
	status := "已启用"
	if item.Status != 1 {
		status = "已禁用"
	}
	suffix := item.Suffix
	text := fmt.Sprintf("频道：%s\n频道 ID：%s%s\n后缀：%s\n状态：%s", html.EscapeString(name), html.EscapeString(item.ChannelId), collectManageChannelLinkLine(item.ChannelLink), html.EscapeString(suffix), status)
	buttons := [][]models.InlineKeyboardButton{{{Text: "编辑后缀", CallbackData: fmt.Sprintf("cm:channeledit:%d:%s:%d", ruleID, item.ChannelId, item.Id)}}}
	if item.Status == 1 {
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: "禁用", CallbackData: fmt.Sprintf("cm:channelstatus:%d:%d:%s:2", ruleID, item.Id, item.ChannelId)}})
	} else {
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: "启用", CallbackData: fmt.Sprintf("cm:channelstatus:%d:%d:%s:1", ruleID, item.Id, item.ChannelId)}})
	}
	buttons = append(buttons, []models.InlineKeyboardButton{{Text: "删除并下架", CallbackData: fmt.Sprintf("cm:channeldelete:%d:%d:%s:0", ruleID, item.Id, item.ChannelId)}})
	buttons = append(buttons, []models.InlineKeyboardButton{{Text: "返回频道列表", CallbackData: fmt.Sprintf("cm:channels:%d:1", ruleID)}})
	return s.sendCollectManageMarkup(ctx, botId, chatId, text, buttons)
}

func (s *sSysBot) collectRuleChannelItem(ctx context.Context, botId int64, account *botProfileAccount, ruleID, rowID int64, channelID string) (*publishsysin.IntroFeeChannelRuleItem, error) {
	in := &publishsysin.IntroFeeChannelRuleListInp{RuleId: ruleID, BotId: botId, IncludeCandidates: true}
	in.Page, in.PerPage = 1, 100
	for {
		rows, total, err := publishService.SysPublish().BotIntroFeeChannelRuleList(ctx, in, account.TenantId, account.AccountId)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row != nil && (row.Id == rowID && rowID > 0 || row.ChannelId == channelID) {
				return row, nil
			}
		}
		if len(rows) == 0 || in.Page*in.PerPage >= total {
			break
		}
		in.Page++
	}
	return nil, nil
}

func (s *sSysBot) sendCollectManageMarkup(ctx context.Context, botId int64, chatId, text string, buttons [][]models.InlineKeyboardButton) error {
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

func (s *sSysBot) showCollectRuleField(ctx context.Context, botId int64, chatId string, account *botProfileAccount, ruleID int64, field string) error {
	r, err := publishService.SysPublish().BotCollectRuleView(ctx, ruleID, account.TenantId, account.AccountId)
	if err != nil {
		return err
	}
	value := collectRuleFieldDisplay(r, field)
	if value == "" {
		value = "未配置"
	}
	buttons := [][]models.InlineKeyboardButton{{{Text: "修改配置", CallbackData: fmt.Sprintf("cm:editstart:%d:%s", ruleID, field)}}, {{Text: "返回配置项", CallbackData: fmt.Sprintf("cm:rule:%d", ruleID)}}, {{Text: "返回资料管理", CallbackData: "cm:back"}}, {{Text: "取消当前操作", CallbackData: "pf:cancel"}}}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	display := strings.Join(splitCollectBotValues(value), ",")
	if display == "" {
		display = "未配置"
	}
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, fmt.Sprintf("规则：%s\n配置项：%s\n\n当前配置：\n<blockquote>%s</blockquote>", html.EscapeString(r.Name), collectRuleFieldLabel(field), html.EscapeString(display)), "HTML", false, &models.InlineKeyboardMarkup{InlineKeyboard: buttons})
	return err
}

func collectRuleFieldDisplay(r *publishsysin.CollectRuleModel, field string) string {
	switch field {
	case "delete_line":
		return strings.Join(r.DeleteLineTexts, ",")
	case "delete_text":
		return strings.Join(r.DeleteTexts, ",")
	case "header":
		return r.HeaderMarkdown
	case "footer":
		return r.FooterMarkdown
	case "intro_suffix":
		return r.IntroFeeSuffix
	case "replace":
		p := make([]string, 0, len(r.Replacements))
		for _, v := range r.Replacements {
			p = append(p, v.From+">"+v.To)
		}
		return strings.Join(p, ",")
	}
	return ""
}

func collectRuleFieldLabel(field string) string {
	switch field {
	case "delete_line":
		return "删除整行"
	case "delete_text":
		return "删除文本"
	case "replace":
		return "文本替换"
	case "header":
		return "前置文案"
	case "footer":
		return "后置文案"
	case "intro_suffix":
		return "介绍费后缀"
	}
	return "配置"
}

func (s *sSysBot) startCollectRuleEdit(ctx context.Context, botId int64, chatId string, account *botProfileAccount, ruleID int64, field string) error {
	r, err := publishService.SysPublish().BotCollectRuleView(ctx, ruleID, account.TenantId, account.AccountId)
	if err != nil {
		return err
	}
	value := ""
	switch field {
	case "delete_line":
		value = strings.Join(r.DeleteLineTexts, ",")
	case "delete_text":
		value = strings.Join(r.DeleteTexts, ",")
	case "replace":
		pairs := make([]string, 0, len(r.Replacements))
		for _, item := range r.Replacements {
			pairs = append(pairs, item.From+">"+item.To)
		}
		value = strings.Join(pairs, ",")
	case "header":
		value = r.HeaderMarkdown
	case "footer":
		value = r.FooterMarkdown
	case "intro_suffix":
		value = r.IntroFeeSuffix
	}
	example := "亚朵,卖家,联系方式"
	if field == "replace" {
		example = "原文>替换,原文A>替换B"
	}
	prompt := fmt.Sprintf("规则：%s\n配置项：%s\n\n当前配置：\n<blockquote>%s</blockquote>\n\n请发送新配置，多个内容使用逗号分隔。\n例如：%s\n\n发送 /cancel 取消。", html.EscapeString(r.Name), html.EscapeString(collectRuleFieldLabel(field)), html.EscapeString(value), html.EscapeString(example))
	return s.startProfileSessionByIds(ctx, botId, telegramUserIdFromCtx(ctx), chatId, account, "collect_rule_edit", field, ruleID, "", map[string]interface{}{"field": field, "accountId": account.AccountId}, prompt)
}

func (s *sSysBot) handleCollectRuleEditSession(ctx context.Context, botId int64, chatId string, account *botProfileAccount, session *profileSessionRow, text string) error {
	var payload struct {
		Field     string `json:"field"`
		AccountId int64  `json:"accountId"`
	}
	_ = json.Unmarshal([]byte(session.PayloadJson), &payload)
	ownerAccountID := payload.AccountId
	if ownerAccountID <= 0 {
		ownerAccountID = account.AccountId
	}
	owner := *account
	owner.AccountId = ownerAccountID
	r, err := publishService.SysPublish().BotCollectRuleView(ctx, session.ProfileId, owner.TenantId, owner.AccountId)
	if err != nil {
		return err
	}
	if session.Step != "confirming" {
		payloadJSON, _ := json.Marshal(map[string]interface{}{"field": payload.Field, "draft": text, "accountId": ownerAccountID})
		_, err = g.DB().Model(profileSessionTable).Ctx(ctx).Where("id", session.Id).Data(g.Map{"payload_json": string(payloadJSON), "updated_at": gtime.Now()}).Update()
		if err != nil {
			return err
		}
		buttons := [][]models.InlineKeyboardButton{{{Text: "确定修改", CallbackData: "cm:editconfirm"}}, {{Text: "取消修改", CallbackData: "cm:editcancel"}}}
		row, err := s.botById(ctx, botId)
		if err != nil {
			return err
		}
		_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, fmt.Sprintf("规则：%s\n配置项：%s\n\n修改后的配置：\n<blockquote>%s</blockquote>\n\n确认保存吗？", html.EscapeString(r.Name), html.EscapeString(collectRuleFieldLabel(payload.Field)), html.EscapeString(text)), "HTML", false, &models.InlineKeyboardMarkup{InlineKeyboard: buttons})
		return err
	}
	in := &publishsysin.CollectRuleSaveInp{Id: r.Id, Name: r.Name, GlobalEnabled: r.GlobalEnabled, TargetChannelIds: r.TargetChannelIds, ReviewEnabled: r.ReviewEnabled, DedupeEnabled: r.DedupeEnabled, DedupeDays: r.DedupeDays, FullMatchEnabled: r.FullMatchEnabled, Keywords: r.Keywords, Tags: r.Tags, Replacements: r.Replacements, DeleteLineTexts: r.DeleteLineTexts, DeleteTexts: r.DeleteTexts, TruncateIntroFeeEnabled: r.TruncateIntroFeeEnabled, IntroFeeSuffixEnabled: r.IntroFeeSuffixEnabled, IntroFeeSuffix: r.IntroFeeSuffix, BlockTexts: r.BlockTexts, BlockLink: r.BlockLink, BlockUsername: r.BlockUsername, BlockPlainText: r.BlockPlainText, HeaderEnabled: r.HeaderEnabled, HeaderMarkdown: r.HeaderMarkdown, FooterEnabled: r.FooterEnabled, FooterMarkdown: r.FooterMarkdown, Sort: r.Sort, Status: r.Status}
	values := splitCollectBotValues(text)
	switch payload.Field {
	case "delete_line":
		in.DeleteLineTexts = values
	case "delete_text":
		in.DeleteTexts = values
	case "header":
		in.HeaderEnabled = 1
		in.HeaderMarkdown = text
	case "footer":
		in.FooterEnabled = 1
		in.FooterMarkdown = text
	case "intro_suffix":
		in.IntroFeeSuffixEnabled = 1
		in.IntroFeeSuffix = strings.TrimSpace(text)
	case "replace":
		in.Replacements = nil
		for _, item := range values {
			p := strings.SplitN(item, ">", 2)
			if len(p) != 2 || strings.TrimSpace(p[0]) == "" {
				return s.sendCollectManageNotice(ctx, botId, chatId, "替换格式不正确，请使用：原文>替换,原文A>替换B")
			}
			in.Replacements = append(in.Replacements, publishsysin.CollectRuleReplaceModel{From: strings.TrimSpace(p[0]), To: strings.TrimSpace(p[1])})
		}
	}
	if _, err = publishService.SysPublish().BotCollectRuleSave(ctx, in, owner.TenantId, owner.AccountId); err != nil {
		return err
	}
	_ = s.completeProfileSession(ctx, session.Id)
	return s.showCollectRuleEditor(ctx, botId, chatId, &owner, session.ProfileId)
}

func splitCollectBotValues(text string) []string {
	parts := strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == '，' || r == '\n' || r == '\r' })
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			result = append(result, p)
		}
	}
	return result
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

func collectChannelDeleteDetails(item *publishsysin.IntroFeeChannelRuleItem, res *publishsysin.IntroFeeChannelPurgeModel) string {
	name := firstNonEmpty(item.ChannelTitle, item.ChannelId)
	suffix := firstNonEmpty(item.Suffix, "未配置")
	return fmt.Sprintf("删除频道代理资料并下架\n操作原因：管理员主动删除该频道的代理资料\n处理范围：当前账户下该来源频道的全部规则和 BOT\n频道：%s\n频道 ID：%s%s\n频道后缀：%s\n关联代理资料：%d 条\n已推送消息：%d 条", html.EscapeString(name), html.EscapeString(item.ChannelId), collectManageChannelLinkLine(item.ChannelLink), html.EscapeString(suffix), res.ProfileCount, res.MessageCount)
}

func collectChannelDeleteNotice(item *publishsysin.IntroFeeChannelRuleItem, res *publishsysin.IntroFeeChannelPurgeModel) string {
	status := "已完成。该频道采集已禁用，频道后缀和计数已清理，无关联代理资料或已推送消息需要删除。"
	if res.Queued == 1 {
		status = "任务已提交，该频道采集已禁用。后台正在删除上述代理资料及其已推送消息，完成后会再次通知。"
	}
	return collectChannelDeleteDetails(item, res) + "\n状态：" + status
}

func collectManageChannelLinkLine(link string) string {
	if strings.TrimSpace(link) == "" {
		return ""
	}
	return "\n频道链接：" + html.EscapeString(strings.TrimSpace(link))
}

func collectChannelSuffixSessionMatches(session *profileSessionRow, parts []string, requireDraft bool) bool {
	if session == nil || len(parts) != 4 || session.Scene != "collect_channel_add" || session.Status != profileSessionStatusActive {
		return false
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || id != session.Id {
		return false
	}
	var payload collectChannelSuffixPayload
	if json.Unmarshal([]byte(session.PayloadJson), &payload) != nil || payload.ConfirmationToken == "" || payload.ConfirmationToken != parts[3] {
		return false
	}
	return !requireDraft || (session.Step == "confirming" && strings.TrimSpace(payload.Draft) != "")
}
