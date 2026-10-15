package sys

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
)

var relayManagementCommands = regexp.MustCompile(`<code>/relayadmin([^<]*)</code>`)

func (s *sSysBot) relayManagementReply(ctx context.Context, botID int64, msg *models.Message, text string) error {
	row, err := s.botById(ctx, botID)
	if err != nil {
		return err
	}
	bot, err := s.relayBot(ctx, row.BotToken)
	if err != nil {
		return err
	}
	buttons := [][]models.InlineKeyboardButton{}
	for _, match := range relayManagementCommands.FindAllStringSubmatch(text, -1) {
		args := strings.TrimSpace(match[1])
		label := args
		if args == "" {
			label = "返回Bot列表"
		} else if fields := strings.Fields(args); len(fields) > 0 {
			if _, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
				if len(fields) == 1 {
					label = "管理 Bot #" + fields[0]
				} else {
					label = strings.Join(fields[1:], " ")
				}
			} else if len(fields) == 2 && fields[0] == "列表" {
				label = "Bot列表 · 第" + fields[1] + "页"
			}
		}
		if strings.Contains(args, "@alice") || strings.Contains(args, "-1001234567890") || strings.Contains(args, "123456789") || strings.Contains(args, "关键词") || len("rm:"+args) > 64 {
			continue
		}
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: label, CallbackData: "rm:" + args}})
	}
	_, err = bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: msg.Chat.ID, Text: text, ParseMode: models.ParseModeHTML, ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: buttons}})
	return err
}

func (s *sSysBot) handleRelayManagementCallback(ctx context.Context, botID int64, q *models.CallbackQuery) (bool, error) {
	if q == nil || !strings.HasPrefix(q.Data, "rm:") {
		return false, nil
	}
	row, err := s.botById(ctx, botID)
	if err != nil {
		return true, err
	}
	bot, err := s.relayBot(ctx, row.BotToken)
	if err != nil {
		return true, err
	}
	if _, err = bot.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		g.Log().Warningf(ctx, "应答双向管理按钮失败 botId:%d err:%+v", botID, err)
	}
	if q.Message.Message == nil {
		return true, nil
	}
	msg := &models.Message{ID: q.Message.Message.ID, Chat: q.Message.Message.Chat, From: &q.From}
	_, err = (relayManagementFeature{}).Handle(ctx, s, &botFeatureContext{BotId: botID, Msg: msg, Args: strings.TrimPrefix(q.Data, "rm:")})
	return true, err
}

func relayManagementActionPrompt(botID int64, action string) string {
	prefix := fmt.Sprintf("/relayadmin %d ", botID)
	examples := map[string]string{
		"绑定": "绑定 @alice @bob", "转发群": "转发群 -1001234567890", "解绑": "解绑 @alice", "添加管理员": "添加管理员 123456789", "删除管理员": "删除管理员 123456789", "屏蔽关键词": "屏蔽 广告", "删除关键词": "删除屏蔽 广告", "屏蔽用户": "屏蔽用户 123456789", "解除屏蔽用户": "解除屏蔽用户 123456789",
	}
	example, ok := examples[action]
	if !ok {
		return ""
	}
	text := "请复制命令，替换示例后发送：\n<code>" + prefix + example + "</code>"
	if action == "添加管理员" {
		text += "\n也支持 @用户名。对方需到目标双向Bot私聊发送 /start 确认身份，才能使用管理命令。所有管理员需先私聊目标Bot才能收到通知。"
	}
	return text + "\n\n返回：<code>" + strings.TrimSpace(prefix) + "</code>"
}

func relayManagementPage(args []string) (int, bool) {
	if len(args) == 0 || args[0] != "列表" {
		return 0, false
	}
	if len(args) != 2 {
		return 0, true
	}
	page, err := strconv.Atoi(args[1])
	if err != nil || page < 1 {
		return 0, true
	}
	return page, true
}

func (s *sSysBot) relayExpiredButton(ctx context.Context, botID int64, q *models.CallbackQuery) error {
	row, err := s.botById(ctx, botID)
	if err != nil {
		return err
	}
	bot, err := s.relayBot(ctx, row.BotToken)
	if err != nil {
		return err
	}
	_, err = bot.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: q.ID, Text: "这是旧的主机器人按钮，请到主机器人使用；此Bot仅用于双向聊天。", ShowAlert: true})
	return err
}
