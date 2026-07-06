// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishImportTask is the golang structure of table hg_youban_publish_import_task for DAO operations like Where/Data.
type YoubanPublishImportTask struct {
	g.Meta           `orm:"table:hg_youban_publish_import_task, do:true"`
	Id               any         // 主键
	TenantId         any         // 租户ID
	AccountId        any         // 上架账号ID
	SourceName       any         // 来源名称
	BaseUrl          any         // 旧站域名
	Username         any         // 旧站账号
	PasswordCipher   any         // 旧站密码密文
	LimitCount       any         // 测试采集数量
	PerPage          any         // 每页数量
	ProxyEnabled     any         // 是否启用代理
	ProxyPool        any         // 代理池
	MediaConcurrency any         // 媒体并发数
	ChannelIdJson    any         // 匹配频道ID JSON
	TgStartAt        *gtime.Time // TG匹配开始时间
	TgEndAt          *gtime.Time // TG匹配结束时间
	Status           any         // 任务状态
	Stage            any         // 执行阶段
	ProgressTotal    any         // 总进度
	ProgressDone     any         // 已完成进度
	PageTotal        any         // 总页数
	PageDone         any         // 已完成页数
	ItemTotal        any         // 资料总数
	ItemDone         any         // 已处理资料数
	Imported         any         // 导入数量
	Duplicate        any         // 重复数量
	MediaTotal       any         // 媒体总数
	MediaDone        any         // 已处理媒体数
	MediaImported    any         // 媒体导入数量
	TgTotal          any         // TG消息总数
	TgDone           any         // TG已处理数
	TgMatched        any         // TG匹配数量
	LastSourceNoteId any         // 最近旧站资料ID
	ErrorMessage     any         // 错误信息
	ResultJson       any         // 执行结果JSON
	Remark           any         // 备注
	CreatedBy        any         // 创建人
	UpdatedBy        any         // 更新人
	DeletedBy        any         // 删除人
	StartedAt        *gtime.Time // 开始时间
	FinishedAt       *gtime.Time // 结束时间
	CreatedAt        *gtime.Time // 创建时间
	UpdatedAt        *gtime.Time // 更新时间
	DeletedAt        *gtime.Time // 删除时间
}
