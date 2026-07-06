// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTag is the golang structure of table hg_youban_publish_tag for DAO operations like Where/Data.
type YoubanPublishTag struct {
	g.Meta       `orm:"table:hg_youban_publish_tag, do:true"`
	Id           any         // 主键
	Name         any         // 标签名称
	ReviewStatus any         // 审核状态
	Status       any         // 状态
	UseCount     any         // 使用数量
	CreatedBy    any         // 创建人
	UpdatedBy    any         // 更新人
	DeletedBy    any         // 删除人
	CreatedAt    *gtime.Time // 创建时间
	UpdatedAt    *gtime.Time // 更新时间
	DeletedAt    *gtime.Time // 删除时间
}
