package sys

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/lib/pq"

	"hotgo/addons/youban_bot/model/input/sysin"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
	"hotgo/internal/library/botregistry"
)

const (
	uploaderReviewTable    = "hg_youban_uploader_bot_review"
	uploaderPending        = "pending"
	uploaderApproved       = "approved"
	uploaderDirect         = "direct"
	uploaderRejected       = "rejected"
	uploaderReviewCallback = "upr:"
)

type uploaderReview struct {
	Id                  int64
	TenantId            int64
	BotId               int64
	OwnerAccountId      int64
	TelegramUserId      int64
	TelegramUsername    string
	TelegramDisplayName string
	Status              string
	AccountId           int64
}

func uploaderReviewByUser(ctx context.Context, bot *collectorBotRow, userID int64) (*uploaderReview, error) {
	var review *uploaderReview
	err := g.DB().Model(uploaderReviewTable).Ctx(ctx).Where("tenant_id", bot.TenantId).
		Where("bot_id", bot.Id).Where("telegram_user_id", userID).Scan(&review)
	return review, err
}

func (s *sSysBot) submitUploaderReview(ctx context.Context, bot *collectorBotRow, user *models.User) (*uploaderReview, error) {
	if err := uploaderReviewOwner(ctx, bot.TenantId, bot.AccountId); err != nil {
		return nil, err
	}
	if err := s.ensureUploaderReviewRecipient(ctx, bot.AccountId); err != nil {
		return nil, err
	}
	var review *uploaderReview
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		result, err := tx.Model(uploaderReviewTable).Data(g.Map{
			"tenant_id": bot.TenantId, "bot_id": bot.Id, "owner_account_id": bot.AccountId,
			"telegram_user_id": user.ID, "telegram_username": user.Username,
			"telegram_display_name": shortText(strings.TrimSpace(user.FirstName+" "+user.LastName), 256),
		}).InsertIgnore()
		if err != nil {
			return gerror.Wrap(err, "保存上架机器人注册申请失败")
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return err
		}
		review, err = uploaderReviewByUser(ctx, bot, user.ID)
		if err != nil {
			return err
		}
		if review == nil {
			return gerror.New("读取上架机器人注册申请失败")
		}
		if inserted == 0 {
			return nil
		}
		return ps.SysPublish().EnqueueUploaderReviewNotice(ctx, &pi.UploaderReviewNotice{
			Kind: pi.UploaderReviewRequestNotice, TenantID: review.TenantId, AccountID: review.OwnerAccountId, ReviewID: review.Id,
		})
	})
	return review, err
}

func uploaderReviewOwner(ctx context.Context, tenantID, accountID int64) error {
	owner, err := ps.SysPublish().AccountCapability(ctx, "api", accountID)
	if err != nil {
		return err
	}
	if owner == nil || owner.TenantId != tenantID || owner.AccountType != pi.PublishAccountTypeAdmin {
		return gerror.New("机器人所属管理账户不可用")
	}
	return nil
}

func (s *sSysBot) ensureUploaderReviewRecipient(ctx context.Context, accountID int64) error {
	official, err := s.officialBot(ctx)
	if err != nil {
		return err
	}
	telegramID, err := g.DB().Model(accountBindTbl).Ctx(ctx).
		Where("app", "api").Where("account_id", accountID).Where("status", 1).
		WhereNull("deleted_at").Fields("telegram_user_id").Value()
	if err != nil {
		return err
	}
	if telegramID.IsEmpty() {
		return gerror.New("管理员尚未绑定 Telegram，请联系管理员")
	}
	chatID, err := s.notifyAccountChatId(ctx, official.Id, telegramID.String())
	if err != nil {
		return err
	}
	if chatID != telegramID.String() {
		return gerror.New("管理员尚未在官方机器人开启私聊，请联系管理员")
	}
	return nil
}

