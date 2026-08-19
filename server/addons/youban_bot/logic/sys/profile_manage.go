package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/grand"

	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
	"hotgo/internal/library/cache"
	hglock "hotgo/internal/library/hgrds/lock"
)

const (
	profileSessionTable     = "hg_youban_bot_profile_session"
	profileInlineShareTable = "hg_youban_bot_inline_share"

	profileSessionStatusActive    = "active"
	profileSessionStatusCanceled  = "canceled"
	profileSessionStatusCompleted = "completed"

	telegramTextMessageMaxChars = 4096
	profileCreateMaxMediaBytes  = 100 * 1024 * 1024
	profileMediaGroupDebounce   = 2 * time.Second
	profileSessionTimeout       = 6 * time.Minute
	profileMediaGroupCacheTTL   = 10 * time.Minute
	templateInlinePhotoCacheTTL = 24 * time.Hour
)

var (
	profileNoFindRegexp   = regexp.MustCompile(`(?i)\b[A-Z][A-Z0-9]{4,}\b`)
	profileMarkFindRegexp = regexp.MustCompile(`^\S{0,32}[0-9]{3}$`)
)

type botProfileAccount struct {
	TenantId    int64
	AccountId   int64
	AccountType string
	App         string
}

type profileManageMessageHandler struct{}

func (profileManageMessageHandler) Handle(ctx context.Context, bot *sSysBot, event *botMessageEvent) (bool, error) {
	if event == nil || event.Msg == nil || event.Msg.From == nil {
		return false, nil
	}
	text := strings.TrimSpace(firstNonEmpty(event.Msg.Text, event.Msg.Caption, event.Text))
	if botListenerBindCode(text) != "" {
		return false, nil
	}
	if isCancelProfileCommand(text) {
		return true, bot.cancelProfileSession(ctx, event.BotId, event.Msg)
	}
	if session := bot.activeProfileSession(ctx, event.BotId, event.Msg); session != nil {
		g.Log().Infof(ctx, "Bot资料事件命中会话 trace:PF-%d bot_id:%d message_id:%d media_group_id:%s scene:%s step:%s text_len:%d photo_count:%d has_video:%t", session.Id, event.BotId, event.Msg.ID, strings.TrimSpace(event.Msg.MediaGroupID), session.Scene, session.Step, len(strings.TrimSpace(text)), len(event.Msg.Photo), event.Msg.Video != nil)
		account := &botProfileAccount{TenantId: session.TenantId, AccountId: session.AccountId, AccountType: session.AccountType, App: session.App}
		if session.Scene == "create" || session.Scene == "replace" {
			return true, bot.consumeProfileSessionMessage(ctx, event.BotId, event.Msg, account, session, text)
		}
		if session.Scene == "search" && len(event.Msg.Photo) > 0 {
			return true, bot.consumeProfileSearchImageMessage(ctx, event.BotId, event.Msg, account, session)
		}
		if text != "" {
			return true, bot.consumeProfileSessionText(ctx, event.BotId, event.Msg, account, session, text, extractProfileNos(text))
		}
	}
	if isTelegramSearchChat(event.Msg) && text != "" {
		if !bot.isProfileSearchTrigger(ctx, text) {
			return false, nil
		}
		account, err := bot.boundProfileAccount(ctx, event.Msg)
		if err != nil {
			return true, bot.reply(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), err.Error())
		}
		chatID := fmt.Sprintf("%d", event.Msg.Chat.ID)
		if nos := extractProfileNos(text); len(nos) == 1 && strings.EqualFold(strings.TrimSpace(text), nos[0]) {
			return true, bot.searchProfilesAndReply(ctx, event.BotId, chatID, account, nos[0], "view")
		}
		return true, bot.searchProfilesAndReply(ctx, event.BotId, chatID, account, text, "view")
	}
	if text == "" || !looksLikeProfileCommand(text) {
		return false, nil
	}
	return bot.handleProfileTextCommand(ctx, event.BotId, event.Msg, text)
}

func isTelegramSearchChat(msg *models.Message) bool {
	if msg == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(string(msg.Chat.Type))) {
	case "group", "supergroup", "channel":
		return true
	default:
		return false
	}
}

func (s *sSysBot) isProfileSearchTrigger(ctx context.Context, text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if botListenerBindCode(text) != "" {
		return false
	}
	if looksLikeProfileSearchIdentifier(text) {
		return true
	}
	command, args := botCommandAndArgs(text)
	if command == "" {
		return false
	}
	profile := profileFeature{}
	if !s.featureCommandMatches(ctx, profile, command) {
		return false
	}
	return strings.TrimSpace(args) != ""
}

func isCancelProfileCommand(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	return text == "/cancel" || text == "取消" || strings.Contains(text, "取消当前")
}

func looksLikeProfileCommand(text string) bool {
	if botListenerBindCode(text) != "" {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(text))
	if strings.HasPrefix(lower, "/send") || strings.HasPrefix(lower, "/note") || strings.HasPrefix(lower, "/profile") {
		return true
	}
	for _, kw := range []string{"上架", "下架", "发送", "预览", "搜索", "编辑", "新建", "笔记", "资料"} {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return looksLikeProfileSearchIdentifier(text)
}

func looksLikeProfileSearchIdentifier(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	normalized := strings.TrimSpace(strings.NewReplacer("资料编号", "", "编号", "", "：", "", ":", "", "=", "").Replace(text))
	if nos := extractProfileNos(normalized); len(nos) == 1 && strings.EqualFold(normalized, nos[0]) {
		return true
	}
	return profileMarkFindRegexp.MatchString(normalized)
}

func (s *sSysBot) profileMenuMarkup() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "笔记列表", CallbackData: "pf:list:1"}, {Text: "新建笔记", CallbackData: "pf:create"}},
		{{Text: "搜索笔记", CallbackData: "pf:search"}, {Text: "发送笔记", CallbackData: "pf:asksend"}},
		{{Text: "上架笔记", CallbackData: "pf:askup"}, {Text: "下架笔记", CallbackData: "pf:askdown"}},
		{{Text: "编辑笔记", CallbackData: "pf:askedit"}, {Text: "频道管理", CallbackData: "ch:list"}},
		{{Text: "取消当前操作", CallbackData: "pf:cancel"}},
	}}
}

func (s *sSysBot) showProfileMenu(ctx context.Context, botId int64, msg *models.Message) error {
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, fmt.Sprintf("%d", msg.Chat.ID), "资料管理已启用，请选择操作：", "HTML", false, s.profileMenuMarkup())
	return err
}

func (s *sSysBot) showProfileMenuToChat(ctx context.Context, botId int64, chatId string, text string) error {
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		text = "资料管理已启用，请选择操作："
	}
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, text, "HTML", false, s.profileMenuMarkup())
	return err
}

func (s *sSysBot) handleProfileTextCommand(ctx context.Context, botId int64, msg *models.Message, text string) (bool, error) {
	account, err := s.boundProfileAccount(ctx, msg)
	if err != nil {
		return true, s.reply(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), err.Error())
	}
	lower := strings.ToLower(strings.TrimSpace(text))
	nos := extractProfileNos(text)
	chatId := fmt.Sprintf("%d", msg.Chat.ID)
	if session := s.activeProfileSession(ctx, botId, msg); session != nil && session.Scene != "create" {
		return true, s.consumeProfileSessionText(ctx, botId, msg, account, session, text, nos)
	}
	if strings.Contains(text, "新建") {
		if err = s.startProfileSession(ctx, botId, msg, "create", "waiting_display", 0, "", nil, ""); err != nil {
			return true, err
		}
		return true, s.sendProfileCreateStepPrompt(ctx, botId, chatId, "waiting_display")
	}
	if strings.Contains(text, "上架") {
		if len(nos) == 0 {
			kw := profileActionKeyword(text, "上架")
			if kw != "" {
				return true, s.searchProfilesAndReply(ctx, botId, chatId, account, kw, "up")
			}
			return true, s.startProfileSession(ctx, botId, msg, "up", "waiting_profile_no", 0, "", nil, "请输入要上架的笔记编号、标题或正文关键词，例如：A00001")
		}
		return true, s.changeProfilesStatus(ctx, botId, chatId, account, nos, 1)
	}
	if strings.Contains(text, "下架") {
		if len(nos) == 0 {
			kw := profileActionKeyword(text, "下架")
			if kw != "" {
				return true, s.searchProfilesAndReply(ctx, botId, chatId, account, kw, "down")
			}
			return true, s.startProfileSession(ctx, botId, msg, "down", "waiting_profile_no", 0, "", nil, "请输入要下架的笔记编号或标题关键词。")
		}
		return true, s.changeProfilesStatus(ctx, botId, chatId, account, nos, 2)
	}
	if strings.Contains(text, "编辑") {
		if len(nos) == 0 {
			kw := profileActionKeyword(text, "编辑")
			if kw != "" {
				return true, s.searchProfilesAndReply(ctx, botId, chatId, account, kw, "edit")
			}
			return true, s.startProfileSession(ctx, botId, msg, "edit", "waiting_profile_no", 0, "", nil, "请输入要编辑的笔记编号、标题或正文关键词。")
		}
		return true, s.showProfileEditMenu(ctx, botId, chatId, account, nos[0])
	}
	if strings.Contains(text, "取消队列") || strings.Contains(text, "取消推送") {
		return true, s.showChannelList(ctx, botId, chatId, account, "")
	}
	if strings.Contains(text, "频道") || strings.Contains(text, "循环") {
		return true, s.handleChannelTextCommand(ctx, botId, msg, account, text)
	}
	if strings.Contains(text, "发送") || strings.Contains(text, "预览") || strings.HasPrefix(lower, "/send") {
		if len(nos) == 0 {
			kw := profileActionKeyword(text, "发送", "预览", "/send")
			if kw != "" {
				return true, s.searchProfilesAndReply(ctx, botId, chatId, account, kw, "send")
			}
			return true, s.startProfileSession(ctx, botId, msg, "send", "waiting_profile_no", 0, "", nil, "请输入要发送的笔记编号、标题或正文关键词，例如：A00001")
		}
		for _, no := range nos {
			if err := s.sendProfileByNo(ctx, botId, chatId, account, no); err != nil {
				return true, err
			}
		}
		return true, nil
	}
	if strings.Contains(text, "搜索") {
		kw := strings.TrimSpace(strings.NewReplacer("搜索", "", "/note", "", "/profile", "").Replace(text))
		if kw == "" {
			return true, s.startProfileSession(ctx, botId, msg, "search", "waiting_keyword", 0, "", nil, "请输入笔记编号、标题、正文关键词，或直接发送图片进行相似搜索。")
		}
		return true, s.searchProfilesAndReply(ctx, botId, chatId, account, kw, "view")
	}
	if len(nos) > 0 {
		return true, s.searchProfilesAndReply(ctx, botId, chatId, account, nos[0], "view")
	}
	if looksLikeProfileSearchIdentifier(text) {
		return true, s.searchProfilesAndReply(ctx, botId, chatId, account, text, "view")
	}
	return false, nil
}

func (s *sSysBot) handleProfileCallback(ctx context.Context, botId int64, query *models.CallbackQuery) (bool, error) {
	if query == nil || (!strings.HasPrefix(query.Data, "pf:") && !strings.HasPrefix(query.Data, "ch:")) {
		return false, nil
	}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return true, err
	}
	bot, err := s.telegramBot(ctx, row.BotToken)
	if err != nil {
		return true, err
	}
	callCtx, cancel := telegramAPICtx()
	defer cancel()
	_, _ = bot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
	msg := query.Message.Message
	if msg == nil {
		return true, nil
	}
	chatId := fmt.Sprintf("%d", msg.Chat.ID)
	account, accErr := s.boundProfileAccountByUser(ctx, query.From.ID)
	if accErr != nil {
		return true, s.replyBotError(ctx, botId, chatId, "资料管理账号绑定", accErr)
	}
	if msg.Chat.Type != "private" && account.AccountType != "admin" {
		_, _ = bot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "无权限",
			ShowAlert:       false,
		})
		return true, nil
	}
	parts := strings.Split(query.Data, ":")
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	no := ""
	if len(parts) > 2 {
		no = strings.ToUpper(parts[2])
	}
	if strings.HasPrefix(query.Data, "ch:") {
		return s.handleChannelCallback(ctx, botId, chatId, fmt.Sprintf("%d", query.From.ID), account, query.Data, query)
	}
	switch action {
	case "list":
		page := parseInt(no)
		if page <= 0 {
			page = 1
		}
		return true, s.showProfileList(ctx, botId, chatId, account, page)
	case "askpage":
		return true, s.startProfileSessionByIds(ctx, botId, fmt.Sprintf("%d", query.From.ID), chatId, account, "note_page", "waiting_page", 0, "", nil, "请输入要跳转的页码，例如：3")
	case "menu":
		_ = s.cancelProfileSessionByIdsSilent(ctx, botId, fmt.Sprintf("%d", query.From.ID), chatId)
		return true, s.showProfileMenuToChat(ctx, botId, chatId, "已返回资料管理，请选择操作：")
	case "create":
		if err = s.startProfileSessionByIds(ctx, botId, fmt.Sprintf("%d", query.From.ID), chatId, account, "create", "waiting_display", 0, "", nil, ""); err != nil {
			return true, err
		}
		return true, s.sendProfileCreateStepPrompt(ctx, botId, chatId, "waiting_display")
	case "search":
		return true, s.startProfileSessionByIds(ctx, botId, fmt.Sprintf("%d", query.From.ID), chatId, account, "search", "waiting_keyword", 0, "", nil, "请输入笔记编号、标题、正文关键词，或直接发送图片进行相似搜索。")
	case "askup":
		return true, s.startProfileSessionByIds(ctx, botId, fmt.Sprintf("%d", query.From.ID), chatId, account, "up", "waiting_profile_no", 0, "", nil, "请输入要上架的笔记编号、标题或正文关键词，例如：A00001")
	case "askdown":
		return true, s.startProfileSessionByIds(ctx, botId, fmt.Sprintf("%d", query.From.ID), chatId, account, "down", "waiting_profile_no", 0, "", nil, "请输入要下架的笔记编号或标题关键词。")
	case "asksend":
		return true, s.startProfileSessionByIds(ctx, botId, fmt.Sprintf("%d", query.From.ID), chatId, account, "send", "waiting_profile_no", 0, "", nil, "请输入要发送的笔记编号、标题或正文关键词，例如：A00001")
	case "askedit":
		return true, s.startProfileSessionByIds(ctx, botId, fmt.Sprintf("%d", query.From.ID), chatId, account, "edit", "waiting_profile_no", 0, "", nil, "请输入要编辑的笔记编号、标题或正文关键词，例如：A00001")
	case "askcancelq":
		return true, s.showChannelList(ctx, botId, chatId, account, "")
	case "cancel":
		return true, s.cancelProfileSessionByIds(ctx, botId, fmt.Sprintf("%d", query.From.ID), chatId)
	case "createback":
		session := s.activeProfileSessionByIds(ctx, botId, fmt.Sprintf("%d", query.From.ID), chatId)
		if session == nil || (session.Scene != "create" && session.Scene != "replace") {
			return true, s.sendProfileCreateStepPrompt(ctx, botId, chatId, "waiting_display")
		}
		draft := decodeProfileCreateDraft(session.PayloadJson)
		draft.DisplayText = ""
		draft.DisplayMedia = nil
		draft.VerifyText = ""
		draft.VerifyMedia = nil
		if err = s.updateProfileSession(ctx, session.Id, "waiting_display", draft); err != nil {
			return true, err
		}
		if err = s.sendMessageOnly(ctx, botId, chatId, "已返回第 1 步，原展示资料和验证资料已清空。"); err != nil {
			return true, err
		}
		return true, s.sendProfileCreateStepPrompt(ctx, botId, chatId, "waiting_display")
	case "createskip":
		session := s.activeProfileSessionByIds(ctx, botId, fmt.Sprintf("%d", query.From.ID), chatId)
		if session == nil || (session.Scene != "create" && session.Scene != "replace") || session.Step != "waiting_verify" {
			return true, s.replyBotError(ctx, botId, chatId, "资料管理", gerror.New("当前没有可跳过的验证资料步骤"))
		}
		return true, s.consumeProfileCreatePart(ctx, botId, chatId, account, session, "跳过", nil)
	case "up":
		return true, s.changeProfilesStatus(ctx, botId, chatId, account, []string{no}, 1)
	case "down":
		return true, s.changeProfilesStatus(ctx, botId, chatId, account, []string{no}, 2)
	case "send":
		return true, s.sendProfileByNo(ctx, botId, chatId, account, no)
	case "view":
		return true, s.showProfileCard(ctx, botId, chatId, account, no)
	case "backview":
		_ = s.cancelProfileSessionByIdsSilent(ctx, botId, fmt.Sprintf("%d", query.From.ID), chatId)
		return true, s.showProfileCard(ctx, botId, chatId, account, no)
	case "edit":
		return true, s.showProfileEditMenu(ctx, botId, chatId, account, no)
	case "edtitle":
		return true, s.startProfileSessionByIds(ctx, botId, fmt.Sprintf("%d", query.From.ID), chatId, account, "edit_title", "waiting_title", 0, no, nil, "请输入新的标题：")
	case "edtext":
		return true, s.startProfileSessionByIds(ctx, botId, fmt.Sprintf("%d", query.From.ID), chatId, account, "edit_text", "waiting_text", 0, no, nil, "请输入新的正文：")
	case "edno":
		return true, s.startProfileSessionByIds(ctx, botId, fmt.Sprintf("%d", query.From.ID), chatId, account, "edit_no", "waiting_no", 0, no, nil, "请输入新的编号，格式如 A00001：")
	case "replace":
		if err = s.startProfileSessionByIds(ctx, botId, fmt.Sprintf("%d", query.From.ID), chatId, account, "replace", "waiting_display", 0, no, nil, ""); err != nil {
			return true, err
		}
		return true, s.sendProfileCreateStepPrompt(ctx, botId, chatId, "waiting_display")
	}
	return true, nil
}

