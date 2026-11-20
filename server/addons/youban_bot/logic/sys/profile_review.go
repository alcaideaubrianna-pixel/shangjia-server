package sys

import (
	"context"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"

	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
)

func (s *sSysBot) submitCollectorProfileReview(ctx context.Context, state *collectorInteraction, chatID string, query *models.CallbackQuery, no string) error {
	if err := s.ensureUploaderReviewRecipient(ctx, state.bot.AccountId); err != nil {
		return s.replyBotError(ctx, state.botID, chatID, "提交上架审核", err)
	}
	applicant := strings.TrimSpace(state.user.FirstName + " " + state.user.LastName)
	if state.user.Username != "" {
		applicant = strings.TrimSpace(applicant + " @" + state.user.Username)
	}
	if err := ps.SysPublish().SubmitBotProfileReview(ctx, &pi.BotProfileReviewSubmit{
		BotID: state.bot.Id, TenantID: state.account.TenantId, AccountID: state.account.AccountId,
		ProfileNo: no, ChatID: chatID, Applicant: shortText(applicant, 256),
	}); err != nil {
		s.answerProfileCallback(ctx, state.botID, query, err.Error())
		return s.replyBotError(ctx, state.botID, chatID, "提交上架审核", err)
	}
	s.answerProfileCallback(ctx, state.botID, query, "已提交审核")
	return s.sendCollectorReply(ctx, state.botID, chatID, "资料已提交审核，审核通过后将上架。")
}

func (s *sSysBot) handleProfileReviewCallback(ctx context.Context, botID int64, query *models.CallbackQuery) (bool, error) {
	payload, matches := strings.CutPrefix(query.Data, "pur:")
	if !matches {
		return false, nil
	}
	msg := query.Message.Message
	if msg == nil || msg.Chat.Type != models.ChatTypePrivate || query.From.ID <= 0 || msg.Chat.ID != query.From.ID {
		return true, nil
	}
	official, err := s.officialBot(ctx)
	if err != nil || official.Id != botID {
		return true, err
	}
	action, rawID, ok := strings.Cut(payload, ":")
	id, err := strconv.ParseInt(rawID, 10, 64)
	if !ok || err != nil || id <= 0 {
		return true, gerror.New("审核操作无效")
	}
	chatID := strconv.FormatInt(query.From.ID, 10)
	bind, err := s.bindingByTelegram(ctx, "api", chatID)
	if err != nil {
		return true, err
	}
	if bind == nil {
		s.answerProfileCallback(ctx, botID, query, "当前账号无权审批")
		return true, nil
	}
	review, err := ps.SysPublish().HandleBotProfileReview(ctx, id, bind.AccountId, action)
	if err != nil {
		s.answerProfileCallback(ctx, botID, query, err.Error())
		return true, err
	}
	if action == "preview" {
		s.answerProfileCallback(ctx, botID, query, "正在加载预览")
		note, err := ps.SysPublish().BotProfileView(ctx, &pi.BotProfileViewInp{
			TenantId: review.TenantId, AccountId: bind.AccountId, AccountIds: []int64{review.AccountId},
			AccountType: pi.PublishAccountTypeAdmin, ProfileId: review.ProfileId,
		})
		if err != nil {
			return true, err
		}
		if note == nil || note.Id != review.ProfileId || note.AccountId != review.AccountId || note.TenantId != review.TenantId {
			return true, gerror.New("资料不存在或无权操作")
		}
		return true, s.sendProfileContent(ctx, botID, chatID, note)
	}
	text := "已拒绝上架"
	if action == "approve" {
		text = "已同意，资料已加入上架队列"
	}
	s.answerProfileCallback(ctx, botID, query, text)
	return true, nil
}

func collectorOwnedNoteMarkup(note *pi.NoteModel, requireReview bool) *models.InlineKeyboardMarkup {
	no := strings.ToUpper(strings.TrimSpace(note.ProfileNo))
	markup := &models.InlineKeyboardMarkup{}
	button := models.InlineKeyboardButton{Text: "上架", CallbackData: "pf:publishup:" + no}
	switch {
	case note.Status == pi.ProfileStatusReviewPending:
		button = models.InlineKeyboardButton{Text: "审核中", CallbackData: "pf:view:" + no}
	case profileTaskInProgress(note):
		button = models.InlineKeyboardButton{Text: "处理中", CallbackData: "pf:view:" + no}
	case note.Status == 1:
		button = models.InlineKeyboardButton{Text: "下架", CallbackData: "pf:down:" + no}
	case requireReview:
		button.Text = "提交审核"
	}
	markup.InlineKeyboard = [][]models.InlineKeyboardButton{
		{button},
		{{Text: "删除", CallbackData: "pf:del:" + no}, {Text: "编辑", CallbackData: "pf:edit:" + no}},
		{{Text: "预览", CallbackData: "pf:preview:" + no}},
		{{Text: "返回列表", CallbackData: "pf:list:1"}},
	}
	return markup
}

func (s *sSysBot) DeliverBotProfileApproval(ctx context.Context, review *pi.BotProfileReview) error {
	bot, err := collectorBotByID(ctx, review.BotId)
	if err != nil {
		return err
	}
	if bot.TenantId != review.TenantId || bot.AccountId != review.OwnerAccountId {
		return nil
	}
	chatID, err := strconv.ParseInt(review.ChatId, 10, 64)
	if err != nil || chatID <= 0 {
		return gerror.New("上架审核通知的申请人私聊无效")
	}
	text := "资料审核已通过，已加入上架队列。\n标题：" + review.Title + "\n编号：" + review.ProfileNo
	_, err = s.sendMessageWithMarkup(ctx, bot.BotToken, review.ChatId, text, "", false, nil)
	return err
}
