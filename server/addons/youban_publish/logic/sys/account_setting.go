package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"

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
	if err = s.ensureAdminManageableAccount(ctx, admin, in.AccountId); err != nil {
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

func (s *sSysPublish) MyAccountSettingSave(ctx context.Context, in *sysin.AccountSettingSaveInp) (*sysin.AccountSettingModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("账号设置不能为空")
	}
	in.AccountId = account.Id
	// 权限开关只能由管理员授予，上架账号不能通过个人设置自行提权。
	current, settingErr := s.accountSetting(ctx, account.TenantId, account.Id)
	if settingErr != nil {
		return nil, settingErr
	}
	in.ProfileDetailViewEnabled = current.ProfileDetailViewEnabled
	in.SharedResourceEnabled = current.SharedResourceEnabled
	in.TelegramBindingEnabled = current.TelegramBindingEnabled
	in.GroupPushEnabled = current.GroupPushEnabled
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if err = s.saveAccountSetting(ctx, account.TenantId, account.Id, in); err != nil {
		return nil, err
	}
	_ = bumpAccountVisibilityVersion(ctx, account.TenantId)
	return s.accountSetting(ctx, account.TenantId, account.Id)
}

// SaveAccountNotificationSetting changes one preference without overwriting
// suffixes, permissions or other notification preferences.
func (s *sSysPublish) SaveAccountNotificationSetting(ctx context.Context, in *sysin.AccountNotificationSettingInp) error {
	if in == nil {
		return gerror.New("推送设置不能为空")
	}
	if err := in.Filter(ctx); err != nil {
		return err
	}
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if err = s.ensureAdminManageableAccount(ctx, account, in.AccountId); err != nil {
		return err
	}
	columns := map[string]string{"daily": "daily_report_enabled", "channel": "channel_alert_enabled", "account": "account_event_enabled", "task": "task_failure_enabled"}
	column := columns[in.Kind]
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// Serialize creation of a previously absent settings row across workers.
		cols := pdao.YoubanPublishAccount.Columns()
		row, err := pdao.YoubanPublishAccount.Ctx(ctx).Where(cols.Id, in.AccountId).Where(cols.TenantId, account.TenantId).LockUpdate().One()
		if err != nil {
			return err
		}
		if row.IsEmpty() {
			return gerror.New("账号不存在或已删除")
		}
		count, err := g.DB().Model(publishAccountSettingTable).Safe().Ctx(ctx).Where("tenant_id", account.TenantId).Where("account_id", in.AccountId).Count()
		if err != nil {
			return err
		}
		data := g.Map{column: in.Enabled, "updated_by": account.Id}
		if count > 0 {
			_, err = g.DB().Model(publishAccountSettingTable).Safe().Ctx(ctx).Where("tenant_id", account.TenantId).Where("account_id", in.AccountId).Data(data).Update()
		} else {
			data["tenant_id"], data["account_id"], data["created_by"] = account.TenantId, in.AccountId, account.Id
			_, err = g.DB().Model(publishAccountSettingTable).Safe().Ctx(ctx).Data(data).Insert()
		}
		return err
	})
	if err != nil {
		return gerror.Wrap(err, "保存推送设置失败")
	}
	return nil
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
	if err = s.ensureAdminManageableAccount(ctx, admin, in.AccountId); err != nil {
		return nil, err
	}
	var target sysin.AccountModel
	if err = g.DB().Model(pdao.YoubanPublishAccount.Table()).Safe().Ctx(ctx).
		Where("id", in.AccountId).Where("tenant_id", admin.TenantId).WhereNull("deleted_at").Scan(&target); err != nil {
		return nil, gerror.Wrap(err, "读取上架账号失败")
	}
	if target.AccountType != sysin.PublishAccountTypeUploader {
		in.ProfileDetailViewEnabled = 0
		in.SharedResourceEnabled = 0
		in.TelegramBindingEnabled = 0
		in.GroupPushEnabled = 0
	}
	if err = s.saveAccountSetting(ctx, admin.TenantId, admin.Id, in); err != nil {
		return nil, err
	}
	_ = bumpAccountVisibilityVersion(ctx, admin.TenantId)
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
	// Online migrations may run after this instance cached the old schema.
	// Refresh before writing so ORM field filtering cannot silently drop permissions.
	if err := g.DB().GetCore().ClearTableFields(ctx, publishAccountSettingTable); err != nil {
		return gerror.Wrap(err, "刷新账号设置字段失败")
	}
	fields, err := g.DB().TableFields(ctx, publishAccountSettingTable)
	if err != nil {
		return gerror.Wrap(err, "读取账号设置字段失败")
	}
	if _, ok := fields["profile_detail_view_enabled"]; !ok {
		return gerror.New("账号设置数据库尚未升级，请先完成数据库迁移")
	}
	now := gtime.Now()
	data := g.Map{
		"tenant_id":                   tenantId,
		"account_id":                  in.AccountId,
		"enable_suffix":               in.EnableSuffix,
		"suffix_content":              in.SuffixContent,
		"enable_title_mark":           in.EnableTitleMark,
		"mark_mode":                   in.MarkMode,
		"number_source":               in.NumberSource,
		"custom_mark_text":            in.CustomMarkText,
		"mark_position":               in.MarkPosition,
		"profile_detail_view_enabled": in.ProfileDetailViewEnabled,
		"shared_resource_enabled":     in.SharedResourceEnabled,
		"telegram_binding_enabled":    in.TelegramBindingEnabled,
		"group_push_enabled":          in.GroupPushEnabled,
		"updated_by":                  operatorId,
		"updated_at":                  now,
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
	saved, err := s.accountSetting(ctx, tenantId, in.AccountId)
	if err != nil {
		return err
	}
	if saved.ProfileDetailViewEnabled != in.ProfileDetailViewEnabled ||
		saved.SharedResourceEnabled != in.SharedResourceEnabled ||
		saved.TelegramBindingEnabled != in.TelegramBindingEnabled ||
		saved.GroupPushEnabled != in.GroupPushEnabled {
		return gerror.New("账号权限保存结果不一致，请刷新后重试")
	}
	return nil
}

