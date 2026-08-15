package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/grand"

	"hotgo/addons/youban_bot/model/input/sysin"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
	"hotgo/internal/library/cache"
)

const (
	quickPushSessionStateWaiting     = "waiting_message"
	quickPushSessionStateSelecting   = "selecting_plans"
	quickPushSessionStateEditName    = "editing_template_name"
	quickPushSessionStateEditText    = "editing_template_text"
	quickPushSessionStateEditMedia   = "editing_template_media"
	quickPushSessionStateEditButtons = "editing_template_buttons"
	quickPushSessionTTL              = 30 * time.Minute
	quickPushCallbackPrefix          = "yb_qp"
)

type quickPushFeature struct{}

func (quickPushFeature) Key() string     { return "quick_push" }
func (quickPushFeature) Command() string { return "quickpush" }
func (quickPushFeature) Description() string {
	return "快速推送"
}
func (quickPushFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return []*sysin.FeatureConfigSchema{
		{Field: "waitingText", Label: "等待输入提示", Component: "textarea", Default: "请发送或转发需要快速推送的文本、图片、视频或图文媒体组。", Placeholder: "进入快速推送后的提示文案"},
		{Field: "unboundText", Label: "无权限提示", Component: "textarea", Default: "仅绑定上架端管理员账号后可使用快速推送。"},
	}
}
func (quickPushFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	if featureCtx.Msg == nil || featureCtx.Msg.From == nil {
		return true, nil
	}
	telegramUserId := fmt.Sprintf("%d", featureCtx.Msg.From.ID)
	bind, account, err := bot.quickPushBoundAccount(ctx, telegramUserId)
	if err != nil {
		return true, err
	}
	chatId := fmt.Sprintf("%d", featureCtx.Msg.Chat.ID)
	if bind == nil || account == nil || account.AccountId <= 0 {
		return true, bot.reply(ctx, featureCtx.BotId, chatId, bot.featureConfigValue(ctx, quickPushFeature{}.Key(), "unboundText"))
	}
	session := &quickPushSession{
		SessionId:         grand.S(10),
		BotId:             featureCtx.BotId,
		TelegramUserId:    telegramUserId,
		ChatId:            chatId,
		State:             quickPushSessionStateWaiting,
		TenantId:          account.TenantId,
		OperatorAccountId: account.AccountId,
		CreatedAt:         time.Now().Unix(),
	}
	if err = bot.saveQuickPushSession(ctx, session); err != nil {
		return true, err
	}
	text := bot.featureConfigValue(ctx, quickPushFeature{}.Key(), "waitingText")
	if strings.TrimSpace(text) == "" {
		text = "请发送或转发需要快速推送的文本、图片、视频或图文媒体组。"
	}
	row, err := bot.botById(ctx, featureCtx.BotId)
	if err != nil {
		return true, err
	}
	_, err = bot.sendMessageWithMarkup(ctx, row.BotToken, chatId, text, "HTML", false, quickPushEntryKeyboard(session))
	return true, err
}

type quickPushSession struct {
	SessionId             string                                  `json:"sessionId"`
	BotId                 int64                                   `json:"botId"`
	TelegramUserId        string                                  `json:"telegramUserId"`
	ChatId                string                                  `json:"chatId"`
	State                 string                                  `json:"state"`
	TenantId              int64                                   `json:"tenantId"`
	OperatorAccountId     int64                                   `json:"operatorAccountId"`
	SourceMessageRecordId int64                                   `json:"sourceMessageRecordId"`
	Text                  string                                  `json:"text"`
	Media                 []*publishsysin.MessageTemplateMediaInp `json:"media"`
	PlanIds               []int64                                 `json:"planIds"`
	SelectedPlanIds       []int64                                 `json:"selectedPlanIds"`
	SavedTemplateId       int64                                   `json:"savedTemplateId"`
	ButtonConfig          string                                  `json:"buttonConfig"`
	EditingTemplateId     int64                                   `json:"editingTemplateId"`
	CreatedAt             int64                                   `json:"createdAt"`
}

type quickPushSessionMessageHandler struct{}

