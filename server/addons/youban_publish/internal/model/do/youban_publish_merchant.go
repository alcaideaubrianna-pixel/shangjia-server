// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishMerchant is the golang structure of table hg_youban_publish_merchant for DAO operations like Where/Data.
type YoubanPublishMerchant struct {
	g.Meta       `orm:"table:hg_youban_publish_merchant, do:true"`
	Id           any         // 主键
	Name         any         // 商家名称
	ContactName  any         // 联系人
	ContactPhone any         // 联系电话
	Remark       any         // 备注
	Status       any         // 状态
	CreatedBy    any         // 创建人
	UpdatedBy    any         // 更新人
	DeletedBy    any         // 删除人
	CreatedAt    *gtime.Time // 创建时间
	UpdatedAt    *gtime.Time // 更新时间
	DeletedAt    *gtime.Time // 删除时间
}
