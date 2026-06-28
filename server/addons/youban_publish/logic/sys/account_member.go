package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/model/input/adminin"
	baseservice "hotgo/internal/service"
	"hotgo/utility/charset"
)

const publishGeneratedPasswordLength = 12

func (s *sSysPublish) prepareAdminMemberBinding(ctx context.Context, in *sysin.AccountSaveInp) error {
	if in.Id <= 0 {
		in.AdminMemberId = 0
		return nil
	}
	value, err := g.DB().Model(publishAccountTable).Safe().Ctx(ctx).
		Fields(publishAccountFieldAdminMemberId).
		Where(publishFieldId, in.Id).
		WhereNull(publishAccountFieldDeletedAt).
		Value()
	if err != nil {
		return gerror.Wrap(err, "读取账号绑定关系失败")
	}
	if value.IsNil() || value.Int64() <= 0 {
		return gerror.New("上架账号不存在")
	}
	in.AdminMemberId = value.Int64()
	return nil
}

func (s *sSysPublish) prepareAccountParent(ctx context.Context, in *sysin.AccountSaveInp) error {
	if in.AccountType == sysin.PublishAccountTypeAdmin {
		in.ParentId = 0
		return nil
	}
	if in.ParentId <= 0 {
		return gerror.New("上架账号必须选择管理员账号")
	}
	if in.Id > 0 && in.ParentId == in.Id {
		return gerror.New("上架账号不能选择自己作为管理员账号")
	}
	count, err := g.DB().Model(publishAccountTable).Safe().Ctx(ctx).
		Where(publishFieldId, in.ParentId).
		Where(publishAccountFieldMerchantId, in.MerchantId).
		Where(publishAccountFieldAccountType, sysin.PublishAccountTypeAdmin).
		Where(publishAccountFieldStatus, consts.StatusEnabled).
		WhereNull(publishAccountFieldDeletedAt).
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查管理员账号失败")
	}
	if count == 0 {
		return gerror.New("管理员账号不存在或不可用")
	}
	return nil
}

func (s *sSysPublish) adminMemberIdsForAccounts(ctx context.Context, tx gdb.TX, ids []int64) (memberIds []int64, err error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if err = tx.Model(publishAccountTable).Safe().Ctx(ctx).
		Fields(publishAccountFieldAdminMemberId).
		WhereIn(publishFieldId, ids).
		WhereNull(publishAccountFieldDeletedAt).
		Scan(&memberIds); err != nil {
		return nil, gerror.Wrap(err, "读取账号绑定关系失败")
	}
	return memberIds, nil
}

func (s *sSysPublish) disableAdminMembers(ctx context.Context, tx gdb.TX, memberIds []int64) error {
	if len(memberIds) == 0 {
		return nil
	}
	memberColumns := dao.AdminMember.Columns()
	if _, err := tx.Model(dao.AdminMember.Table()).Safe().Ctx(ctx).
		WhereIn(memberColumns.Id, memberIds).
		Data(g.Map{memberColumns.Status: consts.StatusDisable}).
		Update(); err != nil {
		return gerror.Wrap(err, "禁用后台登录账号失败")
	}
	return nil
}

func (s *sSysPublish) ensureAdminMemberForAccount(ctx context.Context, in *sysin.AccountSaveInp) (memberId int64, err error) {
	accountConf, err := service.SysConfig().GetAccount(ctx)
	if err != nil {
		return 0, err
	}

	if in.AdminMemberId > 0 {
		memberId = in.AdminMemberId
		return memberId, s.saveAdminMember(ctx, memberId, in, accountConf.DefaultRoleId, accountConf.DefaultDeptId)
	}

	memberColumns := dao.AdminMember.Columns()
	existing, err := dao.AdminMember.Ctx(ctx).
		Fields(memberColumns.Id).
		Where(memberColumns.Username, in.Username).
		Value()
	if err != nil {
		return 0, gerror.Wrap(err, "检查后台账号失败")
	}
	if existing.Int64() > 0 {
		memberId = existing.Int64()
		bound, err := g.DB().Model(publishAccountTable).Safe().Ctx(ctx).
			Where(publishAccountFieldAdminMemberId, memberId).
			WhereNull(publishAccountFieldDeletedAt).
			Count()
		if err != nil {
			return 0, gerror.Wrap(err, "检查账号绑定关系失败")
		}
		if in.Id == 0 {
			return 0, gerror.New("后台账号已存在，请更换账号")
		}
		if bound == 0 {
			return 0, gerror.New("后台账号已存在，请更换账号")
		}
		return memberId, s.saveAdminMember(ctx, memberId, in, accountConf.DefaultRoleId, accountConf.DefaultDeptId)
	}

	if err = s.saveAdminMember(ctx, 0, in, accountConf.DefaultRoleId, accountConf.DefaultDeptId); err != nil {
		return 0, gerror.Wrap(err, "创建后台登录账号失败")
	}

	value, err := dao.AdminMember.Ctx(ctx).
		Fields(memberColumns.Id).
		Where(memberColumns.Username, in.Username).
		Value()
	if err != nil {
		return 0, gerror.Wrap(err, "读取后台账号失败")
	}
	if value.Int64() <= 0 {
		return 0, gerror.New("创建后台账号失败")
	}
	return value.Int64(), nil
}

func (s *sSysPublish) saveAdminMember(ctx context.Context, id int64, in *sysin.AccountSaveInp, roleId int64, deptId int64) error {
	status := in.Status
	if id == 0 {
		status = consts.StatusEnabled
		if in.Password == "" {
			in.Password = string(charset.RandomCreateBytes(publishGeneratedPasswordLength))
		}
	}
	return baseservice.AdminMember().Edit(ctx, &adminin.MemberEditInp{
		Id:       id,
		RoleId:   roleId,
		DeptId:   deptId,
		Username: in.Username,
		Password: in.Password,
		RealName: in.Nickname,
		Remark:   in.Remark,
		Status:   status,
	})
}