func (quickPushSessionMessageHandler) Handle(ctx context.Context, bot *sSysBot, event *botMessageEvent) (bool, error) {
	if event == nil || event.Msg == nil || event.Msg.From == nil {
		return false, nil
	}
	telegramUserId := fmt.Sprintf("%d", event.Msg.From.ID)
	session, err := bot.quickPushSession(ctx, event.BotId, telegramUserId)
	if err != nil || session == nil {
		return false, err
	}
	text := quickPushTelegramMessageText(event.Msg, event.Text)
	// Navigation commands and menu labels always leave the current flow first.
	// Without this guard, a stale waiting session consumes every later message.
	if quickPushNavigationText(text) {
		_ = bot.removeQuickPushSession(ctx, event.BotId, telegramUserId)
		return false, nil
	}
	if session.State == quickPushSessionStateEditButtons {
		config, parseErr := parseQuickPushButtons(text)
		if parseErr != nil {
			return true, bot.reply(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), parseErr.Error())
		}
		session.ButtonConfig = config
		session.State = quickPushSessionStateWaiting
		if err = bot.saveQuickPushSession(ctx, session); err != nil {
			return true, err
		}
		return true, bot.reply(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), "按钮设置已保存。")
	}
	if session.State == quickPushSessionStateEditName || session.State == quickPushSessionStateEditText {
		if text == "" {
			return true, bot.reply(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), "请输入有效内容，或点击取消返回模板详情。")
		}
		update := &publishsysin.QuickPushBotTemplateUpdateInp{OperatorAccountId: session.OperatorAccountId, TemplateId: session.EditingTemplateId}
		if session.State == quickPushSessionStateEditName {
			update.Name = text
		} else {
			template, viewErr := publishService.SysPublish().QuickPushBotTemplateView(ctx, session.OperatorAccountId, session.EditingTemplateId)
			if viewErr != nil {
				return true, viewErr
			}
			update.Name = template.Name
			update.Text = text
			update.Media = messageTemplateMediaToInputs(template.Media)
		}
		if err = publishService.SysPublish().QuickPushBotTemplateUpdate(ctx, update); err != nil {
			return true, bot.reply(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), "修改模板失败："+err.Error())
		}
		session.State = quickPushSessionStateWaiting
		_ = bot.saveQuickPushSession(ctx, session)
		return true, bot.sendQuickPushTemplateDetail(ctx, event.BotId, session, session.EditingTemplateId, "模板修改成功。\n\n")
	}
	if session.State != quickPushSessionStateWaiting && session.State != quickPushSessionStateEditMedia {
		return false, nil
	}
	chatId := fmt.Sprintf("%d", event.Msg.Chat.ID)
	row, err := bot.botById(ctx, event.BotId)
	if err != nil {
		return true, err
	}
	media, err := bot.resolveTelegramMessageMedia(ctx, row.BotToken, event.Msg)
	if err != nil {
		return true, err
	}
	sourceMessageRecordId, err := bot.telegramMessageRecordId(ctx, event.BotId, chatId, event.Msg.ID)
	if err != nil {
		return true, err
	}
	for _, item := range media {
		if item == nil {
			continue
		}
		item.SourceMessageRecordId = sourceMessageRecordId
		item.TgFileId = ""
	}
	media, err = bot.persistQuickPushMedia(ctx, session.OperatorAccountId, media)
	if err != nil {
		g.Log().Warningf(ctx, "快速推送Telegram媒体转存失败 botId:%d chatId:%s messageId:%d err:%+v", event.BotId, chatId, event.Msg.ID, err)
		return true, bot.reply(ctx, event.BotId, chatId, "图片或视频下载保存失败，请稍后重新发送。")
	}
	if session.State == quickPushSessionStateEditMedia {
		if len(media) == 0 {
			return true, bot.reply(ctx, event.BotId, chatId, "请发送图片、视频或图文媒体组，或点击取消返回。")
		}
		if strings.TrimSpace(event.Msg.MediaGroupID) != "" {
			return true, bot.reply(ctx, event.BotId, chatId, "修改模板媒体暂不支持分次接收媒体组，请将媒体作为一组重新发送。")
		}
		template, viewErr := publishService.SysPublish().QuickPushBotTemplateView(ctx, session.OperatorAccountId, session.EditingTemplateId)
		if viewErr != nil {
			return true, viewErr
		}
		if err = publishService.SysPublish().QuickPushBotTemplateUpdate(ctx, &publishsysin.QuickPushBotTemplateUpdateInp{OperatorAccountId: session.OperatorAccountId, TemplateId: template.Id, Name: template.Name, Text: template.Text, Media: media, SourceMessageRecordId: sourceMessageRecordId}); err != nil {
			return true, bot.reply(ctx, event.BotId, chatId, "修改模板媒体失败："+err.Error())
		}
		session.State = quickPushSessionStateWaiting
		_ = bot.saveQuickPushSession(ctx, session)
		return true, bot.sendQuickPushTemplateDetail(ctx, event.BotId, session, session.EditingTemplateId, "模板媒体修改成功。\n\n")
	}
	if strings.TrimSpace(event.Msg.MediaGroupID) != "" && len(media) > 0 {
		return true, bot.collectQuickPushMediaGroup(ctx, row.BotToken, session, event.Msg, text, media)
	}
	if text == "" && len(media) == 0 {
		return true, bot.reply(ctx, event.BotId, chatId, "当前支持快速推送文本、图片、视频和图文媒体组，请重新发送。")
	}
	return true, bot.startQuickPushSelection(ctx, row.BotToken, session, chatId, sourceMessageRecordId, text, media)
}

func quickPushTelegramMessageText(msg *models.Message, fallback string) string {
	if msg == nil {
		return strings.TrimSpace(fallback)
	}
	text := msg.Text
	entities := msg.Entities
	if text == "" {
		text = msg.Caption
		entities = msg.CaptionEntities
	}
	if text == "" {
		return strings.TrimSpace(fallback)
	}
	runes := []rune(text)
	spans := make([]telegramHTMLEntitySpan, 0, len(entities))
	for _, entity := range entities {
		openTag, closeTag, ok := telegramEntityHTMLTags(entity)
		if !ok || entity.Length <= 0 {
			continue
		}
		start, startOK := telegramUTF16RuneIndex(runes, entity.Offset)
		end, endOK := telegramUTF16RuneIndex(runes, entity.Offset+entity.Length)
		if !startOK || !endOK || end <= start {
			continue
		}
		spans = append(spans, telegramHTMLEntitySpan{Start: start, End: end, OpenTag: openTag, CloseTag: closeTag, Rank: telegramEntityNestingRank(entity.Type)})
	}
	if len(spans) == 0 {
		return strings.TrimSpace(text)
	}
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].Start != spans[j].Start {
			return spans[i].Start < spans[j].Start
		}
		if spans[i].End != spans[j].End {
			return spans[i].End > spans[j].End
		}
		return spans[i].Rank < spans[j].Rank
	})
	openings := make(map[int][]telegramHTMLEntitySpan)
	closings := make(map[int][]telegramHTMLEntitySpan)
	for order := range spans {
		spans[order].Order = order
		openings[spans[order].Start] = append(openings[spans[order].Start], spans[order])
		closings[spans[order].End] = append(closings[spans[order].End], spans[order])
	}
	var builder strings.Builder
	for index := 0; index <= len(runes); index++ {
		closing := closings[index]
		sort.SliceStable(closing, func(i, j int) bool { return closing[i].Order > closing[j].Order })
		for _, span := range closing {
			builder.WriteString(span.CloseTag)
		}
		for _, span := range openings[index] {
			builder.WriteString(span.OpenTag)
		}
		if index < len(runes) {
			builder.WriteString(html.EscapeString(string(runes[index])))
		}
	}
	return strings.TrimSpace(builder.String())
}

