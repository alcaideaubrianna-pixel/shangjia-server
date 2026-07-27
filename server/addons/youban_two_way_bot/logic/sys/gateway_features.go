package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
	"hotgo/addons/youban_two_way_bot/internal/model/entity"
	"hotgo/internal/library/cache"
)

const gatewayFeatureSessionTTL = 24 * time.Hour

type twoWayChatGatewayFeature struct{ twoWayBot *sSysTwoWayBot }

func (f *twoWayChatGatewayFeature) Key() string   { return "youban_two_way_chat" }
func (f *twoWayChatGatewayFeature) Priority() int { return 100 }
func (f *twoWayChatGatewayFeature) Menus(_ context.Context, bot gatewayservice.BotContext) (gatewayservice.FeatureMenus, error) {
	if _, ok := twoWayBinding(bot); !ok {
		return gatewayservice.FeatureMenus{}, nil
	}
	return gatewayservice.FeatureMenus{Managed: true, Items: []gatewayservice.MenuItem{
		{Command: "start", Description: "开始使用", Order: 1},
		{Command: "chat", Description: "双向聊天", Order: 20},
	}}, nil
}
func (f *twoWayChatGatewayFeature) HandleUpdate(ctx context.Context, botCtx gatewayservice.BotContext, update *models.Update) (bool, error) {
	binding, ok := twoWayBinding(botCtx)
	if !ok || update == nil || update.Message == nil || update.Message.From == nil || update.Message.Chat.ID == 0 || !strings.EqualFold(string(update.Message.Chat.Type), "private") {
		return false, nil
	}
	text := strings.TrimSpace(update.Message.Text)
	command := gatewayCommand(text)
	if command != "chat" && command != "start" && text != "双向聊天" {
		return false, nil
	}
	row, err := f.twoWayBot.botById(ctx, binding.ReferenceID, binding.TenantID)
	if err != nil {
		return true, err
	}
	bot, err := f.twoWayBot.telegramBot(ctx, row.BotToken)
	if err != nil {
		return true, err
	}
	if command == "start" {
		_, _ = cache.Instance().Remove(ctx, gatewayFeatureSessionKey(botCtx.Key, update.Message.From.ID))
		welcome := strings.TrimSpace(row.WelcomeMessage)
		if welcome == "" {
			welcome = "欢迎使用，请通过底部菜单选择需要的功能。"
		}
		_, err = bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: welcome, ParseMode: models.ParseModeHTML, ReplyMarkup: gatewayReplyKeyboard(ctx, botCtx)})
		return true, err
	}
	_ = cache.Instance().Set(ctx, gatewayFeatureSessionKey(botCtx.Key, update.Message.From.ID), "chat", gatewayFeatureSessionTTL)
	_, err = bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: "已进入双向聊天，请直接发送需要咨询的内容。", ReplyMarkup: gatewayReplyKeyboard(ctx, botCtx)})
	return true, err
}

type cooperationGatewayFeature struct{ twoWayBot *sSysTwoWayBot }

