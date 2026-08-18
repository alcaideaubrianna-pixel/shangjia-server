package dao

import (
	"context"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type TenantColumns struct{ Id, Name string }
type tenantDao struct {
	table   string
	columns TenantColumns
}

func (d *tenantDao) Table() string                      { return d.table }
func (d *tenantDao) Columns() TenantColumns             { return d.columns }
func (d *tenantDao) Ctx(ctx context.Context) *gdb.Model { return g.DB().Model(d.table).Safe().Ctx(ctx) }

var YoubanPublishTenant = &tenantDao{table: "hg_youban_publish_tenant", columns: TenantColumns{Id: "id", Name: "name"}}
