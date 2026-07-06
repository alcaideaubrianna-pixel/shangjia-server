// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectDispatch is the golang structure of table hg_youban_publish_collect_dispatch for DAO operations like Where/Data.
type YoubanPublishCollectDispatch struct {
	g.Meta              `orm:"table:hg_youban_publish_collect_dispatch, do:true"`
	Id                  any         // 主键
	TenantId            any         // 租户ID
	AccountId           any         // 所属账号ID
	SourceId            any         // 采集源ID
	RuleId              any         // 规则ID
	EventId             any         // 采集事件ID
	ReviewId            any         // 审核ID
	ProfileId           any         // 资料ID
	TaskId              any         // 上架任务ID
	TargetChannelIdJson any         // 目标频道ID JSON
	BotIdJson           any         // 推送BOT ID JSON
	MatchJson           any         // 命中详情JSON
	Status              any         // 状态
	ErrorMessage        any         // 错误信息
	CreatedAt           *gtime.Time // 创建时间
	UpdatedAt           *gtime.Time // 更新时间
	FinishedAt          *gtime.Time // 完成时间
}