func (f *cooperationGatewayFeature) Key() string   { return "youban_platform_cooperation" }
func (f *cooperationGatewayFeature) Priority() int { return 200 }
func (f *cooperationGatewayFeature) Menus(ctx context.Context, bot gatewayservice.BotContext) (gatewayservice.FeatureMenus, error) {
	config, err := cooperationConfigByToken(ctx, bot.Token)
	if err != nil || config == nil {
		return gatewayservice.FeatureMenus{}, err
	}
	return gatewayservice.FeatureMenus{Managed: true, Items: []gatewayservice.MenuItem{{Command: "cooperation", Description: "平台合作", Order: 10}}}, nil
}
func (f *cooperationGatewayFeature) HandleUpdate(ctx context.Context, botCtx gatewayservice.BotContext, update *models.Update) (bool, error) {
	if update == nil {
		return false, nil
	}
	config, err := cooperationConfigByToken(ctx, botCtx.Token)
	if err != nil || config == nil {
		return false, err
	}
	row, err := f.botRow(ctx, botCtx, config)
	if err != nil {
		return false, err
	}
	bot, err := f.twoWayBot.telegramBot(ctx, botCtx.Token)
	if err != nil {
		return false, err
	}
	if update.CallbackQuery != nil {
		return f.twoWayBot.handleCooperationCallback(ctx, bot, row, update.CallbackQuery)
	}
	msg := update.Message
	if msg == nil {
		msg = update.EditedMessage
	}
	if msg == nil || msg.From == nil || msg.Chat.ID == 0 || !strings.EqualFold(string(msg.Chat.Type), "private") {
		return false, nil
	}
	command := gatewayCommand(msg.Text)
	if command == "start" || command == "chat" || strings.TrimSpace(msg.Text) == "双向聊天" {
		_, _ = cache.Instance().Remove(ctx, gatewayFeatureSessionKey(botCtx.Key, msg.From.ID))
		_, _ = cache.Instance().Remove(ctx, cooperationSubmissionSessionKey(row.Id, msg.From.ID))
		return false, nil
	}
	trigger := command == "cooperation" || strings.TrimSpace(msg.Text) == "平台合作"
	if !trigger {
		state, _ := cache.Instance().Get(ctx, gatewayFeatureSessionKey(botCtx.Key, msg.From.ID))
		if state == nil || state.String() != "cooperation" {
			return false, nil
		}
	}
	_ = cache.Instance().Set(ctx, gatewayFeatureSessionKey(botCtx.Key, msg.From.ID), "cooperation", gatewayFeatureSessionTTL)
	handled, handleErr := f.twoWayBot.handleCooperationPrivateMessage(ctx, bot, row, msg)
	if handled {
		state, _ := cache.Instance().Get(ctx, cooperationSubmissionSessionKey(row.Id, msg.From.ID))
		if state == nil || state.IsNil() {
			_, _ = cache.Instance().Remove(ctx, gatewayFeatureSessionKey(botCtx.Key, msg.From.ID))
		}
	}
	return handled, handleErr
}

func gatewayFeatureSessionKey(botKey string, userId int64) string {
	return fmt.Sprintf("ybtg:feature:session:%s:%d", botKey, userId)
}

func gatewayReplyKeyboard(ctx context.Context, bot gatewayservice.BotContext) *models.ReplyKeyboardMarkup {
	buttons := make([]models.KeyboardButton, 0)
	seen := map[string]struct{}{}
	for _, feature := range gatewayservice.Features() {
		menus, err := feature.Menus(ctx, bot)
		if err != nil || !menus.Managed {
			continue
		}
		for _, item := range menus.Items {
			if strings.EqualFold(strings.TrimSpace(item.Command), "start") {
				continue
			}
			label := strings.TrimSpace(item.Description)
			if label == "" {
				continue
			}
			if _, ok := seen[label]; ok {
				continue
			}
			seen[label] = struct{}{}
			buttons = append(buttons, models.KeyboardButton{Text: label})
		}
	}
	rows := make([][]models.KeyboardButton, 0, (len(buttons)+1)/2)
	for len(buttons) > 0 {
		count := 2
		if len(buttons) < count {
			count = len(buttons)
		}
		rows = append(rows, append([]models.KeyboardButton(nil), buttons[:count]...))
		buttons = buttons[count:]
	}
	return &models.ReplyKeyboardMarkup{Keyboard: rows, IsPersistent: true, ResizeKeyboard: true, InputFieldPlaceholder: "请选择功能或发送消息"}
}
func (f *cooperationGatewayFeature) botRow(ctx context.Context, bot gatewayservice.BotContext, config *cooperationConfigRuntime) (*entity.YoubanTwoWayBotBot, error) {
	if binding, ok := twoWayBinding(bot); ok {
		return f.twoWayBot.botById(ctx, binding.ReferenceID, binding.TenantID)
	}
	return &entity.YoubanTwoWayBotBot{Id: -config.Id, TenantId: config.TenantId, BotToken: bot.Token}, nil
}

func twoWayBinding(bot gatewayservice.BotContext) (gatewayservice.BotBinding, bool) {
	for _, binding := range bot.Bindings {
		if binding.Owner == "youban_two_way_bot" {
			return binding, true
		}
	}
	return gatewayservice.BotBinding{}, false
}

func gatewayCommand(text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return ""
	}
	command := strings.TrimPrefix(strings.ToLower(fields[0]), "/")
	if index := strings.Index(command, "@"); index >= 0 {
		command = command[:index]
	}
	return command
}