func (s *sSysBot) handleProfileInlineQuery(ctx context.Context, botId int64, query *models.InlineQuery) error {
	if query == nil {
		return nil
	}
	q := strings.TrimSpace(query.Query)
	if strings.HasPrefix(strings.ToUpper(q), "XX") {
		return s.handleTemplateInlineQuery(ctx, botId, query, q)
	}
	return s.answerInlinePromotion(ctx, botId, query)
	/*
		share, err := s.inlineShareByToken(ctx, q)
		if err != nil {
			return err
		}
		row, err := s.botById(ctx, botId)
		if err != nil {
			return err
		}
		bot, err := s.telegramBot(ctx, row.BotToken)
		if err != nil {
			return err
		}
		results := []models.InlineQueryResult{}
		if share != nil && share.ProfileNo != "" {
			note, viewErr := publishService.SysPublish().BotProfileView(ctx, &publishsysin.BotProfileViewInp{TenantId: share.TenantId, AccountId: share.AccountId, ProfileNo: share.ProfileNo, PublicOnly: true})
			if viewErr == nil && note != nil {
				text := profileShareText(note)
				results = append(results, &models.InlineQueryResultArticle{ID: "profile_" + note.ProfileNo, Title: note.Title, Description: note.ProfileNo, InputMessageContent: &models.InputTextMessageContent{MessageText: text, ParseMode: models.ParseModeHTML}})
				_ = s.incrementInlineShareUsage(ctx, share.Token)
			}
		}
		callCtx, cancel := telegramAPICtx()
		defer cancel()
		_, err = bot.AnswerInlineQuery(callCtx, &tgbot.AnswerInlineQueryParams{InlineQueryID: query.ID, Results: results, CacheTime: 0, IsPersonal: false})
		return err
	*/
}

func (s *sSysBot) handleTemplateInlineQuery(ctx context.Context, botId int64, query *models.InlineQuery, serial string) error {
	startedAt := time.Now()
	g.Log().Infof(ctx, "Inline模板请求开始 botId:%d queryId:%s serial:%s", botId, query.ID, serial)
	defer func() {
		g.Log().Infof(ctx, "Inline模板请求结束 botId:%d queryId:%s serial:%s duration:%s", botId, query.ID, serial, time.Since(startedAt))
	}()
	var row struct {
		Id           int64  `json:"id"`
		SerialNo     string `json:"serial_no"`
		Name         string `json:"name"`
		Text         string `json:"text"`
		Status       int    `json:"status"`
		ButtonConfig string `json:"button_config"`
	}
	templateQueryStartedAt := time.Now()
	if err := g.DB().Model("hg_youban_publish_message_template").Safe().Ctx(ctx).
		Where("serial_no", strings.ToUpper(strings.TrimSpace(serial))).Where("status", 1).WhereNull("deleted_at").Scan(&row); err != nil {
		return err
	}
	g.Log().Infof(ctx, "Inline模板查询完成 botId:%d queryId:%s serial:%s templateId:%d duration:%s", botId, query.ID, serial, row.Id, time.Since(templateQueryStartedAt))
	results := []models.InlineQueryResult{}
	if row.Id > 0 {
		var mediaRows []struct {
			MediaType             string `json:"media_type"`
			FileURL               string `json:"file_url"`
			StoragePath           string `json:"storage_path"`
			PosterURL             string `json:"poster_url"`
			PosterStoragePath     string `json:"poster_storage_path"`
			TgFileID              string `json:"tg_file_id"`
			SourceMessageRecordID int64  `json:"source_message_record_id"`
		}
		mediaQueryStartedAt := time.Now()
		mediaErr := g.DB().Model("hg_youban_publish_message_media").Safe().Ctx(ctx).
			Where("template_id", row.Id).
			OrderAsc("sort_index").
			Limit(2).
			Scan(&mediaRows)
		if mediaErr != nil {
			return mediaErr
		}
		g.Log().Infof(ctx, "Inline模板媒体查询完成 botId:%d queryId:%s templateId:%d mediaCount:%d duration:%s", botId, query.ID, row.Id, len(mediaRows), time.Since(mediaQueryStartedAt))
		mediaCount := len(mediaRows)
		var media struct {
			MediaType             string `json:"media_type"`
			FileURL               string `json:"file_url"`
			StoragePath           string `json:"storage_path"`
			PosterURL             string `json:"poster_url"`
			PosterStoragePath     string `json:"poster_storage_path"`
			TgFileID              string `json:"tg_file_id"`
			SourceMessageRecordID int64  `json:"source_message_record_id"`
		}
		if mediaCount == 1 {
			media = mediaRows[0]
		}
		caption := publishService.SysPublish().TelegramRichTextHTML(row.Text)
		buttonMarkup, buttonCount, buttonError := templateInlineButtonMarkupWithStats(row.ButtonConfig)
		inlineButtonMarkup := inlineQueryReplyMarkup(buttonMarkup)
		if buttonError != nil {
			g.Log().Errorf(ctx, "Inline按钮配置解析失败 botId:%d queryId:%s templateId:%d serial:%s err:%+v", botId, query.ID, row.Id, row.SerialNo, buttonError)
		} else {
			g.Log().Infof(ctx, "Inline按钮配置解析完成 botId:%d queryId:%s templateId:%d mode:%s buttonCount:%d hasReplyMarkup:%t", botId, query.ID, row.Id, buttonConfigMode(row.ButtonConfig), buttonCount, buttonMarkup != nil)
		}
		if mediaCount == 1 && strings.EqualFold(strings.TrimSpace(media.MediaType), "image") {
			photoStartedAt := time.Now()
			cachedPhoto, cachedErr := s.templateInlineCachedPhoto(ctx, botId, media.SourceMessageRecordID, media.TgFileID)
			g.Log().Infof(ctx, "Inline模板来源图片处理完成 botId:%d queryId:%s templateId:%d cacheKey:%s duration:%s", botId, query.ID, row.Id, templateInlinePhotoCacheKey(botId, media.SourceMessageRecordID), time.Since(photoStartedAt))
			if cachedErr != nil {
				g.Log().Warningf(ctx, "读取Inline缓存图片失败 templateId:%d sourceMessageRecordId:%d err:%+v", row.Id, media.SourceMessageRecordID, cachedErr)
			}
			if cachedPhoto != nil && strings.TrimSpace(cachedPhoto.FileID) != "" {
				cachedCaption := cachedPhoto.Caption
				if strings.TrimSpace(cachedCaption) == "" {
					cachedCaption = caption
				}
				results = append(results, &models.InlineQueryResultCachedPhoto{
					ID:              row.SerialNo,
					PhotoFileID:     cachedPhoto.FileID,
					Title:           row.Name,
					Description:     row.SerialNo,
					Caption:         cachedCaption,
					CaptionEntities: cachedPhoto.CaptionEntities,
					ReplyMarkup:     inlineButtonMarkup,
				})
			}
			photoURL := normalizePreviewMediaURL(s.absoluteMediaURL(ctx, firstNonEmpty(media.FileURL, media.StoragePath)))
			thumbnailURL := normalizePreviewMediaURL(s.absoluteMediaURL(ctx, firstNonEmpty(media.PosterURL, media.PosterStoragePath, media.FileURL, media.StoragePath)))
			if len(results) == 0 && photoURL != "" && thumbnailURL != "" {
				results = append(results, &models.InlineQueryResultPhoto{
					ID:           row.SerialNo,
					PhotoURL:     photoURL,
					ThumbnailURL: thumbnailURL,
					Title:        row.Name,
					Description:  row.SerialNo,
					Caption:      caption,
					ParseMode:    models.ParseModeHTML,
					ReplyMarkup:  inlineButtonMarkup,
				})
			}
		} else {
			results = append(results, &models.InlineQueryResultArticle{
				ID:          row.SerialNo,
				Title:       row.Name,
				Description: row.SerialNo,
				InputMessageContent: &models.InputTextMessageContent{
					MessageText: caption,
					ParseMode:   models.ParseModeHTML,
				},
				ReplyMarkup: inlineButtonMarkup,
			})
		}
	}
	botLookupStartedAt := time.Now()
	botRow, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	g.Log().Infof(ctx, "Inline Bot配置查询完成 botId:%d queryId:%s duration:%s", botId, query.ID, time.Since(botLookupStartedAt))
	bot, err := s.telegramBot(ctx, botRow.BotToken)
	if err != nil {
		return err
	}
	callCtx, cancel := telegramAPICtx()
	defer cancel()
	g.Log().Infof(ctx, "Inline模板开始响应 botId:%d queryId:%s serial:%s resultCount:%d", botId, query.ID, serial, len(results))
	answerStartedAt := time.Now()
	_, err = bot.AnswerInlineQuery(callCtx, &tgbot.AnswerInlineQueryParams{InlineQueryID: query.ID, Results: results, CacheTime: 0, IsPersonal: false})
	g.Log().Infof(ctx, "Inline模板响应调用完成 botId:%d queryId:%s duration:%s", botId, query.ID, time.Since(answerStartedAt))
	if err != nil {
		g.Log().Errorf(ctx, "Inline模板响应失败 botId:%d queryId:%s serial:%s duration:%s err:%+v", botId, query.ID, serial, time.Since(startedAt), err)
	}
	return err
}

func inlineQueryReplyMarkup(markup *models.InlineKeyboardMarkup) models.ReplyMarkup {
	if markup == nil {
		return nil
	}
	return *markup
}

func templateInlineButtonMarkup(value string) *models.InlineKeyboardMarkup {
	markup, _, _ := templateInlineButtonMarkupWithStats(value)
	return markup
}

func templateInlineButtonMarkupWithStats(value string) (*models.InlineKeyboardMarkup, int, error) {
	if strings.TrimSpace(value) == "" {
		return nil, 0, nil
	}
	var config publishsysin.MessageTemplateButtonConfig
	if err := json.Unmarshal([]byte(value), &config); err != nil || config.Mode != "inline" {
		if err != nil {
			return nil, 0, err
		}
		return nil, 0, gerror.Newf("按钮模式不是inline：%s", config.Mode)
	}
	rows := make([][]models.InlineKeyboardButton, 0, len(config.Rows))
	buttonCount := 0
	for _, row := range config.Rows {
		buttons := make([]models.InlineKeyboardButton, 0, len(row))
		for _, button := range row {
			text := strings.TrimSpace(button.Text)
			url := templateInlineButtonURL(button.URL)
			if text == "" || url == "" {
				continue
			}
			buttons = append(buttons, models.InlineKeyboardButton{Text: text, URL: url, Style: publishsysin.TelegramButtonStyle(button.Color)})
			buttonCount++
		}
		if len(buttons) > 0 {
			rows = append(rows, buttons)
		}
	}
	if len(rows) == 0 {
		return nil, 0, gerror.New("按钮配置没有有效按钮")
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}, buttonCount, nil
}

func buttonConfigMode(value string) string {
	var config publishsysin.MessageTemplateButtonConfig
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return "invalid"
	}
	return strings.TrimSpace(config.Mode)
}

func templateInlineButtonURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if strings.HasPrefix(rawURL, "@") && len(rawURL) > 1 {
		return "https://t.me/" + strings.TrimPrefix(rawURL, "@")
	}
	return rawURL
}

type templateInlineCachedPhoto struct {
	FileID          string
	Caption         string
	CaptionEntities []models.MessageEntity
}

func templateInlinePhotoCacheKey(botId, sourceMessageRecordId int64) string {
	return fmt.Sprintf("youban_bot:inline_photo:%d:%d", botId, sourceMessageRecordId)
}

func (s *sSysBot) templateInlineCachedPhoto(ctx context.Context, botId int64, sourceMessageRecordId int64, fallbackFileID string) (*templateInlineCachedPhoto, error) {
	if strings.TrimSpace(fallbackFileID) != "" {
		return &templateInlineCachedPhoto{FileID: strings.TrimSpace(fallbackFileID)}, nil
	}
	if botId <= 0 || sourceMessageRecordId <= 0 {
		return nil, nil
	}
	cacheKey := templateInlinePhotoCacheKey(botId, sourceMessageRecordId)
	if value, err := cache.Instance().Get(ctx, cacheKey); err == nil && !value.IsNil() {
		if value.String() == "__miss__" {
			return nil, nil
		}
		var cached templateInlineCachedPhoto
		if json.Unmarshal([]byte(value.String()), &cached) == nil && strings.TrimSpace(cached.FileID) != "" {
			return &cached, nil
		}
	}
	var row struct {
		RawJSON string `json:"raw_json"`
	}
	if err := g.DB().Model(messageTable).Safe().Ctx(ctx).
		Fields("raw_json").
		Where("id", sourceMessageRecordId).
		Where("bot_id", botId).
		Scan(&row); err != nil {
		return nil, gerror.Wrap(err, "读取Telegram来源消息失败")
	}
	photo := decodeTemplateInlineCachedPhoto(row.RawJSON)
	if photo == nil {
		_ = cache.Instance().Set(ctx, cacheKey, "__miss__", time.Minute)
		return nil, nil
	}
	if data, marshalErr := json.Marshal(photo); marshalErr == nil {
		_ = cache.Instance().Set(ctx, cacheKey, string(data), templateInlinePhotoCacheTTL)
	}
	return photo, nil
}

func decodeTemplateInlineCachedPhoto(rawJSON string) *templateInlineCachedPhoto {
	if strings.TrimSpace(rawJSON) == "" {
		return nil
	}
	var message models.Message
	if json.Unmarshal([]byte(rawJSON), &message) != nil || len(message.Photo) == 0 {
		return nil
	}
	photo := message.Photo[len(message.Photo)-1]
	if strings.TrimSpace(photo.FileID) == "" {
		return nil
	}
	return &templateInlineCachedPhoto{
		FileID:          strings.TrimSpace(photo.FileID),
		Caption:         message.Caption,
		CaptionEntities: message.CaptionEntities,
	}
}

func (s *sSysBot) boundProfileAccount(ctx context.Context, msg *models.Message) (*botProfileAccount, error) {
	if msg == nil || msg.From == nil {
		return nil, gerror.New("无法识别Telegram用户")
	}
	return s.boundProfileAccountByUser(ctx, msg.From.ID)
}

func (s *sSysBot) boundProfileAccountByUser(ctx context.Context, telegramUserId int64) (*botProfileAccount, error) {
	bind, err := s.bindingByTelegram(ctx, botsysin.BotAppApi, fmt.Sprintf("%d", telegramUserId))
	if err != nil {
		return nil, err
	}
	if bind == nil || bind.AccountId <= 0 {
		return nil, gerror.New("请先绑定上架端账号后再使用资料管理。")
	}
	var row struct {
		Id          int64  `json:"id"`
		TenantId    int64  `json:"tenant_id"`
		AccountType string `json:"account_type"`
	}
	if err = g.DB().Model(publishAccountTable).Safe().Ctx(ctx).Fields("id,tenant_id,account_type").Where("id", bind.AccountId).Where("status", 1).WhereNull("deleted_at").Scan(&row); err != nil {
		return nil, gerror.Wrap(err, "读取上架账号失败")
	}
	if row.Id <= 0 || row.TenantId <= 0 {
		return nil, gerror.New("绑定的上架账号不可用")
	}
	return &botProfileAccount{TenantId: row.TenantId, AccountId: row.Id, AccountType: row.AccountType, App: botsysin.BotAppApi}, nil
}

func extractProfileNos(text string) []string {
	matches := profileNoFindRegexp.FindAllString(strings.ToUpper(text), -1)
	seen := map[string]struct{}{}
	res := make([]string, 0, len(matches))
	for _, item := range matches {
		if _, ok := seen[item]; !ok {
			seen[item] = struct{}{}
			res = append(res, item)
		}
	}
	return res
}

func profileActionKeyword(text string, actionWords ...string) string {
	kw := strings.TrimSpace(text)
	for _, word := range actionWords {
		kw = strings.ReplaceAll(kw, word, "")
	}
	kw = strings.TrimSpace(strings.NewReplacer("笔记", "", "资料", "", "：", " ", ":", " ").Replace(kw))
	return strings.TrimSpace(kw)
}

