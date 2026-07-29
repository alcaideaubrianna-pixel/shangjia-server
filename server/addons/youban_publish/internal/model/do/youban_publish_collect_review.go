// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectReview is the golang structure of table hg_youban_publish_collect_review for DAO operations like Where/Data.
type YoubanPublishCollectReview struct {
	g.Meta              `orm:"table:hg_youban_publish_collect_review, do:true"`
	Id                  any         // 主键
	TenantId            any         // 租户ID
	AccountId           any         // 所属账号ID
	SourceId            any         // 采集源ID
	RuleId              any         // 规则ID
	EventId             any         // 采集事件ID
	DispatchId          any         // 分发记录ID
	RawText             any         // 原始文本
	MediaCount          any         // 媒体数量
	TargetChannelIdJson any         // 目标频道ID JSON
	BotIdJson           any         // 推送BOT ID JSON
	Status              any         // 审核状态
	ReviewReason        any         // 审核原因
	ReviewedBy          any         // 审核人
	ReviewedAt          *gtime.Time // 审核时间
	CreatedAt           *gtime.Time // 创建时间
	UpdatedAt           *gtime.Time // 更新时间
}
