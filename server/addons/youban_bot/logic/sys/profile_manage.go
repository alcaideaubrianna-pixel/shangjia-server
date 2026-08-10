package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"regexp"
	"strconv"
	"strings"
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
	profileMediaGroupDebounce   = 3 * time.Minute
	profileMediaGroupCacheTTL   = 10 * time.Minute
)

var profileNoFindRegexp = regexp.MustCompile(`(?i)\b[A-Z][A-Z0-9]{4,}\b`)

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
			return true, bot.showProfileCard(ctx, event.BotId, chatID, account, nos[0])
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
	if nos := extractProfileNos(text); len(nos) == 1 && strings.EqualFold(text, nos[0]) {
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
	return profileNoFindRegexp.MatchString(text)
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
		return true, s.showProfileCard(ctx, botId, chatId, account, nos[0])
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
	var row struct {
		Id       int64  `json:"id"`
		SerialNo string `json:"serial_no"`
		Name     string `json:"name"`
		Text     string `json:"text"`
		Status   int    `json:"status"`
	}
	if err := g.DB().Model("hg_youban_publish_message_template").Safe().Ctx(ctx).
		Where("serial_no", strings.ToUpper(strings.TrimSpace(serial))).Where("status", 1).WhereNull("deleted_at").Scan(&row); err != nil {
		return err
	}
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
		mediaErr := g.DB().Model("hg_youban_publish_message_media").Safe().Ctx(ctx).
			Where("template_id", row.Id).
			OrderAsc("sort_index").
			Limit(2).
			Scan(&mediaRows)
		if mediaErr != nil {
			return mediaErr
		}
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
		if mediaCount == 1 && strings.EqualFold(strings.TrimSpace(media.MediaType), "image") {
			cachedPhoto, cachedErr := s.templateInlineCachedPhoto(ctx, botId, media.SourceMessageRecordID, media.TgFileID)
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
			})
		}
	}
	botRow, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	bot, err := s.telegramBot(ctx, botRow.BotToken)
	if err != nil {
		return err
	}
	callCtx, cancel := telegramAPICtx()
	defer cancel()
	_, err = bot.AnswerInlineQuery(callCtx, &tgbot.AnswerInlineQueryParams{InlineQueryID: query.ID, Results: results, CacheTime: 0, IsPersonal: false})
	return err
}

type templateInlineCachedPhoto struct {
	FileID          string
	Caption         string
	CaptionEntities []models.MessageEntity
}

func (s *sSysBot) templateInlineCachedPhoto(ctx context.Context, botId int64, sourceMessageRecordId int64, fallbackFileID string) (*templateInlineCachedPhoto, error) {
	if strings.TrimSpace(fallbackFileID) != "" {
		return &templateInlineCachedPhoto{FileID: strings.TrimSpace(fallbackFileID)}, nil
	}
	if botId <= 0 || sourceMessageRecordId <= 0 {
		return nil, nil
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
	return decodeTemplateInlineCachedPhoto(row.RawJSON), nil
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
	in := &publishsysin.BotProfileSearchInp{TenantId: account.TenantId, Keyword: keyword}
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
	if profileHasPurposeMedia(note.Media, "display") {
		if err = s.sendProfileMediaPurpose(ctx, callCtx, bot, chatId, note.Media, "display", displayCaption, false); err != nil {
			return err
		}
	} else if strings.TrimSpace(note.PlainText) != "" {
		if _, err = s.sendMessage(ctx, row.BotToken, chatId, profileShareText(note), "HTML", false); err != nil {
			return err
		}
	}
	if profileHasPurposeMedia(note.Media, "verify") {
		if err = s.sendProfileMediaPurpose(ctx, callCtx, bot, chatId, note.Media, "verify", "验证资料", false); err != nil {
			return err
		}
	}
	return s.sendMessageOnly(ctx, botId, chatId, "预览完成。")
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
		return "第 2 步/共 2 步：请发送【验证资料】（支持文本、图片、视频或图文媒体组）。没有验证资料可点击“跳过验证资料”。"
	}
	return "第 1 步/共 2 步：请发送【展示资料】（支持文本、图片、视频或图文媒体组）。"
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
	DisplayText  string                                  `json:"displayText"`
	DisplayMedia []*publishsysin.MessageTemplateMediaInp `json:"displayMedia"`
	VerifyText   string                                  `json:"verifyText"`
	VerifyMedia  []*publishsysin.MessageTemplateMediaInp `json:"verifyMedia"`
}

type profilePendingMediaGroup struct {
	SessionId int64                                   `json:"sessionId"`
	BotId     int64                                   `json:"botId"`
	ChatId    string                                  `json:"chatId"`
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
			if ackErr = s.sendMessageOnly(ctx, botId, chatId, "已收到，正在处理，请稍候..."); ackErr != nil {
				g.Log().Warningf(ctx, "发送资料处理中反馈失败 sessionId:%d groupId:%s err:%+v", session.Id, msg.MediaGroupID, ackErr)
			}
		}
	}
	media, err := s.resolveTelegramMessageMedia(ctx, row.BotToken, msg)
	if err != nil {
		return err
	}
	if strings.TrimSpace(msg.MediaGroupID) != "" && len(media) > 0 {
		return s.collectProfileMediaGroup(ctx, row.BotToken, account, session, msg, text, media)
	}
	return s.consumeProfileCreatePart(ctx, botId, chatId, account, session, strings.TrimSpace(text), media)
}