type telegramHTMLEntitySpan struct {
	Start    int
	End      int
	OpenTag  string
	CloseTag string
	Rank     int
	Order    int
}

func telegramEntityHTMLTags(entity models.MessageEntity) (string, string, bool) {
	switch entity.Type {
	case models.MessageEntityTypeBold:
		return "<b>", "</b>", true
	case models.MessageEntityTypeItalic:
		return "<i>", "</i>", true
	case models.MessageEntityTypeUnderline:
		return "<u>", "</u>", true
	case models.MessageEntityTypeStrikethrough:
		return "<s>", "</s>", true
	case models.MessageEntityTypeSpoiler:
		return "<tg-spoiler>", "</tg-spoiler>", true
	case models.MessageEntityTypeBlockquote:
		return "<blockquote>", "</blockquote>", true
	case models.MessageEntityTypeExpandableBlockquote:
		return "<blockquote expandable>", "</blockquote>", true
	case models.MessageEntityTypeCode:
		return "<code>", "</code>", true
	case models.MessageEntityTypePre:
		language := strings.TrimSpace(entity.Language)
		if language == "" {
			return "<pre>", "</pre>", true
		}
		return `<pre><code class="language-` + html.EscapeString(language) + `">`, "</code></pre>", true
	case models.MessageEntityTypeTextLink:
		if strings.TrimSpace(entity.URL) == "" {
			return "", "", false
		}
		return `<a href="` + html.EscapeString(strings.TrimSpace(entity.URL)) + `">`, "</a>", true
	case models.MessageEntityTypeTextMention:
		if entity.User == nil || entity.User.ID <= 0 {
			return "", "", false
		}
		return fmt.Sprintf(`<a href="tg://user?id=%d">`, entity.User.ID), "</a>", true
	case models.MessageEntityTypeCustomEmoji:
		if strings.TrimSpace(entity.CustomEmojiID) == "" {
			return "", "", false
		}
		return `<tg-emoji emoji-id="` + html.EscapeString(strings.TrimSpace(entity.CustomEmojiID)) + `">`, "</tg-emoji>", true
	case models.MessageEntityTypeDateTime:
		if entity.UnixTime <= 0 {
			return "", "", false
		}
		openTag := fmt.Sprintf(`<tg-time unix="%d"`, entity.UnixTime)
		if format := strings.TrimSpace(entity.DateTimeFormat); format != "" {
			openTag += ` format="` + html.EscapeString(format) + `"`
		}
		return openTag + ">", "</tg-time>", true
	default:
		return "", "", false
	}
}

func telegramEntityNestingRank(entityType models.MessageEntityType) int {
	switch entityType {
	case models.MessageEntityTypeBlockquote, models.MessageEntityTypeExpandableBlockquote:
		return 10
	case models.MessageEntityTypeBold, models.MessageEntityTypeItalic, models.MessageEntityTypeUnderline, models.MessageEntityTypeStrikethrough, models.MessageEntityTypeSpoiler:
		return 20
	case models.MessageEntityTypeTextLink, models.MessageEntityTypeTextMention:
		return 30
	case models.MessageEntityTypeCode, models.MessageEntityTypePre:
		return 40
	case models.MessageEntityTypeCustomEmoji, models.MessageEntityTypeDateTime:
		return 50
	default:
		return 100
	}
}

func telegramUTF16RuneIndex(runes []rune, offset int) (int, bool) {
	if offset < 0 {
		return 0, false
	}
	units := 0
	for index, char := range runes {
		if units == offset {
			return index, true
		}
		if char > 0xFFFF {
			units += 2
		} else {
			units++
		}
		if units > offset {
			return 0, false
		}
	}
	return len(runes), units == offset
}

