package sys

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_bot/model/input/sysin"
)

const inlinePromotionFeatureKey = "inline_promotion"

var inlinePromotionStartParameterRegexp = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type inlinePromotionFeature struct{}

func (inlinePromotionFeature) Key() string         { return inlinePromotionFeatureKey }
func (inlinePromotionFeature) Command() string     { return "" }
func (inlinePromotionFeature) Description() string { return "Inline 宣传" }
func (inlinePromotionFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return []*sysin.FeatureConfigSchema{
		{Field: "menuVisible", Label: "菜单可见", Component: "hidden", Default: 0},
		{Field: "title", Label: "候选标题", Component: "input", Default: "悦伴资料管理机器人", Placeholder: "仅显示在输入 @机器人 后的 Inline 候选列表"},
		{Field: "description", Label: "候选摘要", Component: "textarea", Default: "快速管理、发布和推广您的资料", Placeholder: "仅显示在 Inline 候选标题下方"},
		{Field: "messageText", Label: "发送文案", Component: "telegram_rich_text", Default: "<b>悦伴资料管理机器人</b>\n\n快速管理、发布和推广您的资料。\n\n点击上方按钮进入机器人。", Placeholder: "用户选中候选结果后真正发送到群聊的内容"},
		{Field: "imageUrl", Label: "宣传图片", Component: "image_upload", Default: "", Placeholder: "支持上传或填写 Telegram 可访问的 JPEG 外链"},
		{Field: "buttonText", Label: "打开按钮文案", Component: "input", Default: "打开机器人", Placeholder: "Inline 结果顶部按钮文案"},
		{Field: "targetFeatureKey", Label: "点击后执行", Component: "select", Default: "start", Placeholder: "选择打开机器人后执行的插件命令", Options: inlinePromotionTargetFeatureOptions()},
		{Field: "targetCommandArgs", Label: "命令参数", Component: "input", Default: "", Placeholder: "可选，例如插件命令需要的编号或参数"},
		{Field: "startParameter", Label: "启动参数", Component: "hidden", Default: "inline_entry"},
	}
}
func (inlinePromotionFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	if featureCtx == nil || featureCtx.Msg == nil {
		return true, nil
	}
	startParameter := normalizeInlinePromotionStartParameter(bot.featureConfigValue(ctx, inlinePromotionFeatureKey, "startParameter"))
	if handled, err := bot.dispatchInlinePromotionStart(ctx, &botFeatureContext{
		BotId: featureCtx.BotId,
		Msg:   featureCtx.Msg,
		Args:  startParameter,
	}); handled || err != nil {
		return true, err
	}
	botRow, err := bot.botById(ctx, featureCtx.BotId)
	if err != nil {
		return true, err
	}
	username := strings.TrimPrefix(strings.TrimSpace(botRow.BotUsername), "@")
	if username == "" {
		return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), "合作推广功能已开启，请在群聊中输入 @机器人选择推广内容。")
	}
	return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), fmt.Sprintf("请在群聊中输入 @%s，选择推广内容后发送。", username))
}

