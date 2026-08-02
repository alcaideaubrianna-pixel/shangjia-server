package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const botInviteCodeTable = "hg_youban_bot_invite_code"

const (
	registerInviteStatusActive  = "active"
	registerInviteStatusUsed    = "used"
	registerInviteStatusExpired = "expired"
)

type registerInviteCodeRow struct {
	Id        int64       `json:"id"`
	Code      string      `json:"code"`
	Status    string      `json:"status"`
	ExpiresAt *gtime.Time `json:"expires_at"`
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

func normalizeRegisterInviteCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}
