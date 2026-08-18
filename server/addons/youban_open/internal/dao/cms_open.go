package dao

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type cmsTableDao struct {
	table   string
	columns CmsOpenColumns
}

type CmsOpenColumns struct {
	Id              string
	AppId           string
	AppSecret       string
	Name            string
	BaseUrl         string
	InstanceId      string
	EnrollHash      string
	SourceIp        string
	CmsVersion      string
	LastHeartbeatAt string
	ReviewMode      string
	CodeHash        string
	CodeHint        string
	Version         string
	TenantId        string
	CodeVersion     string
	Status          string
	Reason          string
	RequestedAt     string
	ReviewedAt      string
	CreatedAt       string
	UpdatedAt       string
}

func newCmsTableDao(table string) *cmsTableDao {
	return &cmsTableDao{
		table: table,
		columns: CmsOpenColumns{
			Id: "id", AppId: "app_id", AppSecret: "app_secret", Name: "name",
			BaseUrl: "base_url", InstanceId: "instance_id", EnrollHash: "enroll_hash",
			SourceIp: "source_ip", CmsVersion: "cms_version", LastHeartbeatAt: "last_heartbeat_at",
			ReviewMode: "review_mode",
			CodeHash:   "code_hash", CodeHint: "code_hint",
			Version: "version", TenantId: "tenant_id", CodeVersion: "code_version",
			Status: "status", Reason: "reason", RequestedAt: "requested_at",
			ReviewedAt: "reviewed_at", CreatedAt: "created_at", UpdatedAt: "updated_at",
		},
	}
}

func (d *cmsTableDao) Table() string           { return d.table }
func (d *cmsTableDao) Columns() CmsOpenColumns { return d.columns }
func (d *cmsTableDao) Ctx(ctx context.Context) *gdb.Model {
	return g.DB().Model(d.table).Safe().Ctx(ctx)
}

var (
	CmsApp           = newCmsTableDao("hg_youban_publish_cms_app")
	CmsBindingCode   = newCmsTableDao("hg_youban_publish_cms_binding_code")
	CmsTenantBinding = newCmsTableDao("hg_youban_publish_cms_tenant_binding")
)
