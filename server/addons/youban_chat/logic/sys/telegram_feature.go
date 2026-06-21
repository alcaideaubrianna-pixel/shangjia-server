package sys

import (
	"context"
	"fmt"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_chat/model/input/sysin"
)

type telegramFeatureContext struct {
	BotId int64
	Input *sysin.TelegramWebhookInp
	Msg   *sysin.TelegramMessageInp
	Text  string
	Args  string
}

type telegramFeature interface {
	Key() string
	Command() string
	Description() string
	Enabled(ctx context.Context, chat *sSysChat) bool
	Handle(ctx context.Context, chat *sSysChat, featureCtx *telegramFeatureContext) (handled bool, err error)
}

var telegramFeatures []telegramFeature

func registerTelegramFeature(feature telegramFeature) {
	if feature == nil || strings.TrimSpace(feature.Key()) == "" {
		return
	}
	telegramFeatures = append(telegramFeatures, feature)
}

func init() {
	registerTelegramFeature(bindTelegramFeature{})
	registerTelegramFeature(helpTelegramFeature{})
	registerTelegramFeature(startTelegramFeature{})
}

func (s *sSysChat) dispatchTelegramFeatures(ctx context.Context, in *sysin.TelegramWebhookInp, msg *sysin.TelegramMessageInp) (bool, error) {
	text := telegramMessageText(msg)
	command, args := telegramCommandAndArgs(text)
	if command != "" && msg != nil && msg.Chat != nil {
		g.Log().Infof(ctx, "收到Telegram命令 bot:%d chat:%d command:%s args:%s", in.BotId, msg.Chat.Id, command, args)
	}
	if command == "" {
		return false, nil
	}
	for _, feature := range telegramFeatures {
		if feature == nil || !feature.Enabled(ctx, s) {
			continue
		}
		if command == strings.ToLower(s.telegramFeatureCommand(ctx, feature)) {
			return feature.Handle(ctx, s, &telegramFeatureContext{BotId: in.BotId, Input: in, Msg: msg, Text: text, Args: args})
		}
	}
	return false, nil
}

func (s *sSysChat) syncTelegramBotMenu(ctx context.Context, botToken string) error {
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	defaultCommands := s.telegramBotCommands(ctx, false)
	groupCommands := s.telegramBotCommands(ctx, true)
	if len(defaultCommands) == 0 && len(groupCommands) == 0 {
		return nil
	}
	if len(defaultCommands) > 0 {
		if _, err = bot.SetMyCommands(ctx, &tgbot.SetMyCommandsParams{
			Commands: defaultCommands,
			Scope:    &models.BotCommandScopeDefault{},
		}); err != nil {
			return err
		}
	}
	if len(groupCommands) > 0 {
		for _, scope := range []models.BotCommandScope{
			&models.BotCommandScopeAllGroupChats{},
			&models.BotCommandScopeAllChatAdministrators{},
		} {
			if _, err = bot.SetMyCommands(ctx, &tgbot.SetMyCommandsParams{
				Commands: groupCommands,
				Scope:    scope,
			}); err != nil {
				return err
			}
		}
	}
	if _, err = bot.SetChatMenuButton(ctx, &tgbot.SetChatMenuButtonParams{
		MenuButton: &models.MenuButtonCommands{},
	}); err != nil {
		return err
	}
	g.Log().Infof(ctx, "同步Telegram菜单成功 default:%d group:%d", len(defaultCommands), len(groupCommands))
	return nil
}

func (s *sSysChat) telegramBotCommands(ctx context.Context, group bool) []models.BotCommand {
	commands := make([]models.BotCommand, 0, len(telegramFeatures))
	for _, feature := range telegramFeatures {
		if feature == nil || !feature.Enabled(ctx, s) || strings.TrimSpace(s.telegramFeatureCommand(ctx, feature)) == "" {
			continue
		}
		if group && feature.Key() == "start" {
			continue
		}
		commands = append(commands, models.BotCommand{
			Command:     strings.TrimPrefix(s.telegramFeatureCommand(ctx, feature), "/"),
			Description: s.telegramFeatureDescription(ctx, feature),
		})
	}
	return commands
}

func telegramCommandAndArgs(text string) (command string, args string) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return "", ""
	}
	first := strings.TrimSpace(fields[0])
	if !strings.HasPrefix(first, "/") {
		return "", ""
	}
	if at := strings.Index(first, "@"); at > 0 {
		first = first[:at]
	}
	command = strings.ToLower(strings.TrimPrefix(first, "/"))
	if len(fields) > 1 {
		args = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), fields[0]))
	}
	return command, args
}

func (s *sSysChat) telegramFeatureCommand(ctx context.Context, feature telegramFeature) string {
	if feature == nil {
		return ""
	}
	if row, _ := s.telegramFeatureConfig(ctx, feature.Key()); row != nil && strings.TrimSpace(row.Command) != "" {
		return strings.TrimPrefix(strings.TrimSpace(row.Command), "/")
	}
	return strings.TrimPrefix(strings.TrimSpace(feature.Command()), "/")
}

