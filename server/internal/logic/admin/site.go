// Package admin
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Ms <133814250@qq.com>
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package admin

import (
	"context"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/token"
	"hotgo/internal/model"
	"hotgo/internal/model/entity"
	"hotgo/internal/model/input/adminin"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
	"hotgo/utility/simple"
	"strings"

	"github.com/gogf/gf/v2/crypto/gmd5"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/grand"
)

type sAdminSite struct{}

func NewAdminSite() *sAdminSite {
	return &sAdminSite{}
}

func init() {
	service.RegisterAdminSite(NewAdminSite())
}

// Register 账号注册
func (s *sAdminSite) Register(ctx context.Context, in *adminin.RegisterInp) (err error) {
	config, err := service.SysConfig().GetLogin(ctx)
	if err != nil {
		return
	}

	if config.ForceInvite == 1 && in.InviteCode == "" {
		err = gerror.New("请填写邀请码")
		return
	}

	var data adminin.MemberAddInp

	// 默认上级
	data.Pid = 1

	// 存在邀请人
	if in.InviteCode != "" {
		pmb, err := service.AdminMember().GetIdByCode(ctx, &adminin.GetIdByCodeInp{Code: in.InviteCode})
		if err != nil {
			return err
		}

		if pmb == nil {
			err = gerror.New("邀请人信息不存在")
			return err
		}

		data.Pid = pmb.Id
	}

	if config.RegisterSwitch != 1 {
		err = gerror.New("管理员未开放注册")
		return
	}

	if config.RoleId < 1 {
		err = gerror.New("管理员未配置默认角色")
		return
	}

	if config.DeptId < 1 {
		err = gerror.New("管理员未配置默认部门")
		return
	}

	// 验证唯一性
	err = service.AdminMember().VerifyUnique(ctx, &adminin.VerifyUniqueInp{
		Where: g.Map{
			dao.AdminMember.Columns().Username: in.Username,
			dao.AdminMember.Columns().Mobile:   in.Mobile,
		},
	})
	if err != nil {
		return
	}

	// 验证短信验证码
	err = service.SysSmsLog().VerifyCode(ctx, &sysin.VerifyCodeInp{
		Event:  consts.SmsTemplateRegister,
		Mobile: in.Mobile,
		Code:   in.Code,
	})
	if err != nil {
		return
	}

	data.MemberEditInp = &adminin.MemberEditInp{
		Id:       0,
		RoleId:   config.RoleId,
		PostIds:  config.PostIds,
		DeptId:   config.DeptId,
		Username: in.Username,
		Password: in.Password,
		RealName: "",
		Avatar:   config.Avatar,
		Sex:      3, // 保密
		Mobile:   in.Mobile,
		Status:   consts.StatusEnabled,
	}
	data.Salt = grand.S(6)
	data.InviteCode = grand.S(12)
	data.PasswordHash = gmd5.MustEncryptString(data.Password + data.Salt)
	data.Level, data.Tree, err = service.AdminMember().GenTree(ctx, data.Pid)
	if err != nil {
		return
	}

	// 提交注册信息
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) (err error) {
		id, err := dao.AdminMember.Ctx(ctx).Data(data).OmitEmptyData().InsertAndGetId()
		if err != nil {
			err = gerror.Wrap(err, consts.ErrorORM)
			return
		}

		// 更新岗位
		if err = service.AdminMemberPost().UpdatePostIds(ctx, id, config.PostIds); err != nil {
			err = gerror.Wrap(err, consts.ErrorORM)
		}
		return
	})
}

