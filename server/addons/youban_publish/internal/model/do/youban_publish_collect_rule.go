// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectRule is the golang structure of table hg_youban_publish_collect_rule for DAO operations like Where/Data.
type YoubanPublishCollectRule struct {
	g.Meta               `orm:"table:hg_youban_publish_collect_rule, do:true"`
	Id                   any         // 主键
	TenantId             any         // 租户ID
	AccountId            any         // 所属账号ID
	Name                 any         // 规则名称
	GlobalEnabled        any         // 是否全局应用
	TargetChannelIdJson  any         // 目标频道ID JSON
	BotIdJson            any         // 推送BOT ID JSON
	BackupChannelId      any         // 备份群ID
	ReviewEnabled        any         // 是否需要审核
	DedupeEnabled        any         // 是否图文去重
	DedupeDays           any         // 去重天数
	KeywordJson          any         // 关键词JSON
	TagJson              any         // 标签JSON
	ReplaceJson          any         // 替换规则JSON
	BlockTextJson        any         // 屏蔽文本JSON
	BlockLink            any         // 屏蔽链接
	BlockUsername        any         // 屏蔽用户名
	BlockPlainText       any         // 屏蔽纯文本
	MinMediaCountEnabled any         // 是否限制媒体数量
	MinMediaCount        any         // 最少媒体数
	HeaderEnabled        any         // 是否启用前置文案
	HeaderMarkdown       any         // 前置Markdown文案
	FooterEnabled        any         // 是否启用后置文案
	FooterMarkdown       any         // 后置Markdown文案
	Sort                 any         // 排序
	Status               any         // 状态
	CreatedBy            any         // 创建人
	UpdatedBy            any         // 更新人
	DeletedBy            any         // 删除人
	CreatedAt            *gtime.Time // 创建时间
	UpdatedAt            *gtime.Time // 更新时间
	DeletedAt            *gtime.Time // 删除时间
}
