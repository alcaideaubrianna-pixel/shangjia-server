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
	"hotgo/internal/model/input/form"
)

const (
	uploaderCollectCallback    = "cc:"
	uploaderCollectSuffixScene = "uploader_collect_suffix"
	uploaderCollectPageSize    = 6
)

type uploaderCollectSuffixDraft struct {
	RelationId int64
	RuleId     int64
	Suffix     string
	Token      string
}

func uploaderCollectAllowed(account *botProfileAccount) bool {
	return account != nil && account.AccountType == publishsysin.PublishAccountTypeUploader && account.CollectPushEnabled
}

func uploaderAccountKeyboard(account *botProfileAccount) *models.ReplyKeyboardMarkup {
	keyboard := collectorReplyKeyboard(false)
	if uploaderCollectAllowed(account) {
		keyboard.Keyboard = append(keyboard.Keyboard, []models.KeyboardButton{{Text: "采集推送"}})
	}
	return keyboard
}

func uploaderCollectSessionAllowed(account *botProfileAccount, session *profileSessionRow) bool {
	return uploaderCollectAllowed(account) && session != nil && session.Scene == uploaderCollectSuffixScene &&
		session.Status == profileSessionStatusActive && session.App == account.App &&
		session.TenantId == account.TenantId && session.AccountId == account.AccountId
}

func (s *sSysBot) cancelUploaderCollectSession(ctx context.Context, session *profileSessionRow) error {
	if session == nil || session.Scene != uploaderCollectSuffixScene {
		return nil
	}
	_, err := g.DB().Model(profileSessionTable).Ctx(ctx).Where("id", session.Id).
		Where("scene", uploaderCollectSuffixScene).Where("status", profileSessionStatusActive).
		Data(g.Map{"status": profileSessionStatusCanceled}).Update()
	return err
}

func (s *sSysBot) handleUploaderCollectCallback(ctx context.Context, botID int64, chatID string, account *botProfileAccount, session *profileSessionRow, data string) error {
	parts := strings.Split(strings.TrimPrefix(data, uploaderCollectCallback), ":")
	if len(parts) < 2 {
		return gerror.New("采集推送按钮已失效")
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return gerror.New("采集推送按钮已失效")
	}
	switch parts[0] {
	case "list":
		page, err := strconv.Atoi(parts[1])
		if err != nil || len(parts) != 2 || page > int(^uint(0)>>1)/uploaderCollectPageSize {
			return gerror.New("频道列表页码无效")
		}
		return s.showUploaderCollectChannels(ctx, botID, chatID, account, page)
	case "suffix":
		if len(parts) != 3 {
			return gerror.New("采集规则按钮已失效")
		}
		ruleID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || ruleID <= 0 {
			return gerror.New("请选择采集规则")
		}
		draft := uploaderCollectSuffixDraft{RelationId: id, RuleId: ruleID}
		if err := s.startProfileSessionByIds(ctx, botID, telegramUserIdFromCtx(ctx), chatID, account,
			uploaderCollectSuffixScene, "waiting_value", 0, "", draft, ""); err != nil {
			return err
		}
		return s.sendCollectorReply(ctx, botID, chatID, "请输入新的频道后缀，或发送 /cancel 取消。")
	case "save":
		if len(parts) != 3 || !uploaderCollectSessionAllowed(account, session) || session.Id != id {
			return gerror.New("后缀确认已失效，请重新操作")
		}
		return s.saveUploaderCollectSuffix(ctx, botID, chatID, account, session, parts[2])
	case "disable", "approve", "reject":
		if len(parts) != 2 {
			return gerror.New("采集频道按钮已失效")
		}
		if err := publishService.SysPublish().BotCollectManageChannelAction(ctx, account.TenantId, account.AccountId,
			&publishsysin.CollectManageChannelActionInp{RelationId: id, Action: parts[0]}); err != nil {
			return err
		}
		return s.sendCollectManageMarkup(ctx, botID, chatID, "频道操作已完成。", uploaderCollectListButton())
	default:
		return gerror.New("不支持此采集频道操作")
	}
}

func uploaderCollectListButton() [][]models.InlineKeyboardButton {
	return [][]models.InlineKeyboardButton{{{Text: "返回频道列表", CallbackData: uploaderCollectCallback + "list:1"}}}
}