func (s *sSysBot) handleQuickPushCallback(ctx context.Context, botId int64, query *models.CallbackQuery) (bool, error) {
	if query == nil {
		return false, nil
	}
	action, sessionId, planId, ok := parseQuickPushCallbackData(query.Data)
	if !ok {
		return false, nil
	}
	telegramUserId := ""
	if query.From.ID != 0 {
		telegramUserId = fmt.Sprintf("%d", query.From.ID)
	}
	if telegramUserId == "" {
		return true, nil
	}
	session, err := s.quickPushSession(ctx, botId, telegramUserId)
	if err != nil {
		return true, err
	}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return true, err
	}
	tgBot, err := s.telegramBot(ctx, row.BotToken)
	if err != nil {
		return true, err
	}
	callCtx, cancel := telegramAPICtx()
	defer cancel()
	if session == nil || session.SessionId != sessionId {
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "快速推送会话已失效", ShowAlert: false})
		return true, nil
	}
	if handled, callbackErr := s.handleQuickPushTemplateCallback(ctx, callCtx, tgBot, query, session, action, planId); handled {
		if callbackErr != nil {
			text := quickPushCallbackAlertText(callbackErr.Error())
			if text == "" {
				text = "模板操作失败，请稍后重试"
			}
			_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: text, ShowAlert: true})
			g.Log().Errorf(ctx, "快速推送模板回调失败 botId:%d userId:%s action:%s templateId:%d err:%+v", botId, telegramUserId, action, planId, callbackErr)
			return true, nil
		}
		return true, callbackErr
	}
	if session.State != quickPushSessionStateSelecting {
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "当前操作已失效", ShowAlert: false})
		return true, nil
	}
	plans, err := publishService.SysPublish().QuickPushBotPlanList(ctx, session.OperatorAccountId)
	if err != nil {
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: err.Error(), ShowAlert: false})
		return true, nil
	}
	switch action {
	case "toggle":
		session.SelectedPlanIds = toggleQuickPushPlanId(session.SelectedPlanIds, planId)
		_ = s.saveQuickPushSession(ctx, session)
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return true, s.editQuickPushSelection(callCtx, tgBot, query, session, plans)
	case "all":
		session.SelectedPlanIds = append([]int64(nil), session.PlanIds...)
		_ = s.saveQuickPushSession(ctx, session)
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return true, s.editQuickPushSelection(callCtx, tgBot, query, session, plans)
	case "none":
		session.SelectedPlanIds = []int64{}
		_ = s.saveQuickPushSession(ctx, session)
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return true, s.editQuickPushSelection(callCtx, tgBot, query, session, plans)
	case "save":
		if session.SavedTemplateId > 0 {
			_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "当前内容已保存为模板", ShowAlert: false})
			return true, nil
		}
		result, saveErr := publishService.SysPublish().QuickPushSaveTemplateByBot(ctx, &publishsysin.QuickPushBotSaveTemplateInp{
			TenantId:              session.TenantId,
			OperatorAccountId:     session.OperatorAccountId,
			Text:                  session.Text,
			Media:                 session.Media,
			SourceMessageRecordId: session.SourceMessageRecordId,
			ButtonConfig:          session.ButtonConfig,
		})
		if saveErr != nil {
			_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "保存模板失败：" + quickPushCallbackAlertText(saveErr.Error()), ShowAlert: true})
			return true, nil
		}
		session.SavedTemplateId = result.Id
		if err = s.saveQuickPushSession(ctx, session); err != nil {
			return true, err
		}
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "模板已保存，可在上架后台查看", ShowAlert: false})
		return true, s.editQuickPushSelection(callCtx, tgBot, query, session, plans)
	case "cancel":
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "已取消快速推送", ShowAlert: false})
		_ = s.removeQuickPushSession(ctx, botId, telegramUserId)
		return true, s.editQuickPushMessageText(callCtx, tgBot, query, "已取消快速推送。", nil)
	case "send":
		if len(session.SelectedPlanIds) == 0 {
			_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "请至少选择一个计划", ShowAlert: false})
			return true, nil
		}
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "已开始执行快速推送", ShowAlert: false})
		_ = s.editQuickPushMessageText(callCtx, tgBot, query, "快速推送已开始执行，请稍候…", nil)
		res, execErr := publishService.SysPublish().QuickPushExecuteByBot(ctx, &publishsysin.QuickPushBotExecuteInp{TenantId: session.TenantId, OperatorAccountId: session.OperatorAccountId, TemplateId: session.SavedTemplateId, PlanIds: session.SelectedPlanIds, Text: session.Text, Media: session.Media, SourceMessageRecordId: session.SourceMessageRecordId, ButtonConfig: session.ButtonConfig})
		_ = s.removeQuickPushSession(ctx, botId, telegramUserId)
		if execErr != nil {
			return true, s.editQuickPushMessageText(callCtx, tgBot, query, "快速推送执行失败："+html.EscapeString(execErr.Error()), nil)
		}
		text := fmt.Sprintf("快速推送任务已提交完成。\n总数：%d\n成功入队：%d\n失败：%d", res.Total, res.Success, res.Failed)
		if err = s.editQuickPushMessageText(callCtx, tgBot, query, text, nil); err != nil {
			return true, err
		}
		_, err = s.sendMessage(ctx, row.BotToken, session.ChatId, "快速推送完成。", "HTML", false)
		return true, err
	}
	return true, nil
}

type quickPushPendingMediaGroup struct {
	SessionId             string                                  `json:"sessionId"`
	BotId                 int64                                   `json:"botId"`
	TelegramUserId        string                                  `json:"telegramUserId"`
	ChatId                string                                  `json:"chatId"`
	SourceMessageRecordId int64                                   `json:"sourceMessageRecordId"`
	Text                  string                                  `json:"text"`
	Media                 []*publishsysin.MessageTemplateMediaInp `json:"media"`
	CreatedAt             int64                                   `json:"createdAt"`
}

