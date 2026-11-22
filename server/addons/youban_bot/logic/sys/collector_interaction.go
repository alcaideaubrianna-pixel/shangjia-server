package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_bot/model/input/sysin"
	botservice "hotgo/addons/youban_bot/service"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
	"hotgo/internal/library/botregistry"
)

const collectorDisabledMessage = "机器人未启用功能, 请联系管理员"

type collectorGatewayFeature struct{}

func (*collectorGatewayFeature) Key() string   { return "collector_interaction" }
func (*collectorGatewayFeature) Priority() int { return 140 }

func (*collectorGatewayFeature) Menus(context.Context, gatewayservice.BotContext) (gatewayservice.FeatureMenus, error) {
	return gatewayservice.FeatureMenus{}, nil
}

type collectorBotRow struct {
	sysin.BotModel
	TenantId             int64
	AccountId            int64
	RequirePublishReview bool
}

func collectorBotByID(ctx context.Context, id int64) (*collectorBotRow, error) {
	var row *collectorBotRow
	err := g.DB().Model("hg_youban_publish_bot").Ctx(ctx).
		Fields("id", "bot_token", "bot_username", "tenant_id", "account_id", "require_publish_review").
		Where("id", id).Where("status", 1).Where("bot_type", botregistry.RoleUploader).WhereNull("deleted_at").Scan(&row)
	if err != nil {
		return nil, gerror.Wrap(err, "读取上架机器人失败")
	}
	if row == nil || row.Id <= 0 {
		return nil, gerror.New("上架机器人不存在或已停用")
	}
	return row, nil
}

type collectorInteractionKey struct{}

type collectorInteraction struct {
	botID    int64
	userID   int64
	account  *botProfileAccount
	bot      *collectorBotRow
	user     *models.User
	keyboard *models.ReplyKeyboardMarkup
}

func collectorInteractionFromContext(ctx context.Context) *collectorInteraction {
	state, _ := ctx.Value(collectorInteractionKey{}).(*collectorInteraction)
	return state
}

func (*collectorGatewayFeature) HandleUpdate(ctx context.Context, bc gatewayservice.BotContext, update *models.Update) (bool, error) {
	if update == nil || (update.Message == nil && update.CallbackQuery == nil && update.InlineQuery == nil) {
		return false, nil
	}
	msg := update.Message
	if query := update.CallbackQuery; query != nil {
		msg = query.Message.Message
		if inaccessible := query.Message.InaccessibleMessage; inaccessible != nil && inaccessible.Chat.Type != models.ChatTypePrivate {
			return false, nil
		}
	}
	if msg != nil && msg.Chat.Type != models.ChatTypePrivate {
		return false, nil
	}

	var binding gatewayservice.BotBinding
	var legacy bool
	for _, item := range bc.Bindings {
		switch item.Owner {
		case botregistry.UploaderGatewayOwner:
			if binding.ReferenceID == 0 || item.ReferenceID < binding.ReferenceID {
				binding = item
			}
		case "youban_bot":
			legacy = true
		}
	}
	if binding.ReferenceID <= 0 {
		return false, nil
	}
	configured, requireRegistration, err := botregistry.UploaderConfig(ctx, binding.Token, binding.TenantID)
	if err != nil {
		return true, err
	}
	bot, ok := botservice.SysBot().(*sSysBot)
	if !ok {
		return false, nil
	}
	if !configured {
		if legacy {
			return false, nil
		}
		if update.Message != nil {
			_, err = bot.sendMessageWithMarkup(ctx, binding.Token, fmt.Sprint(msg.Chat.ID), collectorDisabledMessage, "", false, &models.ReplyKeyboardRemove{RemoveKeyboard: true})
		}
		return true, err
	}
	// Inline queries and callbacks without a visible message cannot establish
	// the private-chat scope required by collector features.
	if msg == nil {
		return true, nil
	}
	row, err := collectorBotByID(ctx, binding.ReferenceID)
	if err != nil {
		return true, err
	}
	if row.TenantId != binding.TenantID || strings.TrimSpace(row.BotToken) != strings.TrimSpace(binding.Token) {
		return true, gerror.New("上架机器人配置已变更")
	}
	return true, bot.handleCollectorPrivateUpdate(ctx, row, requireRegistration, update.CallbackQuery, msg)
}