func (s *sSysBot) showUploaderCollectChannels(ctx context.Context, botID int64, chatID string, account *botProfileAccount, page int) error {
	rows, total, err := publishService.SysPublish().BotCollectManageChannels(ctx, account.TenantId, account.AccountId,
		&publishsysin.CollectManageChannelListInp{PageReq: form.PageReq{Page: page, PerPage: uploaderCollectPageSize, Pagination: true}})
	if err != nil {
		return err
	}

	var text strings.Builder
	fmt.Fprintf(&text, "<b>采集推送</b> · 第 %d 页 · 共 %d 条", page, total)
	if len(rows) == 0 {
		text.WriteString("\n当前页没有采集频道。")
	}
	var buttons [][]models.InlineKeyboardButton
	for index, row := range rows {
		if row == nil {
			continue
		}
		number := index + 1
		title := shortText(firstNonEmpty(row.ChannelTitle, row.ChannelId), 80)
		rule := shortText(firstNonEmpty(row.RuleName, "无规则"), 60)
		suffix := shortText(firstNonEmpty(row.Suffix, "未设置"), 100)
		fmt.Fprintf(&text, "\n\n%d. %s\n频道 ID：%s%s\n规则：%s\n后缀：%s\n采集：%s · 审核：%s",
			number, html.EscapeString(title), html.EscapeString(row.ChannelId),
			collectManageChannelLinkLine(shortText(row.ChannelLink, 180)), html.EscapeString(rule),
			html.EscapeString(suffix), html.EscapeString(row.Status), html.EscapeString(row.ApprovalStatus))
		var actions []models.InlineKeyboardButton
		if row.RuleId > 0 {
			actions = append(actions, models.InlineKeyboardButton{Text: fmt.Sprintf("%d 改后缀", number),
				CallbackData: fmt.Sprintf("%ssuffix:%d:%d", uploaderCollectCallback, row.RelationId, row.RuleId)})
		}
		if row.ApprovalStatus == "pending" {
			actions = append(actions, uploaderCollectActionButton(number, row.RelationId, "通过", "approve"),
				uploaderCollectActionButton(number, row.RelationId, "拒绝", "reject"))
		} else if row.Status == publishsysin.CollectAccountChannelEnabled {
			actions = append(actions, uploaderCollectActionButton(number, row.RelationId, "禁用", "disable"))
		}
		if len(actions) > 0 {
			buttons = append(buttons, actions)
		}
	}
	var navigation []models.InlineKeyboardButton
	if page > 1 {
		navigation = append(navigation, models.InlineKeyboardButton{Text: "上一页", CallbackData: fmt.Sprintf("%slist:%d", uploaderCollectCallback, page-1)})
	}
	if page*uploaderCollectPageSize < total {
		navigation = append(navigation, models.InlineKeyboardButton{Text: "下一页", CallbackData: fmt.Sprintf("%slist:%d", uploaderCollectCallback, page+1)})
	}
	if len(navigation) > 0 {
		buttons = append(buttons, navigation)
	}
	if len(buttons) == 0 {
		return s.sendCollectorReply(ctx, botID, chatID, text.String())
	}
	return s.sendCollectManageMarkup(ctx, botID, chatID, text.String(), buttons)
}

func uploaderCollectActionButton(number int, relationID int64, label, action string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: fmt.Sprintf("%d %s", number, label),
		CallbackData: fmt.Sprintf("%s%s:%d", uploaderCollectCallback, action, relationID)}
}

func (s *sSysBot) previewUploaderCollectSuffix(ctx context.Context, botID int64, chatID string, session *profileSessionRow, text string) error {
	var draft uploaderCollectSuffixDraft
	if json.Unmarshal([]byte(session.PayloadJson), &draft) != nil || draft.RelationId <= 0 || draft.RuleId <= 0 {
		return gerror.New("频道后缀会话已失效，请重新操作")
	}
	draft.Suffix = text
	draft.Token = grand.S(12)
	payload, err := json.Marshal(draft)
	if err != nil {
		return err
	}
	result, err := g.DB().Model(profileSessionTable).Ctx(ctx).Where("id", session.Id).
		Where("status", profileSessionStatusActive).WhereGT("expires_at", gtime.Now()).
		Where("payload_json", session.PayloadJson).
		Data(g.Map{"step": "confirming", "payload_json": string(payload)}).Update()
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return s.sendCollectorReply(ctx, botID, chatID, "后缀会话已变更，请重新操作。")
	}
	buttons := [][]models.InlineKeyboardButton{{{Text: "确认修改", CallbackData: fmt.Sprintf("%ssave:%d:%s", uploaderCollectCallback, session.Id, draft.Token)}}}
	return s.sendCollectManageMarkup(ctx, botID, chatID, "确认新后缀：\n"+html.EscapeString(shortText(draft.Suffix, 500))+"\n\n发送 /cancel 可取消。", buttons)
}

func (s *sSysBot) saveUploaderCollectSuffix(ctx context.Context, botID int64, chatID string, account *botProfileAccount, session *profileSessionRow, token string) error {
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var current profileSessionRow
		if err := tx.Model(profileSessionTable).Where("id", session.Id).Where("bot_id", botID).
			Where("telegram_user_id", telegramUserIdFromCtx(ctx)).Where("chat_id", chatID).
			WhereGT("expires_at", gtime.Now()).LockUpdate().Scan(&current); err != nil {
			return err
		}
		var draft uploaderCollectSuffixDraft
		if !uploaderCollectSessionAllowed(account, &current) || current.Step != "confirming" ||
			json.Unmarshal([]byte(current.PayloadJson), &draft) != nil || draft.Token == "" || token != draft.Token {
			return gerror.New("后缀确认已失效，请使用最新预览")
		}
		if err := publishService.SysPublish().BotCollectManageChannelAction(ctx, account.TenantId, account.AccountId,
			&publishsysin.CollectManageChannelActionInp{RelationId: draft.RelationId, RuleId: draft.RuleId, Action: "suffix", Suffix: draft.Suffix}); err != nil {
			return err
		}
		return s.completeProfileSession(ctx, current.Id)
	})
	if err != nil {
		return err
	}
	return s.sendCollectManageMarkup(ctx, botID, chatID, "频道后缀已保存。", uploaderCollectListButton())
}