func (s *sSysBot) startQuickPushSelection(ctx context.Context, botToken string, session *quickPushSession, chatId string, sourceMessageRecordId int64, text string, media []*publishsysin.MessageTemplateMediaInp) error {
	plans, err := publishService.SysPublish().QuickPushBotPlanList(ctx, session.OperatorAccountId)
	if err != nil {
		_ = s.removeQuickPushSession(ctx, session.BotId, session.TelegramUserId)
		return err
	}
	if len(plans) == 0 {
		_ = s.removeQuickPushSession(ctx, session.BotId, session.TelegramUserId)
		return s.reply(ctx, session.BotId, chatId, "暂无已启用的快速推送计划，请先在上架后台创建。")
	}
	session.State = quickPushSessionStateSelecting
	session.SourceMessageRecordId = sourceMessageRecordId
	session.Text = strings.TrimSpace(text)
	session.Media = media
	session.PlanIds = quickPushPlanIds(plans)
	session.SelectedPlanIds = append([]int64(nil), session.PlanIds...)
	if err = s.saveQuickPushSession(ctx, session); err != nil {
		return err
	}
	_, err = s.sendMessageWithMarkup(ctx, botToken, chatId, quickPushSelectionText(session, plans), "HTML", false, quickPushPlanKeyboard(session, plans))
	return err
}

func (s *sSysBot) collectQuickPushMediaGroup(ctx context.Context, botToken string, session *quickPushSession, msg *models.Message, text string, media []*publishsysin.MessageTemplateMediaInp) error {
	groupId := strings.TrimSpace(msg.MediaGroupID)
	if groupId == "" || len(media) == 0 {
		return nil
	}
	key := quickPushMediaGroupKey(session.BotId, session.TelegramUserId, groupId)
	value, _ := cache.Instance().Get(ctx, key)
	pending := &quickPushPendingMediaGroup{}
	isFirst := true
	if value != nil && !value.IsNil() && strings.TrimSpace(value.String()) != "" {
		if err := json.Unmarshal([]byte(value.String()), pending); err == nil && pending.SessionId == session.SessionId {
			isFirst = false
		}
	}
	if isFirst {
		pending = &quickPushPendingMediaGroup{SessionId: session.SessionId, BotId: session.BotId, TelegramUserId: session.TelegramUserId, ChatId: session.ChatId, CreatedAt: time.Now().Unix()}
	}
	if strings.TrimSpace(pending.Text) == "" {
		pending.Text = strings.TrimSpace(text)
		if pending.Text != "" && len(media) > 0 && media[0] != nil {
			pending.SourceMessageRecordId = media[0].SourceMessageRecordId
		}
	}
	if pending.SourceMessageRecordId <= 0 && len(media) > 0 && media[0] != nil {
		pending.SourceMessageRecordId = media[0].SourceMessageRecordId
	}
	for _, item := range media {
		if item == nil {
			continue
		}
		item.SortIndex = len(pending.Media) + 1
		pending.Media = append(pending.Media, item)
	}
	bs, err := json.Marshal(pending)
	if err != nil {
		return gerror.Wrap(err, "保存快速推送媒体组失败")
	}
	if err = cache.Instance().Set(ctx, key, string(bs), 2*time.Minute); err != nil {
		return err
	}
	if isFirst {
		go s.finishQuickPushMediaGroup(botToken, session.BotId, session.TelegramUserId, groupId)
	}
	return nil
}

func (s *sSysBot) finishQuickPushMediaGroup(botToken string, botId int64, telegramUserId string, groupId string) {
	time.Sleep(1500 * time.Millisecond)
	ctx := context.Background()
	key := quickPushMediaGroupKey(botId, telegramUserId, groupId)
	value, err := cache.Instance().Get(ctx, key)
	if err != nil || value == nil || value.IsNil() {
		return
	}
	var pending quickPushPendingMediaGroup
	if err = json.Unmarshal([]byte(value.String()), &pending); err != nil || pending.SessionId == "" {
		return
	}
	_, _ = cache.Instance().Remove(ctx, key)
	session, err := s.quickPushSession(ctx, botId, telegramUserId)
	if err != nil || session == nil || session.SessionId != pending.SessionId || session.State != quickPushSessionStateWaiting {
		return
	}
	if len(pending.Media) == 0 && strings.TrimSpace(pending.Text) == "" {
		return
	}
	if err = s.startQuickPushSelection(ctx, botToken, session, pending.ChatId, pending.SourceMessageRecordId, pending.Text, pending.Media); err != nil {
		_ = s.reply(ctx, botId, pending.ChatId, "快速推送媒体组解析失败："+html.EscapeString(err.Error()))
	}
}

func quickPushMediaGroupKey(botId int64, telegramUserId string, groupId string) string {
	return fmt.Sprintf("youban_bot:quick_push:media_group:%d:%s:%s", botId, telegramUserId, groupId)
}

func (s *sSysBot) editQuickPushSelection(callCtx context.Context, tgBot *tgbot.Bot, query *models.CallbackQuery, session *quickPushSession, plans []*publishsysin.QuickPushPlanModel) error {
	return s.editQuickPushMessageText(callCtx, tgBot, query, quickPushSelectionText(session, plans), quickPushPlanKeyboard(session, plans))
}

func (s *sSysBot) editQuickPushMessageText(callCtx context.Context, tgBot *tgbot.Bot, query *models.CallbackQuery, text string, markup models.ReplyMarkup) error {
	if query == nil || query.Message.Message == nil {
		return nil
	}
	_, err := tgBot.EditMessageText(callCtx, &tgbot.EditMessageTextParams{ChatID: fmt.Sprintf("%d", query.Message.Message.Chat.ID), MessageID: query.Message.Message.ID, Text: text, ParseMode: models.ParseModeHTML, ReplyMarkup: markup})
	return err
}