func (s *sSysBot) handleCollectorPrivateUpdate(ctx context.Context, row *collectorBotRow, requireRegistration bool, query *models.CallbackQuery, msg *models.Message) error {
	user := msg.From
	if query != nil {
		user = &query.From
	}
	if user == nil || user.ID <= 0 || user.ID != msg.Chat.ID {
		return nil
	}

	// The profile session tables also store legacy Bot IDs. A negative ID
	// identifies a publish Bot across updates and background media recovery.
	botID, chatID := -row.Id, fmt.Sprint(msg.Chat.ID)
	ctx = context.WithValue(ctx, telegramUserIdCtxKey{}, fmt.Sprint(user.ID))
	state := &collectorInteraction{botID: botID, userID: user.ID, bot: row, user: user}
	ctx = context.WithValue(ctx, collectorInteractionKey{}, state)
	authCtx := withCollectorAuthorization(ctx, row.TenantId)
	text := strings.TrimSpace(firstNonEmpty(msg.Text, msg.Caption))
	command, args := botCommandAndArgs(text)

	review, err := uploaderReviewByUser(ctx, row, user.ID)
	if err != nil {
		return err
	}
	legacyAuthorization := !requireRegistration && (review == nil || review.Status != uploaderApproved && review.Status != uploaderDirect)

	if legacyAuthorization && query != nil && strings.HasPrefix(query.Data, "acctauth:") {
		handled, err := s.handleAccountAuthorizationCallback(authCtx, botID, query)
		if handled || err != nil {
			if err != nil {
				return err
			}
			return s.refreshCollectorKeyboardAfterAuthorization(ctx, row, user, state, chatID)
		}
	}
	if legacyAuthorization && query == nil && command == "start" && strings.HasPrefix(args, accountAuthorizationPrefix) {
		return s.beginAccountAuthorization(authCtx, &botFeatureContext{BotId: botID, Msg: msg, Args: args})
	}

	session := s.activeProfileSessionByIds(ctx, botID, fmt.Sprint(user.ID), chatID)
	if legacyAuthorization && query == nil && session != nil && session.Scene == "account_authorize" {
		_, err := s.handleAccountManagementInput(authCtx, botID, msg)
		return err
	}

	action := collectorMessageAction(command, text)
	if query != nil {
		action = collectorCallbackAction(query.Data)
	}
	if action == "" && (query != nil || session == nil || !collectorAllowsSession(session.Scene) || strings.HasPrefix(text, "/")) {
		if query != nil {
			s.answerProfileCallback(ctx, botID, query, "当前机器人不支持此操作")
			return nil
		}
		action = "menu"
	}

	var account *botProfileAccount
	if review != nil && (review.Status == uploaderApproved || review.Status == uploaderDirect) {
		if review.OwnerAccountId != row.AccountId {
			return s.sendUploaderReplyWithoutKeyboard(ctx, row, chatID, uploaderReviewMessage(review))
		}
		account, err = scopedUploaderAccount(ctx, review.TenantId, row.AccountId, review.AccountId)
	} else if requireRegistration {
		if review == nil {
			account, err = s.existingScopedUploaderAccount(ctx, row, user.ID)
		}
		if account == nil && err == nil {
			return s.handleUploaderRegistration(ctx, row, user, query, action, review)
		}
	} else {
		allowProvision := query == nil &&
			(action == "publish" || action == "manage" || (strings.HasPrefix(action, "pf:") && action != "pf:cancel"))
		account, err = s.resolveCollectorAccount(ctx, row, user, allowProvision)
	}
	if err != nil {
		if query != nil {
			s.answerProfileCallback(ctx, botID, query, "上架账户不可用")
		}
		if requireRegistration || (review != nil && review.Status == uploaderApproved) {
			return s.sendUploaderReplyWithoutKeyboard(ctx, row, chatID, "上架账户不可用，请联系管理员。")
		}
		return s.replyBotError(ctx, botID, chatID, "上架机器人上架账户", err)
	}

	state.account = account
	state.keyboard = collectorReplyKeyboard(false)

	if action != "menu" && action != "register" && session == nil {
		if err := s.ensureReplyKeyboardCurrent(ctx, botID, chatID); err != nil {
			return err
		}
	}

	switch action {
	case "publish":
		_, err = (profilePublishFeature{}).Handle(ctx, s, &botFeatureContext{BotId: botID, Msg: msg})
	case "manage":
		if err = s.cancelProfileSessionByIdsSilent(ctx, botID, fmt.Sprint(user.ID), chatID); err != nil {
			return err
		}
		err = s.showProfileMenu(ctx, botID, msg)
	case "menu", "register":
		return s.sendCollectorReply(ctx, botID, chatID, "请选择底部功能。")
	default:
		if account == nil {
			return s.sendCollectorReply(ctx, botID, chatID, "请选择底部功能。")
		}
		if action != "" {
			if query == nil {
				query = &models.CallbackQuery{From: *user, Message: models.MaybeInaccessibleMessage{Message: msg}, Data: action}
			}
			_, err = s.handleProfileCallback(ctx, botID, query)
		} else {
			if account.AccountId != session.AccountId || account.TenantId != session.TenantId {
				return s.sendCollectorReply(ctx, botID, chatID, "当前绑定账号已变更，请重新操作")
			}
			if err := s.storeTelegramMessage(ctx, botID, msg); err != nil {
				return err
			}
			return s.consumeCollectorProfileInput(ctx, botID, msg, account, session, text)
		}
	}
	return err
}

