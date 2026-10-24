package sys

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
)

const accountAuthorizationPrefix = "account_auth_"
const accountShareStatusAvailable = "available"

type accountAuthorizationDraft struct {
	ShareID          int64  `json:"shareId"`
	Username         string `json:"username"`
	RecipientID      string `json:"recipientId,omitempty"`
	CreatedAccountID int64  `json:"createdAccountId,omitempty"`
}

func accountAuthorizationDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:16])
}

// The existing durable operation-session store provides expiry, revocation,
// audit ownership and atomic consumption. Only a digest of the bearer is stored.
func (s *sSysBot) showAccountAuthorizationLink(ctx context.Context, botID int64, userID, chatID string, account *botProfileAccount) error {
	bot, err := s.botById(ctx, botID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(bot.BotUsername) == "" {
		return gerror.New("机器人用户名尚未同步，请先同步机器人信息。")
	}
	raw := make([]byte, 24)
	if _, err = rand.Read(raw); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err = s.createAccountAuthorizationShare(ctx, botID, userID, chatID, account, accountAuthorizationDigest(token)); err != nil {
		return err
	}
	link := "https://t.me/" + strings.TrimPrefix(bot.BotUsername, "@") + "?start=" + accountAuthorizationPrefix + token
	return s.replyWithMarkup(ctx, botID, chatID, "<b>分享授权链接</b>\n\n将链接发给需要创建上架账户的人。对方确认后自动创建账户并绑定其 TG。\n链接30分钟有效，仅可使用一次；重新生成会使旧链接失效。\n\n<code>"+html.EscapeString(link)+"</code>", &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "撤销授权链接", CallbackData: "acct:share:revoke"}}, {{Text: "返回上架账户", CallbackData: "acct:accounts"}}}})
}

func (s *sSysBot) beginAccountAuthorization(ctx context.Context, fc *botFeatureContext) error {
	msg := fc.Msg
	if msg == nil || msg.From == nil || msg.Chat.Type != models.ChatTypePrivate {
		return nil
	}
	chatID := fmt.Sprint(msg.Chat.ID)
	if _, enabled := s.featureConfig(ctx, accountManagementFeature{}.Key()); !enabled {
		return s.reply(ctx, fc.BotId, chatID, "账户管理当前未启用。")
	}
	token := strings.TrimPrefix(strings.TrimSpace(fc.Args), accountAuthorizationPrefix)
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 24 {
		return s.reply(ctx, fc.BotId, chatID, "授权链接无效。")
	}
	var share *profileSessionRow
	err = g.DB().Model(profileSessionTable).Ctx(ctx).Where("bot_id", fc.BotId).Where("scene", "account_share").Where("profile_no", accountAuthorizationDigest(token)).Where("status", accountShareStatusAvailable).WhereGT("expires_at", gtime.Now()).Scan(&share)
	if err != nil {
		return err
	}
	if share == nil {
		return s.reply(ctx, fc.BotId, chatID, "授权链接已过期、撤销或使用，请向管理员索取新链接。")
	}
	if err = s.accountAuthorizationRecipientAvailable(ctx, fmt.Sprint(msg.From.ID)); err != nil {
		return s.reply(ctx, fc.BotId, chatID, html.EscapeString(err.Error()))
	}
	capability, err := ps.SysPublish().AccountCapability(ctx, "api", share.AccountId)
	if err != nil || capability == nil || capability.AccountType != pi.PublishAccountTypeAdmin || capability.TenantId != share.TenantId {
		return s.reply(ctx, fc.BotId, chatID, "邀请方账户已失效，请联系管理员。")
	}
	username := strings.TrimPrefix(msg.From.Username, "@")
	if !accountUsernamePattern.MatchString(username) {
		username = ""
	}
	draft := accountAuthorizationDraft{ShareID: share.Id, Username: username}
	account := &botProfileAccount{AccountId: share.AccountId, TenantId: share.TenantId, App: "api"}
	if err = s.startProfileSessionByIds(ctx, fc.BotId, fmt.Sprint(msg.From.ID), chatID, account, "account_authorize", "username", 0, "", draft, ""); err != nil {
		return err
	}
	session := s.activeProfileSessionByIds(ctx, fc.BotId, fmt.Sprint(msg.From.ID), chatID)
	if session == nil {
		return gerror.New("创建授权会话失败，请重新打开链接。")
	}
	if username == "" {
		return s.reply(ctx, fc.BotId, chatID, "请输入上架账户用户名（3～32位字母、数字或下划线）。发送 /cancel 取消。")
	}
	return s.showAccountAuthorizationConfirmation(ctx, fc.BotId, chatID, session.Id, username)
}

