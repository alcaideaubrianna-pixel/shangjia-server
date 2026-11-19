package sys

import (
	"context"
	"fmt"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
)

func (s *sSysBot) resolveCollectorAccount(ctx context.Context, botID, tenantID, ownerAccountID int64, user *models.User, allowProvision bool) (*botProfileAccount, error) {
	if user == nil || user.ID <= 0 || tenantID <= 0 {
		return nil, gerror.New("无法识别当前机器人或Telegram用户")
	}
	userID := fmt.Sprint(user.ID)
	bind, err := s.bindingByTelegram(ctx, "api", userID)
	if err != nil {
		return nil, err
	}
	if bind != nil {
		capability, err := ps.SysPublish().AccountCapability(ctx, "api", bind.AccountId)
		if err != nil {
			return nil, err
		}
		if capability == nil || capability.AccountId <= 0 {
			return nil, gerror.New("已绑定的上架账户不可用，请联系管理员")
		}
		if capability.TenantId != tenantID {
			return nil, gerror.New("已绑定其他租户的上架账户，请联系管理员")
		}
		if capability.AccountType == pi.PublishAccountTypeUploader && capability.TelegramBindingEnabled != 1 {
			return nil, gerror.New("当前上架账户未开启TG绑定权限，请联系管理员")
		}
		return &botProfileAccount{
			TenantId: capability.TenantId, AccountId: capability.AccountId,
			AccountType: capability.AccountType, App: "api",
			SharedResourceEnabled: capability.SharedResourceEnabled == 1,
		}, nil
	}
	if !allowProvision {
		return nil, nil
	}
	owner, err := ps.SysPublish().AccountCapability(ctx, "api", ownerAccountID)
	if err != nil {
		return nil, err
	}
	if owner == nil || owner.TenantId != tenantID || owner.AccountType != pi.PublishAccountTypeAdmin {
		return nil, gerror.New("机器人所属管理账户不可用，请联系管理员")
	}

	err = g.DB().Transaction(ctx, func(ctx context.Context, _ gdb.TX) error {
		_, err := s.createAndBindUploader(ctx, botID, ownerAccountID, fmt.Sprintf("tg_%d", user.ID), *user)
		return err
	})
	account, lookupErr := s.resolveCollectorAccount(ctx, botID, tenantID, ownerAccountID, user, false)
	if account != nil || lookupErr != nil {
		return account, lookupErr
	}
	if err != nil {
		return nil, err
	}
	return nil, gerror.New("上架账户绑定未完成")
}

// The caller supplies an authorized administrator and owns the transaction.
func (s *sSysBot) createAndBindUploader(ctx context.Context, botID, ownerAccountID int64, username string, user models.User) (*pi.AccountSaveModel, error) {
	authCtx := quickPushMediaUploadContext(ctx, ownerAccountID)
	in := uploaderCreateInput(username)
	in.TelegramUserId = fmt.Sprint(user.ID)
	in.TelegramUsername = user.Username
	created, err := ps.SysPublish().AdminAccountSave(authCtx, in)
	if err != nil {
		return nil, err
	}
	if created == nil || created.Id <= 0 {
		return nil, gerror.New("未返回创建结果")
	}
	_, err = ps.SysPublish().PatchAccountSetting(authCtx, &pi.AccountSettingPatchInp{AccountId: created.Id, Field: "telegramBindingEnabled", Value: "1"})
	if err != nil {
		return nil, err
	}
	_, err = g.DB().Model(accountBindTbl).Ctx(ctx).Data(g.Map{
		"app": "api", "account_id": created.Id, "telegram_user_id": in.TelegramUserId,
		"telegram_username": user.Username, "telegram_first_name": user.FirstName,
		"telegram_last_name": user.LastName, "bot_id": botID, "status": 1,
	}).Insert()
	if err != nil {
		return nil, gerror.Wrap(err, "绑定TG失败（可能已绑定其他账户）")
	}
	return created, nil
}
