package sys

import (
	"context"
	"html"
	"strings"

	"hotgo/addons/youban_bot/internal/model/entity"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
)

func (s *sSysBot) relayMainMenuInput(ctx context.Context, text string) bool {
	command, _ := botCommandAndArgs(text)
	if command != "" {
		return command != "help" && command != "id" && command != "start"
	}
	for _, feature := range botFeatures {
		if feature == nil || feature.Key() == (relayFeature{}).Key() {
			continue
		}
		row, _ := s.featureConfig(ctx, feature.Key())
		if s.matchFeatureLabel(ctx, row, feature, strings.TrimSpace(text)) {
			return true
		}
	}
	return false
}

func (s *sSysBot) relayWelcome(ctx context.Context, cfg *entity.YoubanBotRelayConfig, msg *models.Message) error {
	active, state, err := s.relayRuntimeState(ctx, cfg)
	if err != nil {
		return err
	}
	text := "<b>双向聊天机器人</b>\n绑定后，双方的新消息会互相传递。\n请联系管理员完成绑定；发送 <code>/id</code> 查看聊天ID。\n不补发历史记录，编辑和撤回暂不同步。"
	if !active {
		text += "\n\n当前状态：" + html.EscapeString(state)
	}
	if msg.From != nil {
		admin, err := s.relayIsAdmin(ctx, cfg, msg.From.ID)
		if err != nil {
			return err
		}
		if admin {
			text += "\n\n管理员身份已确认。请私聊发送 <code>/help</code> 查看管理命令。\n绑定用户首次发送普通消息时，会向已确认的管理员发送一次通知。"
		} else {
			text += "\n\n若你刚被添加为 @用户名管理员，发送本命令后会自动确认当前身份。未获得权限时，请联系配置人员核对用户名或改用用户ID。"
		}
	}
	row, err := s.botById(ctx, cfg.BotId)
	if err != nil {
		return err
	}
	bot, err := s.relayBot(ctx, row.BotToken)
	if err != nil {
		return err
	}
	_, err = bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: msg.Chat.ID, Text: text, ParseMode: models.ParseModeHTML, ReplyMarkup: &models.ReplyKeyboardRemove{RemoveKeyboard: true}})
	return err
}

func (s *sSysBot) relayInteractionError(ctx context.Context, botID int64, msg *models.Message, cause error) error {
	g.Log().Warningf(ctx, "双向机器人交互失败 botId:%d chatId:%d err:%+v", botID, msg.Chat.ID, cause)
	if err := s.relayReply(ctx, botID, msg, "操作暂未完成，请稍后重试；持续失败请联系管理员检查配置。已发送的聊天消息请勿反复重发。"); err != nil {
		return err
	}
	return cause
}