func (s *sSysBot) handleUploaderReviewCallback(ctx context.Context, botID int64, query *models.CallbackQuery) (bool, error) {
	if !strings.HasPrefix(query.Data, uploaderReviewCallback) {
		return false, nil
	}
	msg := query.Message.Message
	if msg == nil || msg.Chat.Type != models.ChatTypePrivate || query.From.ID <= 0 || msg.Chat.ID != query.From.ID {
		return true, nil
	}
	official, err := s.officialBot(ctx)
	if err != nil {
		return true, err
	}
	if official.Id != botID {
		return true, nil
	}
	action, id, valid := parseUploaderReviewCallback(query.Data)
	if !valid {
		s.answerProfileCallback(ctx, botID, query, "审核操作无效")
		return true, nil
	}
	binding, err := s.bindingByTelegram(ctx, "api", strconv.FormatInt(query.From.ID, 10))
	if err != nil {
		return true, err
	}
	if binding == nil {
		s.answerProfileCallback(ctx, botID, query, "当前账号无权审批")
		return true, nil
	}
	message, err := s.decideUploaderReview(ctx, id, binding.AccountId, action)
	if err != nil {
		s.answerProfileCallback(ctx, botID, query, "审批失败，请稍后重试")
		return true, err
	}
	s.answerProfileCallback(ctx, botID, query, message)
	return true, nil
}

func parseUploaderReviewCallback(data string) (string, int64, bool) {
	payload, hasPrefix := strings.CutPrefix(data, uploaderReviewCallback)
	action, rawID, ok := strings.Cut(payload, ":")
	id, err := strconv.ParseInt(rawID, 10, 64)
	return action, id, hasPrefix && ok && err == nil && id > 0 && (action == "approve" || action == "reject")
}

func (s *sSysBot) decideUploaderReview(ctx context.Context, id, reviewerID int64, action string) (string, error) {
	var message string
	err := g.DB().TransactionWithOptions(ctx, gdb.TxOptions{Propagation: gdb.PropagationNested, Isolation: sql.LevelReadCommitted}, func(ctx context.Context, tx gdb.TX) error {
		var review *uploaderReview
		target := tx.Model(uploaderReviewTable).Where("id", id).Where("owner_account_id", reviewerID)
		if err := target.Clone().Scan(&review); err != nil {
			return err
		}
		if review != nil {
			if err := lockPersonalUploader(tx, review.TenantId, review.TelegramUserId); err != nil {
				return err
			}
			review = nil
			if err := target.LockUpdate().Scan(&review); err != nil {
				return err
			}
		}
		if review == nil || review.OwnerAccountId != reviewerID {
			message = "申请不存在或当前账号无权审批"
			return nil
		}
		bot, err := review.currentBot(ctx)
		if err != nil {
			return err
		}
		configured, _, err := botregistry.UploaderConfig(ctx, bot.BotToken, bot.TenantId)
		if err != nil {
			return err
		}
		if !configured {
			return gerror.New("上架机器人未启用")
		}
		if err = uploaderReviewOwner(ctx, review.TenantId, reviewerID); err != nil {
			return err
		}
		if review.Status != uploaderPending {
			message = "该申请已处理"
			return nil
		}
		status := uploaderRejected
		accountID := int64(0)
		message = "已拒绝"
		if action == "approve" {
			accountID, err = s.findOrCreatePersonalUploader(ctx, tx, review)
			if err != nil {
				return err
			}
			status, message = uploaderApproved, "已通过"
		}
		if _, err = tx.Model(uploaderReviewTable).Where("id", review.Id).Data(g.Map{
			"status": status, "account_id": accountID, "reviewed_by": reviewerID,
			"reviewed_at": gtime.Now(), "updated_at": gtime.Now(),
		}).Update(); err != nil {
			return err
		}
		return ps.SysPublish().EnqueueUploaderReviewNotice(ctx, &pi.UploaderReviewNotice{
			Kind: pi.UploaderReviewResultNotice, TenantID: review.TenantId, AccountID: reviewerID, ReviewID: review.Id,
		})
	})
	return message, err
}

func (review *uploaderReview) currentBot(ctx context.Context) (*collectorBotRow, error) {
	bot, err := collectorBotByID(ctx, review.BotId)
	if err != nil {
		return nil, err
	}
	if bot.TenantId != review.TenantId || bot.AccountId != review.OwnerAccountId {
		return nil, gerror.New("上架机器人归属已变更")
	}
	return bot, nil
}