func (s *sSysChat) telegramFeatureDescription(ctx context.Context, feature telegramFeature) string {
	if feature == nil {
		return ""
	}
	if row, _ := s.telegramFeatureConfig(ctx, feature.Key()); row != nil && strings.TrimSpace(row.Description) != "" {
		return strings.TrimSpace(row.Description)
	}
	return strings.TrimSpace(feature.Description())
}

type bindTelegramFeature struct{}

func (bindTelegramFeature) Key() string         { return "bind" }
func (bindTelegramFeature) Command() string     { return "bind" }
func (bindTelegramFeature) Description() string { return "绑定当前群聊" }
func (bindTelegramFeature) Enabled(ctx context.Context, chat *sSysChat) bool {
	return telegramFeatureEnabled(ctx, chat, "bind")
}

func (bindTelegramFeature) Handle(ctx context.Context, chat *sSysChat, featureCtx *telegramFeatureContext) (bool, error) {
	code := strings.TrimSpace(featureCtx.Args)
	if code == "" {
		token, _ := chat.telegramTokenForIncoming(ctx, featureCtx.BotId)
		if token != "" && featureCtx.Msg != nil && featureCtx.Msg.Chat != nil {
			_, _ = chat.telegramSendMessage(ctx, token, fmt.Sprintf("%d", featureCtx.Msg.Chat.Id), featureCtx.Msg.MessageThreadId, "请发送：/bind 绑定码")
		}
		return true, nil
	}
	return true, chat.bindTelegramChatByCode(ctx, code, featureCtx.Msg, featureCtx.BotId)
}

type helpTelegramFeature struct{}

func (helpTelegramFeature) Key() string         { return "help" }
func (helpTelegramFeature) Command() string     { return "help" }
func (helpTelegramFeature) Description() string { return "查看使用帮助" }
func (helpTelegramFeature) Enabled(ctx context.Context, chat *sSysChat) bool {
	return telegramFeatureEnabled(ctx, chat, "help")
}

func (helpTelegramFeature) Handle(ctx context.Context, chat *sSysChat, featureCtx *telegramFeatureContext) (bool, error) {
	if featureCtx.Msg == nil || featureCtx.Msg.Chat == nil {
		return true, nil
	}
	token, err := chat.telegramTokenForIncoming(ctx, featureCtx.BotId)
	if err != nil || token == "" {
		return true, err
	}
	_, err = chat.telegramSendMessage(ctx, token, fmt.Sprintf("%d", featureCtx.Msg.Chat.Id), featureCtx.Msg.MessageThreadId, "可用命令：\n/bind 绑定码 - 绑定当前群聊\n/help - 查看帮助")
	return true, err
}

type startTelegramFeature struct{}

func (startTelegramFeature) Key() string         { return "start" }
func (startTelegramFeature) Command() string     { return "start" }
func (startTelegramFeature) Description() string { return "开始使用" }
func (startTelegramFeature) Enabled(ctx context.Context, chat *sSysChat) bool {
	return telegramFeatureEnabled(ctx, chat, "start")
}

func (startTelegramFeature) Handle(ctx context.Context, chat *sSysChat, featureCtx *telegramFeatureContext) (bool, error) {
	return helpTelegramFeature{}.Handle(ctx, chat, featureCtx)
}

func telegramFeatureEnabled(ctx context.Context, chat *sSysChat, key string) bool {
	if chat != nil {
		if _, enabled := chat.telegramFeatureConfig(ctx, key); !enabled {
			return false
		}
	}
	configKey := "youbanChat.telegram.features." + strings.TrimSpace(key)
	value := g.Cfg().MustGet(ctx, configKey)
	if value.IsNil() {
		return true
	}
	return value.Bool()
}

func (s *sSysChat) botTokenById(ctx context.Context, botId int64) (string, error) {
	if botId <= 0 {
		return "", nil
	}
	row, err := s.getBotById(ctx, botId)
	if err != nil {
		return "", err
	}
	if row == nil || strings.TrimSpace(row.BotToken) == "" {
		return "", gerror.New("Bot不存在或未启用")
	}
	return strings.TrimSpace(row.BotToken), nil
}

func (s *sSysChat) telegramTokenForIncoming(ctx context.Context, botId int64) (string, error) {
	if botId > 0 {
		return s.botTokenById(ctx, botId)
	}
	bots, err := s.enabledBots(ctx)
	if err != nil {
		return "", err
	}
	if len(bots) == 1 && bots[0] != nil {
		return strings.TrimSpace(bots[0].BotToken), nil
	}
	g.Log().Warningf(ctx, "无法确定Telegram Bot Token botId:%d enabledBots:%d", botId, len(bots))
	return "", nil
}