func (s *sSysBot) showProfileList(ctx context.Context, botId int64, chatId string, account *botProfileAccount, page int) error {
	if page <= 0 {
		page = 1
	}
	const perPage = 15
	list, total, err := publishService.SysPublish().BotProfileSearch(ctx, botProfileSearchInput(account, "", page, perPage))
	if err != nil {
		return s.replyBotError(ctx, botId, chatId, "笔记列表", err)
	}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		markup := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "返回资料管理", CallbackData: "pf:menu"}}}}
		_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, "<b>笔记列表</b>\n\n暂无笔记。", "HTML", false, markup)
		return err
	}
	totalPage := (total + perPage - 1) / perPage
	if totalPage <= 0 {
		totalPage = 1
	}
	buttons := make([][]models.InlineKeyboardButton, 0, len(list)+3)
	for _, note := range list {
		if note == nil {
			continue
		}
		title := strings.TrimSpace(note.Title)
		if title == "" {
			title = shortText(note.PlainText, 18)
		}
		label := fmt.Sprintf("%s · %s", note.ProfileNo, shortButtonText(title, 24))
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: label, CallbackData: "pf:view:" + note.ProfileNo}})
	}
	pager := []models.InlineKeyboardButton{}
	if page > 1 {
		pager = append(pager, models.InlineKeyboardButton{Text: "上一页", CallbackData: fmt.Sprintf("pf:list:%d", page-1)})
	}
	pager = append(pager, models.InlineKeyboardButton{Text: "跳转页码", CallbackData: "pf:askpage"})
	if page < totalPage {
		pager = append(pager, models.InlineKeyboardButton{Text: "下一页", CallbackData: fmt.Sprintf("pf:list:%d", page+1)})
	}
	buttons = append(buttons, pager)
	buttons = append(buttons, []models.InlineKeyboardButton{{Text: "返回资料管理", CallbackData: "pf:menu"}})
	text := fmt.Sprintf("<b>笔记列表</b>\n共 %d 条，当前第 %d/%d 页。\n点击笔记打开详情。", total, page, totalPage)
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, text, "HTML", false, &models.InlineKeyboardMarkup{InlineKeyboard: buttons})
	return err
}

func (s *sSysBot) changeProfilesStatus(ctx context.Context, botId int64, chatId string, account *botProfileAccount, nos []string, status int) error {
	res, err := publishService.SysPublish().BotProfileStatus(ctx, &publishsysin.BotProfileStatusInp{TenantId: account.TenantId, AccountId: botProfileScopeAccountId(account), Nos: nos, Status: status})
	if err != nil {
		return s.replyBotError(ctx, botId, chatId, "资料管理", err)
	}
	label := "上架"
	if status == 2 {
		label = "下架"
	}
	msg := fmt.Sprintf("已提交%s：%s", label, html.EscapeString(strings.Join(nos, ", ")))
	if res != nil && strings.TrimSpace(res.Message) != "" {
		msg = html.EscapeString(res.Message)
	}
	return s.sendMessageOnly(ctx, botId, chatId, msg)
}

func (s *sSysBot) searchProfilesAndReply(ctx context.Context, botId int64, chatId string, account *botProfileAccount, keyword string, purpose string) error {
	list, _, err := publishService.SysPublish().BotProfileSearch(ctx, botProfileSearchInput(account, keyword, 1, 5))
	if err != nil {
		return s.replyBotError(ctx, botId, chatId, "资料管理", err)
	}
	if len(list) == 0 {
		return s.sendMessageOnly(ctx, botId, chatId, "未找到匹配笔记。")
	}
	for _, note := range list {
		if err := s.sendProfileCard(ctx, botId, chatId, note, purpose); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysBot) consumeProfileSearchImageMessage(ctx context.Context, botId int64, msg *models.Message, account *botProfileAccount, session *profileSessionRow) error {
	chatId := fmt.Sprintf("%d", msg.Chat.ID)
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	media, err := s.resolveTelegramMessageMedia(ctx, row.BotToken, msg)
	if err != nil {
		g.Log().Warning(ctx, "Bot图片搜索读取TG文件失败", g.Map{"botId": botId, "chatId": chatId, "err": err})
		return s.replyBotError(ctx, botId, chatId, "图片搜索", err)
	}
	if len(media) == 0 || strings.TrimSpace(media[0].FileUrl) == "" || media[0].MediaType != "image" {
		return s.sendMessageOnly(ctx, botId, chatId, "请发送图片，或输入笔记编号、标题、正文关键词。")
	}
	in := &publishsysin.BotProfileImageSearchInp{
		ProfileImageSearchInp: publishsysin.ProfileImageSearchInp{
			ProfileListInp: publishsysin.ProfileListInp{
				TenantId:  account.TenantId,
				AccountId: botProfileScopeAccountId(account),
			},
			Threshold: 12,
		},
		ImageUrl:    media[0].FileUrl,
		AccountType: account.AccountType,
	}
	in.AccountId = account.AccountId
	in.Page = 1
	in.PerPage = 5
	list, _, err := publishService.SysPublish().BotProfileImageSearch(ctx, in)
	if err != nil {
		g.Log().Warning(ctx, "Bot图片搜索失败", g.Map{"botId": botId, "chatId": chatId, "tenantId": account.TenantId, "accountId": account.AccountId, "err": err})
		return s.replyBotError(ctx, botId, chatId, "图片搜索", err)
	}
	_ = s.completeProfileSession(ctx, session.Id)
	if len(list) == 0 {
		return s.sendMessageOnly(ctx, botId, chatId, "未找到相似图片笔记。")
	}
	for _, note := range list {
		if err := s.sendProfileCard(ctx, botId, chatId, note, "view"); err != nil {
			return err
		}
	}
	return nil
}

func botProfileScopeAccountId(account *botProfileAccount) int64 {
	if account == nil || account.AccountType == "admin" {
		return 0
	}
	return account.AccountId
}

func botProfileSearchInput(account *botProfileAccount, keyword string, page int, perPage int) *publishsysin.BotProfileSearchInp {
	in := &publishsysin.BotProfileSearchInp{TenantId: account.TenantId, AccountId: account.AccountId, AccountType: account.AccountType, Keyword: keyword}
	if account.AccountType != "admin" {
		in.AccountId = account.AccountId
	}
	in.Page = page
	in.PerPage = perPage
	return in
}

func (s *sSysBot) showProfileCard(ctx context.Context, botId int64, chatId string, account *botProfileAccount, no string) error {
	note, err := publishService.SysPublish().BotProfileView(ctx, &publishsysin.BotProfileViewInp{TenantId: account.TenantId, AccountId: botProfileScopeAccountId(account), ProfileNo: no})
	if err != nil {
		return s.replyBotError(ctx, botId, chatId, "资料管理", err)
	}
	return s.sendProfileCard(ctx, botId, chatId, note, "view")
}

func (s *sSysBot) sendProfileCard(ctx context.Context, botId int64, chatId string, note *publishsysin.NoteModel, purpose string) error {
	if note == nil {
		return nil
	}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	status := "下架"
	if note.Status == 1 {
		status = "上架"
	}
	mediaSummary := profileMediaSummary(note.Media)
	header := fmt.Sprintf("<b>%s</b>\n编号：<code>%s</code>\n状态：%s", html.EscapeString(note.Title), html.EscapeString(note.ProfileNo), status)
	if mediaSummary != "" {
		header += "\n" + mediaSummary
	}
	if source := profileSourceDisplay(note); source != "" {
		header += "\n" + source
	}
	text := profileCardText(header, note.PlainText)
	markup := profileCardMarkupForNote(note, purpose)
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, text, "HTML", false, markup)
	return err
}

func (s *sSysBot) sendProfileByNo(ctx context.Context, botId int64, chatId string, account *botProfileAccount, no string) error {
	note, err := publishService.SysPublish().BotProfileView(ctx, &publishsysin.BotProfileViewInp{TenantId: account.TenantId, AccountId: botProfileScopeAccountId(account), ProfileNo: no})
	if err != nil {
		return s.replyBotError(ctx, botId, chatId, "资料管理", err)
	}
	return s.sendProfileContent(ctx, botId, chatId, note)
}

func (s *sSysBot) sendProfileContent(ctx context.Context, botId int64, chatId string, note *publishsysin.NoteModel) error {
	if note == nil {
		return nil
	}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	bot, err := s.telegramBot(ctx, row.BotToken)
	if err != nil {
		return err
	}
	callCtx, cancel := telegramAPICtx()
	defer cancel()
	displayCaption := profilePreviewDisplayCaption(note)
	if profileHasSendablePurposeMedia(ctx, s, note.Media, "display") {
		if err = s.sendProfileMediaPurpose(ctx, callCtx, bot, chatId, note.Media, "display", displayCaption, false); err != nil {
			return err
		}
	} else if strings.TrimSpace(note.PlainText) != "" {
		g.Log().Warningf(ctx, "Bot资料预览展示媒体不可发送，改为发送原文 chatId:%s profileId:%d profileNo:%s", chatId, note.Id, note.ProfileNo)
		if _, err = s.sendMessage(ctx, row.BotToken, chatId, profileShareText(note), "HTML", false); err != nil {
			return err
		}
	}
	if profileHasSendablePurposeMedia(ctx, s, note.Media, "verify") {
		if err = s.sendProfileMediaPurpose(ctx, callCtx, bot, chatId, note.Media, "verify", "验证资料", false); err != nil {
			return err
		}
	}
	return s.sendPlainMessageOnly(ctx, botId, chatId, "预览完成。")
}

func (s *sSysBot) sendProfileMediaPurpose(ctx context.Context, callCtx context.Context, bot *tgbot.Bot, chatId string, media []*publishsysin.MediaModel, purpose string, captionPrefix string, forceUpload bool) error {
	items := make([]*publishsysin.MediaModel, 0)
	for _, item := range media {
		if item == nil || strings.TrimSpace(item.Purpose) != purpose {
			continue
		}
		if strings.TrimSpace(item.MediaType) != "image" && strings.TrimSpace(item.MediaType) != "video" {
			continue
		}
		if profileMediaSource(ctx, s, item) == "" {
			continue
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		g.Log().Warningf(ctx, "Bot资料预览没有可发送媒体 chatId:%s purpose:%s mediaCount:%d", chatId, purpose, len(media))
	}
	for start := 0; start < len(items); start += 10 {
		end := start + 10
		if end > len(items) {
			end = len(items)
		}
		group := make([]models.InputMedia, 0, end-start)
		groupItems := make([]*publishsysin.MediaModel, 0, end-start)
		groupCaptions := make([]string, 0, end-start)
		closers := make([]*os.File, 0)
		for index, item := range items[start:end] {
			caption := ""
			if start == 0 && index == 0 && strings.TrimSpace(captionPrefix) != "" {
				caption = captionPrefix
			}
			input, file, err := s.profilePreviewInputMediaWithUpload(ctx, item, caption, forceUpload)
			if err != nil {
				g.Log().Warningf(ctx, "Bot资料预览准备媒体失败 chatId:%s purpose:%s mediaId:%d err:%+v", chatId, purpose, item.Id, err)
				continue
			}
			if file != nil {
				closers = append(closers, file)
			}
			if input != nil {
				group = append(group, input)
				groupItems = append(groupItems, item)
				groupCaptions = append(groupCaptions, caption)
			}
		}
		defer func(files []*os.File) {
			for _, file := range files {
				_ = file.Close()
			}
		}(closers)
		if len(group) == 0 {
			continue
		}
		if len(group) == 1 {
			if err := s.sendSingleProfileMediaWithFallback(ctx, callCtx, bot, chatId, groupItems[0], groupCaptions[0], group[0]); err != nil {
				return err
			}
			continue
		}
		if _, err := bot.SendMediaGroup(callCtx, &tgbot.SendMediaGroupParams{ChatID: chatId, Media: group}); err != nil {
			g.Log().Warningf(ctx, "Bot资料预览媒体组发送失败 chatId:%s purpose:%s err:%+v", chatId, purpose, err)
			var lastErr error
			for index, single := range group {
				item := groupItems[index]
				if sendErr := s.sendSingleProfileMediaWithFallback(ctx, callCtx, bot, chatId, item, groupCaptions[index], single); sendErr != nil {
					lastErr = sendErr
					g.Log().Warningf(ctx, "Bot资料预览单媒体发送失败 chatId:%s purpose:%s mediaId:%d err:%+v", chatId, purpose, item.Id, sendErr)
				}
			}
			if lastErr != nil {
				return lastErr
			}
		}
	}
	return nil
}

func (s *sSysBot) profilePreviewInputMedia(ctx context.Context, media *publishsysin.MediaModel, caption string) (models.InputMedia, *os.File, error) {
	return s.profilePreviewInputMediaWithUpload(ctx, media, caption, false)
}

func (s *sSysBot) profilePreviewInputMediaWithUpload(ctx context.Context, media *publishsysin.MediaModel, caption string, forceUpload bool) (models.InputMedia, *os.File, error) {
	if media == nil {
		return nil, nil, nil
	}
	source := profileMediaSource(ctx, s, media)
	var file *os.File
	if forceUpload {
		source = ""
	}
	if source == "" || strings.HasPrefix(strings.TrimSpace(media.TgFileId), "copy:") {
		cached, err := publishService.SysPublish().BotMediaCacheFile(ctx, &publishsysin.BotMediaCacheFileInp{Media: media})
		if err != nil {
			return nil, nil, err
		}
		if cached != nil && strings.TrimSpace(cached.Path) != "" {
			opened, err := os.Open(cached.Path)
			if err != nil {
				return nil, nil, gerror.Wrapf(err, "打开预览媒体缓存失败:%s", cached.Path)
			}
			file = opened
			source = "attach://" + fmt.Sprintf("preview_%d_%s", media.Id, telegramSafeUploadFilename(cached.Path))
		}
	}
	if forceUpload && file == nil {
		return nil, nil, gerror.New("无法读取资料媒体文件")
	}
	if strings.TrimSpace(source) == "" {
		return nil, file, gerror.New("媒体文件地址为空")
	}
	switch strings.TrimSpace(media.MediaType) {
	case "image":
		photo := &models.InputMediaPhoto{Media: source, Caption: caption}
		if strings.TrimSpace(caption) != "" {
			photo.ParseMode = models.ParseModeHTML
		}
		if file != nil {
			photo.MediaAttachment = file
		}
		return photo, file, nil
	case "video":
		video := &models.InputMediaVideo{Media: source, Caption: caption, SupportsStreaming: true}
		if strings.TrimSpace(caption) != "" {
			video.ParseMode = models.ParseModeHTML
		}
		if file != nil {
			video.MediaAttachment = file
		}
		thumb := profileMediaThumbSource(ctx, s, media)
		if forceUpload && strings.TrimSpace(media.TgThumbFileId) != "" {
			thumb = normalizePreviewMediaURL(s.absoluteMediaURL(ctx, firstNonEmpty(media.PosterUrl, media.PosterStoragePath)))
		}
		if thumb != "" {
			video.Thumbnail = &models.InputFileString{Data: thumb}
		}
		return video, file, nil
	default:
		return nil, file, nil
	}
}

func (s *sSysBot) sendSingleProfileMediaWithFallback(ctx context.Context, callCtx context.Context, bot *tgbot.Bot, chatId string, media *publishsysin.MediaModel, caption string, input models.InputMedia) error {
	err := s.sendSingleProfileMedia(callCtx, bot, chatId, input)
	if err == nil || !isInvalidTelegramMediaReference(err) {
		return err
	}
	fallback, file, fallbackErr := s.profilePreviewInputMediaWithUpload(ctx, media, caption, true)
	if fallbackErr != nil {
		return err
	}
	if file != nil {
		defer file.Close()
	}
	return s.sendSingleProfileMedia(callCtx, bot, chatId, fallback)
}

func isInvalidTelegramMediaReference(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "wrong file identifier") || strings.Contains(message, "file identifier/ http url")
}

func telegramSafeUploadFilename(path string) string {
	name := strings.TrimSpace(path)
	if index := strings.LastIndex(name, "/"); index >= 0 && index < len(name)-1 {
		name = name[index+1:]
	}
	if name == "" {
		return "media"
	}
	return strings.NewReplacer(" ", "_", "?", "_", "&", "_", "=", "_", ":", "_").Replace(name)
}

func (s *sSysBot) sendSingleProfileMedia(ctx context.Context, bot *tgbot.Bot, chatId string, media models.InputMedia) error {
	switch item := media.(type) {
	case *models.InputMediaPhoto:
		photo := models.InputFile(&models.InputFileString{Data: item.Media})
		if item.MediaAttachment != nil {
			if file, ok := item.MediaAttachment.(*os.File); ok {
				_, _ = file.Seek(0, 0)
			}
			photo = &models.InputFileUpload{Filename: strings.TrimPrefix(item.Media, "attach://"), Data: item.MediaAttachment}
		}
		_, err := bot.SendPhoto(ctx, &tgbot.SendPhotoParams{ChatID: chatId, Photo: photo, Caption: item.Caption, ParseMode: item.ParseMode})
		return err
	case *models.InputMediaVideo:
		video := models.InputFile(&models.InputFileString{Data: item.Media})
		if item.MediaAttachment != nil {
			if file, ok := item.MediaAttachment.(*os.File); ok {
				_, _ = file.Seek(0, 0)
			}
			video = &models.InputFileUpload{Filename: strings.TrimPrefix(item.Media, "attach://"), Data: item.MediaAttachment}
		}
		_, err := bot.SendVideo(ctx, &tgbot.SendVideoParams{ChatID: chatId, Video: video, Thumbnail: item.Thumbnail, Caption: item.Caption, ParseMode: item.ParseMode, SupportsStreaming: true})
		return err
	default:
		return nil
	}
}

func profileHasSendablePurposeMedia(ctx context.Context, s *sSysBot, media []*publishsysin.MediaModel, purpose string) bool {
	for _, item := range media {
		if item != nil && strings.TrimSpace(item.Purpose) == purpose && (strings.TrimSpace(item.MediaType) == "image" || strings.TrimSpace(item.MediaType) == "video") {
			if profileMediaSource(ctx, s, item) != "" {
				return true
			}
		}
	}
	return false
}

func profileHasPurposeMedia(media []*publishsysin.MediaModel, purpose string) bool {
	for _, item := range media {
		if item != nil && strings.TrimSpace(item.Purpose) == purpose && (strings.TrimSpace(item.MediaType) == "image" || strings.TrimSpace(item.MediaType) == "video") {
			return true
		}
	}
	return false
}