func createReviewedUploader(ctx context.Context, review *uploaderReview) (int64, error) {
	username := strings.TrimPrefix(strings.TrimSpace(review.TelegramUsername), "@")
	for attempt := 0; attempt < 5; attempt++ {
		var secret [16]byte
		if _, err := rand.Read(secret[:]); err != nil {
			return 0, err
		}
		if username == "" {
			username = "tg_" + hex.EncodeToString(secret[:8])
			if _, err := rand.Read(secret[:]); err != nil {
				return 0, err
			}
		}
		in := uploaderCreateInput(username)
		in.Password = hex.EncodeToString(secret[:])
		in.TelegramUsername = review.TelegramUsername
		if review.TelegramUserId > 0 {
			in.TelegramUserId = strconv.FormatInt(review.TelegramUserId, 10)
		}
		var created *pi.AccountSaveModel
		// A duplicate insert on PostgreSQL aborts its savepoint only.
		err := g.DB().TransactionWithOptions(ctx, gdb.TxOptions{Propagation: gdb.PropagationNested}, func(ctx context.Context, _ gdb.TX) error {
			authCtx := quickPushMediaUploadContext(ctx, review.OwnerAccountId)
			var err error
			created, err = ps.SysPublish().AdminAccountSave(authCtx, in)
			return err
		})
		if err != nil {
			if uploaderUsernameConflict(err) {
				username = ""
				continue
			}
			return 0, err
		}
		if created == nil || created.Id <= 0 {
			return 0, gerror.New("未返回创建结果")
		}
		_, err = ps.SysPublish().PatchAccountSetting(quickPushMediaUploadContext(ctx, review.OwnerAccountId),
			&pi.AccountSettingPatchInp{AccountId: created.Id, Field: "telegramBindingEnabled", Value: "1"})
		return created.Id, err
	}
	return 0, gerror.New("生成唯一上架账号失败")
}

func uploaderUsernameConflict(err error) bool {
	var pg *pq.Error
	var my *mysql.MySQLError
	return err.Error() == "账号已存在，请更换账号" ||
		(errors.As(err, &pg) && pg.Code == "23505") ||
		(errors.As(err, &my) && my.Number == 1062)
}

func uploaderReviewMessage(review *uploaderReview) string {
	if review == nil {
		return "请点击底部「注册申请」，管理员审核通过后即可使用。"
	}
	switch review.Status {
	case uploaderPending:
		return "注册申请审核中，请等待管理员处理。"
	case uploaderRejected:
		return "注册申请未通过，请联系管理员。"
	default:
		return "上架账户不可用，请联系管理员。"
	}
}

func (s *sSysBot) DeliverUploaderReviewNotice(ctx context.Context, in *pi.UploaderReviewNotice) error {
	var review *uploaderReview
	mod := g.DB().Model(uploaderReviewTable).Ctx(ctx).Where("tenant_id", in.TenantID)
	if in.Kind == pi.UploaderAccountStatusNotice {
		mod = mod.Where("account_id", in.AccountID).Where("status", uploaderApproved).OrderDesc("id").Limit(1)
	} else {
		mod = mod.Where("id", in.ReviewID).Where("owner_account_id", in.AccountID)
	}
	if err := mod.Scan(&review); err != nil || review == nil {
		return err
	}
	bot, err := review.currentBot(ctx)
	if err != nil {
		return err
	}
	configured, _, err := botregistry.UploaderConfig(ctx, bot.BotToken, bot.TenantId)
	if err != nil || !configured {
		return err
	}
	switch in.Kind {
	case pi.UploaderReviewRequestNotice:
		if review.Status != uploaderPending {
			return nil
		}
		if err = uploaderReviewOwner(ctx, review.TenantId, review.OwnerAccountId); err != nil {
			return err
		}
		if err = s.ensureUploaderReviewRecipient(ctx, review.OwnerAccountId); err != nil {
			return err
		}
		return s.NotifyAccount(ctx, &sysin.NotifyAccountInp{
			NoticeId: in.NoticeID, App: "api", AccountId: review.OwnerAccountId, BotStrategy: "official",
			IgnoreFeatureSwitch: true, RequireDelivery: true,
			Text: uploaderReviewRequestText(bot.Id, review),
			Buttons: [][]sysin.NotifyButton{{
				{Label: "通过", Data: fmt.Sprintf("%sapprove:%d", uploaderReviewCallback, review.Id)},
				{Label: "拒绝", Data: fmt.Sprintf("%sreject:%d", uploaderReviewCallback, review.Id)},
			}},
		})
	case pi.UploaderReviewResultNotice, pi.UploaderAccountStatusNotice:
		message := "注册申请未通过，请联系管理员。"
		var markup models.ReplyMarkup = &models.ReplyKeyboardRemove{RemoveKeyboard: true}
		if review.Status == uploaderApproved {
			_, accountErr := scopedUploaderAccount(ctx, review.TenantId, review.OwnerAccountId, review.AccountId)
			if accountErr != nil && in.Status == 1 {
				return accountErr
			}
			if accountErr == nil && (in.Kind != pi.UploaderAccountStatusNotice || in.Status == 1) {
				message = "注册申请已通过，已绑定上架账户。请选择底部功能。"
				if in.Kind == pi.UploaderAccountStatusNotice {
					message = "上架账户已启用。请选择底部功能。"
				}
				markup = collectorReplyKeyboard(false)
			} else {
				message = "上架账户已停用，请联系管理员。"
			}
		}
		if review.TelegramUserId <= 0 {
			return nil
		}
		chatID := strconv.FormatInt(review.TelegramUserId, 10)
		_, err = s.sendMessageWithMarkup(ctx, bot.BotToken, chatID, message, "", false, markup)
		s.trackReplyResult(context.WithValue(ctx, telegramUserIdCtxKey{}, chatID), -bot.Id, markup, err)
		return err
	default:
		return gerror.New("未知上架机器人通知类型")
	}
}