func (s *sSysBot) answerInlinePromotion(ctx context.Context, botId int64, query *models.InlineQuery) error {
	if query == nil {
		return nil
	}
	botRow, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	bot, err := s.telegramBot(ctx, botRow.BotToken)
	if err != nil {
		return err
	}
	params := &tgbot.AnswerInlineQueryParams{
		InlineQueryID: query.ID,
		Results:       []models.InlineQueryResult{},
		CacheTime:     0,
		IsPersonal:    true,
	}
	if _, enabled := s.featureConfig(ctx, inlinePromotionFeatureKey); enabled {
		title := defaultInlinePromotionValue(s.featureConfigValue(ctx, inlinePromotionFeatureKey, "title"), "悦伴资料管理机器人")
		description := defaultInlinePromotionValue(s.featureConfigValue(ctx, inlinePromotionFeatureKey, "description"), "快速管理、发布和推广您的资料")
		messageText := sanitizeTelegramHTML(defaultInlinePromotionValue(s.featureConfigValue(ctx, inlinePromotionFeatureKey, "messageText"), "<b>悦伴资料管理机器人</b>\n\n快速管理、发布和推广您的资料。"))
		g.Log().Debugf(ctx, "响应Inline宣传 botId:%d username:%s title:%s messageLength:%d", botId, botRow.BotUsername, title, len(messageText))
		buttonText := defaultInlinePromotionValue(s.featureConfigValue(ctx, inlinePromotionFeatureKey, "buttonText"), "打开机器人")
		startParameter := normalizeInlinePromotionStartParameter(s.featureConfigValue(ctx, inlinePromotionFeatureKey, "startParameter"))
		params.Button = &models.InlineQueryResultsButton{Text: buttonText, StartParameter: startParameter}

		username := strings.TrimPrefix(strings.TrimSpace(botRow.BotUsername), "@")
		var markup models.ReplyMarkup
		if username != "" {
			markup = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{
				Text: buttonText,
				URL:  fmt.Sprintf("https://t.me/%s?start=%s", username, startParameter),
			}}}}
		}
		imageURL := normalizePreviewMediaURL(s.absoluteMediaURL(ctx, s.featureConfigValue(ctx, inlinePromotionFeatureKey, "imageUrl")))
		if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
			thumbnailURL := normalizePreviewMediaURL(s.absoluteMediaURL(ctx, s.featureConfigValue(ctx, inlinePromotionFeatureKey, "imageThumbnailUrl")))
			if thumbnailURL == "" {
				thumbnailURL = imageURL
			}
			params.Results = append(params.Results, &models.InlineQueryResultPhoto{
				ID:           "inline_promotion_photo",
				PhotoURL:     imageURL,
				ThumbnailURL: thumbnailURL,
				PhotoWidth:   inlinePromotionConfigValueInt(s.featureConfigValue(ctx, inlinePromotionFeatureKey, "imageWidth")),
				PhotoHeight:  inlinePromotionConfigValueInt(s.featureConfigValue(ctx, inlinePromotionFeatureKey, "imageHeight")),
				Title:        title,
				Description:  description,
				Caption:      messageText,
				ParseMode:    models.ParseModeHTML,
				ReplyMarkup:  markup,
			})
		} else {
			params.Results = append(params.Results, &models.InlineQueryResultArticle{
				ID:          "inline_promotion_article",
				Title:       title,
				Description: description,
				InputMessageContent: &models.InputTextMessageContent{
					MessageText: messageText,
					ParseMode:   models.ParseModeHTML,
				},
				ReplyMarkup: markup,
			})
		}
	}
	callCtx, cancel := telegramAPICtx()
	defer cancel()
	_, err = bot.AnswerInlineQuery(callCtx, params)
	return err
}

func normalizeInlinePromotionStartParameter(value string) string {
	value = strings.TrimSpace(value)
	if !inlinePromotionStartParameterRegexp.MatchString(value) {
		return "inline_entry"
	}
	return value
}

func defaultInlinePromotionValue(value string, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func inlinePromotionTargetFeatureOptions() []*sysin.FeatureConfigOption {
	options := make([]*sysin.FeatureConfigOption, 0, len(botFeatures))
	for _, feature := range botFeatures {
		if feature == nil || feature.Key() == inlinePromotionFeatureKey || strings.TrimSpace(feature.Command()) == "" {
			continue
		}
		options = append(options, &sysin.FeatureConfigOption{
			Label: fmt.Sprintf("%s (/%s)", feature.Description(), strings.TrimPrefix(feature.Command(), "/")),
			Value: feature.Key(),
		})
	}
	return options
}

func normalizeInlinePromotionTargetFeatureKey(value string) string {
	value = strings.TrimSpace(value)
	for _, feature := range botFeatures {
		if feature != nil && feature.Key() == value && feature.Key() != inlinePromotionFeatureKey && strings.TrimSpace(feature.Command()) != "" {
			return value
		}
	}
	return (startFeature{}).Key()
}

func (s *sSysBot) dispatchInlinePromotionStart(ctx context.Context, featureCtx *botFeatureContext) (bool, error) {
	if featureCtx == nil || strings.TrimSpace(featureCtx.Args) != normalizeInlinePromotionStartParameter(s.featureConfigValue(ctx, inlinePromotionFeatureKey, "startParameter")) {
		return false, nil
	}
	targetKey := normalizeInlinePromotionTargetFeatureKey(s.featureConfigValue(ctx, inlinePromotionFeatureKey, "targetFeatureKey"))
	if targetKey == (startFeature{}).Key() {
		return false, nil
	}
	for _, feature := range botFeatures {
		if feature == nil || feature.Key() != targetKey {
			continue
		}
		if _, enabled := s.featureConfig(ctx, targetKey); !enabled {
			return true, s.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), "该功能暂未开启")
		}
		args := strings.TrimSpace(s.featureConfigValue(ctx, inlinePromotionFeatureKey, "targetCommandArgs"))
		command := "/" + s.featureCommand(ctx, feature)
		text := strings.TrimSpace(command + " " + args)
		return feature.Handle(ctx, s, &botFeatureContext{BotId: featureCtx.BotId, Msg: featureCtx.Msg, Text: text, Args: args})
	}
	return false, nil
}

func inlinePromotionConfigValueInt(value string) int {
	var result int
	_, _ = fmt.Sscan(strings.TrimSpace(value), &result)
	return result
}