// MobileRegister 移动端账号注册
func (s *sAdminSite) MobileRegister(ctx context.Context, in *adminin.MemberRegisterInp) (res *adminin.LoginModel, err error) {
	config, err := service.SysConfig().GetLogin(ctx)
	if err != nil {
		return
	}

	if config.ForceInvite == 1 && in.InviteCode == "" {
		return nil, gerror.New("请填写邀请码")
	}
	if config.RegisterSwitch != 1 {
		return nil, gerror.New("管理员未开放注册")
	}
	if config.RoleId < 1 {
		return nil, gerror.New("管理员未配置默认角色")
	}
	if config.DeptId < 1 {
		return nil, gerror.New("管理员未配置默认部门")
	}

	account := strings.TrimSpace(in.Account)
	if account == "" {
		return nil, gerror.New("账号不能为空")
	}

	var data adminin.MemberAddInp
	data.Pid = 1
	if in.ShareToken != "" && in.InviteCode == "" {
		share, shareErr := service.SysMemberApp().OpenShare(ctx, &sysin.MemberShareOpenInp{ShareToken: in.ShareToken})
		if shareErr == nil && share != nil {
			in.InviteCode = share.InviteCode
		}
	}
	if in.InviteCode != "" {
		pmb, err := service.AdminMember().GetIdByCode(ctx, &adminin.GetIdByCodeInp{Code: in.InviteCode})
		if err != nil {
			return nil, err
		}
		if pmb == nil {
			return nil, gerror.New("邀请人信息不存在")
		}
		data.Pid = pmb.Id
	}

	cols := dao.AdminMember.Columns()
	if err = s.verifyRegisterAccountUnique(ctx, cols.Username, account, "账号已存在"); err != nil {
		return nil, err
	}

	email := ""
	mobile := ""
	if strings.Contains(account, "@") {
		email = account
		if err = s.verifyRegisterAccountUnique(ctx, cols.Email, email, "邮箱已存在"); err != nil {
			return nil, err
		}
	} else if isMobileAccount(account) {
		mobile = account
		if err = s.verifyRegisterAccountUnique(ctx, cols.Mobile, mobile, "手机号已存在"); err != nil {
			return nil, err
		}
	}

	data.MemberEditInp = &adminin.MemberEditInp{
		Id:       0,
		RoleId:   config.RoleId,
		PostIds:  config.PostIds,
		DeptId:   config.DeptId,
		Username: account,
		Password: in.Password,
		RealName: "",
		Avatar:   config.Avatar,
		Sex:      3,
		Email:    email,
		Mobile:   mobile,
		Status:   consts.StatusEnabled,
	}
	data.Salt = grand.S(6)
	data.InviteCode = grand.S(12)
	data.PasswordHash = gmd5.MustEncryptString(data.Password + data.Salt)
	data.Level, data.Tree, err = service.AdminMember().GenTree(ctx, data.Pid)
	if err != nil {
		return nil, err
	}

	var member *entity.AdminMember
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) (err error) {
		id, err := dao.AdminMember.Ctx(ctx).Data(data).OmitEmptyData().InsertAndGetId()
		if err != nil {
			return gerror.Wrap(err, consts.ErrorORM)
		}

		if err = service.AdminMemberPost().UpdatePostIds(ctx, id, config.PostIds); err != nil {
			return gerror.Wrap(err, consts.ErrorORM)
		}

		if err = dao.AdminMember.Ctx(ctx).WherePri(id).Scan(&member); err != nil {
			return gerror.Wrap(err, consts.ErrorORM)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if member == nil {
		return nil, gerror.New("注册失败，请稍后重试")
	}
	if in.ShareToken != "" {
		if bindErr := service.SysMemberApp().BindShareRegister(ctx, &sysin.MemberShareRegisterInp{ShareToken: in.ShareToken, MemberId: member.Id}); bindErr != nil {
			g.Log().Warning(ctx, "绑定分享注册归因失败", g.Map{"memberId": member.Id, "shareToken": in.ShareToken, "err": bindErr.Error()})
		}
	}

	return s.handleLogin(ctx, member)
}

// AccountLogin 账号登录
func (s *sAdminSite) AccountLogin(ctx context.Context, in *adminin.AccountLoginInp) (res *adminin.LoginModel, err error) {
	defer func() {
		service.SysLoginLog().Push(ctx, &sysin.LoginLogPushInp{Response: res, Err: err})
	}()

	var mb *entity.AdminMember
	cols := dao.AdminMember.Columns()
	if err = dao.AdminMember.Ctx(ctx).
		Where(cols.Username, in.Username).
		WhereOr(cols.Mobile, in.Username).
		WhereOr(cols.Email, in.Username).
		Scan(&mb); err != nil {
		err = gerror.Wrap(err, consts.ErrorORM)
		return
	}

	if mb == nil {
		err = gerror.New("用户名或密码错误")
		return
	}

	res = new(adminin.LoginModel)
	res.Id = mb.Id
	res.Username = mb.Username
	if mb.Salt == "" {
		err = gerror.New("用户信息错误")
		return
	}

	if err = simple.CheckPassword(in.Password, mb.Salt, mb.PasswordHash); err != nil {
		return
	}

	if mb.Status != consts.StatusEnabled {
		err = gerror.New("账号已被禁用")
		return
	}

	res, err = s.handleLogin(ctx, mb)
	return
}

func (s *sAdminSite) verifyRegisterAccountUnique(ctx context.Context, column, value, message string) (err error) {
	if value == "" {
		return nil
	}
	count, err := dao.AdminMember.Ctx(ctx).Where(column, value).Count()
	if err != nil {
		return gerror.Wrap(err, consts.ErrorORM)
	}
	if count > 0 {
		return gerror.New(message)
	}
	return nil
}

func isMobileAccount(account string) bool {
	if len(account) != 11 {
		return false
	}
	for _, v := range account {
		if v < '0' || v > '9' {
			return false
		}
	}
	return true
}

// MobileLogin 手机号登录
func (s *sAdminSite) MobileLogin(ctx context.Context, in *adminin.MobileLoginInp) (res *adminin.LoginModel, err error) {
	defer func() {
		service.SysLoginLog().Push(ctx, &sysin.LoginLogPushInp{Response: res, Err: err})
	}()

	var mb *entity.AdminMember
	if err = dao.AdminMember.Ctx(ctx).Where("mobile ", in.Mobile).Scan(&mb); err != nil {
		err = gerror.Wrap(err, consts.ErrorORM)
		return
	}

	if mb == nil {
		err = gerror.New("账号不存在")
		return
	}

	res = new(adminin.LoginModel)
	res.Id = mb.Id
	res.Username = mb.Username

	err = service.SysSmsLog().VerifyCode(ctx, &sysin.VerifyCodeInp{
		Event:  consts.SmsTemplateLogin,
		Mobile: in.Mobile,
		Code:   in.Code,
	})

	if err != nil {
		return
	}

	if mb.Status != consts.StatusEnabled {
		err = gerror.New("账号已被禁用")
		return
	}

	res, err = s.handleLogin(ctx, mb)
	return
}

// handleLogin .
func (s *sAdminSite) handleLogin(ctx context.Context, mb *entity.AdminMember) (res *adminin.LoginModel, err error) {
	role, dept, err := s.getLoginRoleAndDept(ctx, mb.RoleId, mb.DeptId)
	if err != nil {
		return nil, err
	}

	user := &model.Identity{
		Id:       mb.Id,
		Pid:      mb.Pid,
		DeptId:   dept.Id,
		DeptType: dept.Type,
		RoleId:   role.Id,
		RoleKey:  role.Key,
		Username: mb.Username,
		RealName: mb.RealName,
		Avatar:   mb.Avatar,
		Email:    mb.Email,
		Mobile:   mb.Mobile,
		App:      consts.AppAdmin,
		LoginAt:  gtime.Now(),
	}

	lt, expires, err := token.Login(ctx, user)
	if err != nil {
		return nil, err
	}

	res = &adminin.LoginModel{
		Username: user.Username,
		Id:       user.Id,
		Token:    lt,
		Expires:  expires,
	}
	return
}

// getLoginRoleAndDept 获取登录的角色和部门信息
func (s *sAdminSite) getLoginRoleAndDept(ctx context.Context, roleId, deptId int64) (role *entity.AdminRole, dept *entity.AdminDept, err error) {
	if err = dao.AdminRole.Ctx(ctx).Fields(dao.AdminRole.Columns().Id, dao.AdminRole.Columns().Key, dao.AdminRole.Columns().Status).WherePri(roleId).Scan(&role); err != nil {
		err = gerror.Wrap(err, consts.ErrorORM)
		return
	}

	if role == nil {
		err = gerror.New("角色不存在或已被删除")
		return
	}

	if role.Status != consts.StatusEnabled {
		err = gerror.New("角色已被禁用，如有疑问请联系管理员")
		return
	}

	if err = dao.AdminDept.Ctx(ctx).Fields(dao.AdminDept.Columns().Id, dao.AdminDept.Columns().Type, dao.AdminDept.Columns().Status).WherePri(deptId).Scan(&dept); err != nil {
		err = gerror.Wrap(err, "获取部门信息失败，请稍后重试！")
		return
	}

	if dept == nil {
		err = gerror.New("部门不存在或已被删除")
		return
	}

	if dept.Status != consts.StatusEnabled {
		err = gerror.New("部门已被禁用，如有疑问请联系管理员")
		return
	}
	return
}

// BindUserContext 绑定用户上下文
func (s *sAdminSite) BindUserContext(ctx context.Context, claims *model.Identity) (err error) {
	//// 如果不想每次访问都重新加载用户信息，可以放开注释。但在本次登录未失效前，用户信息不会刷新
	// contexts.SetUser(ctx, claims)
	// return

	var mb *entity.AdminMember
	if err = dao.AdminMember.Ctx(ctx).WherePri(claims.Id).Scan(&mb); err != nil {
		err = gerror.Wrap(err, "获取用户信息失败，请稍后重试！")
		return
	}

	if mb == nil {
		err = gerror.Wrap(err, "账号不存在或已被删除！")
		return
	}

	if mb.Status != consts.StatusEnabled {
		err = gerror.New("账号已被禁用，如有疑问请联系管理员")
		return
	}

	role, dept, err := s.getLoginRoleAndDept(ctx, mb.RoleId, mb.DeptId)
	if err != nil {
		return err
	}

	user := &model.Identity{
		Id:       mb.Id,
		Pid:      mb.Pid,
		DeptId:   dept.Id,
		DeptType: dept.Type,
		RoleId:   mb.RoleId,
		RoleKey:  role.Key,
		Username: mb.Username,
		RealName: mb.RealName,
		Avatar:   mb.Avatar,
		Email:    mb.Email,
		Mobile:   mb.Mobile,
		App:      claims.App,
		LoginAt:  claims.LoginAt,
	}

	contexts.SetUser(ctx, user)
	return
}