func (s *sSysBot) refreshCollectorKeyboardAfterAuthorization(ctx context.Context, row *collectorBotRow, user *models.User, state *collectorInteraction, chatID string) error {
	account, err := s.resolveCollectorAccount(ctx, row, user, false)
	if err != nil || account == nil {
		return err
	}
	if account.TenantId != row.TenantId {
		return gerror.New("上架账户不属于当前机器人租户")
	}
	state.account = account
	state.keyboard = collectorReplyKeyboard(false)
	return s.ensureReplyKeyboardCurrent(ctx, state.botID, chatID)
}

func collectorMessageAction(command, text string) string {
	switch {
	case command == "start":
		return "menu"
	case command == "publish" || text == "上架资料":
		return "publish"
	case command == "profile" || command == "note" || text == "资料管理":
		return "manage"
	case text == "注册申请":
		return "register"
	case text == "笔记列表":
		return "pf:list:1"
	case text == "搜索笔记":
		return "pf:search"
	case text == "编辑笔记":
		return "pf:askedit"
	case text == "新建笔记":
		return "pf:create"
	case isCancelProfileCommand(text):
		return "pf:cancel"
	default:
		return ""
	}
}

func collectorCallbackAction(data string) string {
	parts := strings.SplitN(data, ":", 3)
	if len(parts) < 2 || parts[0] != "pf" {
		return ""
	}
	switch parts[1] {
	case "menu", "list", "askpage", "search", "view", "backview", "askedit", "edit",
		"edtitle", "edtext", "edno", "replace", "editback", "create", "createback",
		"createskip", "cancel", "publishup", "down", "del", "delconfirm", "delcancel", "preview":
		return data
	default:
		return ""
	}
}

func collectorAllowsSession(scene string) bool {
	switch scene {
	case "note_page", "search", "edit", "edit_title", "edit_text", "edit_verify", "replace", "create", "batch_create":
		return true
	default:
		return false
	}
}

func collectorNoteMarkup(markup *models.InlineKeyboardMarkup) *models.InlineKeyboardMarkup {
	filtered := &models.InlineKeyboardMarkup{}
	for _, row := range markup.InlineKeyboard {
		var buttons []models.InlineKeyboardButton
		for _, button := range row {
			if button.CallbackData == "" || collectorCallbackAction(button.CallbackData) != "" {
				buttons = append(buttons, button)
			}
		}
		if len(buttons) != 0 {
			filtered.InlineKeyboard = append(filtered.InlineKeyboard, buttons)
		}
	}
	return filtered
}

func (s *sSysBot) consumeCollectorProfileInput(ctx context.Context, botID int64, msg *models.Message, account *botProfileAccount, session *profileSessionRow, text string) error {
	switch session.Scene {
	case "create", "batch_create", "replace", "edit_verify":
		return s.consumeProfileSessionMessage(ctx, botID, msg, account, session, text)
	case "search":
		if len(msg.Photo) > 0 {
			return s.consumeProfileSearchImageMessage(ctx, botID, msg, account, session)
		}
	}
	if text == "" {
		return nil
	}
	return s.consumeProfileSessionText(ctx, botID, msg, account, session, text, extractProfileNos(text))
}

func (s *sSysBot) profileBotById(ctx context.Context, botID int64) (*sysin.BotModel, error) {
	if botID >= 0 {
		return s.botById(ctx, botID)
	}
	row, err := collectorBotByID(ctx, -botID)
	if err != nil {
		return nil, err
	}
	return &row.BotModel, nil
}