func (s *sSysBot) accountAuthorizationRecipientAvailable(ctx context.Context, userID string) error {
	count, err := g.DB().Model(accountBindTbl).Ctx(ctx).Where("telegram_user_id", userID).Where("status", 1).WhereNull("deleted_at").Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG绑定失败")
	}
	if count > 0 {
		return gerror.New("当前 TG 已绑定管理员或上架账户，不能重复授权创建。")
	}
	return nil
}

func (s *sSysBot) showAccountAuthorizationConfirmation(ctx context.Context, botID int64, chatID string, sessionID int64, username string) error {
	return s.replyWithMarkup(ctx, botID, chatID, "<b>创建并绑定上架账户</b>\n用户名：<code>"+html.EscapeString(username)+"</code>\n确认后将创建在邀请方名下，并绑定你当前的 TG。", &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "确认创建并绑定", CallbackData: fmt.Sprintf("acctauth:confirm:%d", sessionID)}}, {{Text: "取消", CallbackData: "acctauth:cancel"}}}})
}

func (s *sSysBot) accountAuthorizationInput(ctx context.Context, botID int64, msg *models.Message, session *profileSessionRow) error {
	chatID := fmt.Sprint(msg.Chat.ID)
	username := strings.TrimSpace(msg.Text)
	if strings.HasPrefix(username, "/") {
		return s.cancelProfileSessionByIds(ctx, botID, fmt.Sprint(msg.From.ID), chatID)
	}
	if !accountUsernamePattern.MatchString(username) {
		return s.reply(ctx, botID, chatID, "用户名需为3～32位字母、数字或下划线，请重新输入。")
	}
	var draft accountAuthorizationDraft
	if err := json.Unmarshal([]byte(session.PayloadJson), &draft); err != nil {
		return err
	}
	draft.Username = username
	raw, err := json.Marshal(draft)
	if err != nil {
		return err
	}
	result, err := g.DB().Model(profileSessionTable).Ctx(ctx).Where("id", session.Id).Where("bot_id", botID).Where("telegram_user_id", fmt.Sprint(msg.From.ID)).Where("chat_id", chatID).Where("scene", "account_authorize").Where("status", profileSessionStatusActive).WhereGT("expires_at", gtime.Now()).Where("payload_json", session.PayloadJson).Data(g.Map{"payload_json": string(raw), "step": "confirm"}).Update()
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return s.reply(ctx, botID, chatID, "授权会话已改变，请重新打开链接。")
	}
	return s.showAccountAuthorizationConfirmation(ctx, botID, chatID, session.Id, username)
}

