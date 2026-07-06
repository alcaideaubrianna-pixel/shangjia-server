// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectSourceRule is the golang structure of table hg_youban_publish_collect_source_rule for DAO operations like Where/Data.
type YoubanPublishCollectSourceRule struct {
	g.Meta    `orm:"table:hg_youban_publish_collect_source_rule, do:true"`
	Id        any         // 主键
	TenantId  any         // 租户ID
	SourceId  any         // 采集源ID
	RuleId    any         // 规则ID
	Sort      any         // 排序
	Status    any         // 状态
	CreatedAt *gtime.Time // 创建时间
	UpdatedAt *gtime.Time // 更新时间
}
