// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTenantAutoDeleteConfig is the golang structure of table hg_youban_publish_tenant_auto_delete_config for DAO operations like Where/Data.
type YoubanPublishTenantAutoDeleteConfig struct {
	g.Meta             `orm:"table:hg_youban_publish_tenant_auto_delete_config, do:true"`
	Id                 any         //
	TenantId           any         //
	Enabled            any         //
	BotIdsJson         any         //
	CustomKeywordsJson any         //
	CustomRulesJson    any         //
	CreatedBy          any         //
	UpdatedBy          any         //
	CreatedAt          *gtime.Time //
	UpdatedAt          *gtime.Time //
}