func (s *sSysPublish) ensureAdminManageableAccount(ctx context.Context, admin *sysin.AccountModel, accountId int64) error {
	if admin == nil || admin.Id <= 0 || admin.TenantId <= 0 {
		return gerror.New("当前账号无管理权限")
	}
	if accountId <= 0 {
		return gerror.New("账号ID不能为空")
	}
	accountColumns := pdao.YoubanPublishAccount.Columns()
	count, err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(accountColumns.Id, accountId).
		Where(accountColumns.TenantId, admin.TenantId).
		Wheref("(%s = ? OR %s = ?)", accountColumns.ParentId, accountColumns.Id, admin.Id, admin.Id).
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
		AccountId:           accountId,
		EnableTitleMark:     0,
		MarkMode:            "nickname",
		NumberSource:        "sequence",
		MarkPosition:        "top",
		DailyReportEnabled:  1,
		ChannelAlertEnabled: 1,
		AccountEventEnabled: 1,
		TaskFailureEnabled:  1,
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
	model.ProfileDetailViewEnabled = row["profile_detail_view_enabled"].Int()
	model.SharedResourceEnabled = row["shared_resource_enabled"].Int()
	model.TelegramBindingEnabled = row["telegram_binding_enabled"].Int()
	model.GroupPushEnabled = row["group_push_enabled"].Int()
	if value, ok := row["daily_report_enabled"]; ok && value != nil && !value.IsNil() {
		model.DailyReportEnabled = value.Int()
	}
	if value, ok := row["channel_alert_enabled"]; ok && value != nil && !value.IsNil() {
		model.ChannelAlertEnabled = value.Int()
	}
	if value, ok := row["account_event_enabled"]; ok && value != nil && !value.IsNil() {
		model.AccountEventEnabled = value.Int()
	}
	if value, ok := row["task_failure_enabled"]; ok && value != nil && !value.IsNil() {
		model.TaskFailureEnabled = value.Int()
	}
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

// PatchAccountSetting shares validation and persistence with web settings.
// Only administrators can edit self or an uploader they manage.
func (s *sSysPublish) PatchAccountSetting(ctx context.Context, in *sysin.AccountSettingPatchInp) (*sysin.AccountSettingModel, error) {
	if in == nil {
		return nil, gerror.New("账号设置不能为空")
	}
	if err := in.Filter(ctx); err != nil {
		return nil, err
	}
	admin, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	var result *sysin.AccountSettingModel
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		cols := pdao.YoubanPublishAccount.Columns()
		target, err := pdao.YoubanPublishAccount.Ctx(ctx).Where(cols.Id, in.AccountId).Where(cols.TenantId, admin.TenantId).LockUpdate().One()
		if err != nil {
			return err
		}
		if target.IsEmpty() || (in.AccountId != admin.Id && target[cols.ParentId].Int64() != admin.Id) {
			return gerror.New("账号不存在或无权操作")
		}
		current, err := s.accountSetting(ctx, admin.TenantId, in.AccountId)
		if err != nil {
			return err
		}
		save := accountSettingSaveInput(current)
		applyAccountSettingPatch(save, in)
		if err = save.Filter(ctx); err != nil {
			return err
		}
		if err = s.saveAccountSetting(ctx, admin.TenantId, admin.Id, save); err != nil {
			return err
		}
		result, err = s.accountSetting(ctx, admin.TenantId, in.AccountId)
		return err
	})
	if err == nil {
		_ = bumpAccountVisibilityVersion(ctx, admin.TenantId)
	}
	return result, err
}