func profileMediaSource(ctx context.Context, s *sSysBot, media *publishsysin.MediaModel) string {
	if media == nil {
		return ""
	}
	if source := strings.TrimSpace(media.TgFileId); source != "" && !strings.HasPrefix(source, "copy:") {
		return source
	}
	return normalizePreviewMediaURL(s.absoluteMediaURL(ctx, firstNonEmpty(media.FileUrl, media.OriginalFileUrl, media.EditedFileUrl)))
}

func profileMediaThumbSource(ctx context.Context, s *sSysBot, media *publishsysin.MediaModel) string {
	if media == nil {
		return ""
	}
	if source := strings.TrimSpace(media.TgThumbFileId); source != "" {
		return source
	}
	return normalizePreviewMediaURL(s.absoluteMediaURL(ctx, firstNonEmpty(media.PosterUrl, media.PosterStoragePath)))
}

func profileShareText(note *publishsysin.NoteModel) string {
	return profileShareHeader(note) + "\n\n" + html.EscapeString(note.PlainText)
}

func profileShareHeader(note *publishsysin.NoteModel) string {
	if note == nil {
		return ""
	}
	header := fmt.Sprintf("<b>%s</b>\n编号：<code>%s</code>", html.EscapeString(note.Title), html.EscapeString(note.ProfileNo))
	if source := profileSourceDisplay(note); source != "" {
		header += "\n" + source
	}
	return header
}

func profileSourceName(note *publishsysin.NoteModel) string {
	if note == nil || !note.IsCollected {
		return ""
	}
	name := strings.TrimSpace(note.CollectSourceName)
	username := strings.TrimPrefix(strings.TrimSpace(note.CollectSourceUsername), "@")
	if name == "" {
		name = username
	}
	return name
}

func profileSourceDisplay(note *publishsysin.NoteModel) string {
	name := profileSourceName(note)
	if name == "" {
		return ""
	}
	url := strings.TrimSpace(note.CollectSourceUrl)
	if url == "" {
		return "资料来源：" + html.EscapeString(name)
	}
	return "资料来源：<a href=\"" + html.EscapeString(url) + "\">" + html.EscapeString(name) + "</a>"
}

func profilePreviewDisplayCaption(note *publishsysin.NoteModel) string {
	if note == nil {
		return ""
	}
	caption := profileShareText(note)
	runes := []rune(caption)
	// Telegram caption 上限约 1024 字符；预览需要优先保证媒体组结构，超出时截断正文。
	if len(runes) <= 1000 {
		return caption
	}
	header := profileShareHeader(note) + "\n\n"
	remain := 1000 - len([]rune(header)) - 3
	if remain <= 0 {
		return shortText(header, 997) + "..."
	}
	raw := []rune(strings.TrimSpace(note.PlainText))
	low, high := 0, len(raw)
	for low < high {
		mid := (low + high + 1) / 2
		if len([]rune(html.EscapeString(string(raw[:mid])))) <= remain {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return header + html.EscapeString(string(raw[:low])) + "..."
}

func shortText(text string, limit int) string {
	r := []rune(strings.TrimSpace(text))
	if len(r) <= limit {
		return string(r)
	}
	return string(r[:limit]) + "..."
}

func profileCardText(header string, plainText string) string {
	raw := strings.TrimSpace(plainText)
	if raw == "" {
		return header
	}
	prefix := header + "\n\n"
	maxBody := telegramTextMessageMaxChars - len([]rune(prefix))
	if maxBody <= 0 {
		return header
	}
	rawRunes := []rune(raw)
	escaped := html.EscapeString(raw)
	if len([]rune(escaped)) <= maxBody {
		return prefix + escaped
	}
	if maxBody <= 3 {
		return prefix
	}
	limit := maxBody - 3
	low, high := 0, len(rawRunes)
	for low < high {
		mid := (low + high + 1) / 2
		if len([]rune(html.EscapeString(string(rawRunes[:mid])))) <= limit {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return prefix + html.EscapeString(string(rawRunes[:low])) + "..."
}

func profileCardMarkup(profileNo string, purpose string) *models.InlineKeyboardMarkup {
	profileNo = strings.ToUpper(strings.TrimSpace(profileNo))
	switch purpose {
	case "down":
		return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "确认下架", CallbackData: "pf:down:" + profileNo}, {Text: "查看详情", CallbackData: "pf:view:" + profileNo}},
			{{Text: "返回资料管理", CallbackData: "pf:menu"}},
		}}
	case "up":
		return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "确认上架", CallbackData: "pf:up:" + profileNo}, {Text: "查看详情", CallbackData: "pf:view:" + profileNo}},
			{{Text: "返回资料管理", CallbackData: "pf:menu"}},
		}}
	case "send":
		return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "发送此笔记", CallbackData: "pf:send:" + profileNo}},
			{{Text: "查看详情", CallbackData: "pf:view:" + profileNo}, {Text: "返回资料管理", CallbackData: "pf:menu"}},
		}}
	case "edit":
		return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "编辑此笔记", CallbackData: "pf:edit:" + profileNo}, {Text: "查看详情", CallbackData: "pf:view:" + profileNo}},
			{{Text: "返回资料管理", CallbackData: "pf:menu"}},
		}}
	case "readonly":
		return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "预览", CallbackData: "pf:send:" + profileNo}},
			{{Text: "返回资料管理", CallbackData: "pf:menu"}},
		}}
	default:
		return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "预览", CallbackData: "pf:send:" + profileNo}},
			{{Text: "编辑", CallbackData: "pf:edit:" + profileNo}, {Text: "返回列表", CallbackData: "pf:list:1"}},
			{{Text: "上架", CallbackData: "pf:up:" + profileNo}, {Text: "下架", CallbackData: "pf:down:" + profileNo}},
			{{Text: "返回资料管理", CallbackData: "pf:menu"}},
		}}
	}
}

func profileCardMarkupForNote(note *publishsysin.NoteModel, purpose string) *models.InlineKeyboardMarkup {
	if note == nil {
		return profileCardMarkup("", purpose)
	}
	markup := profileCardMarkup(note.ProfileNo, purpose)
	url := strings.TrimSpace(note.CollectSourceUrl)
	if !note.IsCollected || url == "" {
		return markup
	}
	markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: "来源频道 >", URL: url}})
	return markup
}

func profileMediaSummary(media []*publishsysin.MediaModel) string {
	lines := make([]string, 0, 2)
	if line := profileMediaPurposeSummary(media, "display"); line != "" {
		lines = append(lines, "展示资料："+line)
	}
	if line := profileMediaPurposeSummary(media, "verify"); line != "" {
		lines = append(lines, "验证资料："+line)
	}
	return strings.Join(lines, "\n")
}

func profileMediaPurposeSummary(media []*publishsysin.MediaModel, purpose string) string {
	imageCount := 0
	videoCount := 0
	for _, item := range media {
		if item == nil || strings.TrimSpace(item.Purpose) != purpose {
			continue
		}
		switch strings.TrimSpace(item.MediaType) {
		case "image":
			imageCount++
		case "video":
			videoCount++
		}
	}
	parts := make([]string, 0, 2)
	if imageCount > 0 {
		parts = append(parts, fmt.Sprintf("%d 图片", imageCount))
	}
	if videoCount > 0 {
		parts = append(parts, fmt.Sprintf("%d 视频", videoCount))
	}
	return strings.Join(parts, ", ")
}

func profileCreateStepMarkup(step string) *models.InlineKeyboardMarkup {
	switch step {
	case "waiting_verify":
		return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "返回上一步", CallbackData: "pf:createback"}, {Text: "跳过验证资料", CallbackData: "pf:createskip"}},
			{{Text: "取消", CallbackData: "pf:cancel"}},
		}}
	default:
		return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "返回资料管理", CallbackData: "pf:menu"}, {Text: "取消", CallbackData: "pf:cancel"}},
		}}
	}
}

func profileCreateStepText(step string) string {
	if step == "waiting_verify" {
		return "第 2 步/共 2 步：请发送【验证资料】（通常是一条验证视频，也支持文本、图片、视频或图文媒体组）。\n没有验证资料可点击“跳过验证资料”。"
	}
	return "请发送【展示资料】\n如果有验证视频，可按照验证资料的前后发送顺序发送消息, 推荐做好资料直接转发，系统会自动识别为验证资料。"
}

func (s *sSysBot) sendProfileCreateStepPrompt(ctx context.Context, botId int64, chatId string, step string) error {
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, profileCreateStepText(step), "HTML", false, profileCreateStepMarkup(step))
	return err
}

func profileMessageMediaSizeError(msg *models.Message) error {
	if msg == nil {
		return nil
	}
	if len(msg.Photo) > 0 {
		photo := msg.Photo[len(msg.Photo)-1]
		if int64(photo.FileSize) > profileCreateMaxMediaBytes {
			return gerror.Newf("图片大小超过 100M，请压缩后重新发送。当前约 %.1fM", float64(photo.FileSize)/1024/1024)
		}
	}
	if msg.Video != nil && msg.Video.FileSize > profileCreateMaxMediaBytes {
		return gerror.Newf("视频大小超过 100M，请压缩后重新发送。当前约 %.1fM", float64(msg.Video.FileSize)/1024/1024)
	}
	return nil
}

func (s *sSysBot) sendMessageOnly(ctx context.Context, botId int64, chatId string, text string) error {
	return s.reply(ctx, botId, chatId, text)
}

func (s *sSysBot) sendPlainMessageOnly(ctx context.Context, botId int64, chatId string, text string) error {
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, text, "HTML", false, nil)
	return err
}

func (s *sSysBot) absoluteMediaURL(ctx context.Context, u string) string {
	u = strings.TrimSpace(u)
	if u == "" || strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	base := strings.TrimRight(g.Cfg().MustGet(ctx, "youbanBot.mediaDomain").String(), "/")
	if base == "" {
		base = strings.TrimRight(g.Cfg().MustGet(ctx, "youbanBot.adminUrl").String(), "/")
	}
	if base == "" {
		return ""
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "https://" + base
	}
	return base + "/" + strings.TrimLeft(u, "/")
}

func normalizePreviewMediaURL(u string) string {
	u = strings.TrimSpace(u)
	if strings.HasPrefix(u, "/https://") {
		return strings.TrimPrefix(u, "/")
	}
	if strings.HasPrefix(u, "/http://") {
		return strings.TrimPrefix(u, "/")
	}
	if strings.HasPrefix(u, "https:/") && !strings.HasPrefix(u, "https://") {
		return "https://" + strings.TrimPrefix(u, "https:/")
	}
	if strings.HasPrefix(u, "http:/") && !strings.HasPrefix(u, "http://") {
		return "http://" + strings.TrimPrefix(u, "http:/")
	}
	return u
}

func (s *sSysBot) startProfileSession(ctx context.Context, botId int64, msg *models.Message, scene, step string, profileId int64, profileNo string, payload interface{}, replyText string) error {
	account, err := s.boundProfileAccount(ctx, msg)
	if err != nil {
		return s.reply(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), err.Error())
	}
	return s.startProfileSessionByIds(ctx, botId, fmt.Sprintf("%d", msg.From.ID), fmt.Sprintf("%d", msg.Chat.ID), account, scene, step, profileId, profileNo, payload, replyText)
}

func (s *sSysBot) startProfileSessionByIds(ctx context.Context, botId int64, telegramUserId string, chatId string, account *botProfileAccount, scene, step string, profileId int64, profileNo string, payload interface{}, replyText string) error {
	_ = s.cancelProfileSessionByIdsSilent(ctx, botId, telegramUserId, chatId)
	payloadText := ""
	if payload != nil {
		payloadText = gjson.MustEncodeString(payload)
	}
	now := gtime.Now()
	_, err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Data(g.Map{"bot_id": botId, "telegram_user_id": telegramUserId, "chat_id": chatId, "app": account.App, "account_id": account.AccountId, "tenant_id": account.TenantId, "scene": scene, "step": step, "profile_id": profileId, "profile_no": profileNo, "payload_json": payloadText, "expires_at": now.Add(30 * time.Minute), "status": profileSessionStatusActive, "created_at": now, "updated_at": now}).Insert()
	if err != nil {
		return gerror.Wrap(err, "创建资料操作会话失败")
	}
	if strings.TrimSpace(replyText) == "" {
		return nil
	}
	return s.sendProfileSessionPrompt(ctx, botId, chatId, scene, profileId, profileNo, replyText)
}

func (s *sSysBot) sendProfileSessionPrompt(ctx context.Context, botId int64, chatId string, scene string, profileId int64, profileNo string, text string) error {
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	buttons := [][]models.InlineKeyboardButton{}
	switch scene {
	case "edit_title", "edit_text", "edit_no":
		if strings.TrimSpace(profileNo) != "" {
			buttons = append(buttons, []models.InlineKeyboardButton{{Text: "返回笔记详情", CallbackData: "pf:backview:" + strings.ToUpper(profileNo)}})
		}
	case "channel_cycle":
		if profileId > 0 {
			buttons = append(buttons, []models.InlineKeyboardButton{{Text: "返回频道详情", CallbackData: fmt.Sprintf("ch:view:%d", profileId)}})
		}
	default:
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: "返回资料管理", CallbackData: "pf:menu"}})
	}
	buttons = append(buttons, []models.InlineKeyboardButton{{Text: "取消当前操作", CallbackData: "pf:cancel"}})
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, text, "HTML", false, &models.InlineKeyboardMarkup{InlineKeyboard: buttons})
	return err
}

func (s *sSysBot) cancelProfileSession(ctx context.Context, botId int64, msg *models.Message) error {
	if msg == nil || msg.From == nil {
		return nil
	}
	return s.cancelProfileSessionByIds(ctx, botId, fmt.Sprintf("%d", msg.From.ID), fmt.Sprintf("%d", msg.Chat.ID))
}
func (s *sSysBot) cancelProfileSessionByIds(ctx context.Context, botId int64, telegramUserId string, chatId string) error {
	_, _ = g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Where("bot_id", botId).Where("telegram_user_id", telegramUserId).Where("chat_id", chatId).Where("status", profileSessionStatusActive).Data(g.Map{"status": profileSessionStatusCanceled, "updated_at": gtime.Now()}).Update()
	return s.sendMessageOnly(ctx, botId, chatId, "当前操作已取消。")
}

type profileSessionRow struct {
	BotId       int64  `json:"bot_id"`
	Id          int64  `json:"id"`
	Scene       string `json:"scene"`
	Step        string `json:"step"`
	Status      string `json:"status"`
	ProfileId   int64  `json:"profile_id"`
	ProfileNo   string `json:"profile_no"`
	PayloadJson string `json:"payload_json"`
	TenantId    int64  `json:"tenant_id"`
	AccountId   int64  `json:"account_id"`
	AccountType string `json:"account_type"`
	App         string `json:"app"`
}

func (s *sSysBot) activeProfileSession(ctx context.Context, botId int64, msg *models.Message) *profileSessionRow {
	if msg == nil || msg.From == nil {
		return nil
	}
	var row *profileSessionRow
	_ = g.DB().Model(profileSessionTable).Safe().Ctx(ctx).
		Where("bot_id", botId).
		Where("telegram_user_id", fmt.Sprintf("%d", msg.From.ID)).
		Where("chat_id", fmt.Sprintf("%d", msg.Chat.ID)).
		Where("status", profileSessionStatusActive).
		WhereGT("expires_at", gtime.Now()).
		OrderDesc("id").
		Scan(&row)
	if row != nil && row.AccountId > 0 && strings.TrimSpace(row.AccountType) == "" {
		var account struct {
			AccountType string `json:"account_type"`
		}
		_ = g.DB().Model(publishAccountTable).Safe().Ctx(ctx).Fields("account_type").Where("id", row.AccountId).WhereNull("deleted_at").Scan(&account)
		row.AccountType = account.AccountType
	}
	return row
}

func (s *sSysBot) activeProfileSessionByIds(ctx context.Context, botId int64, telegramUserId string, chatId string) *profileSessionRow {
	var row *profileSessionRow
	_ = g.DB().Model(profileSessionTable).Safe().Ctx(ctx).
		Where("bot_id", botId).
		Where("telegram_user_id", telegramUserId).
		Where("chat_id", chatId).
		Where("status", profileSessionStatusActive).
		WhereGT("expires_at", gtime.Now()).
		OrderDesc("id").
		Scan(&row)
	if row != nil && row.AccountId > 0 && strings.TrimSpace(row.AccountType) == "" {
		var account struct {
			AccountType string `json:"account_type"`
		}
		_ = g.DB().Model(publishAccountTable).Safe().Ctx(ctx).Fields("account_type").Where("id", row.AccountId).WhereNull("deleted_at").Scan(&account)
		row.AccountType = account.AccountType
	}
	return row
}

