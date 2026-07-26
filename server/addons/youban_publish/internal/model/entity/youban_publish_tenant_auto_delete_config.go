// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTenantAutoDeleteConfig is the golang structure for table youban_publish_tenant_auto_delete_config.
type YoubanPublishTenantAutoDeleteConfig struct {
	Id                 int64       `json:"id"                 orm:"id"                   description:""`
	TenantId           int64       `json:"tenantId"           orm:"tenant_id"            description:""`
	Enabled            int         `json:"enabled"            orm:"enabled"              description:""`
	BotIdsJson         string      `json:"botIdsJson"         orm:"bot_ids_json"         description:""`
	CustomKeywordsJson string      `json:"customKeywordsJson" orm:"custom_keywords_json" description:""`
	CustomRulesJson    string      `json:"customRulesJson"    orm:"custom_rules_json"    description:""`
	CreatedBy          int64       `json:"createdBy"          orm:"created_by"           description:""`
	UpdatedBy          int64       `json:"updatedBy"          orm:"updated_by"           description:""`
	CreatedAt          *gtime.Time `json:"createdAt"          orm:"created_at"           description:""`
	UpdatedAt          *gtime.Time `json:"updatedAt"          orm:"updated_at"           description:""`
}
