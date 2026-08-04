package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	botService "hotgo/addons/youban_bot/service"
)

const (
	botInviteCodeTable       = "hg_youban_bot_invite_code"
	botAccountBindTable      = "hg_youban_bot_account_bind"
	registerInviteSourceSelf = "self_register"
	registerBindAppApi       = "api"
)

const (
	registerInviteStatusActive  = "active"
	registerInviteStatusUsed    = "used"
	registerInviteStatusExpired = "expired"
)

type registerInviteCodeRow struct {
	Id                            int64       `json:"id"`
	Code                          string      `json:"code"`
	Source                        string      `json:"source"`
	Status                        string      `json:"status"`
	ExpiresAt                     *gtime.Time `json:"expires_at"`
	RegistrationTelegramUserId    string      `json:"registration_telegram_user_id"`
	RegistrationTelegramUsername  string      `json:"registration_telegram_username"`
	RegistrationTelegramFirstName string      `json:"registration_telegram_first_name"`
	RegistrationTelegramLastName  string      `json:"registration_telegram_last_name"`
	RegistrationBotId             int64       `json:"registration_bot_id"`
}

func (s *sSysPublish) validateRegisterInviteCode(ctx context.Context, code string) (*registerInviteCodeRow, error) {
	return s.validateRegisterInviteCodeTx(ctx, nil, code, false)
}

func (s *sSysPublish) validateRegisterInviteCodeTx(ctx context.Context, tx gdb.TX, code string, lock bool) (*registerInviteCodeRow, error) {
	code = normalizeRegisterInviteCode(code)
	if code == "" {
		return nil, gerror.New("邀请码不能为空")
	}
	model := g.DB().Model(botInviteCodeTable).Safe().Ctx(ctx)
	if tx != nil {
		model = tx.Model(botInviteCodeTable).Safe().Ctx(ctx)
	}
	model = model.Where("code", code).WhereNull("deleted_at")
	if lock {
		model = model.LockUpdate()
	}
	var row *registerInviteCodeRow
	if err := model.Scan(&row); err != nil {
		return nil, gerror.Wrap(err, "校验邀请码失败")
	}
	if row == nil || row.Id <= 0 {
		return nil, gerror.New("邀请码不存在或已失效")
	}
	if row.Status != registerInviteStatusActive {
		return nil, gerror.New("邀请码已使用或已失效")
	}
	if row.ExpiresAt != nil && row.ExpiresAt.Before(gtime.Now()) {
		_, _ = model.Data(g.Map{"status": registerInviteStatusExpired, "updated_at": gtime.Now()}).Update()
		return nil, gerror.New("邀请码已过期")
	}
	return row, nil
}

func (s *sSysPublish) markRegisterInviteUsedTx(ctx context.Context, tx gdb.TX, inviteId int64, tenantId int64, accountId int64, username string) error {
	result, err := tx.Model(botInviteCodeTable).Safe().Ctx(ctx).
		Where("id", inviteId).
		Where("status", registerInviteStatusActive).
		WhereNull("deleted_at").
		Data(g.Map{
			"status":          registerInviteStatusUsed,
			"used_tenant_id":  tenantId,
			"used_account_id": accountId,
			"used_username":   strings.TrimSpace(username),
			"used_at":         gtime.Now(),
			"updated_at":      gtime.Now(),
		}).Update()
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return gerror.New("邀请码状态已变化，未记录使用关系")
	}
	return nil
}

func (s *sSysPublish) bindRegisterTelegramTx(ctx context.Context, tx gdb.TX, invite *registerInviteCodeRow, accountId int64) (*botService.AccountBoundEvent, error) {
	if invite == nil || strings.TrimSpace(invite.Source) != registerInviteSourceSelf {
		return nil, nil
	}
	telegramUserId := strings.TrimSpace(invite.RegistrationTelegramUserId)
	if telegramUserId == "" || accountId <= 0 {
		return nil, gerror.New("立即注册链接缺少 Telegram 绑定信息，请返回机器人重新生成")
	}
	var active struct {
		AccountId      int64  `json:"account_id"`
		TelegramUserId string `json:"telegram_user_id"`
	}
	if err := tx.Model(botAccountBindTable).Safe().Ctx(ctx).
		Fields("account_id,telegram_user_id").
		Where("app", registerBindAppApi).
		Where("status", 1).
		WhereNull("deleted_at").
		Where("(account_id=? OR telegram_user_id=?)", accountId, telegramUserId).
		Scan(&active); err != nil {
		return nil, gerror.Wrap(err, "检查注册账号Telegram绑定失败")
	}
	if active.AccountId > 0 && (active.AccountId != accountId || active.TelegramUserId != telegramUserId) {
		return nil, gerror.New("当前 Telegram 已绑定其他账号，不能重复注册绑定")
	}
	now := gtime.Now()
	data := g.Map{
		"app":                 registerBindAppApi,
		"account_id":          accountId,
		"telegram_user_id":    telegramUserId,
		"telegram_username":   strings.TrimPrefix(strings.TrimSpace(invite.RegistrationTelegramUsername), "@"),
		"telegram_first_name": strings.TrimSpace(invite.RegistrationTelegramFirstName),
		"telegram_last_name":  strings.TrimSpace(invite.RegistrationTelegramLastName),
		"bot_id":              invite.RegistrationBotId,
		"status":              1,
		"updated_at":          now,
		"deleted_at":          nil,
	}
	var existing struct {
		Id int64 `json:"id"`
	}
	if err := tx.Model(botAccountBindTable).Safe().Ctx(ctx).
		Fields("id").
		Where("app", registerBindAppApi).
		Where("(account_id=? OR telegram_user_id=?)", accountId, telegramUserId).
		OrderDesc("status").OrderDesc("id").Scan(&existing); err != nil {
		return nil, gerror.Wrap(err, "读取Telegram历史绑定失败")
	}
	if existing.Id > 0 {
		if _, err := tx.Model(botAccountBindTable).Safe().Ctx(ctx).Where("id", existing.Id).Data(data).Update(); err != nil {
			return nil, gerror.Wrap(err, "恢复注册账号Telegram绑定失败")
		}
	} else {
		data["created_at"] = now
		if _, err := tx.Model(botAccountBindTable).Safe().Ctx(ctx).Data(data).Insert(); err != nil {
			return nil, gerror.Wrap(err, "创建注册账号Telegram绑定失败")
		}
	}
	if _, err := tx.Model(botInviteCodeTable).Safe().Ctx(ctx).Where("id", invite.Id).Data(g.Map{"registration_bound_at": now, "updated_at": now}).Update(); err != nil {
		return nil, gerror.Wrap(err, "更新立即注册绑定状态失败")
	}
	return &botService.AccountBoundEvent{
		App:              registerBindAppApi,
		AccountId:        accountId,
		BotId:            invite.RegistrationBotId,
		TelegramUserId:   telegramUserId,
		TelegramUsername: strings.TrimPrefix(strings.TrimSpace(invite.RegistrationTelegramUsername), "@"),
	}, nil
}

func normalizeRegisterInviteCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}