func (s *sSysBot) consumeProfileSessionText(ctx context.Context, botId int64, msg *models.Message, account *botProfileAccount, session *profileSessionRow, text string, nos []string) error {
	chatId := fmt.Sprintf("%d", msg.Chat.ID)
	switch session.Scene {
	case "up":
		if len(nos) == 0 {
			_ = s.completeProfileSession(ctx, session.Id)
			return s.searchProfilesAndReply(ctx, botId, chatId, account, text, "up")
		}
		_ = s.completeProfileSession(ctx, session.Id)
		return s.changeProfilesStatus(ctx, botId, chatId, account, nos, 1)
	case "down":
		if len(nos) > 0 {
			_ = s.completeProfileSession(ctx, session.Id)
			return s.changeProfilesStatus(ctx, botId, chatId, account, nos, 2)
		}
		_ = s.completeProfileSession(ctx, session.Id)
		return s.searchProfilesAndReply(ctx, botId, chatId, account, text, "down")
	case "edit":
		if len(nos) == 0 {
			_ = s.completeProfileSession(ctx, session.Id)
			return s.searchProfilesAndReply(ctx, botId, chatId, account, text, "edit")
		}
		_ = s.completeProfileSession(ctx, session.Id)
		return s.showProfileEditMenu(ctx, botId, chatId, account, nos[0])
	case "edit_title":
		if err := s.editProfileBySession(ctx, botId, chatId, account, session.ProfileNo, text, "", ""); err != nil {
			return err
		}
		return s.completeProfileSession(ctx, session.Id)
	case "edit_text":
		if err := s.editProfileBySession(ctx, botId, chatId, account, session.ProfileNo, "", text, ""); err != nil {
			return err
		}
		return s.completeProfileSession(ctx, session.Id)
	case "edit_no":
		if err := s.editProfileBySession(ctx, botId, chatId, account, session.ProfileNo, "", "", text); err != nil {
			return err
		}
		return s.completeProfileSession(ctx, session.Id)
	case "cancel_queue":
		_ = s.completeProfileSession(ctx, session.Id)
		if strings.TrimSpace(text) == "全部" {
			nos = nil
		}
		return s.cancelProfileQueue(ctx, botId, chatId, account, nos)
	case "channel_cycle":
		if err := s.saveChannelCycleByText(ctx, botId, chatId, account, session.ProfileId, text); err != nil {
			return err
		}
		return s.completeProfileSession(ctx, session.Id)
	case "note_page":
		page := parseInt(text)
		if page <= 0 {
			return s.sendMessageOnly(ctx, botId, chatId, "页码不正确，请输入数字，例如：3")
		}
		_ = s.completeProfileSession(ctx, session.Id)
		return s.showProfileList(ctx, botId, chatId, account, page)
	case "send":
		if len(nos) == 0 {
			_ = s.completeProfileSession(ctx, session.Id)
			return s.searchProfilesAndReply(ctx, botId, chatId, account, text, "send")
		}
		_ = s.completeProfileSession(ctx, session.Id)
		for _, no := range nos {
			if err := s.sendProfileByNo(ctx, botId, chatId, account, no); err != nil {
				return err
			}
		}
		return nil
	case "search":
		_ = s.completeProfileSession(ctx, session.Id)
		return s.searchProfilesAndReply(ctx, botId, chatId, account, text, "view")
	}
	return nil
}

func (s *sSysBot) completeProfileSession(ctx context.Context, id int64) error {
	if id <= 0 {
		return nil
	}
	_, err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Where("id", id).Data(g.Map{"status": profileSessionStatusCompleted, "updated_at": gtime.Now()}).Update()
	return err
}

func (s *sSysBot) cancelProfileSessionByIdsSilent(ctx context.Context, botId int64, telegramUserId string, chatId string) error {
	_, err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Where("bot_id", botId).Where("telegram_user_id", telegramUserId).Where("chat_id", chatId).Where("status", profileSessionStatusActive).Data(g.Map{"status": profileSessionStatusCanceled, "updated_at": gtime.Now()}).Update()
	return err
}

func (s *sSysBot) isWaitingProfileCreate(ctx context.Context, botId int64, msg *models.Message) bool {
	if msg == nil || msg.From == nil {
		return false
	}
	var row struct {
		Id int64 `json:"id"`
	}
	_ = g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Fields("id").Where("bot_id", botId).Where("telegram_user_id", fmt.Sprintf("%d", msg.From.ID)).Where("chat_id", fmt.Sprintf("%d", msg.Chat.ID)).Where("scene", "create").WhereIn("step", []string{"waiting_display", "waiting_verify"}).Where("status", profileSessionStatusActive).WhereGT("expires_at", gtime.Now()).Scan(&row)
	return row.Id > 0
}

type profileCreateDraft struct {
	DisplayText             string                                  `json:"displayText"`
	DisplayMedia            []*publishsysin.MessageTemplateMediaInp `json:"displayMedia"`
	VerifyText              string                                  `json:"verifyText"`
	VerifyMedia             []*publishsysin.MessageTemplateMediaInp `json:"verifyMedia"`
	PendingGroupId          string                                  `json:"pendingGroupId,omitempty"`
	PendingGroup            *profilePendingMediaGroup               `json:"pendingGroup,omitempty"`
	PendingVerify           *profilePendingMediaGroup               `json:"pendingVerify,omitempty"`
	CompletedDisplayGroupId string                                  `json:"completedDisplayGroupId,omitempty"`
}

type profilePendingMediaGroup struct {
	SessionId int64                                   `json:"sessionId"`
	BotId     int64                                   `json:"botId"`
	ChatId    string                                  `json:"chatId"`
	GroupId   string                                  `json:"groupId"`
	Purpose   string                                  `json:"purpose"`
	Text      string                                  `json:"text"`
	Media     []*publishsysin.MessageTemplateMediaInp `json:"media"`
	CreatedAt int64                                   `json:"createdAt"`
	UpdatedAt int64                                   `json:"updatedAt"`
}

func (s *sSysBot) createProfileFromMessage(ctx context.Context, botId int64, msg *models.Message, text string) error {
	session := s.activeProfileSession(ctx, botId, msg)
	if session == nil || session.Scene != "create" {
		return nil
	}
	account := &botProfileAccount{TenantId: session.TenantId, AccountId: session.AccountId, AccountType: session.AccountType, App: session.App}
	return s.consumeProfileSessionMessage(ctx, botId, msg, account, session, text)
}

func (s *sSysBot) consumeProfileSessionMessage(ctx context.Context, botId int64, msg *models.Message, account *botProfileAccount, session *profileSessionRow, text string) error {
	if session == nil || msg == nil {
		return nil
	}
	var preparedMedia []*publishsysin.MessageTemplateMediaInp
	if strings.TrimSpace(msg.MediaGroupID) != "" {
		if err := s.reserveProfileMediaGroup(ctx, session, msg, text); err != nil {
			return err
		}
		row, err := s.botById(ctx, botId)
		if err != nil {
			return err
		}
		preparedMedia, err = s.resolveTelegramMessageMedia(ctx, row.BotToken, msg)
		if err != nil {
			chatId := fmt.Sprintf("%d", msg.Chat.ID)
			_ = s.replyBotError(ctx, botId, chatId, "资料管理", gerror.Wrap(err, "当前资料媒体解析失败，请重新发送当前步骤"))
			return err
		}
		g.Log().Infof(ctx, "Bot资料媒体并发解析完成 trace:PF-%d message_id:%d media_count:%d", session.Id, msg.ID, len(preparedMedia))
	}
	lockCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	lock := hglock.NewConfig(20*time.Second, 100*time.Millisecond).Mutex(profileSessionLockKey(session.Id))
	if err := lock.Lock(lockCtx); err != nil {
		return s.replyBotError(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), "资料管理", gerror.New("资料消息正在处理中，请稍后重新发送"))
	}
	defer func() { _ = lock.Unlock(context.Background()) }()
	latest := s.activeProfileSession(ctx, botId, msg)
	if latest == nil || latest.Id != session.Id {
		var state struct {
			Step   string `json:"step"`
			Status string `json:"status"`
		}
		_ = g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Fields("step,status").Where("id", session.Id).Scan(&state)
		if state.Status == profileSessionStatusCompleted {
			g.Log().Infof(ctx, "Bot资料消息忽略已完成会话的延迟重复事件 trace:PF-%d bot_id:%d message_id:%d media_group_id:%s step:%s status:%s", session.Id, botId, msg.ID, strings.TrimSpace(msg.MediaGroupID), state.Step, state.Status)
			return nil
		}
		g.Log().Warningf(ctx, "Bot资料会话校验失败 trace:PF-%d bot_id:%d message_id:%d media_group_id:%s expected_step:%s current_step:%s current_status:%s", session.Id, botId, msg.ID, strings.TrimSpace(msg.MediaGroupID), session.Step, state.Step, state.Status)
		return s.replyBotError(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), "资料管理", gerror.New("资料会话已失效，请重新开始新增资料"))
	}
	g.Log().Infof(ctx, "Bot资料会话校验通过 trace:PF-%d bot_id:%d message_id:%d media_group_id:%s step:%s", latest.Id, botId, msg.ID, strings.TrimSpace(msg.MediaGroupID), latest.Step)
	return s.consumeProfileSessionMessageLocked(ctx, botId, msg, account, latest, text, preparedMedia)
}

func (s *sSysBot) consumeProfileSessionMessageLocked(ctx context.Context, botId int64, msg *models.Message, account *botProfileAccount, session *profileSessionRow, text string, preparedMedia []*publishsysin.MessageTemplateMediaInp) error {
	trace := fmt.Sprintf("PF-%d", session.Id)
	startedAt := time.Now()
	mediaKind := "text"
	if len(msg.Photo) > 0 {
		mediaKind = "photo"
	} else if msg.Video != nil {
		mediaKind = "video"
	}
	g.Log().Infof(ctx, "Bot资料消息收到 trace:%s session_id:%d bot_id:%d message_id:%d chat_id:%d media_group_id:%s media_kind:%s text_len:%d", trace, session.Id, botId, msg.ID, msg.Chat.ID, strings.TrimSpace(msg.MediaGroupID), mediaKind, len(strings.TrimSpace(text)))
	chatId := fmt.Sprintf("%d", msg.Chat.ID)
	if err := profileMessageMediaSizeError(msg); err != nil {
		return s.replyBotError(ctx, botId, chatId, "资料管理", err)
	}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) != "" || len(msg.Photo) > 0 || msg.Video != nil {
		acknowledged, ackErr := s.acknowledgeProfileInput(ctx, session.Id, msg.MediaGroupID)
		if ackErr != nil {
			g.Log().Warningf(ctx, "发送资料接收反馈失败 sessionId:%d groupId:%s err:%+v", session.Id, msg.MediaGroupID, ackErr)
		} else if acknowledged {
			g.Log().Infof(ctx, "Bot资料处理反馈已发送 trace:%s message_id:%d", trace, msg.ID)
			ackText := fmt.Sprintf("已收到，正在处理，请稍候...\n处理编号：PF-%d", session.Id)
			if ackErr = s.sendPlainMessageOnly(ctx, botId, chatId, ackText); ackErr != nil {
				g.Log().Warningf(ctx, "发送资料处理中反馈失败 sessionId:%d groupId:%s err:%+v", session.Id, msg.MediaGroupID, ackErr)
			}
			s.scheduleProfileSessionTimeout(session.Id, botId, chatId)
		}
	}
	media := preparedMedia
	if media == nil {
		media, err = s.resolveTelegramMessageMedia(ctx, row.BotToken, msg)
		if err != nil {
			g.Log().Warningf(ctx, "Bot资料处理失败 trace:%s stage:resolve_media elapsed_ms:%d err:%v", trace, time.Since(startedAt).Milliseconds(), err)
			_ = s.replyBotError(ctx, botId, chatId, "资料管理", gerror.Wrap(err, "当前资料媒体解析失败，请重新发送当前步骤"))
			_ = s.sendProfileCreateStepPrompt(ctx, botId, chatId, session.Step)
			return err
		}
	}
	g.Log().Infof(ctx, "Bot资料处理阶段 trace:%s stage:media_resolved elapsed_ms:%d media_count:%d", trace, time.Since(startedAt).Milliseconds(), len(media))
	draft := decodeProfileCreateDraft(session.PayloadJson)
	if session.Step == "waiting_display" && draft.PendingGroup != nil && strings.TrimSpace(msg.MediaGroupID) == "" && (strings.TrimSpace(text) != "" || len(media) > 0) {
		pendingVerify := &profilePendingMediaGroup{SessionId: session.Id, BotId: botId, ChatId: chatId, Purpose: "verify", Text: strings.TrimSpace(text), Media: media, CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().UnixNano()}
		draft.PendingVerify = pendingVerify
		if err := s.updateProfileSession(ctx, session.Id, session.Step, draft); err != nil {
			return err
		}
		g.Log().Infof(ctx, "Bot资料第二媒体消息已归入验证资料 trace:%s media_count:%d", trace, len(media))
		go s.finishProfileMediaGroup(botId, strings.TrimSpace(draft.PendingGroupId), account, session)
		return nil
	}
	if strings.TrimSpace(msg.MediaGroupID) != "" && len(media) > 0 {
		return s.collectProfileMediaGroup(ctx, account, session, msg, text, media)
	}
	return s.consumeProfileCreatePart(ctx, botId, chatId, account, session, strings.TrimSpace(text), media)
}

func (s *sSysBot) reserveProfileMediaGroup(ctx context.Context, session *profileSessionRow, msg *models.Message, text string) error {
	if session == nil || msg == nil || strings.TrimSpace(msg.MediaGroupID) == "" || session.Step != "waiting_display" {
		return nil
	}
	lockCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	lock := hglock.NewConfig(15*time.Second, 100*time.Millisecond).Mutex(profileSessionLockKey(session.Id))
	if err := lock.Lock(lockCtx); err != nil {
		return gerror.Wrap(err, "登记资料媒体组失败")
	}
	defer func() { _ = lock.Unlock(context.Background()) }()
	var latest profileSessionRow
	if err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Fields("id,step,status,payload_json").Where("id", session.Id).Where("status", profileSessionStatusActive).Scan(&latest); err != nil {
		return err
	}
	if latest.Id == 0 || latest.Step != "waiting_display" {
		return nil
	}
	draft := decodeProfileCreateDraft(latest.PayloadJson)
	groupId := strings.TrimSpace(msg.MediaGroupID)
	if draft.PendingGroupId != "" && draft.PendingGroupId != groupId && draft.PendingGroup != nil {
		return nil
	}
	if draft.PendingGroupId == groupId && draft.PendingGroup != nil {
		return nil
	}
	draft.PendingGroupId = groupId
	draft.PendingGroup = &profilePendingMediaGroup{SessionId: session.Id, BotId: session.BotId, ChatId: fmt.Sprintf("%d", msg.Chat.ID), GroupId: groupId, Purpose: "display", Text: strings.TrimSpace(text), CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().UnixNano()}
	return s.updateProfileSession(ctx, session.Id, latest.Step, draft)
}

func profileSessionLockKey(sessionId int64) string {
	return fmt.Sprintf("youban_bot:profile_session:lock:%d", sessionId)
}

func (s *sSysBot) scheduleProfileSessionTimeout(sessionId, botId int64, chatId string) {
	go func() {
		timer := time.NewTimer(profileSessionTimeout)
		defer timer.Stop()
		<-timer.C
		ctx := context.Background()
		var row struct {
			Step   string `json:"step"`
			Status string `json:"status"`
		}
		if err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Fields("step,status").Where("id", sessionId).Scan(&row); err != nil || row.Status != profileSessionStatusActive {
			return
		}
		result, err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).
			Where("id", sessionId).
			Where("status", profileSessionStatusActive).
			WhereLTE("updated_at", gtime.Now().Add(-profileSessionTimeout)).
			Data(g.Map{"status": profileSessionStatusCanceled, "updated_at": gtime.Now()}).Update()
		if err != nil {
			g.Log().Warningf(ctx, "Bot资料会话超时更新失败 trace:PF-%d step:%s err:%v", sessionId, row.Step, err)
			return
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return
		}
		g.Log().Warningf(ctx, "Bot资料会话处理超时 trace:PF-%d step:%s", sessionId, row.Step)
		_ = s.sendMessageOnly(ctx, botId, chatId, fmt.Sprintf("资料处理超时，请重新发送。\n处理编号：PF-%d", sessionId))
	}()
}