func (s *sSysBot) handleAccountAuthorizationCallback(ctx context.Context, botID int64, query *models.CallbackQuery) (bool, error) {
	if query == nil || !strings.HasPrefix(query.Data, "acctauth:") {
		return false, nil
	}
	if query.Message.Message == nil || query.Message.Message.Chat.Type != models.ChatTypePrivate {
		return true, nil
	}
	chatID := fmt.Sprint(query.Message.Message.Chat.ID)
	if _, enabled := s.featureConfig(ctx, accountManagementFeature{}.Key()); !enabled {
		return true, s.reply(ctx, botID, chatID, "账户管理当前未启用。")
	}
	if client, err := s.telegramBot(ctx, s.replyBotToken(ctx, botID)); err == nil {
		callCtx, cancel := telegramAPICtx()
		defer cancel()
		_, _ = client.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
	}
	if query.Data == "acctauth:cancel" {
		return true, s.cancelProfileSessionByIds(ctx, botID, fmt.Sprint(query.From.ID), chatID)
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(query.Data, "acctauth:confirm:"), 10, 64)
	if err != nil || id <= 0 {
		return true, s.reply(ctx, botID, chatID, "授权确认参数无效。")
	}
	session := s.activeProfileSessionByIds(ctx, botID, fmt.Sprint(query.From.ID), chatID)
	if session == nil || session.Id != id || session.Scene != "account_authorize" {
		return true, s.reply(ctx, botID, chatID, "确认已失效或已完成，请勿重复创建。")
	}
	created, err := s.consumeAccountAuthorization(ctx, botID, query.From, chatID, session)
	if err != nil {
		return true, s.reply(ctx, botID, chatID, "授权失败："+html.EscapeString(err.Error()))
	}
	if created == nil {
		return true, s.reply(ctx, botID, chatID, "授权已完成，请勿重复创建。")
	}
	s.afterAccountBound(ctx, botID, chatID, "api", created.Id, fmt.Sprint(query.From.ID), query.From.Username)
	var draft accountAuthorizationDraft
	if err := json.Unmarshal([]byte(session.PayloadJson), &draft); err != nil {
		return true, err
	}
	return true, s.reply(ctx, botID, chatID, fmt.Sprintf("上架账户创建并绑定成功\n用户名：<code>%s</code>\n账号 ID：%d\n初始密码：<code>%s</code>\n请妥善保存并及时修改密码。", html.EscapeString(draft.Username), created.Id, html.EscapeString(created.Password)))
}

