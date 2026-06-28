package sys

import (
	"context"

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

func (s *sSysPublish) ensureAdminMemberForAccount(ctx context.Context, in *sysin.AccountSaveInp) (memberId int64, err error) {
	accountConf, err := service.SysConfig().GetAccount(ctx)
	if err != nil {
		return 0, err
	}

	if in.AdminMemberId > 0 {
		memberId = in.AdminMemberId
		if in.Password == "" {
			return memberId, nil
		}
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
		if bound == 0 && in.Id == 0 {
			return 0, gerror.New("后台账号已存在，请更换账号")
		}
		if in.Password == "" {
			return memberId, nil
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
