// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTask is the golang structure of table hg_youban_publish_task for DAO operations like Where/Data.
type YoubanPublishTask struct {
	g.Meta          `orm:"table:hg_youban_publish_task, do:true"`
	Id              any         // 主键
	MerchantId      any         // 商家ID
	AccountId       any         // 账号ID
	ProfileId       any         // 资料ID
	ClientRequestId any         // 客户端幂等ID
	Title           any         // 标题
	Province        any         // 省份
	City            any         // 城市
	PlainText       any         // 正文
	MediaCount      any         // 媒体数量
	TgPushEnabled   any         // 是否推送TG
	TgStatus        any         // TG状态
	Status          any         // 任务状态
	ErrorMessage    any         // 错误信息
	SubmittedAt     *gtime.Time // 提交时间
	PublishedAt     *gtime.Time // 发布时间
	CreatedBy       any         // 创建人
	UpdatedBy       any         // 更新人
	DeletedBy       any         // 删除人
	CreatedAt       *gtime.Time // 创建时间
	UpdatedAt       *gtime.Time // 更新时间
	DeletedAt       *gtime.Time // 删除时间
}