func (s *sSysBot) quickPushBoundAccount(ctx context.Context, telegramUserId string) (*botBindRow, *publishsysin.QuickPushBotAccountModel, error) {
	bind, err := s.bindingByTelegram(ctx, sysin.BotAppApi, telegramUserId)
	if err != nil || bind == nil || bind.AccountId <= 0 {
		return bind, nil, err
	}
	account, err := publishService.SysPublish().QuickPushBotAccount(ctx, bind.AccountId)
	if err != nil {
		return bind, nil, nil
	}
	return bind, account, nil
}

func (s *sSysBot) quickPushSession(ctx context.Context, botId int64, telegramUserId string) (*quickPushSession, error) {
	value, err := cache.Instance().Get(ctx, quickPushSessionKey(botId, telegramUserId))
	if err != nil || value == nil || value.IsNil() {
		return nil, nil
	}
	var session quickPushSession
	if err = json.Unmarshal([]byte(value.String()), &session); err != nil {
		return nil, gerror.Wrap(err, "读取快速推送会话失败")
	}
	return &session, nil
}

func (s *sSysBot) saveQuickPushSession(ctx context.Context, session *quickPushSession) error {
	if session == nil {
		return gerror.New("快速推送会话不能为空")
	}
	bs, err := json.Marshal(session)
	if err != nil {
		return gerror.Wrap(err, "保存快速推送会话失败")
	}
	return cache.Instance().Set(ctx, quickPushSessionKey(session.BotId, session.TelegramUserId), string(bs), quickPushSessionTTL)
}

func (s *sSysBot) removeQuickPushSession(ctx context.Context, botId int64, telegramUserId string) error {
	_, err := cache.Instance().Remove(ctx, quickPushSessionKey(botId, telegramUserId))
	return err
}

func quickPushSessionKey(botId int64, telegramUserId string) string {
	return fmt.Sprintf("youban_bot:quick_push:session:%d:%s", botId, strings.TrimSpace(telegramUserId))
}

func quickPushNavigationText(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if strings.HasPrefix(text, "/") {
		return true
	}
	for _, label := range []string{
		"开始使用", "联系客服", "注册邀请", "扫图搜索", "实时汇率", "快速推送",
	} {
		if text == label {
			return true
		}
	}
	return false
}

func quickPushPlanIds(plans []*publishsysin.QuickPushPlanModel) []int64 {
	ids := make([]int64, 0, len(plans))
	for _, item := range plans {
		if item != nil && item.Id > 0 {
			ids = append(ids, item.Id)
		}
	}
	return ids
}

func quickPushSelectionText(session *quickPushSession, plans []*publishsysin.QuickPushPlanModel) string {
	selected := map[int64]struct{}{}
	if session != nil {
		for _, id := range session.SelectedPlanIds {
			selected[id] = struct{}{}
		}
	}
	lines := []string{"请选择需要执行的快速推送计划：", ""}
	for _, item := range plans {
		if item == nil {
			continue
		}
		mark := "☑️"
		if _, ok := selected[item.Id]; !ok {
			mark = "⬜️"
		}
		lines = append(lines, fmt.Sprintf("%s %s", mark, html.EscapeString(quickPushPlanDisplayName(item))))
	}
	lines = append(lines, "", "默认已全选，点击计划按钮可取消选择。")
	return strings.Join(lines, "\n")
}

func quickPushPlanKeyboard(session *quickPushSession, plans []*publishsysin.QuickPushPlanModel) *models.InlineKeyboardMarkup {
	selected := map[int64]struct{}{}
	if session != nil {
		for _, id := range session.SelectedPlanIds {
			selected[id] = struct{}{}
		}
	}
	rows := make([][]models.InlineKeyboardButton, 0)
	row := make([]models.InlineKeyboardButton, 0, 2)
	for _, item := range plans {
		if item == nil {
			continue
		}
		prefix := "✅"
		if _, ok := selected[item.Id]; !ok {
			prefix = "⬜"
		}
		label := prefix + quickPushPlanDisplayName(item)
		row = append(row, models.InlineKeyboardButton{Text: label, CallbackData: quickPushCallbackData("toggle", session.SessionId, item.Id)})
		if len(row) == 2 {
			rows = append(rows, row)
			row = make([]models.InlineKeyboardButton, 0, 2)
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "全选", CallbackData: quickPushCallbackData("all", session.SessionId, 0)}, {Text: "取消全选", CallbackData: quickPushCallbackData("none", session.SessionId, 0)}})
	saveLabel := "保存模板"
	if session.SavedTemplateId > 0 {
		saveLabel = "✅ 模板已保存"
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: saveLabel, CallbackData: quickPushCallbackData("save", session.SessionId, 0)}})
	rows = append(rows, []models.InlineKeyboardButton{{Text: "返回", CallbackData: quickPushCallbackData("cancel", session.SessionId, 0)}, {Text: "发送", CallbackData: quickPushCallbackData("send", session.SessionId, 0)}})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func quickPushEntryKeyboard(session *quickPushSession) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		{Text: "取消", CallbackData: quickPushCallbackData("cancel", session.SessionId, 0)},
		{Text: "查看已有模板", CallbackData: quickPushCallbackData("templates", session.SessionId, 0)},
		{Text: "设置按钮", CallbackData: quickPushCallbackData("buttons", session.SessionId, 0)},
	}}}
}

