// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishImportTask is the golang structure for table youban_publish_import_task.
type YoubanPublishImportTask struct {
	Id               uint64      `json:"id"               orm:"id"                  description:"主键"`
	TenantId         int64       `json:"tenantId"         orm:"tenant_id"           description:"租户ID"`
	AccountId        int64       `json:"accountId"        orm:"account_id"          description:"上架账号ID"`
	SourceName       string      `json:"sourceName"       orm:"source_name"         description:"来源名称"`
	BaseUrl          string      `json:"baseUrl"          orm:"base_url"            description:"旧站域名"`
	ServerIp         string      `json:"serverIp"         orm:"server_ip"           description:"旧站服务器IP"`
	Username         string      `json:"username"         orm:"username"            description:"旧站账号"`
	PasswordCipher   string      `json:"passwordCipher"   orm:"password_cipher"     description:"旧站密码密文"`
	CookieCipher     string      `json:"cookieCipher"     orm:"cookie_cipher"       description:"旧站Cookie密文"`
	LimitCount       int         `json:"limitCount"       orm:"limit_count"         description:"测试采集数量"`
	PerPage          int         `json:"perPage"          orm:"per_page"            description:"每页数量"`
	ProxyEnabled     int         `json:"proxyEnabled"     orm:"proxy_enabled"       description:"是否启用代理"`
	ProxyPool        string      `json:"proxyPool"        orm:"proxy_pool"          description:"代理池"`
	MediaConcurrency int         `json:"mediaConcurrency" orm:"media_concurrency"   description:"媒体并发数"`
	ChannelIdJson    string      `json:"channelIdJson"    orm:"channel_id_json"     description:"匹配频道ID JSON"`
	TgStartAt        *gtime.Time `json:"tgStartAt"        orm:"tg_start_at"         description:"TG匹配开始时间"`
	TgEndAt          *gtime.Time `json:"tgEndAt"          orm:"tg_end_at"           description:"TG匹配结束时间"`
	Status           string      `json:"status"           orm:"status"              description:"任务状态"`
	Stage            string      `json:"stage"            orm:"stage"               description:"执行阶段"`
	ProgressTotal    int         `json:"progressTotal"    orm:"progress_total"      description:"总进度"`
	ProgressDone     int         `json:"progressDone"     orm:"progress_done"       description:"已完成进度"`
	PageTotal        int         `json:"pageTotal"        orm:"page_total"          description:"总页数"`
	PageDone         int         `json:"pageDone"         orm:"page_done"           description:"已完成页数"`
	ItemTotal        int         `json:"itemTotal"        orm:"item_total"          description:"资料总数"`
	ItemDone         int         `json:"itemDone"         orm:"item_done"           description:"已处理资料数"`
	Imported         int         `json:"imported"         orm:"imported"            description:"导入数量"`
	Duplicate        int         `json:"duplicate"        orm:"duplicate"           description:"重复数量"`
	MediaTotal       int         `json:"mediaTotal"       orm:"media_total"         description:"媒体总数"`
	MediaDone        int         `json:"mediaDone"        orm:"media_done"          description:"已处理媒体数"`
	MediaImported    int         `json:"mediaImported"    orm:"media_imported"      description:"媒体导入数量"`
	TgTotal          int         `json:"tgTotal"          orm:"tg_total"            description:"TG消息总数"`
	TgDone           int         `json:"tgDone"           orm:"tg_done"             description:"TG已处理数"`
	TgMatched        int         `json:"tgMatched"        orm:"tg_matched"          description:"TG匹配数量"`
	LastSourceNoteId int64       `json:"lastSourceNoteId" orm:"last_source_note_id" description:"最近旧站资料ID"`
	ErrorMessage     string      `json:"errorMessage"     orm:"error_message"       description:"错误信息"`
	ResultJson       string      `json:"resultJson"       orm:"result_json"         description:"执行结果JSON"`
	Remark           string      `json:"remark"           orm:"remark"              description:"备注"`
	CreatedBy        int64       `json:"createdBy"        orm:"created_by"          description:"创建人"`
	UpdatedBy        int64       `json:"updatedBy"        orm:"updated_by"          description:"更新人"`
	DeletedBy        int64       `json:"deletedBy"        orm:"deleted_by"          description:"删除人"`
	StartedAt        *gtime.Time `json:"startedAt"        orm:"started_at"          description:"开始时间"`
	FinishedAt       *gtime.Time `json:"finishedAt"       orm:"finished_at"         description:"结束时间"`
	CreatedAt        *gtime.Time `json:"createdAt"        orm:"created_at"          description:"创建时间"`
	UpdatedAt        *gtime.Time `json:"updatedAt"        orm:"updated_at"          description:"更新时间"`
	DeletedAt        *gtime.Time `json:"deletedAt"        orm:"deleted_at"          description:"删除时间"`
}
