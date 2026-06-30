package sys

import (
	"context"

	"github.com/gogf/gf/v2/crypto/gmd5"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/grand"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
	"hotgo/utility/charset"
)

const publishGeneratedPasswordLength = 12

const (
	publishAccountPasswordHashColumn = "password_hash"
	publishAccountSaltColumn         = "salt"
)

func (s *sSysPublish) prepareAccountParent(ctx context.Context, in *sysin.AccountSaveInp) error {
	if in.AccountType == sysin.PublishAccountTypeAdmin {
		in.ParentId = 0
		return nil
	}
	if in.ParentId <= 0 {
		return gerror.New("请选择管理员账号")
	}
	if in.Id > 0 && in.ParentId == in.Id {
		return gerror.New("上架账号不能选择自己作为管理员账号")
	}
	accountColumns := pdao.YoubanPublishAccount.Columns()
	count, err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(accountColumns.Id, in.ParentId).
		Where(accountColumns.TenantId, in.TenantId).
		Where(accountColumns.AccountType, sysin.PublishAccountTypeAdmin).
		Where(accountColumns.Status, consts.StatusEnabled).
		WhereNull(accountColumns.DeletedAt).
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查管理员账号失败")
	}
	if count == 0 {
		return gerror.New("管理员账号不存在或不可用")
	}
	return nil
}

func (s *sSysPublish) ensurePublishAccountUnique(ctx context.Context, id int64, username string) error {
	accountColumns := pdao.YoubanPublishAccount.Columns()
	mod := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(accountColumns.Username, username).
		WhereNull(accountColumns.DeletedAt)
	if id > 0 {
		mod = mod.WhereNot(accountColumns.Id, id)
	}
	count, err := mod.Count()
	if err != nil {
		return gerror.Wrap(err, "检查账号唯一性失败")
	}
	if count > 0 {
		return gerror.New("账号已存在，请更换账号")
	}
	return nil
}

func (s *sSysPublish) fillAccountPasswordData(data g.Map, in *sysin.AccountSaveInp) {
	password := in.Password
	if in.Id == 0 && password == "" {
		password = string(charset.RandomCreateBytes(publishGeneratedPasswordLength))
		in.Password = password
	}
	if password == "" {
		return
	}
	salt := grand.S(6)
	data[publishAccountSaltColumn] = salt
	data[publishAccountPasswordHashColumn] = gmd5.MustEncryptString(password + salt)
}

func (s *sSysPublish) UpdateAccountPassword(ctx context.Context, in *sysin.UpdateAccountPasswordInp) error {
	if err := in.Filter(ctx); err != nil {
		return err
	}
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if _, err = s.verifyPublishAccountPassword(ctx, account.Username, in.OldPassword); err != nil {
		return gerror.New("当前密码不正确")
	}

	accountColumns := pdao.YoubanPublishAccount.Columns()
	data := g.Map{
		accountColumns.UpdatedBy: contexts.GetUserId(ctx),
		accountColumns.UpdatedAt: gtime.Now(),
	}
	s.fillAccountPasswordData(data, &sysin.AccountSaveInp{
		Id:       account.Id,
		Password: in.NewPassword,
	})

	result, err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(accountColumns.Id, account.Id).
		WhereNull(accountColumns.DeletedAt).
		Data(data).
		Update()
	if err != nil {
		return gerror.Wrap(err, "修改密码失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return gerror.New("账号不存在")
	}
	return nil
}

func (s *sSysPublish) UpdateAccountProfile(ctx context.Context, in *sysin.UpdateAccountProfileInp) (*sysin.CurrentAccountModel, error) {
	if err := in.Filter(ctx); err != nil {
		return nil, err
	}
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}

	accountColumns := pdao.YoubanPublishAccount.Columns()
	result, err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(accountColumns.Id, account.Id).
		WhereNull(accountColumns.DeletedAt).
		Data(g.Map{
			accountColumns.Nickname:  in.Nickname,
			accountColumns.Remark:    in.Remark,
			accountColumns.UpdatedBy: contexts.GetUserId(ctx),
			accountColumns.UpdatedAt: gtime.Now(),
		}).
		Update()
	if err != nil {
		return nil, gerror.Wrap(err, "修改基本信息失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, gerror.New("账号不存在")
	}

	return s.CurrentAccount(ctx)
}

func (s *sSysPublish) verifyPublishAccountPassword(ctx context.Context, username string, password string) (*sysin.AccountModel, error) {
	accountColumns := pdao.YoubanPublishAccount.Columns()
	var account *sysin.AccountModel
	err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(accountColumns.Username, username).
		WhereNull(accountColumns.DeletedAt).
		Scan(&account)
	if err != nil {
		return nil, gerror.Wrap(err, "读取上架账号失败")
	}
	if account == nil || account.Id <= 0 {
		return nil, gerror.New("账号或密码错误")
	}
	if account.Status != consts.StatusEnabled {
		return nil, gerror.New("账号已被停用")
	}
	row, err := pdao.YoubanPublishAccount.Ctx(ctx).
		Fields(publishAccountPasswordHashColumn, publishAccountSaltColumn).
		Where(accountColumns.Id, account.Id).
		WhereNull(accountColumns.DeletedAt).
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取账号密码失败")
	}
	salt := row[publishAccountSaltColumn].String()
	passwordHash := row[publishAccountPasswordHashColumn].String()
	if salt == "" || passwordHash == "" {
		return nil, gerror.New("账号密码未初始化，请在后台重置密码")
	}
	if passwordHash != gmd5.MustEncryptString(password+salt) {
		return nil, gerror.New("账号或密码错误")
	}
	return account, nil
}