func parseQuickPushButtons(text string) (string, error) {
	if strings.TrimSpace(text) == "/clear" {
		return "", nil
	}
	mode := "inline"
	rows := make([][]publishsysin.MessageTemplateButton, 0)
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		row := make([]publishsysin.MessageTemplateButton, 0)
		for _, raw := range strings.Split(line, "&&") {
			item := strings.TrimSpace(raw)
			if item == "" {
				continue
			}
			button := publishsysin.MessageTemplateButton{Text: item}
			if dash := strings.Index(item, "-"); dash > 0 {
				button.Text = strings.TrimSpace(item[:dash])
				button.URL = strings.TrimSpace(item[dash+1:])
			} else {
				return "", gerror.New("消息按钮必须使用：按钮名称-跳转链接")
			}
			if strings.HasPrefix(button.Text, "#p") || strings.HasPrefix(button.Text, "#r") || strings.HasPrefix(button.Text, "#g") {
				button.Color = button.Text[:2]
				button.Text = strings.TrimSpace(button.Text[2:])
			}
			if button.Text == "" || (mode == "inline" && button.URL == "") {
				return "", gerror.New("按钮格式不正确，请使用 名称-链接 或直接填写底部按钮名称")
			}
			row = append(row, button)
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return "", gerror.New("按钮配置不能为空")
	}
	data, _ := json.Marshal(publishsysin.MessageTemplateButtonConfig{Mode: mode, Rows: rows})
	return string(data), nil
}

func (s *sSysBot) handleQuickPushTemplateCallback(ctx context.Context, callCtx context.Context, tgBot *tgbot.Bot, query *models.CallbackQuery, session *quickPushSession, action string, templateId int64) (bool, error) {
	switch action {
	case "templates", "backlist":
		list, err := publishService.SysPublish().QuickPushBotTemplateList(ctx, session.OperatorAccountId)
		if err != nil {
			return true, err
		}
		session.State = quickPushSessionStateWaiting
		session.EditingTemplateId = 0
		_ = s.saveQuickPushSession(ctx, session)
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return true, s.editQuickPushMessageText(callCtx, tgBot, query, quickPushTemplateListText(list), quickPushTemplateListKeyboard(session, list))
	case "view":
		template, err := publishService.SysPublish().QuickPushBotTemplateView(ctx, session.OperatorAccountId, templateId)
		if err != nil {
			return true, err
		}
		session.EditingTemplateId = template.Id
		session.State = quickPushSessionStateWaiting
		_ = s.saveQuickPushSession(ctx, session)
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return true, s.editQuickPushMessageText(callCtx, tgBot, query, quickPushTemplateDetailText(template), quickPushTemplateDetailKeyboard(session, template.Id, false))
	case "editname", "edittext", "editmedia":
		state := quickPushSessionStateEditName
		prompt := "请发送新的模板名称。"
		if action == "edittext" {
			state, prompt = quickPushSessionStateEditText, "请发送新的模板文本。"
		} else if action == "editmedia" {
			state, prompt = quickPushSessionStateEditMedia, "请发送新的图片或视频，发送后将整体替换原模板媒体。"
		}
		session.State = state
		session.EditingTemplateId = templateId
		_ = s.saveQuickPushSession(ctx, session)
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return true, s.editQuickPushMessageText(callCtx, tgBot, query, prompt, quickPushEditCancelKeyboard(session, templateId))
	case "delete":
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return true, s.editQuickPushMessageText(callCtx, tgBot, query, "确定删除当前模板吗？删除后不可恢复。", quickPushTemplateDetailKeyboard(session, templateId, true))
	case "confirmdel":
		if err := publishService.SysPublish().QuickPushBotTemplateDelete(ctx, session.OperatorAccountId, templateId); err != nil {
			return true, err
		}
		list, err := publishService.SysPublish().QuickPushBotTemplateList(ctx, session.OperatorAccountId)
		if err != nil {
			return true, err
		}
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "模板已删除"})
		return true, s.editQuickPushMessageText(callCtx, tgBot, query, quickPushTemplateListText(list), quickPushTemplateListKeyboard(session, list))
	case "use":
		template, err := publishService.SysPublish().QuickPushBotTemplateView(ctx, session.OperatorAccountId, templateId)
		if err != nil {
			return true, err
		}
		plans, err := publishService.SysPublish().QuickPushBotPlanList(ctx, session.OperatorAccountId)
		if err != nil {
			return true, err
		}
		if len(plans) == 0 {
			_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "暂无已启用的快速推送计划", ShowAlert: true})
			return true, nil
		}
		session.State = quickPushSessionStateSelecting
		session.SavedTemplateId = template.Id
		session.Text = template.Text
		session.Media = messageTemplateMediaToInputs(template.Media)
		session.SourceMessageRecordId = template.SourceMessageRecordId
		session.ButtonConfig = template.ButtonConfig
		session.PlanIds = quickPushPlanIds(plans)
		session.SelectedPlanIds = append([]int64(nil), session.PlanIds...)
		_ = s.saveQuickPushSession(ctx, session)
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return true, s.editQuickPushMessageText(callCtx, tgBot, query, quickPushSelectionText(session, plans), quickPushPlanKeyboard(session, plans))
	case "buttons":
		session.State = quickPushSessionStateEditButtons
		_ = s.saveQuickPushSession(ctx, session)
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
		return true, s.editQuickPushMessageText(callCtx, tgBot, query, "请发送按钮配置：每行一行按钮，使用 && 分隔同一行按钮。消息按钮格式：名称-链接；底部键盘直接填写按钮名称。发送 /clear 清除按钮。", quickPushEditCancelKeyboard(session, 0))
	case "cancel":
		if session.State == quickPushSessionStateSelecting {
			return false, nil
		}
		_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "已取消快速推送"})
		_ = s.removeQuickPushSession(ctx, session.BotId, session.TelegramUserId)
		return true, s.editQuickPushMessageText(callCtx, tgBot, query, "已取消快速推送。", nil)
	}
	return false, nil
}

