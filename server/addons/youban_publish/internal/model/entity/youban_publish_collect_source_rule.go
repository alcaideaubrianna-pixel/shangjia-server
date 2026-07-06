// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectSourceRule is the golang structure for table youban_publish_collect_source_rule.
type YoubanPublishCollectSourceRule struct {
	Id        uint64      `json:"id"        orm:"id"         description:"主键"`
	TenantId  int64       `json:"tenantId"  orm:"tenant_id"  description:"租户ID"`
	SourceId  int64       `json:"sourceId"  orm:"source_id"  description:"采集源ID"`
	RuleId    int64       `json:"ruleId"    orm:"rule_id"    description:"规则ID"`
	Sort      int         `json:"sort"      orm:"sort"       description:"排序"`
	Status    int         `json:"status"    orm:"status"     description:"状态"`
	CreatedAt *gtime.Time `json:"createdAt" orm:"created_at" description:"创建时间"`
	UpdatedAt *gtime.Time `json:"updatedAt" orm:"updated_at" description:"更新时间"`
}
