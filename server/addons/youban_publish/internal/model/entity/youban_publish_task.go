// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTask is the golang structure for table youban_publish_task.
type YoubanPublishTask struct {
	Id              uint64      `json:"id"              orm:"id"                description:"主键"`
	MerchantId      int64       `json:"merchantId"      orm:"merchant_id"       description:"商家ID"`
	AccountId       int64       `json:"accountId"       orm:"account_id"        description:"账号ID"`
	ProfileId       int64       `json:"profileId"       orm:"profile_id"        description:"资料ID"`
	ClientRequestId string      `json:"clientRequestId" orm:"client_request_id" description:"客户端幂等ID"`
	Title           string      `json:"title"           orm:"title"             description:"标题"`
	Province        string      `json:"province"        orm:"province"          description:"省份"`
	City            string      `json:"city"            orm:"city"              description:"城市"`
	PlainText       string      `json:"plainText"       orm:"plain_text"        description:"正文"`
	MediaCount      int         `json:"mediaCount"      orm:"media_count"       description:"媒体数量"`
	TgPushEnabled   int         `json:"tgPushEnabled"   orm:"tg_push_enabled"   description:"是否推送TG"`
	TgStatus        string      `json:"tgStatus"        orm:"tg_status"         description:"TG状态"`
	Status          string      `json:"status"          orm:"status"            description:"任务状态"`
	ErrorMessage    string      `json:"errorMessage"    orm:"error_message"     description:"错误信息"`
	SubmittedAt     *gtime.Time `json:"submittedAt"     orm:"submitted_at"      description:"提交时间"`
	PublishedAt     *gtime.Time `json:"publishedAt"     orm:"published_at"      description:"发布时间"`
	CreatedBy       int64       `json:"createdBy"       orm:"created_by"        description:"创建人"`
	UpdatedBy       int64       `json:"updatedBy"       orm:"updated_by"        description:"更新人"`
	DeletedBy       int64       `json:"deletedBy"       orm:"deleted_by"        description:"删除人"`
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"        description:"创建时间"`
	UpdatedAt       *gtime.Time `json:"updatedAt"       orm:"updated_at"        description:"更新时间"`
	DeletedAt       *gtime.Time `json:"deletedAt"       orm:"deleted_at"        description:"删除时间"`
}