func scopedUploaderAccount(ctx context.Context, tenantID, ownerID, accountID int64) (*botProfileAccount, error) {
	capability, err := ps.SysPublish().AccountCapability(ctx, "api", accountID)
	if err != nil {
		return nil, err
	}
	if capability == nil || capability.TenantId != tenantID || capability.ParentId != ownerID ||
		capability.AccountType != pi.PublishAccountTypeUploader || capability.TelegramBindingEnabled != 1 {
		return nil, gerror.New("上架账户不可用")
	}
	return &botProfileAccount{
		TenantId: capability.TenantId, AccountId: capability.AccountId,
		AccountType: capability.AccountType, App: "api",
		SharedResourceEnabled: capability.SharedResourceEnabled == 1, CollectPushEnabled: capability.CollectPushEnabled == 1,
	}, nil
}

func (s *sSysBot) sendUploaderReplyWithoutKeyboard(ctx context.Context, bot *collectorBotRow, chatID, text string) error {
	markup := &models.ReplyKeyboardRemove{RemoveKeyboard: true}
	_, err := s.sendMessageWithMarkup(ctx, bot.BotToken, chatID, text, "", false, markup)
	s.trackReplyResult(ctx, -bot.Id, markup, err)
	return err
}

// Legacy authorization was recorded with the negative publish Bot ID.
func (s *sSysBot) existingScopedUploaderAccount(ctx context.Context, bot *collectorBotRow, telegramUserID int64) (*botProfileAccount, error) {
	bind, err := s.bindingByTelegram(ctx, "api", strconv.FormatInt(telegramUserID, 10))
	if err != nil || bind == nil || bind.BotId != -bot.Id {
		return nil, err
	}
	return scopedUploaderAccount(ctx, bot.TenantId, bot.AccountId, bind.AccountId)
}

func (s *sSysBot) handleUploaderRegistration(ctx context.Context, bot *collectorBotRow, user *models.User, query *models.CallbackQuery, action string, review *uploaderReview) error {
	botID, chatID := -bot.Id, strconv.FormatInt(user.ID, 10)
	state := collectorInteractionFromContext(ctx)
	state.keyboard = collectorReplyKeyboard(true)
	if review == nil && query == nil && action == "register" {
		var err error
		review, err = s.submitUploaderReview(ctx, bot, user)
		if err != nil {
			return s.replyBotError(ctx, botID, chatID, "提交注册申请", err)
		}
	}
	if query != nil {
		s.answerProfileCallback(ctx, botID, query, "请先完成注册审核")
	}
	if review != nil {
		return s.sendUploaderReplyWithoutKeyboard(ctx, bot, chatID, uploaderReviewMessage(review))
	}
	return s.sendCollectorReply(ctx, botID, chatID, uploaderReviewMessage(nil))
}

func uploaderReviewRequestText(botID int64, review *uploaderReview) string {
	name := strings.TrimSpace(review.TelegramDisplayName)
	if name == "" {
		name = "未记录昵称"
	}
	text := fmt.Sprintf("收到上架机器人注册申请\n机器人 ID：%d\n申请人昵称：%s", botID, html.EscapeString(name))
	if username := strings.TrimLeft(strings.TrimSpace(review.TelegramUsername), "@"); username != "" {
		text += "\n申请人用户名：@" + html.EscapeString(username)
	}
	return text
}