func (s *sSysBot) consumeProfileCreatePart(ctx context.Context, botId int64, chatId string, account *botProfileAccount, session *profileSessionRow, text string, media []*publishsysin.MessageTemplateMediaInp) error {
	trace := fmt.Sprintf("PF-%d", session.Id)
	startedAt := time.Now()
	draft := decodeProfileCreateDraft(session.PayloadJson)
	switch session.Step {
	case "waiting_display":
		if strings.TrimSpace(text) == "" {
			g.Log().Warningf(ctx, "Bot资料展示组缺少正文 trace:%s session_id:%d media_count:%d", trace, session.Id, len(media))
			_ = s.updateProfileSession(ctx, session.Id, "waiting_display", &profileCreateDraft{})
			return s.replyBotError(ctx, botId, chatId, "资料管理", gerror.New("展示资料必须包含正文，请重新发送第一组展示资料"))
		}
		draft.DisplayText = strings.TrimSpace(text)
		draft.DisplayMedia = media
		g.Log().Infof(ctx, "Bot资料展示内容已解析 trace:%s media_count:%d text_len:%d", trace, len(media), len(draft.DisplayText))
		claimed, err := s.claimProfileSessionStep(ctx, session.Id, "waiting_display", "waiting_verify", draft)
		if err != nil {
			return err
		}
		if !claimed {
			g.Log().Warningf(ctx, "Bot资料展示内容未成功抢占会话步骤 trace:%s session_id:%d", trace, session.Id)
			return nil
		}
		g.Log().Infof(ctx, "Bot资料展示内容已写入会话 trace:%s session_id:%d media_count:%d text_len:%d", trace, session.Id, len(draft.DisplayMedia), len(draft.DisplayText))
		if err := s.sendPlainMessageOnly(ctx, botId, chatId, "已收到展示资料。"); err != nil {
			return err
		}
		s.scheduleVerifyPrompt(session.Id, botId, chatId)
		return nil
	case "waiting_verify":
		if strings.TrimSpace(text) == "跳过" || strings.EqualFold(strings.TrimSpace(text), "skip") {
			text = ""
			media = nil
		} else if strings.TrimSpace(text) == "" && len(media) == 0 {
			return s.sendProfileCreateStepPrompt(ctx, botId, chatId, "waiting_verify")
		}
		if strings.TrimSpace(draft.DisplayText) == "" && len(draft.DisplayMedia) == 0 {
			g.Log().Warningf(ctx, "Bot资料创建阻止空展示资料 trace:%s session_id:%d verify_media_count:%d verify_text_len:%d", trace, session.Id, len(media), len(strings.TrimSpace(text)))
			if err := s.resetProfileSessionStep(ctx, session.Id, "waiting_verify", "waiting_display", draft); err != nil {
				return err
			}
			return s.sendProfileCreateStepPrompt(ctx, botId, chatId, "waiting_display")
		}
		draft.VerifyText = strings.TrimSpace(text)
		draft.VerifyMedia = media
		persistedDisplay, err := s.persistTelegramMedia(ctx, account.AccountId, draft.DisplayMedia)
		if err != nil {
			g.Log().Warningf(ctx, "Bot资料展示媒体持久化失败 trace:%s media_count:%d err:%+v", trace, len(draft.DisplayMedia), err)
			return s.replyBotError(ctx, botId, chatId, "资料管理", gerror.Wrap(err, "展示资料保存失败，请重新发送"))
		}
		persistedVerify, err := s.persistTelegramMedia(ctx, account.AccountId, draft.VerifyMedia)
		if err != nil {
			g.Log().Warningf(ctx, "Bot资料验证媒体持久化失败 trace:%s media_count:%d err:%+v", trace, len(draft.VerifyMedia), err)
			return s.replyBotError(ctx, botId, chatId, "资料管理", gerror.Wrap(err, "验证资料保存失败，请重新发送"))
		}
		draft.DisplayMedia = persistedDisplay
		draft.VerifyMedia = persistedVerify
		claimed, err := s.claimProfileSessionStep(ctx, session.Id, "waiting_verify", "saving", draft)
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
		if session.Scene == "replace" {
			note, err := publishService.SysPublish().BotProfileEdit(ctx, &publishsysin.BotProfileEditInp{TenantId: account.TenantId, AccountId: botProfileScopeAccountId(account), ProfileNo: session.ProfileNo, PlainText: draft.DisplayText, DisplayMedia: draft.DisplayMedia, VerifyText: draft.VerifyText, VerifyMedia: draft.VerifyMedia})
			if err != nil {
				_ = s.resetProfileSessionStep(ctx, session.Id, "saving", "waiting_verify", draft)
				if replyErr := s.replyBotError(ctx, botId, chatId, "资料管理", err); replyErr != nil {
					return replyErr
				}
				return s.sendProfileCreateStepPrompt(ctx, botId, chatId, "waiting_verify")
			}
			_ = s.completeProfileSession(ctx, session.Id)
			if err = s.sendMessageOnly(ctx, botId, chatId, "资料已重新覆盖。"); err != nil {
				return err
			}
			return s.sendProfileCard(ctx, botId, chatId, note, "view")
		}
		res, err := publishService.SysPublish().BotProfileCreate(ctx, &publishsysin.BotProfileCreateInp{TenantId: account.TenantId, AccountId: account.AccountId, PlainText: draft.DisplayText, DisplayMedia: draft.DisplayMedia, VerifyText: draft.VerifyText, VerifyMedia: draft.VerifyMedia, Status: 2})
		if err != nil {
			g.Log().Warningf(ctx, "Bot资料创建失败 trace:%s stage:create_profile elapsed_ms:%d err:%v", trace, time.Since(startedAt).Milliseconds(), err)
			_ = s.resetProfileSessionStep(ctx, session.Id, "saving", "waiting_verify", draft)
			if replyErr := s.replyBotError(ctx, botId, chatId, "资料管理", err); replyErr != nil {
				return replyErr
			}
			return s.sendProfileCreateStepPrompt(ctx, botId, chatId, "waiting_verify")
		}
		g.Log().Infof(ctx, "Bot资料创建完成 trace:%s stage:create_profile elapsed_ms:%d profile_id:%d", trace, time.Since(startedAt).Milliseconds(), res.Id)
		_ = s.completeProfileSession(ctx, session.Id)
		return s.sendProfileCreateSuccess(ctx, botId, chatId, res)
	}
	return nil
}

func (s *sSysBot) scheduleVerifyPrompt(sessionId, botId int64, chatId string) {
	go func() {
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		<-timer.C
		ctx := context.Background()
		lock := hglock.NewConfig(20*time.Second, 100*time.Millisecond).Mutex(profileSessionLockKey(sessionId))
		if err := lock.Lock(ctx); err != nil {
			return
		}
		defer func() { _ = lock.Unlock(context.Background()) }()
		var row profileSessionRow
		if err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Fields("step,status,payload_json").Where("id", sessionId).Scan(&row); err != nil || row.Status != profileSessionStatusActive || row.Step != "waiting_verify" {
			return
		}
		draft := decodeProfileCreateDraft(row.PayloadJson)
		if !profileShouldSendVerifyPrompt(row.Step, row.Status, draft) {
			g.Log().Infof(ctx, "Bot资料验证提示跳过，仍有媒体组待处理 trace:PF-%d pending_group:%t pending_verify:%t", sessionId, draft.PendingGroup != nil, draft.PendingVerify != nil)
			return
		}
		_ = s.sendProfileCreateStepPrompt(ctx, botId, chatId, "waiting_verify")
	}()
}

func profileShouldSendVerifyPrompt(step string, status string, draft *profileCreateDraft) bool {
	if step != "waiting_verify" || status != profileSessionStatusActive {
		return false
	}
	return draft == nil || (draft.PendingGroup == nil && draft.PendingVerify == nil)
}

func (s *sSysBot) sendProfileCreateSuccess(ctx context.Context, botId int64, chatId string, res *publishsysin.ProfileSaveModel) error {
	if res == nil {
		return s.sendMessageOnly(ctx, botId, chatId, "创建成功！\n\n新资料默认下架，可在笔记列表中查看。")
	}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	profileNo := strings.ToUpper(strings.TrimSpace(res.ProfileNo))
	text := fmt.Sprintf("创建成功！\n资料ID：<code>%d</code>", res.Id)
	markup := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "返回资料管理", CallbackData: "pf:menu"}}}}
	if profileNo != "" {
		text = fmt.Sprintf("创建成功！\n资料编号：<code>%s</code>\n资料ID：<code>%d</code>\n\n新资料默认下架，确认无误后可直接上架。", html.EscapeString(profileNo), res.Id)
		markup = profileCardMarkup(profileNo, "view")
	} else {
		text += "\n\n新资料默认下架，可在笔记列表中查看。"
	}
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, text, "HTML", false, markup)
	return err
}

func decodeProfileCreateDraft(payload string) *profileCreateDraft {
	draft := &profileCreateDraft{}
	if strings.TrimSpace(payload) != "" {
		_ = json.Unmarshal([]byte(payload), draft)
	}
	return draft
}

func (s *sSysBot) updateProfileSession(ctx context.Context, id int64, step string, payload interface{}) error {
	payloadText := ""
	if payload != nil {
		payloadText = gjson.MustEncodeString(payload)
	}
	_, err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Where("id", id).Data(g.Map{"step": step, "payload_json": payloadText, "updated_at": gtime.Now()}).Update()
	return err
}

func (s *sSysBot) clearProfilePendingGroupSnapshot(ctx context.Context, sessionId int64) error {
	var row profileSessionRow
	if err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Fields("id,step,status,payload_json").Where("id", sessionId).Where("status", profileSessionStatusActive).Scan(&row); err != nil {
		return err
	}
	if row.Id == 0 {
		return nil
	}
	draft := decodeProfileCreateDraft(row.PayloadJson)
	draft.PendingGroupId = ""
	draft.PendingGroup = nil
	return s.updateProfileSession(ctx, sessionId, row.Step, draft)
}

func (s *sSysBot) markProfileDisplayGroupCompleted(ctx context.Context, sessionId int64, groupId string) error {
	var row profileSessionRow
	if err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Fields("id,step,status,payload_json").Where("id", sessionId).Where("status", profileSessionStatusActive).Scan(&row); err != nil {
		return err
	}
	if row.Id == 0 {
		return nil
	}
	draft := decodeProfileCreateDraft(row.PayloadJson)
	draft.PendingGroupId = ""
	draft.PendingGroup = nil
	draft.CompletedDisplayGroupId = strings.TrimSpace(groupId)
	return s.updateProfileSession(ctx, sessionId, row.Step, draft)
}

func (s *sSysBot) claimProfileSessionStep(ctx context.Context, id int64, fromStep string, toStep string, payload interface{}) (bool, error) {
	payloadText := ""
	if payload != nil {
		payloadText = gjson.MustEncodeString(payload)
	}
	result, err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).
		Where("id", id).
		Where("step", fromStep).
		Where("status", profileSessionStatusActive).
		Data(g.Map{"step": toStep, "payload_json": payloadText, "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func (s *sSysBot) resetProfileSessionStep(ctx context.Context, id int64, fromStep string, toStep string, payload interface{}) error {
	payloadText := ""
	if payload != nil {
		payloadText = gjson.MustEncodeString(payload)
	}
	_, err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).
		Where("id", id).
		Where("step", fromStep).
		Where("status", profileSessionStatusActive).
		Data(g.Map{"step": toStep, "payload_json": payloadText, "updated_at": gtime.Now()}).
		Update()
	return err
}

func (s *sSysBot) collectProfileMediaGroup(ctx context.Context, account *botProfileAccount, session *profileSessionRow, msg *models.Message, text string, media []*publishsysin.MessageTemplateMediaInp) error {
	groupId := strings.TrimSpace(msg.MediaGroupID)
	if groupId == "" || len(media) == 0 {
		return nil
	}
	if session.Step == "waiting_verify" {
		draft := decodeProfileCreateDraft(session.PayloadJson)
		if strings.TrimSpace(draft.CompletedDisplayGroupId) == groupId {
			g.Log().Infof(ctx, "Bot资料忽略已完成展示媒体组的延迟重复事件 trace:PF-%d groupId:%s", session.Id, groupId)
			return nil
		}
	}
	draft := decodeProfileCreateDraft(session.PayloadJson)
	groupPurpose := profileMediaGroupPurpose(session.Step, draft, groupId)
	if groupPurpose == "verify_pending" {
		pending := draft.PendingVerify
		if pending == nil || pending.GroupId != groupId {
			pending = &profilePendingMediaGroup{SessionId: session.Id, BotId: session.BotId, ChatId: fmt.Sprintf("%d", msg.Chat.ID), GroupId: groupId, Purpose: "verify", CreatedAt: time.Now().Unix()}
		}
		pending.UpdatedAt = time.Now().UnixNano()
		if strings.TrimSpace(pending.Text) == "" {
			pending.Text = strings.TrimSpace(text)
		}
		pending.Media = appendProfileGroupMedia(pending.Media, msg.ID, media)
		draft.PendingVerify = pending
		if err := s.updateProfileSession(ctx, session.Id, session.Step, draft); err != nil {
			return gerror.Wrap(err, "保存验证资料媒体组失败")
		}
		g.Log().Infof(ctx, "Bot资料第二媒体组已持久化 trace:PF-%d groupId:%s media_count:%d", session.Id, groupId, len(pending.Media))
		return nil
	}
	pending := draft.PendingGroup
	shouldFinalize := pending == nil || draft.PendingGroupId != groupId
	if shouldFinalize {
		pending = &profilePendingMediaGroup{SessionId: session.Id, BotId: session.BotId, ChatId: fmt.Sprintf("%d", msg.Chat.ID), GroupId: groupId, Purpose: groupPurpose, CreatedAt: time.Now().Unix()}
	}
	pending.UpdatedAt = time.Now().UnixNano()
	if strings.TrimSpace(pending.Text) == "" {
		pending.Text = strings.TrimSpace(text)
	}
	pending.Media = appendProfileGroupMedia(pending.Media, msg.ID, media)
	draft.PendingGroupId = groupId
	draft.PendingGroup = pending
	if err := s.updateProfileSession(ctx, session.Id, session.Step, draft); err != nil {
		return gerror.Wrap(err, "保存资料媒体组失败")
	}
	g.Log().Infof(ctx, "Bot资料媒体组已持久化 trace:PF-%d groupId:%s media_count:%d finalize:%t", session.Id, groupId, len(media), shouldFinalize)
	// 每条媒体消息都触发一次幂等收尾，避免首次 goroutine 丢失后会话永久停留。
	go s.finishProfileMediaGroup(session.BotId, groupId, account, session)
	return nil
}

func profileMediaGroupPurpose(step string, draft *profileCreateDraft, groupId string) string {
	if step == "waiting_verify" {
		return "verify"
	}
	if step == "waiting_display" && draft != nil && draft.PendingGroup != nil && strings.TrimSpace(draft.PendingGroupId) != "" && strings.TrimSpace(draft.PendingGroupId) != strings.TrimSpace(groupId) {
		return "verify_pending"
	}
	return "display"
}

func (s *sSysBot) acknowledgeProfileInput(ctx context.Context, sessionId int64, groupId string) (bool, error) {
	key := fmt.Sprintf("youban_bot:profile_session_ack:%d", sessionId)
	acknowledged := false
	err := s.withProfileMediaGroupLock(ctx, sessionId, groupId, func() error {
		value, err := cache.Instance().Get(ctx, key)
		if err != nil {
			return err
		}
		if value != nil && !value.IsNil() {
			return nil
		}
		if err = cache.Instance().Set(ctx, key, 1, profileMediaGroupCacheTTL); err != nil {
			return err
		}
		acknowledged = true
		return nil
	})
	return acknowledged, err
}

func profileMediaGroupLockKey(sessionId int64, groupId string) string {
	return "youban_bot:profile_group:lock:" + fmt.Sprintf("%d:%s", sessionId, strings.TrimSpace(groupId))
}

func (s *sSysBot) withProfileMediaGroupLock(ctx context.Context, sessionId int64, groupId string, fn func() error) error {
	lockCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	lock := hglock.NewConfig(15*time.Second, 100*time.Millisecond).Mutex(profileMediaGroupLockKey(sessionId, groupId))
	if err := lock.Lock(lockCtx); err != nil {
		return gerror.Wrap(err, "获取资料媒体组锁失败")
	}
	defer s.releaseProfileMediaGroupLock(lock)
	return fn()
}

func (s *sSysBot) releaseProfileMediaGroupLock(lock *hglock.Lock) {
	if lock != nil {
		_ = lock.Unlock(context.Background())
	}
}

func (s *sSysBot) startProfileSessionRecovery(ctx context.Context) {
	s.stopProfileSessionRecovery()
	recoveryCtx, cancel := context.WithCancel(ctx)
	s.profileRecoveryMu.Lock()
	s.profileRecoveryCancel = cancel
	s.profileRecoveryMu.Unlock()
	go func() {
		s.recoverPendingProfileMediaGroups(recoveryCtx)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-recoveryCtx.Done():
				return
			case <-ticker.C:
				s.recoverPendingProfileMediaGroups(recoveryCtx)
			}
		}
	}()
}

func (s *sSysBot) stopProfileSessionRecovery() {
	s.profileRecoveryMu.Lock()
	cancel := s.profileRecoveryCancel
	s.profileRecoveryCancel = nil
	s.profileRecoveryMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *sSysBot) recoverPendingProfileMediaGroups(ctx context.Context) {
	var sessions []profileSessionRow
	err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).
		Fields("id,bot_id,scene,step,status,payload_json,tenant_id,account_id,app").
		Where("status", profileSessionStatusActive).
		WhereIn("step", []string{"waiting_display", "waiting_verify"}).
		WhereLTE("updated_at", gtime.Now().Add(-profileMediaGroupDebounce)).
		WhereGT("expires_at", gtime.Now()).
		OrderAsc("updated_at").
		Limit(20).
		Scan(&sessions)
	if err != nil {
		g.Log().Warningf(ctx, "恢复Bot资料媒体组查询失败 err:%+v", err)
		return
	}
	for index := range sessions {
		session := &sessions[index]
		draft := decodeProfileCreateDraft(session.PayloadJson)
		groupId := strings.TrimSpace(draft.PendingGroupId)
		displayReady := groupId != "" && draft.PendingGroup != nil && profileMediaGroupIdleWait(*draft.PendingGroup, time.Now()) == 0
		verifyReady := session.Step == "waiting_verify" && draft.PendingVerify != nil && profileMediaGroupIdleWait(*draft.PendingVerify, time.Now()) == 0
		if !displayReady && !verifyReady {
			continue
		}
		if session.AccountId > 0 {
			var accountRow struct {
				AccountType string `json:"account_type"`
			}
			_ = g.DB().Model(publishAccountTable).Safe().Ctx(ctx).Fields("account_type").Where("id", session.AccountId).WhereNull("deleted_at").Scan(&accountRow)
			session.AccountType = accountRow.AccountType
		}
		account := &botProfileAccount{TenantId: session.TenantId, AccountId: session.AccountId, AccountType: session.AccountType, App: session.App}
		if displayReady {
			g.Log().Infof(ctx, "恢复Bot资料展示组收尾 trace:PF-%d groupId:%s media_count:%d", session.Id, groupId, len(draft.PendingGroup.Media))
			go s.finishProfileMediaGroup(session.BotId, groupId, account, session)
			continue
		}
		g.Log().Infof(ctx, "恢复Bot资料验证组收尾 trace:PF-%d media_count:%d", session.Id, len(draft.PendingVerify.Media))
		go s.finishPendingProfileVerifyGroup(session.BotId, account, session.Id)
	}
}

