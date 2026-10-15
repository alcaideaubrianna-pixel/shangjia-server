package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_bot/model/input/sysin"
	"hotgo/addons/youban_bot/service"
	"hotgo/internal/consts"
)

func (s *sSysBot) consumeCode(ctx context.Context, botId int64, msg *models.Message, code string) error {
	var row *authCodeRow
	if err := g.DB().Model(authCodeTable).Safe().Ctx(ctx).Where("code", code).Where("status", sysin.BotCodeStatusPending).Scan(&row); err != nil {
		return gerror.Wrap(err, "读取验证码失败")
	}
	chatId := fmt.Sprintf("%d", msg.Chat.ID)
	if row == nil {
		return s.reply(ctx, botId, chatId, "验证码不存在或已失效，请在页面重新生成。")
	}
	if row.ExpiresAt != nil && row.ExpiresAt.Before(gtime.Now()) {
		_, _ = s.markCodeFailed(ctx, code, sysin.BotCodeStatusExpired, "验证码已过期")
		return s.reply(ctx, botId, chatId, "验证码已过期，请在页面重新生成。")
	}
	if row.Scene == sysin.BotCodeSceneBind {
		return s.consumeBindCode(ctx, botId, msg, row)
	}
	return s.consumeLoginCode(ctx, botId, msg, row)
}

func (s *sSysBot) pendingAuthCodeExists(ctx context.Context, code string) (bool, error) {
	count, err := g.DB().Model(authCodeTable).Safe().Ctx(ctx).
		Where("code", strings.TrimSpace(code)).
		Where("status", sysin.BotCodeStatusPending).
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "读取验证码失败")
	}
	return count > 0, nil
}

func (s *sSysBot) consumeBindCode(ctx context.Context, botId int64, msg *models.Message, row *authCodeRow) error {
	if row.AccountId <= 0 {
		_, _ = s.markCodeFailed(ctx, row.Code, sysin.BotCodeStatusFailed, "绑定账号不存在")
		return nil
	}
	if _, accessErr := s.ensureTelegramBindingAccess(ctx, row.App, row.AccountId); accessErr != nil {
		_, _ = s.markCodeFailed(ctx, row.Code, sysin.BotCodeStatusFailed, accessErr.Error())
		return s.reply(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), accessErr.Error())
	}
	telegramUserId := fmt.Sprintf("%d", msg.From.ID)
	telegramUsername := strings.TrimPrefix(strings.TrimSpace(msg.From.Username), "@")
	var err error
	now := gtime.Now()
	if _, err = g.DB().Model(accountBindTbl).Safe().Ctx(ctx).
		Where("app", row.App).
		Where("telegram_user_id", telegramUserId).
		Where("status", 1).
		WhereNull("deleted_at").
		Data(g.Map{"status": 2, "updated_at": now}).Update(); err != nil {
		return gerror.Wrap(err, "停用旧Telegram绑定失败")
	}
	data := g.Map{"app": row.App, "account_id": row.AccountId, "telegram_user_id": telegramUserId, "telegram_username": telegramUsername, "telegram_first_name": msg.From.FirstName, "telegram_last_name": msg.From.LastName, "bot_id": botId, "status": 1, "updated_at": now}
	var exists *struct {
		Id int64 `json:"id"`
	}
	_ = g.DB().Model(accountBindTbl).Safe().Ctx(ctx).
		Fields("id").
		Where("app", row.App).
		Where("account_id", row.AccountId).
		WhereNull("deleted_at").
		OrderAsc("status").
		OrderDesc("id").
		Scan(&exists)
	if exists != nil && exists.Id > 0 {
		_, err = g.DB().Model(accountBindTbl).Safe().Ctx(ctx).Where("id", exists.Id).Data(data).Update()
	} else {
		data["created_at"] = now
		_, err = g.DB().Model(accountBindTbl).Safe().Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存Telegram绑定失败")
	}
	_, err = g.DB().Model(authCodeTable).Safe().Ctx(ctx).Where("code", row.Code).Data(g.Map{"status": sysin.BotCodeStatusAuthorized, "telegram_user_id": telegramUserId, "telegram_username": telegramUsername, "bot_id": botId, "updated_at": now}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新绑定验证码失败")
	}
	text, markup := s.bindSuccessMessage(ctx, row.App, row.AccountId, telegramUserId, telegramUsername)
	replyErr := s.replyWithMarkup(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), text, markup)
	if keyboardErr := s.ensureReplyKeyboardCurrent(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID)); keyboardErr != nil {
		g.Log().Warningf(ctx, "绑定成功后刷新Telegram底部键盘失败 botId:%d chatId:%d userId:%s err:%+v", botId, msg.Chat.ID, telegramUserId, keyboardErr)
	}
	for _, hookErr := range service.TriggerAccountBoundHooks(ctx, &service.AccountBoundEvent{
		App:              row.App,
		AccountId:        row.AccountId,
		BotId:            botId,
		TelegramUserId:   telegramUserId,
		TelegramUsername: telegramUsername,
	}) {
		g.Log().Warningf(ctx, "Telegram绑定后置处理失败 accountId:%d err:%+v", row.AccountId, hookErr)
	}
	_ = s.notifySuperAdmins(ctx, botId, superNotifyBind, botBindNotifyText(row.App, row.AccountId, telegramUsername))
	return replyErr
}

func (s *sSysBot) activeTelegramBindingConflict(ctx context.Context, app string, telegramUserId string, accountId int64) (bool, error) {
	count, err := g.DB().Model(accountBindTbl).Safe().Ctx(ctx).
		Where("app", strings.TrimSpace(app)).
		Where("telegram_user_id", strings.TrimSpace(telegramUserId)).
		WhereNot("account_id", accountId).
		Where("status", consts.StatusEnabled).
		WhereNull("deleted_at").
		Count()
	return count > 0, err
}

func (s *sSysBot) consumeLoginCode(ctx context.Context, botId int64, msg *models.Message, row *authCodeRow) error {
	telegramUserId := fmt.Sprintf("%d", msg.From.ID)
	bind, err := s.bindingByTelegram(ctx, row.App, telegramUserId)
	if err != nil {
		return err
	}
	if bind == nil || bind.AccountId <= 0 {
		_, _ = s.markCodeFailed(ctx, row.Code, sysin.BotCodeStatusFailed, "当前Telegram未绑定账号")
		return s.reply(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), "当前 Telegram 未绑定账号，请先在个人页面完成绑定。")
	}
	login, err := s.loginBoundAccount(ctx, row.App, bind.AccountId)
	if err != nil {
		_, _ = s.markCodeFailed(ctx, row.Code, sysin.BotCodeStatusFailed, err.Error())
		return s.reply(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), err.Error())
	}
	now := gtime.Now()
	_, err = g.DB().Model(authCodeTable).Safe().Ctx(ctx).Where("code", row.Code).Data(g.Map{"status": sysin.BotCodeStatusAuthorized, "telegram_user_id": telegramUserId, "telegram_username": strings.TrimPrefix(msg.From.Username, "@"), "bot_id": botId, "login_token": login.Token, "account_id": bind.AccountId, "updated_at": now}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新登录验证码失败")
	}
	return s.reply(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), "登录确认成功，请回到网页继续。")
}

func (s *sSysBot) markCodeFailed(ctx context.Context, code, status, message string) (int64, error) {
	res, err := g.DB().Model(authCodeTable).Safe().Ctx(ctx).Where("code", code).Data(g.Map{"status": status, "error_message": message, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
