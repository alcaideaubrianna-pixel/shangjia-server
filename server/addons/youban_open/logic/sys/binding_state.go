package sys

import (
	"context"
	"fmt"
	"strings"

	pdao "hotgo/addons/youban_open/internal/dao"
	"hotgo/addons/youban_open/model/input/sysin"
	"hotgo/internal/library/platformbinding"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sOpenAccess) UpdateBinding(ctx context.Context, appId string, in *sysin.CmsBindingStatusInp) (*sysin.CmsBindingModel, error) {
	columns := pdao.CmsTenantBinding.Columns()
	previous, err := s.findBinding(ctx, appId, 0, in.Id)
	if err != nil {
		return nil, err
	}
	result, err := pdao.CmsTenantBinding.Ctx(ctx).Where(columns.Id, in.Id).Where(columns.AppId, appId).Data(g.Map{
		columns.Status: in.Status, columns.Reason: strings.TrimSpace(in.Reason),
		columns.ReviewedAt: gtime.Now(), columns.UpdatedAt: gtime.Now(),
	}).Update()
	if err != nil {
		return nil, gerror.Wrap(err, "更新绑定状态失败")
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return nil, gerror.New("绑定记录不存在")
	}
	binding, err := s.findBinding(ctx, appId, 0, in.Id)
	if err == nil && binding != nil && binding.Status == sysin.CmsBindingApproved && previous.Status != sysin.CmsBindingApproved {
		s.emitApproved(ctx, binding)
	}
	return binding, err
}

func (s *sOpenAccess) emitApproved(ctx context.Context, binding *sysin.CmsBindingModel) {
	for _, err := range platformbinding.EmitApproved(ctx, platformbinding.ApprovedEvent{
		AppID: binding.AppId, AppName: binding.AppName, TenantID: binding.TenantId, BindingID: binding.Id,
	}) {
		g.Log().Warning(ctx, "平台绑定通过通知失败", g.Map{"bindingId": binding.Id, "tenantId": binding.TenantId, "err": err})
	}
}

func (s *sOpenAccess) bindingList(ctx context.Context, model *gdb.Model) ([]*sysin.CmsBindingModel, error) {
	if err := ensureBindingTables(ctx); err != nil {
		return nil, gerror.Wrap(err, "初始化CMS绑定表失败")
	}
	columns, appColumns := pdao.CmsTenantBinding.Columns(), pdao.CmsApp.Columns()
	var list []*sysin.CmsBindingModel
	err := model.LeftJoin(pdao.CmsApp.Table()+" a", "a."+appColumns.AppId+"=b."+columns.AppId).
		LeftJoin("hg_youban_publish_tenant t", "t.id=b."+columns.TenantId).
		Fields(
			"b.*",
			"a."+appColumns.Name+" AS app_name",
			`COALESCE(
				(SELECT NULLIF(pa.nickname, '') FROM hg_youban_publish_account pa
				 WHERE pa.tenant_id=b.tenant_id AND pa.deleted_at IS NULL
				 ORDER BY CASE WHEN pa.parent_id=0 THEN 0 ELSE 1 END, pa.id ASC LIMIT 1),
				NULLIF(t.name, '')
			) AS tenant_name`,
		).OrderDesc("b." + columns.Id).Scan(&list)
	if err != nil {
		return nil, gerror.Wrap(err, "读取CMS绑定记录失败")
	}
	if list == nil {
		list = []*sysin.CmsBindingModel{}
	}
	for _, item := range list {
		if strings.TrimSpace(item.TenantName) == "" {
			item.TenantName = fmt.Sprintf("租户 %d", item.TenantId)
		}
	}
	return list, nil
}

func ensureBindingTables(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS hg_youban_publish_cms_binding_code (id BIGSERIAL PRIMARY KEY, app_id VARCHAR(128) NOT NULL, code_hash VARCHAR(128) NOT NULL, code_hint VARCHAR(32) NOT NULL DEFAULT '', version INTEGER NOT NULL DEFAULT 1, status SMALLINT NOT NULL DEFAULT 1, created_at TIMESTAMP, updated_at TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS hg_youban_publish_cms_tenant_binding (id BIGSERIAL PRIMARY KEY, app_id VARCHAR(128) NOT NULL, tenant_id BIGINT NOT NULL, code_version INTEGER NOT NULL DEFAULT 1, status VARCHAR(32) NOT NULL DEFAULT 'approved', reason VARCHAR(255) NOT NULL DEFAULT '', requested_at TIMESTAMP, reviewed_at TIMESTAMP, created_at TIMESTAMP, updated_at TIMESTAMP)`,
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return err
		}
	}
	_, err := g.DB().Exec(ctx, `ALTER TABLE hg_youban_publish_cms_app ADD COLUMN IF NOT EXISTS review_mode VARCHAR(32) NOT NULL DEFAULT 'auto_approve'`)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		return err
	}
	return nil
}

func (s *sOpenAccess) findBinding(ctx context.Context, appId string, tenantId, id int64) (*sysin.CmsBindingModel, error) {
	columns := pdao.CmsTenantBinding.Columns()
	model := pdao.CmsTenantBinding.Ctx(ctx).As("b").Where("b."+columns.AppId, appId)
	if id > 0 {
		model = model.Where("b."+columns.Id, id)
	} else {
		model = model.Where("b."+columns.TenantId, tenantId)
	}
	list, err := s.bindingList(ctx, model)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return list[0], nil
}