func (s *sSysBot) consumeAccountAuthorization(ctx context.Context, botID int64, user models.User, chatID string, session *profileSessionRow) (*pi.AccountSaveModel, error) {
	var created *pi.AccountSaveModel
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var current *profileSessionRow
		mod := g.DB().Model(profileSessionTable).Ctx(ctx).Where("id", session.Id).Where("bot_id", botID).Where("telegram_user_id", fmt.Sprint(user.ID)).Where("chat_id", chatID).Where("scene", "account_authorize").WhereGT("expires_at", gtime.Now())
		result, err := mod.Clone().Where("status", profileSessionStatusActive).Data(g.Map{"status": profileSessionStatusCompleted}).Update()
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil || n == 0 {
			return err
		}
		if err = mod.Clone().Scan(&current); err != nil {
			return err
		}
		if current == nil {
			return gerror.New("授权会话不存在。")
		}
		var draft accountAuthorizationDraft
		if err = json.Unmarshal([]byte(current.PayloadJson), &draft); err != nil {
			return err
		}
		if draft.ShareID <= 0 || !accountUsernamePattern.MatchString(draft.Username) {
			return gerror.New("请先输入有效用户名。")
		}
		if err = s.accountAuthorizationRecipientAvailable(ctx, fmt.Sprint(user.ID)); err != nil {
			return err
		}
		shareMod := g.DB().Model(profileSessionTable).Ctx(ctx).Where("id", draft.ShareID).Where("bot_id", botID).Where("tenant_id", current.TenantId).Where("account_id", current.AccountId).Where("scene", "account_share").Where("status", accountShareStatusAvailable).WhereGT("expires_at", gtime.Now())
		result, err = shareMod.Clone().Data(g.Map{"status": profileSessionStatusCompleted}).Update()
		if err != nil {
			return err
		}
		n, err = result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return gerror.New("授权链接已过期、撤销或使用。")
		}
		capability, err := ps.SysPublish().AccountCapability(ctx, "api", current.AccountId)
		if err != nil {
			return err
		}
		if capability == nil || capability.AccountType != pi.PublishAccountTypeAdmin || capability.TenantId != current.TenantId {
			return gerror.New("邀请方账户已失效或归属已变更。")
		}
		authCtx := quickPushMediaUploadContext(ctx, current.AccountId)
		in := uploaderCreateInput(draft.Username)
		in.TelegramUserId = fmt.Sprint(user.ID)
		in.TelegramUsername = user.Username
		created, err = ps.SysPublish().AdminAccountSave(authCtx, in)
		if err != nil {
			return err
		}
		if created == nil || created.Id <= 0 {
			return gerror.New("未返回创建结果。")
		}
		// Share authorization explicitly grants TG binding, using the same admin-only
		// partial-setting service as account management rather than bypassing policy.
		if _, err = ps.SysPublish().PatchAccountSetting(authCtx, &pi.AccountSettingPatchInp{AccountId: created.Id, Field: "telegramBindingEnabled", Value: "1"}); err != nil {
			return err
		}
		_, err = g.DB().Model(accountBindTbl).Ctx(ctx).Data(g.Map{"app": "api", "account_id": created.Id, "telegram_user_id": fmt.Sprint(user.ID), "telegram_username": user.Username, "telegram_first_name": user.FirstName, "telegram_last_name": user.LastName, "bot_id": botID, "status": 1}).Insert()
		if err != nil {
			return gerror.Wrap(err, "绑定TG失败（可能已绑定其他账户）")
		}
		audit, _ := json.Marshal(accountAuthorizationDraft{ShareID: draft.ShareID, Username: draft.Username, RecipientID: fmt.Sprint(user.ID), CreatedAccountID: created.Id})
		_, err = g.DB().Model(profileSessionTable).Ctx(ctx).WhereIn("id", []int64{current.Id, draft.ShareID}).Data(g.Map{"payload_json": string(audit)}).Update()
		return err
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// Available shares are not active input sessions. Navigating to another Bot
// operation must not revoke a link that its recipient has not used yet.
func (s *sSysBot) createAccountAuthorizationShare(ctx context.Context, botID int64, userID, chatID string, account *botProfileAccount, digest string) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// Serialize issuance on the existing administrator binding, including when
		// two callback updates are handled by different application instances.
		result, err := g.DB().Model(accountBindTbl).Ctx(ctx).Where("app", "api").Where("account_id", account.AccountId).Where("telegram_user_id", userID).Where("status", 1).WhereNull("deleted_at").Data(g.Map{"updated_at": gtime.Now()}).Update()
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return gerror.New("管理员绑定已改变，请重新打开账户管理。")
		}
		if err = s.revokeAccountAuthorizationShares(ctx, botID, userID, account); err != nil {
			return err
		}
		_, err = g.DB().Model(profileSessionTable).Ctx(ctx).Data(g.Map{"bot_id": botID, "telegram_user_id": userID, "chat_id": chatID, "app": "api", "account_id": account.AccountId, "tenant_id": account.TenantId, "scene": "account_share", "step": "available", "profile_no": digest, "status": accountShareStatusAvailable, "expires_at": gtime.Now().Add(30 * time.Minute)}).Insert()
		return err
	})
}
func (s *sSysBot) revokeAccountAuthorizationShares(ctx context.Context, botID int64, userID string, account *botProfileAccount) error {
	_, err := g.DB().Model(profileSessionTable).Ctx(ctx).Where("bot_id", botID).Where("telegram_user_id", userID).Where("account_id", account.AccountId).Where("tenant_id", account.TenantId).Where("scene", "account_share").Where("status", accountShareStatusAvailable).Data(g.Map{"status": profileSessionStatusCanceled}).Update()
	return err
}

// Account-operation audit is retained for 30 days after expiry. Bounded hard
// deletion keeps this shared session table finite without touching media drafts.
func (s *sSysBot) cleanupAccountManagementSessions(ctx context.Context, now time.Time) error {
	scenes := []string{"account_share", "account_authorize", "account_renewal", "account_create", "account_setting", "account_delete", "account_reset", "account_managed_reset"}
	cutoff := now.AddDate(0, 0, -30)
	mod := g.DB().Model(profileSessionTable).Unscoped().Ctx(ctx).WhereIn("scene", scenes).WhereLT("expires_at", cutoff)
	rows, err := mod.Clone().Fields("id").OrderAsc("expires_at").OrderAsc("id").Limit(200).All()
	if err != nil {
		return gerror.Wrap(err, "读取过期账户管理会话失败")
	}
	if len(rows) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row["id"].Int64())
	}
	_, err = mod.WhereIn("id", ids).Delete()
	return gerror.Wrap(err, "清理过期账户管理会话失败")
}