func accountSettingSaveInput(current *sysin.AccountSettingModel) *sysin.AccountSettingSaveInp {
	return &sysin.AccountSettingSaveInp{
		AccountId: current.AccountId, EnableSuffix: current.EnableSuffix, SuffixContent: current.SuffixContent,
		EnableTitleMark: current.EnableTitleMark, MarkMode: current.MarkMode, NumberSource: current.NumberSource,
		CustomMarkText: current.CustomMarkText, MarkPosition: current.MarkPosition,
		ProfileDetailViewEnabled: current.ProfileDetailViewEnabled, SharedResourceEnabled: current.SharedResourceEnabled,
		TelegramBindingEnabled: current.TelegramBindingEnabled, GroupPushEnabled: current.GroupPushEnabled,
	}
}
func applyAccountSettingPatch(save *sysin.AccountSettingSaveInp, in *sysin.AccountSettingPatchInp) {
	switch in.Field {
	case "profileDetailViewEnabled":
		save.ProfileDetailViewEnabled = gconv.Int(in.Value)
	case "sharedResourceEnabled":
		save.SharedResourceEnabled = gconv.Int(in.Value)
	case "telegramBindingEnabled":
		save.TelegramBindingEnabled = gconv.Int(in.Value)
	case "groupPushEnabled":
		save.GroupPushEnabled = gconv.Int(in.Value)
	case "enableSuffix":
		if in.Value == "1" {
			save.EnableSuffix = 1
		} else {
			save.EnableSuffix = 0
		}
	case "enableTitleMark":
		if in.Value == "1" {
			save.EnableTitleMark = 1
		} else {
			save.EnableTitleMark = 0
		}
	case "suffixContent":
		save.SuffixContent = in.Value
	case "customMarkText":
		save.CustomMarkText = in.Value
	case "markMode":
		save.MarkMode = in.Value
	case "numberSource":
		save.NumberSource = in.Value
	case "markPosition":
		save.MarkPosition = in.Value
	}
}

// Notification checks must not render account title/number previews or read
// unrelated configuration. Missing rows keep the product's enabled default.
func (s *sSysPublish) accountNotificationEnabled(ctx context.Context, tenantID, accountID int64, kind string) (bool, error) {
	fields := map[string]string{"daily": "daily_report_enabled", "channel": "channel_alert_enabled", "account": "account_event_enabled", "task": "task_failure_enabled"}
	field, ok := fields[kind]
	if !ok {
		return false, gerror.New("通知设置类型不合法")
	}
	row, err := g.DB().Model(publishAccountSettingTable).Ctx(ctx).Fields(field).Where("tenant_id", tenantID).Where("account_id", accountID).WhereNull("deleted_at").One()
	if err != nil {
		return false, gerror.Wrap(err, "读取通知设置失败")
	}
	return row.IsEmpty() || row[field].Int() == 1, nil
}
