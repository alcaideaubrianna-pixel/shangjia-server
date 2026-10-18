package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/grand"

	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
	"hotgo/internal/library/cache"
	"hotgo/utility/scantrace"
)

const scanResultTTL = 10 * time.Minute

type scanResultState struct {
	BotId       int64   `json:"botId,omitempty"`
	TraceId     string  `json:"traceId,omitempty"`
	TenantId    int64   `json:"tenantId"`
	AccountId   int64   `json:"accountId"`
	AccountType string  `json:"accountType"`
	ProfileIds  []int64 `json:"profileIds"`
}

func (s *sSysBot) searchScanMediaAndReply(ctx context.Context, botId int64, chatId string, account *botProfileAccount, items []*publishsysin.BotMediaSearchItem) (resultErr error) {
	if account == nil || len(items) == 0 {
		return gerror.New("扫图搜索参数不完整")
	}
	searchStartedAt := time.Now()
	traceId := ensureScanTraceId(scanTraceId(ctx))
	ctx = withScanTrace(ctx, traceId)
	ctx = scantrace.With(ctx, g.Map{"botId": botId, "tenantId": account.TenantId, "accountId": account.AccountId, "items": len(items)})
	finish := scantrace.Begin(ctx, "request.total")
	defer func() { finish(resultErr) }()
	list, total, err := publishService.SysPublish().BotProfileMediaSearch(ctx, &publishsysin.BotMediaSearchInp{
		TenantId:    account.TenantId,
		AccountId:   account.AccountId,
		AccountType: account.AccountType,
		TraceId:     traceId,
		Items:       items,
		Threshold:   12,
	})
	observeScanStage(ctx, botId, "fingerprint_search", searchStartedAt, err)
	g.Log().Info(ctx, "Bot扫图搜索完成", g.Map{"traceId": traceId, "botId": botId, "accountId": account.AccountId, "durationMs": time.Since(searchStartedAt).Milliseconds(), "total": total, "err": err})
	if err != nil {
		step := scantrace.Begin(ctx, "reply.error")
		defer func() { step(resultErr) }()
		return s.replyBotError(ctx, botId, chatId, "扫图搜索", scanSearchUserError(traceId, err))
	}
	if len(list) == 0 {
		step := scantrace.Begin(ctx, "reply.empty")
		defer func() { step(resultErr) }()
		return s.sendMessageOnly(ctx, botId, chatId, fmt.Sprintf("未找到相似资料。\n（追踪号 %s）", traceId))
	}
	token := strings.ToUpper(grand.S(10))
	state := &scanResultState{BotId: botId, TraceId: traceId, TenantId: account.TenantId, AccountId: account.AccountId, AccountType: account.AccountType}
	buttons := make([][]models.InlineKeyboardButton, 0, len(list))
	sourceLinkTotal := 0
	for _, note := range list {
		if note == nil || note.Id <= 0 {
			continue
		}
		state.ProfileIds = append(state.ProfileIds, note.Id)
		accountName := firstNonEmpty(note.AccountName, note.Nickname, note.Username, "资料")
		label := fmt.Sprintf("[%s] %s", shortButtonText(accountName, 18), shortButtonText(note.ProfileNo, 16))
		if sourceName := profileSourceName(note); sourceName != "" {
			label += " [" + shortButtonText(sourceName, 16) + "]"
		}
		row := scanResultButtonRow(token, note, label)
		if len(row) > 1 {
			sourceLinkTotal++
		}
		buttons = append(buttons, row)
	}
	if len(state.ProfileIds) == 0 {
		return s.sendMessageOnly(ctx, botId, chatId, "未找到相似资料。")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	step := scantrace.Begin(ctx, "result.cache")
	err = cache.Instance().Set(ctx, scanResultKey(token), string(data), scanResultTTL)
	step(err)
	if err != nil {
		return err
	}
	text := fmt.Sprintf("找到 %d 条相似资料，请选择要查看的资料：", total)
	if total > len(state.ProfileIds) {
		text = fmt.Sprintf("找到 %d 条相似资料，最多展示 %d 条，请选择要查看的资料：", total, len(state.ProfileIds))
	}
	text += fmt.Sprintf("\n（追踪号 %s）", traceId)
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	g.Log().Info(ctx, "Bot扫图结果已组装", g.Map{
		"traceId": traceId, "botId": botId, "accountId": account.AccountId, "resultTotal": len(state.ProfileIds), "sourceLinkTotal": sourceLinkTotal,
	})
	step = scantrace.Begin(ctx, "reply.results")
	_, err = s.sendMessageWithMarkup(ctx, row.BotToken, chatId, html.EscapeString(text), "HTML", false, &models.InlineKeyboardMarkup{InlineKeyboard: buttons})
	step(err)
	return err
}

func scanSearchUserError(traceId string, err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(message, "context deadline exceeded") {
		return gerror.Newf("扫图搜索超时，请重新发送图片后再试。\n（追踪号 %s）", ensureScanTraceId(traceId))
	}
	return err
}

func scanResultButtonRow(token string, note *publishsysin.NoteModel, label string) []models.InlineKeyboardButton {
	row := []models.InlineKeyboardButton{{Text: label, CallbackData: fmt.Sprintf("scan:view:%s:%d", token, note.Id)}}
	if sourceURL := strings.TrimSpace(note.CollectSourceUrl); sourceURL != "" {
		row = append(row, models.InlineKeyboardButton{Text: "来源频道 >", URL: sourceURL})
	}
	return row
}

func scanResultKey(token string) string {
	return "youban_bot:scan_result:" + strings.TrimSpace(token)
}

func (s *sSysBot) handleScanCallback(ctx context.Context, botId int64, query *models.CallbackQuery) (bool, error) {
	if query == nil || !strings.HasPrefix(strings.TrimSpace(query.Data), "scan:view:") {
		return false, nil
	}
	if query.Message.Message == nil {
		return true, nil
	}
	chatId := fmt.Sprintf("%d", query.Message.Message.Chat.ID)
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
	parts := strings.Split(strings.TrimSpace(query.Data), ":")
	if len(parts) != 4 || parts[2] == "" {
		return true, s.sendMessageOnly(ctx, botId, chatId, "扫图结果已失效，请重新发送媒体。")
	}
	var state scanResultState
	value, err := cache.Instance().Get(ctx, scanResultKey(parts[2]))
	if err != nil || value == nil || value.IsNil() || json.Unmarshal([]byte(value.String()), &state) != nil {
		return true, s.sendMessageOnly(ctx, botId, fmt.Sprintf("%d", query.Message.Message.Chat.ID), "扫图结果已失效，请重新发送媒体。")
	}
	profileId := parseTelegramUserId(parts[3])
	if !containsInt64(state.ProfileIds, profileId) {
		return true, s.sendMessageOnly(ctx, botId, chatId, "扫图资料不存在，请重新发送媒体搜索。")
	}
	account, err := s.boundProfileAccountByUser(ctx, query.From.ID)
	if err != nil {
		return true, s.replyBotError(ctx, botId, chatId, "扫图预览", err)
	}
	if account.TenantId != state.TenantId || account.AccountId != state.AccountId || (state.BotId > 0 && state.BotId != botId) {
		return true, s.sendMessageOnly(ctx, botId, chatId, "扫图结果已失效，请重新发送媒体。")
	}
	ctx = withScanTrace(ctx, state.TraceId)
	ctx = scantrace.With(ctx, g.Map{"botId": botId, "tenantId": account.TenantId, "accountId": account.AccountId, "profileId": profileId})
	finish := scantrace.Begin(ctx, "result.view")
	var viewErr error
	defer func() { finish(viewErr) }()
	step := scantrace.Begin(ctx, "result.profile")
	note, err := publishService.SysPublish().BotProfileView(ctx, &publishsysin.BotProfileViewInp{TenantId: account.TenantId, AccountId: account.AccountId, AccountType: account.AccountType, ProfileId: profileId})
	step(err)
	viewErr = err
	if err != nil {
		return true, s.replyBotError(ctx, botId, chatId, "扫图预览", err)
	}
	// The management card must not wait for full albums/videos to be delivered.
	purpose := profileCardPurpose(account, note, "view")
	ctx = scantrace.With(ctx, g.Map{"cardPurpose": purpose, "accountType": account.AccountType, "profileAccountId": note.AccountId})
	step = scantrace.Begin(ctx, "result.card")
	viewErr = s.sendProfileCard(ctx, botId, chatId, note, purpose)
	step(viewErr)
	if viewErr != nil {
		return true, s.replyBotError(ctx, botId, chatId, "扫图预览", viewErr)
	}
	step = scantrace.Begin(ctx, "result.media")
	viewErr = s.sendScanProfileContent(ctx, botId, chatId, note)
	step(viewErr)
	return true, s.replyBotError(ctx, botId, chatId, "扫图预览", viewErr)
}

func (s *sSysBot) sendScanProfileContent(ctx context.Context, botId int64, chatId string, note *publishsysin.NoteModel) error {
	if note == nil {
		return gerror.New("资料不存在")
	}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return err
	}
	bot, err := s.telegramBot(ctx, row.BotToken)
	if err != nil {
		return err
	}
	callCtx, cancel := telegramMediaAPICtx()
	defer cancel()
	caption := profilePreviewDisplayCaption(note)
	if profileHasPurposeMedia(note.Media, "display") {
		if err = s.sendProfileMediaPurpose(ctx, callCtx, bot, chatId, note.Media, "display", caption, false); err != nil {
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
	_, err = s.sendMessage(ctx, row.BotToken, chatId, "预览完成。", "HTML", false)
	return err
}

func containsInt64(items []int64, value int64) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