func (s *sSysBot) consumeProfileCreatePart(ctx context.Context, botId int64, chatId string, account *botProfileAccount, session *profileSessionRow, text string, media []*publishsysin.MessageTemplateMediaInp) error {
	draft := decodeProfileCreateDraft(session.PayloadJson)
	switch session.Step {
	case "waiting_display":
		if strings.TrimSpace(text) == "" && len(media) == 0 {
			return s.sendProfileCreateStepPrompt(ctx, botId, chatId, "waiting_display")
		}
		draft.DisplayText = strings.TrimSpace(text)
		draft.DisplayMedia = media
		claimed, err := s.claimProfileSessionStep(ctx, session.Id, "waiting_display", "waiting_verify", draft)
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
		if err := s.sendMessageOnly(ctx, botId, chatId, "已收到展示资料。"); err != nil {
			return err
		}
		return s.sendProfileCreateStepPrompt(ctx, botId, chatId, "waiting_verify")
	case "waiting_verify":
		if strings.TrimSpace(text) == "跳过" || strings.EqualFold(strings.TrimSpace(text), "skip") {
			text = ""
			media = nil
		} else if strings.TrimSpace(text) == "" && len(media) == 0 {
			return s.sendProfileCreateStepPrompt(ctx, botId, chatId, "waiting_verify")
		}
		draft.VerifyText = strings.TrimSpace(text)
		draft.VerifyMedia = media
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
			_ = s.resetProfileSessionStep(ctx, session.Id, "saving", "waiting_verify", draft)
			if replyErr := s.replyBotError(ctx, botId, chatId, "资料管理", err); replyErr != nil {
				return replyErr
			}
			return s.sendProfileCreateStepPrompt(ctx, botId, chatId, "waiting_verify")
		}
		_ = s.completeProfileSession(ctx, session.Id)
		return s.sendProfileCreateSuccess(ctx, botId, chatId, res)
	}
	return nil
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

func (s *sSysBot) collectProfileMediaGroup(ctx context.Context, botToken string, account *botProfileAccount, session *profileSessionRow, msg *models.Message, text string, media []*publishsysin.MessageTemplateMediaInp) error {
	groupId := strings.TrimSpace(msg.MediaGroupID)
	if groupId == "" || len(media) == 0 {
		return nil
	}
	key := profileMediaGroupKey(session.Id, groupId)
	shouldFinalize := false
	if err := s.withProfileMediaGroupLock(ctx, session.Id, groupId, func() error {
		value, _ := cache.Instance().Get(ctx, key)
		pending := &profilePendingMediaGroup{}
		isFirst := true
		if value != nil && !value.IsNil() && strings.TrimSpace(value.String()) != "" {
			if err := json.Unmarshal([]byte(value.String()), pending); err == nil && pending.SessionId == session.Id {
				isFirst = false
			}
		}
		now := time.Now()
		if isFirst {
			pending = &profilePendingMediaGroup{SessionId: session.Id, BotId: session.BotId, ChatId: fmt.Sprintf("%d", msg.Chat.ID), CreatedAt: now.Unix()}
		}
		pending.UpdatedAt = now.UnixNano()
		if strings.TrimSpace(pending.Text) == "" {
			pending.Text = strings.TrimSpace(text)
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
			return gerror.Wrap(err, "保存资料媒体组失败")
		}
		if err = cache.Instance().Set(ctx, key, string(bs), profileMediaGroupCacheTTL); err != nil {
			return err
		}
		shouldFinalize = isFirst
		return nil
	}); err != nil {
		return err
	}
	if shouldFinalize {
		go s.finishProfileMediaGroup(session.BotId, groupId, account, session)
	}
	return nil
}

func profileMediaGroupKey(sessionId int64, groupId string) string {
	return fmt.Sprintf("youban_bot:profile_group:%d:%s", sessionId, strings.TrimSpace(groupId))
}

func profileMediaGroupAckKey(sessionId int64, groupId string) string {
	return fmt.Sprintf("youban_bot:profile_group_ack:%d:%s", sessionId, strings.TrimSpace(groupId))
}

func (s *sSysBot) acknowledgeProfileInput(ctx context.Context, sessionId int64, groupId string) (bool, error) {
	groupId = strings.TrimSpace(groupId)
	if groupId == "" {
		return true, nil
	}
	key := profileMediaGroupAckKey(sessionId, groupId)
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

func (s *sSysBot) finishProfileMediaGroup(botId int64, groupId string, account *botProfileAccount, session *profileSessionRow) {
	ctx := context.Background()
	key := profileMediaGroupKey(session.Id, groupId)
	var pending profilePendingMediaGroup
	for {
		found := false
		ready := false
		wait := time.Duration(0)
		err := s.withProfileMediaGroupLock(ctx, session.Id, groupId, func() error {
			value, err := cache.Instance().Get(ctx, key)
			if err != nil || value == nil || value.IsNil() {
				return err
			}
			if err = json.Unmarshal([]byte(value.String()), &pending); err != nil || pending.SessionId == 0 {
				return err
			}
			found = true
			wait = profileMediaGroupIdleWait(pending, time.Now())
			if wait > 0 {
				return nil
			}
			ready = true
			_, err = cache.Instance().Remove(ctx, key)
			return err
		})
		if err != nil || !found {
			return
		}
		if ready {
			break
		}
		time.Sleep(wait)
	}
	var current *profileSessionRow
	if scanErr := g.DB().Model(profileSessionTable).Safe().Ctx(ctx).Where("id", session.Id).Where("status", profileSessionStatusActive).WhereGT("expires_at", gtime.Now()).Scan(&current); scanErr != nil || current == nil || current.Id <= 0 {
		return
	}
	if len(pending.Media) == 0 && strings.TrimSpace(pending.Text) == "" {
		return
	}
	if consumeErr := s.consumeProfileCreatePart(ctx, botId, pending.ChatId, account, current, pending.Text, pending.Media); consumeErr != nil {
		_ = s.reply(ctx, botId, pending.ChatId, "资料媒体组解析失败："+html.EscapeString(consumeErr.Error()))
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