func (s *sSysBot) finishProfileMediaGroup(botId int64, groupId string, account *botProfileAccount, session *profileSessionRow) {
	ctx := context.Background()
	trace := fmt.Sprintf("PF-%d", session.Id)
	startedAt := time.Now()
	defer func() {
		if recovered := recover(); recovered != nil {
			g.Log().Errorf(ctx, "Bot资料媒体组收尾发生未捕获异常 trace:%s groupId:%s elapsed_ms:%d panic:%v", trace, groupId, time.Since(startedAt).Milliseconds(), recovered)
		}
	}()
	g.Log().Infof(ctx, "Bot资料媒体组收尾开始 trace:%s groupId:%s", trace, groupId)
	var pending profilePendingMediaGroup
	for {
		found := false
		wait := time.Duration(0)
		err := s.withProfileMediaGroupLock(ctx, session.Id, groupId, func() error {
			var durable profileSessionRow
			if err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Fields("id,step,status,payload_json").Where("id", session.Id).Where("status", profileSessionStatusActive).Scan(&durable); err != nil {
				return err
			}
			if durable.Id == 0 {
				return nil
			}
			draft := decodeProfileCreateDraft(durable.PayloadJson)
			if draft.PendingGroupId != groupId || draft.PendingGroup == nil {
				return nil
			}
			pending = *draft.PendingGroup
			found = true
			wait = profileMediaGroupIdleWait(pending, time.Now())
			return nil
		})
		if err != nil {
			g.Log().Errorf(ctx, "Bot资料媒体组读取会话快照失败 trace:%s groupId:%s err:%+v", trace, groupId, err)
			return
		}
		if !found {
			g.Log().Infof(ctx, "Bot资料媒体组收尾跳过，会话快照已变化 trace:%s groupId:%s", trace, groupId)
			return
		}
		if wait <= 0 {
			break
		}
		time.Sleep(wait)
	}
	g.Log().Infof(ctx, "Bot资料媒体组开始从原消息重建 trace:%s groupId:%s snapshot_media_count:%d", trace, groupId, len(pending.Media))
	if stored, storedText, rebuildErr := s.rebuildProfileMediaGroupFromStoredMessages(ctx, botId, pending.ChatId, groupId); rebuildErr != nil {
		g.Log().Errorf(ctx, "Bot资料媒体组从原消息重建失败 trace:%s groupId:%s err:%+v", trace, groupId, rebuildErr)
	} else if len(stored) > 0 {
		pending.Media = stored
		if strings.TrimSpace(storedText) != "" {
			pending.Text = storedText
		}
	}
	g.Log().Infof(ctx, "Bot资料媒体组原消息重建完成 trace:%s groupId:%s media_count:%d text_len:%d", trace, groupId, len(pending.Media), len(strings.TrimSpace(pending.Text)))
	lock := hglock.NewConfig(20*time.Second, 100*time.Millisecond).Mutex(profileSessionLockKey(session.Id))
	if err := lock.Lock(ctx); err != nil {
		g.Log().Errorf(ctx, "Bot资料媒体组获取会话顺序锁失败 trace:%s groupId:%s err:%+v", trace, groupId, err)
		return
	}
	defer func() { _ = lock.Unlock(context.Background()) }()
	g.Log().Infof(ctx, "Bot资料媒体组已获取会话顺序锁 trace:%s groupId:%s", trace, groupId)
	var current *profileSessionRow
	if scanErr := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Where("id", session.Id).Where("status", profileSessionStatusActive).WhereGT("expires_at", gtime.Now()).Scan(&current); scanErr != nil || current == nil || current.Id <= 0 {
		g.Log().Errorf(ctx, "Bot资料媒体组读取最终会话失败 trace:%s groupId:%s err:%+v current_nil:%t", trace, groupId, scanErr, current == nil)
		return
	}
	g.Log().Infof(ctx, "Bot资料媒体组最终会话已读取 trace:%s groupId:%s step:%s status:%s", trace, groupId, current.Step, current.Status)
	if len(pending.Media) == 0 && strings.TrimSpace(pending.Text) == "" {
		g.Log().Errorf(ctx, "Bot资料媒体组收尾数据为空 trace:%s groupId:%s", trace, groupId)
		return
	}
	pending.Media = orderedProfileGroupMedia(pending.Media)
	if pending.Purpose == "verify" && current.Step != "waiting_verify" {
		g.Log().Infof(ctx, "Bot资料验证媒体组等待展示资料完成 trace:PF-%d groupId:%s step:%s", session.Id, groupId, current.Step)
		return
	}
	if pending.Purpose == "display" && current.Step != "waiting_display" {
		g.Log().Infof(ctx, "Bot资料展示媒体组已处理，忽略重复收尾 trace:PF-%d groupId:%s step:%s", session.Id, groupId, current.Step)
		return
	}
	if consumeErr := s.consumeProfileCreatePart(ctx, botId, pending.ChatId, account, current, pending.Text, pending.Media); consumeErr != nil {
		g.Log().Errorf(ctx, "Bot资料媒体组消费失败 trace:%s groupId:%s step:%s media_count:%d text_len:%d err:%+v", trace, groupId, current.Step, len(pending.Media), len(strings.TrimSpace(pending.Text)), consumeErr)
		_ = s.reply(ctx, botId, pending.ChatId, "资料媒体组解析失败："+html.EscapeString(consumeErr.Error()))
		return
	}
	g.Log().Infof(ctx, "Bot资料媒体组消费完成 trace:%s groupId:%s step:%s elapsed_ms:%d", trace, groupId, current.Step, time.Since(startedAt).Milliseconds())
	if pending.Purpose == "display" {
		if markErr := s.markProfileDisplayGroupCompleted(ctx, session.Id, groupId); markErr != nil {
			g.Log().Warningf(ctx, "Bot资料展示媒体组完成标记失败 trace:PF-%d groupId:%s err:%+v", session.Id, groupId, markErr)
		}
	}
	if clearErr := s.clearProfilePendingGroupSnapshot(ctx, session.Id); clearErr != nil {
		g.Log().Warningf(ctx, "Bot资料媒体组快照清理失败 trace:PF-%d groupId:%s err:%+v", session.Id, groupId, clearErr)
	}
	if pending.Purpose == "display" {
		go s.finishPendingProfileVerifyGroup(botId, account, session.Id)
	}
}

func (s *sSysBot) rebuildProfileMediaGroupFromStoredMessages(ctx context.Context, botId int64, chatId string, groupId string) ([]*publishsysin.MessageTemplateMediaInp, string, error) {
	if botId <= 0 || strings.TrimSpace(chatId) == "" || strings.TrimSpace(groupId) == "" {
		return nil, "", nil
	}
	var rows []struct {
		MessageId int    `json:"message_id"`
		RawJson   string `json:"raw_json"`
	}
	if err := g.DB().Model(messageTable).Safe().Ctx(ctx).Fields("message_id,raw_json").
		Where("bot_id", botId).Where("chat_id", strings.TrimSpace(chatId)).
		WhereLike("raw_json", "%"+strings.TrimSpace(groupId)+"%").
		OrderAsc("message_id").Limit(20).Scan(&rows); err != nil {
		return nil, "", err
	}
	if len(rows) == 0 {
		return nil, "", nil
	}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return nil, "", err
	}
	mediaByMessage := make([][]*publishsysin.MessageTemplateMediaInp, len(rows))
	textByMessage := make([]string, len(rows))
	var waitGroup sync.WaitGroup
	var firstErr error
	var errorMu sync.Mutex
	for index := range rows {
		index := index
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			var message models.Message
			if unmarshalErr := json.Unmarshal([]byte(rows[index].RawJson), &message); unmarshalErr != nil {
				errorMu.Lock()
				if firstErr == nil {
					firstErr = unmarshalErr
				}
				errorMu.Unlock()
				return
			}
			text := strings.TrimSpace(firstNonEmpty(message.Text, message.Caption))
			textByMessage[index] = text
			items, resolveErr := s.resolveTelegramMessageMedia(ctx, row.BotToken, &message)
			if resolveErr != nil {
				errorMu.Lock()
				if firstErr == nil {
					firstErr = resolveErr
				}
				errorMu.Unlock()
				return
			}
			for _, item := range items {
				if item != nil {
					item.SortIndex = rows[index].MessageId
				}
			}
			mediaByMessage[index] = items
		}()
	}
	waitGroup.Wait()
	if firstErr != nil {
		return nil, "", firstErr
	}
	var media []*publishsysin.MessageTemplateMediaInp
	var text string
	for index := range mediaByMessage {
		media = append(media, mediaByMessage[index]...)
		if text == "" && textByMessage[index] != "" {
			text = textByMessage[index]
		}
	}
	return orderedProfileGroupMedia(media), text, nil
}

func (s *sSysBot) finishPendingProfileVerifyGroup(botId int64, account *botProfileAccount, sessionId int64) {
	ctx := context.Background()
	defer func() {
		if recovered := recover(); recovered != nil {
			g.Log().Errorf(ctx, "Bot资料验证组收尾发生未捕获异常 trace:PF-%d panic:%v", sessionId, recovered)
		}
	}()
	for {
		var row profileSessionRow
		if err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Fields("id,step,status,payload_json").Where("id", sessionId).Where("status", profileSessionStatusActive).Scan(&row); err != nil || row.Id == 0 {
			return
		}
		draft := decodeProfileCreateDraft(row.PayloadJson)
		if draft.PendingVerify == nil {
			return
		}
		if wait := profileMediaGroupIdleWait(*draft.PendingVerify, time.Now()); wait > 0 {
			time.Sleep(wait)
			continue
		}
		lock := hglock.NewConfig(20*time.Second, 100*time.Millisecond).Mutex(profileSessionLockKey(sessionId))
		if err := lock.Lock(ctx); err != nil {
			g.Log().Warningf(ctx, "Bot资料验证组获取会话锁失败 trace:PF-%d err:%+v", sessionId, err)
			return
		}
		var latest profileSessionRow
		err := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Fields("id,step,status,payload_json").Where("id", sessionId).Where("status", profileSessionStatusActive).Scan(&latest)
		if err == nil && latest.Id > 0 && latest.Step == "waiting_verify" {
			latestDraft := decodeProfileCreateDraft(latest.PayloadJson)
			verify := latestDraft.PendingVerify
			if verify != nil && profileMediaGroupIdleWait(*verify, time.Now()) == 0 {
				verify.Media = orderedProfileGroupMedia(verify.Media)
				latestDraft.PendingVerify = nil
				_ = s.updateProfileSession(ctx, sessionId, latest.Step, latestDraft)
				latest.PayloadJson = gjson.MustEncodeString(latestDraft)
				err = s.consumeProfileCreatePart(ctx, botId, verify.ChatId, account, &latest, verify.Text, verify.Media)
			}
		}
		_ = lock.Unlock(context.Background())
		if err != nil {
			g.Log().Warningf(ctx, "Bot资料验证组处理失败 trace:PF-%d err:%+v", sessionId, err)
		}
		return
	}
}

func profileMediaGroupIdleWait(pending profilePendingMediaGroup, now time.Time) time.Duration {
	lastMessageAt := time.Unix(0, pending.UpdatedAt)
	if pending.UpdatedAt <= 0 {
		lastMessageAt = time.Unix(pending.CreatedAt, 0)
	}
	idle := now.Sub(lastMessageAt)
	if idle >= profileMediaGroupDebounce {
		return 0
	}
	if idle < 0 {
		return profileMediaGroupDebounce
	}
	return profileMediaGroupDebounce - idle
}

func appendProfileGroupMedia(current []*publishsysin.MessageTemplateMediaInp, messageId int, incoming []*publishsysin.MessageTemplateMediaInp) []*publishsysin.MessageTemplateMediaInp {
	seen := make(map[string]struct{}, len(current)+len(incoming))
	for _, item := range current {
		if item != nil {
			seen[profileDraftMediaIdentity(item)] = struct{}{}
		}
	}
	for _, item := range incoming {
		if item == nil {
			continue
		}
		identity := profileDraftMediaIdentity(item)
		if _, exists := seen[identity]; exists {
			continue
		}
		item.SortIndex = messageId
		current = append(current, item)
		seen[identity] = struct{}{}
	}
	return current
}

func orderedProfileGroupMedia(media []*publishsysin.MessageTemplateMediaInp) []*publishsysin.MessageTemplateMediaInp {
	ordered := append([]*publishsysin.MessageTemplateMediaInp(nil), media...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i] == nil {
			return false
		}
		if ordered[j] == nil {
			return true
		}
		return ordered[i].SortIndex < ordered[j].SortIndex
	})
	result := ordered[:0]
	for _, item := range ordered {
		if item == nil {
			continue
		}
		item.SortIndex = len(result) + 1
		result = append(result, item)
	}
	return result
}

func profileDraftMediaIdentity(media *publishsysin.MessageTemplateMediaInp) string {
	if media == nil {
		return ""
	}
	return strings.Join([]string{strings.TrimSpace(media.MediaType), strings.TrimSpace(media.TgFileId), strings.TrimSpace(media.FileUrl), strings.TrimSpace(media.Name)}, "|")
}

type inlineShareRow struct {
	Token     string `json:"token"`
	ProfileNo string `json:"profile_no"`
	TenantId  int64  `json:"tenant_id"`
	AccountId int64  `json:"account_id"`
}

func (s *sSysBot) replyInlineShareButton(ctx context.Context, botId int64, chatId string, account *botProfileAccount, no string) error {
	token, err := s.ensureInlineShare(ctx, botId, account, no)
	if err != nil {
		return s.replyBotError(ctx, botId, chatId, "资料管理", err)
	}
	query := token
	markup := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "选择聊天内联分享", SwitchInlineQuery: &query}, {Text: "当前聊天分享", SwitchInlineQueryCurrentChat: &query}}}}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	botUsername := strings.TrimPrefix(strings.TrimSpace(row.BotUsername), "@")
	if botUsername == "" {
		botUsername = "bot"
	}
	text := fmt.Sprintf("└内联分享: @%s <code>%s</code>", html.EscapeString(botUsername), html.EscapeString(token))
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, text, "HTML", false, markup)
	return err
}

func (s *sSysBot) ensureInlineShare(ctx context.Context, botId int64, account *botProfileAccount, no string) (string, error) {
	no = strings.ToUpper(strings.TrimSpace(no))
	if no == "" {
		return "", gerror.New("资料编号不能为空")
	}
	note, err := publishService.SysPublish().BotProfileView(ctx, &publishsysin.BotProfileViewInp{TenantId: account.TenantId, AccountId: botProfileScopeAccountId(account), ProfileNo: no})
	if err != nil {
		return "", err
	}
	token := "inline" + strings.ToUpper(grand.S(12))
	now := gtime.Now()
	_, err = g.DB().Model(profileInlineShareTable).Safe().Ctx(ctx).Data(g.Map{"bot_id": botId, "token": token, "profile_id": note.Id, "profile_no": note.ProfileNo, "telegram_user_id": "", "account_id": account.AccountId, "tenant_id": account.TenantId, "status": 1, "created_at": now, "updated_at": now}).Insert()
	if err != nil {
		return "", gerror.Wrap(err, "创建内联分享失败")
	}
	return token, nil
}

func (s *sSysBot) inlineShareByToken(ctx context.Context, token string) (*inlineShareRow, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil
	}
	var row *inlineShareRow
	err := g.DB().Model(profileInlineShareTable).Safe().Ctx(ctx).Where("token", token).Where("status", 1).WhereNull("deleted_at").Scan(&row)
	if err != nil {
		return nil, gerror.Wrap(err, "读取内联分享失败")
	}
	return row, nil
}

func (s *sSysBot) incrementInlineShareUsage(ctx context.Context, token string) error {
	_, err := g.DB().Model(profileInlineShareTable).Safe().Ctx(ctx).Where("token", token).Increment("usage_count", 1)
	return err
}

func profileCallbackData(action string, no string) string {
	return "pf:" + action + ":" + strings.ToUpper(no)
}

func (s *sSysBot) showProfileEditMenu(ctx context.Context, botId int64, chatId string, account *botProfileAccount, no string) error {
	note, err := publishService.SysPublish().BotProfileView(ctx, &publishsysin.BotProfileViewInp{TenantId: account.TenantId, AccountId: botProfileScopeAccountId(account), ProfileNo: no})
	if err != nil {
		return s.replyBotError(ctx, botId, chatId, "资料管理", err)
	}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	text := fmt.Sprintf("编辑笔记\n\n编号：<code>%s</code>\n标题：%s\n\n请选择要编辑的内容：", html.EscapeString(note.ProfileNo), html.EscapeString(note.Title))
	markup := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "编辑标题", CallbackData: "pf:edtitle:" + note.ProfileNo}, {Text: "编辑正文", CallbackData: "pf:edtext:" + note.ProfileNo}},
		{{Text: "编辑编号", CallbackData: "pf:edno:" + note.ProfileNo}, {Text: "重新覆盖资料", CallbackData: "pf:replace:" + note.ProfileNo}},
		{{Text: "返回", CallbackData: "pf:backview:" + note.ProfileNo}},
	}}
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, text, "HTML", false, markup)
	return err
}