func quickPushTemplateListText(list []*publishsysin.MessageTemplateModel) string {
	if len(list) == 0 {
		return "<b>已有模板</b>\n\n暂无可用模板。"
	}
	return fmt.Sprintf("<b>已有模板</b>\n\n共显示最近 %d 个模板，点击模板名称查看和修改。", len(list))
}

func quickPushTemplateListKeyboard(session *quickPushSession, list []*publishsysin.MessageTemplateModel) *models.InlineKeyboardMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(list)+1)
	for _, item := range list {
		if item != nil {
			rows = append(rows, []models.InlineKeyboardButton{{Text: item.Name, CallbackData: quickPushCallbackData("view", session.SessionId, item.Id)}})
		}
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "返回", CallbackData: quickPushCallbackData("cancel", session.SessionId, 0)}})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func quickPushTemplateDetailText(template *publishsysin.MessageTemplateModel) string {
	text := strings.TrimSpace(template.Text)
	if text == "" {
		text = "无"
	}
	return fmt.Sprintf("<b>%s</b>\n\n<b>文本</b>\n%s\n\n<b>媒体</b>：%d 个", html.EscapeString(template.Name), text, len(template.Media))
}

func quickPushTemplateDetailKeyboard(session *quickPushSession, templateId int64, confirmingDelete bool) *models.InlineKeyboardMarkup {
	if confirmingDelete {
		return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "确认删除", CallbackData: quickPushCallbackData("confirmdel", session.SessionId, templateId)}}, {{Text: "取消", CallbackData: quickPushCallbackData("view", session.SessionId, templateId)}}}}
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "修改名称", CallbackData: quickPushCallbackData("editname", session.SessionId, templateId)}, {Text: "修改文本", CallbackData: quickPushCallbackData("edittext", session.SessionId, templateId)}},
		{{Text: "修改媒体", CallbackData: quickPushCallbackData("editmedia", session.SessionId, templateId)}, {Text: "删除", CallbackData: quickPushCallbackData("delete", session.SessionId, templateId)}},
		{{Text: "返回模板列表", CallbackData: quickPushCallbackData("backlist", session.SessionId, 0)}, {Text: "快速发送", CallbackData: quickPushCallbackData("use", session.SessionId, templateId)}},
	}}
}

func quickPushEditCancelKeyboard(session *quickPushSession, templateId int64) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "取消修改", CallbackData: quickPushCallbackData("view", session.SessionId, templateId)}}}}
}

func messageTemplateMediaToInputs(media []*publishsysin.MessageTemplateMediaModel) []*publishsysin.MessageTemplateMediaInp {
	items := make([]*publishsysin.MessageTemplateMediaInp, 0, len(media))
	for _, item := range media {
		if item != nil {
			items = append(items, &publishsysin.MessageTemplateMediaInp{Id: item.Id, SourceMessageRecordId: item.SourceMessageRecordId, MediaType: item.MediaType, Name: item.Name, FileUrl: item.FileUrl, StoragePath: item.StoragePath, PosterUrl: item.PosterUrl, PosterStoragePath: item.PosterStoragePath, TgFileId: item.TgFileId, TgThumbFileId: item.TgThumbFileId, AssetHash: item.AssetHash, SortIndex: item.SortIndex})
		}
	}
	return items
}

func (s *sSysBot) sendQuickPushTemplateDetail(ctx context.Context, botId int64, session *quickPushSession, templateId int64, prefix string) error {
	template, err := publishService.SysPublish().QuickPushBotTemplateView(ctx, session.OperatorAccountId, templateId)
	if err != nil {
		return err
	}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, session.ChatId, prefix+quickPushTemplateDetailText(template), "HTML", false, quickPushTemplateDetailKeyboard(session, template.Id, false))
	return err
}

func quickPushCallbackAlertText(text string) string {
	const maxRunes = 150
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "…"
}

func quickPushPlanDisplayName(item *publishsysin.QuickPushPlanModel) string {
	if item == nil {
		return ""
	}
	name := strings.TrimSpace(item.Name)
	if name != "" {
		return name
	}
	return strings.TrimSpace(item.SerialNo)
}

func quickPushCallbackData(action string, sessionId string, planId int64) string {
	return fmt.Sprintf("%s|%s|%s|%d", quickPushCallbackPrefix, action, sessionId, planId)
}

func parseQuickPushCallbackData(data string) (action string, sessionId string, planId int64, ok bool) {
	parts := strings.Split(strings.TrimSpace(data), "|")
	if len(parts) != 4 || parts[0] != quickPushCallbackPrefix {
		return "", "", 0, false
	}
	planId, _ = strconv.ParseInt(parts[3], 10, 64)
	return parts[1], parts[2], planId, true
}

func toggleQuickPushPlanId(ids []int64, id int64) []int64 {
	if id <= 0 {
		return ids
	}
	out := make([]int64, 0, len(ids)+1)
	removed := false
	for _, item := range ids {
		if item == id {
			removed = true
			continue
		}
		out = append(out, item)
	}
	if !removed {
		out = append(out, id)
	}
	return out
}
