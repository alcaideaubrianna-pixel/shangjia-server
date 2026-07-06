// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectRule is the golang structure for table youban_publish_collect_rule.
type YoubanPublishCollectRule struct {
	Id                   uint64      `json:"id"                   orm:"id"                      description:"主键"`
	TenantId             int64       `json:"tenantId"             orm:"tenant_id"               description:"租户ID"`
	AccountId            int64       `json:"accountId"            orm:"account_id"              description:"所属账号ID"`
	Name                 string      `json:"name"                 orm:"name"                    description:"规则名称"`
	GlobalEnabled        int         `json:"globalEnabled"        orm:"global_enabled"          description:"是否全局应用"`
	TargetChannelIdJson  string      `json:"targetChannelIdJson"  orm:"target_channel_id_json"  description:"目标频道ID JSON"`
	BotIdJson            string      `json:"botIdJson"            orm:"bot_id_json"             description:"推送BOT ID JSON"`
	BackupChannelId      int64       `json:"backupChannelId"      orm:"backup_channel_id"       description:"备份群ID"`
	ReviewEnabled        int         `json:"reviewEnabled"        orm:"review_enabled"          description:"是否需要审核"`
	DedupeEnabled        int         `json:"dedupeEnabled"        orm:"dedupe_enabled"          description:"是否图文去重"`
	DedupeDays           int         `json:"dedupeDays"           orm:"dedupe_days"             description:"去重天数"`
	KeywordJson          string      `json:"keywordJson"          orm:"keyword_json"            description:"关键词JSON"`
	TagJson              string      `json:"tagJson"              orm:"tag_json"                description:"标签JSON"`
	ReplaceJson          string      `json:"replaceJson"          orm:"replace_json"            description:"替换规则JSON"`
	BlockTextJson        string      `json:"blockTextJson"        orm:"block_text_json"         description:"屏蔽文本JSON"`
	BlockLink            int         `json:"blockLink"            orm:"block_link"              description:"屏蔽链接"`
	BlockUsername        int         `json:"blockUsername"        orm:"block_username"          description:"屏蔽用户名"`
	BlockPlainText       int         `json:"blockPlainText"       orm:"block_plain_text"        description:"屏蔽纯文本"`
	MinMediaCountEnabled int         `json:"minMediaCountEnabled" orm:"min_media_count_enabled" description:"是否限制媒体数量"`
	MinMediaCount        int         `json:"minMediaCount"        orm:"min_media_count"         description:"最少媒体数"`
	ShowUniqueNo         int         `json:"showUniqueNo"         orm:"show_unique_no"          description:"是否显示唯一编号"`
	HeaderEnabled        int         `json:"headerEnabled"        orm:"header_enabled"          description:"是否启用前置文案"`
	HeaderMarkdown       string      `json:"headerMarkdown"       orm:"header_markdown"         description:"前置Markdown文案"`
	FooterEnabled        int         `json:"footerEnabled"        orm:"footer_enabled"          description:"是否启用后置文案"`
	FooterMarkdown       string      `json:"footerMarkdown"       orm:"footer_markdown"         description:"后置Markdown文案"`
	Sort                 int         `json:"sort"                 orm:"sort"                    description:"排序"`
	Status               int         `json:"status"               orm:"status"                  description:"状态"`
	CreatedBy            int64       `json:"createdBy"            orm:"created_by"              description:"创建人"`
	UpdatedBy            int64       `json:"updatedBy"            orm:"updated_by"              description:"更新人"`
	DeletedBy            int64       `json:"deletedBy"            orm:"deleted_by"              description:"删除人"`
	CreatedAt            *gtime.Time `json:"createdAt"            orm:"created_at"              description:"创建时间"`
	UpdatedAt            *gtime.Time `json:"updatedAt"            orm:"updated_at"              description:"更新时间"`
	DeletedAt            *gtime.Time `json:"deletedAt"            orm:"deleted_at"              description:"删除时间"`
}