func (s *sSysBot) editProfileBySession(ctx context.Context, botId int64, chatId string, account *botProfileAccount, no string, title string, plainText string, newNo string) error {
	note, err := publishService.SysPublish().BotProfileEdit(ctx, &publishsysin.BotProfileEditInp{TenantId: account.TenantId, AccountId: botProfileScopeAccountId(account), ProfileNo: no, Title: title, PlainText: plainText, NewNo: newNo})
	if err != nil {
		return s.replyBotError(ctx, botId, chatId, "资料管理", err)
	}
	return s.sendMessageOnly(ctx, botId, chatId, fmt.Sprintf("已保存。\n编号：<code>%s</code>\n标题：%s", html.EscapeString(note.ProfileNo), html.EscapeString(note.Title)))
}

func (s *sSysBot) cancelProfileQueue(ctx context.Context, botId int64, chatId string, account *botProfileAccount, nos []string) error {
	res, err := publishService.SysPublish().BotProfileCancelQueue(ctx, &publishsysin.BotProfileQueueCancelInp{TenantId: account.TenantId, AccountId: botProfileScopeAccountId(account), Nos: nos})
	if err != nil {
		return s.replyBotError(ctx, botId, chatId, "资料管理", err)
	}
	label := "当前账号全部资料"
	if len(nos) > 0 {
		label = strings.Join(nos, ", ")
	}
	return s.sendMessageOnly(ctx, botId, chatId, fmt.Sprintf("已取消 %s 的待推送队列：%d 条。", html.EscapeString(label), res.Cleared))
}

func (s *sSysBot) replyBotError(ctx context.Context, botId int64, chatId string, scope string, err error) error {
	if err == nil {
		return nil
	}
	g.Log().Warningf(ctx, "Bot业务处理失败 scope:%s botId:%d chatId:%s err:%+v", scope, botId, chatId, err)
	return s.sendMessageOnly(ctx, botId, chatId, err.Error())
}

func (s *sSysBot) handleChannelTextCommand(ctx context.Context, botId int64, msg *models.Message, account *botProfileAccount, text string) error {
	chatId := fmt.Sprintf("%d", msg.Chat.ID)
	if account.AccountType != "admin" {
		return s.sendMessageOnly(ctx, botId, chatId, "频道管理仅管理员可用。")
	}
	if strings.Contains(text, "设置") || strings.Contains(text, "循环") {
		return s.startProfileSession(ctx, botId, msg, "channel_cycle", "waiting_channel_cycle", 0, "", nil, "请发送循环设置：频道ID 开 7 09:00\n例如：12 开 7 09:00；关闭：12 关")
	}
	return s.showChannelList(ctx, botId, chatId, account, strings.TrimSpace(strings.ReplaceAll(text, "频道", "")))
}

func (s *sSysBot) handleChannelCallback(ctx context.Context, botId int64, chatId string, telegramUserId string, account *botProfileAccount, data string, query *models.CallbackQuery) (bool, error) {
	if account.AccountType != "admin" {
		return true, s.sendMessageOnly(ctx, botId, chatId, "频道管理仅管理员可用。")
	}
	parts := strings.Split(data, ":")
	if len(parts) < 2 || parts[0] != "ch" {
		return false, nil
	}
	action := parts[1]
	channelId := int64(0)
	if len(parts) > 2 {
		channelId = parseInt64(parts[2])
	}
	switch action {
	case "list":
		return true, s.showChannelList(ctx, botId, chatId, account, "")
	case "view":
		_ = s.cancelProfileSessionByIdsSilent(ctx, botId, telegramUserId, chatId)
		return true, s.showChannelDetail(ctx, botId, chatId, account, channelId)
	case "backprofile":
		_ = s.cancelProfileSessionByIdsSilent(ctx, botId, telegramUserId, chatId)
		return true, s.showProfileMenuToChat(ctx, botId, chatId, "已返回资料管理，请选择操作：")
	case "cycle":
		return true, s.startProfileSessionByIds(ctx, botId, telegramUserId, chatId, account, "channel_cycle", "waiting_channel_cycle", channelId, "", nil, "请直接发送循环设置：\n7 09:00\n\n也可以只发送 7，默认使用 09:00；发送“关”关闭循环。")
	case "toggle":
		return true, s.toggleChannelCycle(ctx, botId, chatId, account, channelId)
	case "start":
		res, err := publishService.SysPublish().BotChannelFullPush(ctx, &publishsysin.BotChannelActionInp{TenantId: account.TenantId, AccountId: account.AccountId, ChannelId: channelId})
		if err != nil {
			return true, s.replyBotError(ctx, botId, chatId, "频道触发循环推送", err)
		}
		return true, s.sendChannelResult(ctx, botId, chatId, fmt.Sprintf("已触发当前频道循环推送。\n本次预计新增队列：%d 条。\n触发前当前频道已有未完成队列：%d 条。\n如果点击取消，会取消当前频道全部未完成发送队列。", res.Queued, res.ExistingQueue), channelId)
	case "clear":
		return true, s.confirmChannelClearQueue(ctx, botId, chatId, channelId, false)
	case "clearok":
		res, err := publishService.SysPublish().BotChannelClearQueue(ctx, &publishsysin.BotChannelActionInp{TenantId: account.TenantId, AccountId: account.AccountId, ChannelId: channelId})
		if err != nil {
			return true, s.replyBotError(ctx, botId, chatId, "频道取消发送队列", err)
		}
		return true, s.sendChannelResult(ctx, botId, chatId, fmt.Sprintf("已取消当前频道全部未完成发送队列：%d 条，发送中：%d 条。", res.Cleared, res.Sending), channelId)
	case "clearall":
		return true, s.confirmChannelClearQueue(ctx, botId, chatId, 0, true)
	case "clearallok":
		res, err := publishService.SysPublish().BotChannelClearQueue(ctx, &publishsysin.BotChannelActionInp{TenantId: account.TenantId, AccountId: account.AccountId})
		if err != nil {
			return true, s.replyBotError(ctx, botId, chatId, "频道取消全部发送队列", err)
		}
		return true, s.sendChannelResult(ctx, botId, chatId, fmt.Sprintf("已取消全部频道发送队列：%d 条，发送中：%d 条。", res.Cleared, res.Sending), 0)
	}
	return true, nil
}

func (s *sSysBot) showChannelList(ctx context.Context, botId int64, chatId string, account *botProfileAccount, keyword string) error {
	in := &publishsysin.ChannelListInp{TenantId: account.TenantId, Keyword: strings.TrimSpace(keyword)}
	in.Page = 1
	in.PerPage = 8
	list, total, err := publishService.SysPublish().BotChannelList(ctx, in)
	if err != nil {
		return s.replyBotError(ctx, botId, chatId, "资料管理", err)
	}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		markup := channelBackMarkup("ch:backprofile")
		_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, "频道管理\n\n未找到频道。", "HTML", false, markup)
		return err
	}
	buttons := make([][]models.InlineKeyboardButton, 0, len(list)+2)
	lines := []string{"<b>频道管理</b>", "请选择一个频道进入编辑。"}
	if strings.TrimSpace(keyword) != "" {
		lines = append(lines, "关键词：<code>"+html.EscapeString(strings.TrimSpace(keyword))+"</code>")
	}
	lines = append(lines, fmt.Sprintf("共 %d 个频道，当前显示前 %d 个。", total, len(list)))
	for _, ch := range list {
		if ch == nil {
			continue
		}
		state := "关"
		if ch.CyclePublishEnabled == 1 {
			state = "开"
		}
		name := strings.TrimSpace(ch.ChannelTitle)
		if name == "" {
			name = ch.TargetChatId
		}
		label := fmt.Sprintf("%s · 循环%s", shortButtonText(name, 28), state)
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: label, CallbackData: fmt.Sprintf("ch:view:%d", ch.Id)}})
	}
	buttons = append(buttons, []models.InlineKeyboardButton{{Text: "取消全部频道未完成队列", CallbackData: "ch:clearall"}})
	buttons = append(buttons, []models.InlineKeyboardButton{{Text: "刷新", CallbackData: "ch:list"}, {Text: "返回资料管理", CallbackData: "ch:backprofile"}})
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, strings.Join(lines, "\n"), "HTML", false, &models.InlineKeyboardMarkup{InlineKeyboard: buttons})
	return err
}

func (s *sSysBot) showChannelDetail(ctx context.Context, botId int64, chatId string, account *botProfileAccount, channelId int64) error {
	ch, err := s.botChannelById(ctx, account, channelId)
	if err != nil {
		return s.replyBotError(ctx, botId, chatId, "资料管理", err)
	}
	if ch == nil {
		return s.sendMessageOnly(ctx, botId, chatId, "频道不存在、无权操作，或不是启用中的上架频道。")
	}
	days := ch.CyclePublishDays
	if days <= 0 {
		days = 7
	}
	publishTime := strings.TrimSpace(ch.CyclePublishTime)
	if publishTime == "" {
		publishTime = "09:00"
	}
	cycle := fmt.Sprintf("关闭（每 %d 天 %s）", days, publishTime)
	toggleText := "开启循环"
	if ch.CyclePublishEnabled == 1 {
		cycle = fmt.Sprintf("开启，每 %d 天 %s", days, publishTime)
		toggleText = "关闭循环"
	}
	status := "停用"
	if ch.Status == 1 {
		status = "启用"
	}
	direction := channelDirectionLabel(ch.PublishDirection)
	text := fmt.Sprintf("<b>频道编辑</b>\n\n频道：%s\nID：<code>%d</code>\nChat：<code>%s</code>\n类型：%s\n状态：%s\n循环：%s\n循环时间：每 %d 天 %s", html.EscapeString(ch.ChannelTitle), ch.Id, html.EscapeString(ch.TargetChatId), html.EscapeString(direction), html.EscapeString(status), html.EscapeString(cycle), days, html.EscapeString(publishTime))
	markup := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: toggleText, CallbackData: fmt.Sprintf("ch:toggle:%d", ch.Id)}, {Text: "修改循环设置", CallbackData: fmt.Sprintf("ch:cycle:%d", ch.Id)}},
		{{Text: "触发循环推送", CallbackData: fmt.Sprintf("ch:start:%d", ch.Id)}},
		{{Text: "取消当前频道全部未完成队列", CallbackData: fmt.Sprintf("ch:clear:%d", ch.Id)}},
		{{Text: "返回频道列表", CallbackData: "ch:list"}, {Text: "返回资料管理", CallbackData: "ch:backprofile"}},
	}}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, text, "HTML", false, markup)
	return err
}

func (s *sSysBot) botChannelById(ctx context.Context, account *botProfileAccount, channelId int64) (*publishsysin.ChannelModel, error) {
	if channelId <= 0 {
		return nil, nil
	}
	in := &publishsysin.ChannelListInp{TenantId: account.TenantId}
	in.Page = 1
	in.PerPage = 100
	list, _, err := publishService.SysPublish().BotChannelList(ctx, in)
	if err != nil {
		return nil, err
	}
	for _, item := range list {
		if item != nil && item.Id == channelId {
			return item, nil
		}
	}
	return nil, nil
}

func (s *sSysBot) toggleChannelCycle(ctx context.Context, botId int64, chatId string, account *botProfileAccount, channelId int64) error {
	ch, err := s.botChannelById(ctx, account, channelId)
	if err != nil {
		return s.replyBotError(ctx, botId, chatId, "资料管理", err)
	}
	if ch == nil {
		return s.sendMessageOnly(ctx, botId, chatId, "频道不存在、无权操作，或不是启用中的上架频道。")
	}
	enabled := 1
	state := "开启"
	if ch.CyclePublishEnabled == 1 {
		enabled = 0
		state = "关闭"
	}
	days := ch.CyclePublishDays
	if days <= 0 {
		days = 7
	}
	publishTime := strings.TrimSpace(ch.CyclePublishTime)
	if publishTime == "" {
		publishTime = "09:00"
	}
	if err = publishService.SysPublish().BotChannelCycleSave(ctx, &publishsysin.BotChannelCycleSaveInp{TenantId: account.TenantId, AccountId: account.AccountId, ChannelId: channelId, Enabled: enabled, Days: days, Time: publishTime}); err != nil {
		return s.replyBotError(ctx, botId, chatId, "资料管理", err)
	}
	return s.sendChannelResult(ctx, botId, chatId, fmt.Sprintf("频道循环已%s：每 %d 天 %s。", state, days, publishTime), channelId)
}

func (s *sSysBot) sendChannelResult(ctx context.Context, botId int64, chatId string, text string, channelId int64) error {
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	buttons := [][]models.InlineKeyboardButton{{{Text: "返回频道列表", CallbackData: "ch:list"}}}
	if channelId > 0 {
		buttons = [][]models.InlineKeyboardButton{{{Text: "返回频道", CallbackData: fmt.Sprintf("ch:view:%d", channelId)}, {Text: "返回频道列表", CallbackData: "ch:list"}}}
	}
	markup := &models.InlineKeyboardMarkup{InlineKeyboard: buttons}
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, html.EscapeString(text), "HTML", false, markup)
	return err
}

func (s *sSysBot) confirmChannelClearQueue(ctx context.Context, botId int64, chatId string, channelId int64, all bool) error {
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	text := "确认取消当前频道全部未完成发送队列？\n\n该操作会取消当前频道所有等待发送的队列，不只取消刚触发的部分。"
	buttons := [][]models.InlineKeyboardButton{}
	if all {
		text = "确认取消全部频道未完成发送队列？\n\n该操作会影响当前租户所有频道的等待发送队列。"
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: "确认取消全部", CallbackData: "ch:clearallok"}})
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: "返回频道管理", CallbackData: "ch:list"}})
	} else {
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: "确认取消当前频道", CallbackData: fmt.Sprintf("ch:clearok:%d", channelId)}})
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: "返回频道详情", CallbackData: fmt.Sprintf("ch:view:%d", channelId)}})
	}
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, html.EscapeString(text), "HTML", false, &models.InlineKeyboardMarkup{InlineKeyboard: buttons})
	return err
}

func channelBackMarkup(callbackData string) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "返回", CallbackData: callbackData}}}}
}

func channelDirectionLabel(direction string) string {
	switch strings.TrimSpace(direction) {
	case "up":
		return "上架频道"
	case "down":
		return "下架频道"
	case "backup":
		return "备份频道"
	default:
		return "未知"
	}
}

func shortButtonText(text string, limit int) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if limit <= 0 || len(runes) <= limit {
		return text
	}
	return string(runes[:limit-1]) + "…"
}

func (s *sSysBot) saveChannelCycleByText(ctx context.Context, botId int64, chatId string, account *botProfileAccount, sessionChannelId int64, text string) error {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return s.sendMessageOnly(ctx, botId, chatId, "格式不正确。请发送：7 09:00；或只发送 7；关闭发送：关")
	}
	channelId := sessionChannelId
	enabled := 1
	days := 7
	publishTime := "09:00"
	if channelId > 0 {
		first := strings.TrimSpace(fields[0])
		if isChannelCycleOffText(first) {
			enabled = 0
		} else {
			if isChannelCycleOnText(first) {
				if len(fields) >= 2 {
					if v := parseInt(fields[1]); v > 0 {
						days = v
					}
				}
				if len(fields) >= 3 {
					publishTime = normalizeChannelCycleTime(fields[2])
				}
			} else {
				if v := parseInt(first); v > 0 {
					days = v
				}
				if len(fields) >= 2 {
					publishTime = normalizeChannelCycleTime(fields[1])
				}
			}
		}
	} else {
		if len(fields) < 2 {
			return s.sendMessageOnly(ctx, botId, chatId, "格式不正确。请发送：频道ID 开 7 09:00；关闭：频道ID 关")
		}
		channelId = parseInt64(fields[0])
		if channelId <= 0 {
			return s.sendMessageOnly(ctx, botId, chatId, "频道ID不正确。")
		}
		if isChannelCycleOffText(fields[1]) {
			enabled = 0
		} else if !isChannelCycleOnText(fields[1]) {
			return s.sendMessageOnly(ctx, botId, chatId, "格式不正确。请发送：频道ID 开 7 09:00；关闭：频道ID 关")
		}
		if len(fields) >= 3 {
			if v := parseInt(fields[2]); v > 0 {
				days = v
			}
		}
		if len(fields) >= 4 {
			publishTime = normalizeChannelCycleTime(fields[3])
		}
	}
	if err := publishService.SysPublish().BotChannelCycleSave(ctx, &publishsysin.BotChannelCycleSaveInp{TenantId: account.TenantId, AccountId: account.AccountId, ChannelId: channelId, Enabled: enabled, Days: days, Time: publishTime}); err != nil {
		return s.replyBotError(ctx, botId, chatId, "资料管理", err)
	}
	state := "关闭"
	if enabled == 1 {
		state = fmt.Sprintf("开启，每 %d 天 %s", days, publishTime)
	}
	return s.sendChannelResult(ctx, botId, chatId, "频道循环设置已保存："+state, channelId)
}

func isChannelCycleOnText(text string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	return text == "开" || text == "开启" || text == "on" || text == "enable" || text == "enabled"
}

func isChannelCycleOffText(text string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	return text == "关" || text == "关闭" || text == "off" || text == "disable" || text == "disabled"
}

func normalizeChannelCycleTime(text string) string {
	text = strings.TrimSpace(text)
	parts := strings.Split(text, ":")
	if len(parts) != 2 {
		return text
	}
	hour := parseInt(parts[0])
	minute := parseInt(parts[1])
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return text
	}
	return fmt.Sprintf("%02d:%02d", hour, minute)
}

func parseInt64(text string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	return v
}

func parseInt(text string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(text))
	return v
}
