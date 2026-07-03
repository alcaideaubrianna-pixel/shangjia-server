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
)

func (s *sSysPublish) AdminAccountSettingView(ctx context.Context, in *sysin.AccountSettingViewInp) (*sysin.AccountSettingModel, error) {
	admin, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.AccountId <= 0 {
		return nil, gerror.New("账号ID不能为空")
	}
	if err = s.ensureTenantAccount(ctx, admin.TenantId, in.AccountId); err != nil {
		return nil, err
	}
	return s.accountSetting(ctx, admin.TenantId, in.AccountId)
}

func (s *sSysPublish) MyAccountSettingView(ctx context.Context) (*sysin.AccountSettingModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.accountSetting(ctx, account.TenantId, account.Id)
}

func (s *sSysPublish) AdminAccountSettingSave(ctx context.Context, in *sysin.AccountSettingSaveInp) (*sysin.AccountSettingModel, error) {
	admin, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("账号设置不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if err = s.ensureTenantAccount(ctx, admin.TenantId, in.AccountId); err != nil {
		return nil, err
	}
	if err = s.saveAccountSetting(ctx, admin.TenantId, admin.Id, in); err != nil {
		return nil, err
	}
	return s.accountSetting(ctx, admin.TenantId, in.AccountId)
}

func (s *sSysPublish) accountSetting(ctx context.Context, tenantId int64, accountId int64) (*sysin.AccountSettingModel, error) {
	model := defaultAccountSetting(accountId)
	row, err := g.DB().Model(publishAccountSettingTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取账号推送设置失败")
	}
	if !row.IsEmpty() {
		fillAccountSettingModel(model, row)
	}
	if err = s.fillAccountSettingPreview(ctx, tenantId, accountId, model); err != nil {
		return nil, err
	}
	return model, nil
}

func (s *sSysPublish) fillAccountSettingPreview(ctx context.Context, tenantId int64, accountId int64, model *sysin.AccountSettingModel) error {
	if model == nil || model.EnableTitleMark != 1 {
		return nil
	}
	accountName, err := s.accountDisplayName(ctx, tenantId, accountId)
	if err != nil {
		return err
	}
	profileNo, err := s.previewAccountProfileNo(ctx, tenantId, accountId, model.NumberSource)
	if err != nil {
		return err
	}
	model.PreviewMark = accountSettingPreviewMark(model, accountName, profileNo)
	return nil
}

func (s *sSysPublish) accountDisplayName(ctx context.Context, tenantId int64, accountId int64) (string, error) {
	accountColumns := pdao.YoubanPublishAccount.Columns()
	row, err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(accountColumns.Id, accountId).
		Where(accountColumns.TenantId, tenantId).
		WhereNull(accountColumns.DeletedAt).
		Fields(accountColumns.Nickname, accountColumns.Username).
		One()
	if err != nil {
		return "", gerror.Wrap(err, "读取账号信息失败")
	}
	if row.IsEmpty() {
		return "", gerror.New("账号不存在或无权操作")
	}
	name := strings.TrimSpace(row[accountColumns.Nickname].String())
	if name == "" {
		name = strings.TrimSpace(row[accountColumns.Username].String())
	}
	return name, nil
}

func (s *sSysPublish) saveAccountSetting(ctx context.Context, tenantId int64, operatorId int64, in *sysin.AccountSettingSaveInp) error {
	now := gtime.Now()
	data := g.Map{
		"tenant_id":            tenantId,
		"account_id":           in.AccountId,
		"enable_suffix":        in.EnableSuffix,
		"suffix_content":       in.SuffixContent,
		"enable_title_mark":    in.EnableTitleMark,
		"mark_mode":            in.MarkMode,
		"number_source":        in.NumberSource,
		"custom_mark_text":     in.CustomMarkText,
		"mark_position":        in.MarkPosition,
		"default_recycle_days": in.DefaultRecycleDays,
		"updated_by":           operatorId,
		"updated_at":           now,
	}
	count, err := g.DB().Model(publishAccountSettingTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", in.AccountId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查账号推送设置失败")
	}
	if count > 0 {
		_, err = g.DB().Model(publishAccountSettingTable).Safe().Ctx(ctx).
			Where("tenant_id", tenantId).
			Where("account_id", in.AccountId).
			WhereNull("deleted_at").
			Data(data).
			Update()
	} else {
		data["created_by"] = operatorId
		data["created_at"] = now
		_, err = g.DB().Model(publishAccountSettingTable).Safe().Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存账号推送设置失败")
	}
	return nil
}

func (s *sSysPublish) ensureTenantAccount(ctx context.Context, tenantId int64, accountId int64) error {
	accountColumns := pdao.YoubanPublishAccount.Columns()
	count, err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(accountColumns.Id, accountId).
		Where(accountColumns.TenantId, tenantId).
		WhereNull(accountColumns.DeletedAt).
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查账号权限失败")
	}
	if count == 0 {
		return gerror.New("账号不存在或无权操作")
	}
	return nil
}

func defaultAccountSetting(accountId int64) *sysin.AccountSettingModel {
	return &sysin.AccountSettingModel{
		AccountId:          accountId,
		EnableTitleMark:    1,
		MarkMode:           "nickname",
		NumberSource:       "sequence",
		MarkPosition:       "top",
		DefaultRecycleDays: 0,
	}
}

func fillAccountSettingModel(model *sysin.AccountSettingModel, row gdb.Record) {
	model.AccountId = row["account_id"].Int64()
	model.EnableSuffix = row["enable_suffix"].Int()
	model.SuffixContent = strings.TrimSpace(row["suffix_content"].String())
	model.EnableTitleMark = row["enable_title_mark"].Int()
	model.MarkMode = strings.TrimSpace(row["mark_mode"].String())
	model.NumberSource = strings.TrimSpace(row["number_source"].String())
	model.CustomMarkText = strings.TrimSpace(row["custom_mark_text"].String())
	model.MarkPosition = strings.TrimSpace(row["mark_position"].String())
	model.DefaultRecycleDays = row["default_recycle_days"].Int()
	model.CreatedAt = row["created_at"].GTime()
	model.UpdatedAt = row["updated_at"].GTime()
	if model.MarkMode == "" {
		model.MarkMode = "nickname"
	}
	if model.NumberSource == "" {
		model.NumberSource = "sequence"
	}
	if model.MarkPosition == "" {
		model.MarkPosition = "bottom"
	}
}
