package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/contexts"
	"hotgo/utility/charset"
)

func (s *sSysPublish) AdminAccountList(ctx context.Context, in *sysin.AccountListInp) (list []*sysin.AccountModel, totalCount int, err error) {
	current, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	in.TenantId = current.TenantId
	return s.accountList(ctx, in)
}

func (s *sSysPublish) AdminAccountSave(ctx context.Context, in *sysin.AccountSaveInp) (res *sysin.AccountSaveModel, err error) {
	current, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in.Id == current.Id {
		return nil, gerror.New("不能在账号管理中操作当前登录账号")
	}
	in.TenantId = current.TenantId
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if err = s.ensureEditableAccount(ctx, in.Id, current.TenantId); err != nil {
		return nil, err
	}
	if err = s.prepareAccountParent(ctx, in); err != nil {
		return nil, err
	}
	if err = s.savePublishAccount(ctx, nil, in); err != nil {
		return nil, err
	}
	return &sysin.AccountSaveModel{Id: in.Id, Password: in.Password}, nil
}

func (s *sSysPublish) AdminAccountResetPassword(ctx context.Context, in *sysin.AccountResetPasswordInp) (res *sysin.AccountSaveModel, err error) {
	current, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in.Id == current.Id {
		return nil, gerror.New("不能重置当前登录账号密码")
	}
	if err = s.ensureEditableAccount(ctx, in.Id, current.TenantId); err != nil {
		return nil, err
	}
	accountColumns := pdao.YoubanPublishAccount.Columns()
	password := strings.TrimSpace(in.Password)
	if password == "" {
		password = string(charset.RandomCreateBytes(publishGeneratedPasswordLength))
	}
	saveIn := &sysin.AccountSaveInp{Id: in.Id, Password: password}
	data := g.Map{accountColumns.UpdatedBy: contexts.GetUserId(ctx), accountColumns.UpdatedAt: gtime.Now()}
	s.fillAccountPasswordData(data, saveIn)
	result, err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(accountColumns.Id, in.Id).
		Where(accountColumns.TenantId, current.TenantId).
		WhereNull(accountColumns.DeletedAt).
		Data(data).
		Update()
	if err != nil {
		return nil, gerror.Wrap(err, "重置账号密码失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, gerror.New("账号不存在")
	}
	return &sysin.AccountSaveModel{Id: in.Id, Password: saveIn.Password}, nil
}

func (s *sSysPublish) AdminAccountDelete(ctx context.Context, in *sysin.AccountDeleteInp) (err error) {
	if len(in.Ids) == 0 {
		return gerror.New("请选择要删除的数据")
	}
	current, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	for _, id := range in.Ids {
		if id == current.Id {
			return gerror.New("不能删除当前登录账号")
		}
	}
	accountColumns := pdao.YoubanPublishAccount.Columns()
	count, err := pdao.YoubanPublishAccount.Ctx(ctx).
		WhereIn(accountColumns.Id, in.Ids).
		Where(accountColumns.TenantId, current.TenantId).
		WhereNull(accountColumns.DeletedAt).
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查账号权限失败")
	}
	if count != len(in.Ids) {
		return gerror.New("存在无权操作的账号")
	}
	_, err = pdao.YoubanPublishAccount.Ctx(ctx).WhereIn(accountColumns.Id, in.Ids).Where(accountColumns.TenantId, current.TenantId).Data(g.Map{
		accountColumns.DeletedBy: contexts.GetUserId(ctx),
		accountColumns.DeletedAt: gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "删除上架账号失败")
	}
	return nil
}

func (s *sSysPublish) savePublishAccount(ctx context.Context, tx gdb.TX, in *sysin.AccountSaveInp) (err error) {
	if err = s.ensurePublishAccountUnique(ctx, in.Id, in.Username); err != nil {
		return err
	}
	accountColumns := pdao.YoubanPublishAccount.Columns()
	data := g.Map{
		accountColumns.TenantId:           in.TenantId,
		accountColumns.MerchantId:         in.TenantId,
		accountColumns.ParentId:           in.ParentId,
		accountColumns.AccountType:        in.AccountType,
		accountColumns.Nickname:           strings.TrimSpace(in.Nickname),
		accountColumns.Username:           strings.TrimSpace(in.Username),
		accountColumns.TelegramUserId:     strings.TrimSpace(in.TelegramUserId),
		accountColumns.TelegramUsername:   strings.TrimSpace(in.TelegramUsername),
		accountColumns.DailyPublishLimit:  in.DailyPublishLimit,
		accountColumns.CanDirectPublish:   in.CanDirectPublish,
		accountColumns.AllowedChannelJson: strings.TrimSpace(in.AllowedChannelJson),
		accountColumns.AllowedRegionJson:  strings.TrimSpace(in.AllowedRegionJson),
		accountColumns.Remark:             strings.TrimSpace(in.Remark),
		accountColumns.Status:             in.Status,
		accountColumns.UpdatedBy:          contexts.GetUserId(ctx),
		accountColumns.UpdatedAt:          gtime.Now(),
	}
	s.fillAccountPasswordData(data, in)
	mod := pdao.YoubanPublishAccount.Ctx(ctx)
	if tx != nil {
		mod = tx.Model(pdao.YoubanPublishAccount.Table()).Safe().Ctx(ctx)
	}
	if in.Id > 0 {
		_, err = mod.Where(accountColumns.Id, in.Id).Where(accountColumns.TenantId, in.TenantId).WhereNull(accountColumns.DeletedAt).Data(data).Update()
	} else {
		data[accountColumns.CreatedBy] = contexts.GetUserId(ctx)
		data[accountColumns.CreatedAt] = gtime.Now()
		id, insertErr := mod.Data(data).InsertAndGetId()
		err = insertErr
		if err == nil {
			in.Id = id
		}
	}
	if err != nil {
		return gerror.Wrap(err, "保存上架账号失败")
	}
	return nil
}
