// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// ContentProfile is the golang structure of table hg_content_profile for DAO operations like Where/Data.
type ContentProfile struct {
	g.Meta               `orm:"table:hg_content_profile, do:true"`
	Id                   any         // ID
	ProfileNo            any         // 资料编号
	SourceType           any         // 来源类型
	SourceNoteId         any         // FeiNiu笔记ID
	SourceNoteUuid       any         // FeiNiu笔记UUID
	SourceKey            any         // 来源唯一键
	SourceTextHash       any         // 来源文本哈希
	ChannelId            any         // 本地来源频道ID
	DuplicateOfId        any         // 重复资料ID
	Title                any         // 标题
	Summary              any         // 摘要
	PlainText            any         // 正文纯文本
	HtmlText             any         // HTML正文
	SourceCategoryCode   any         // FeiNiu分类编码
	DaysWithEscort       any         // 陪伴天数
	ExpectedLivingCost   any         // 期望生活费
	CanFlyToProvince     any         // 可飞外省
	CanGoAbroad          any         // 可出国
	CanOvernight         any         // 可过夜
	CanCohabitate        any         // 可同居
	HasHealthCheck       any         // 有体检
	IsFullMonth          any         // 满月
	IsVirgin             any         // 是否处
	AcceptSm             any         // 接受SM
	NoCondomAfterCheck   any         // 体检后无套
	AllowCreampie        any         // 可内射
	HasTattoo            any         // 有纹身
	IsFavorite           any         // 是否收藏
	SourceEditedAt       *gtime.Time // FeiNiu编辑时间
	GroupParams          any         // 分组参数
	TagParams            any         // 标签参数
	TextBlockCount       any         // 文本块数
	StoragePolicy        any         // 存储策略
	SourceRemark         any         // FeiNiu备注
	SourceCreateBy       any         // FeiNiu创建者
	SourceUpdateBy       any         // FeiNiu更新者
	SourceCreatedAt      *gtime.Time // FeiNiu创建时间
	SourceUpdatedAt      *gtime.Time // FeiNiu更新时间
	Province             any         // 省份
	City                 any         // 城市
	Age                  any         // 年龄
	Height               any         // 身高
	Weight               any         // 体重
	CupSize              any         // 资料标签
	HasVerificationVideo any         // 是否有验证视频
	MemberOnlyVideo      any         // 视频是否仅会员可见
	CoverMediaId         any         // 封面媒体ID
	ImageCount           any         // 图片数
	VideoCount           any         // 视频数
	Visibility           any         // 可见性
	ReviewStatus         any         // 审核状态
	ImportStatus         any         // 导入状态
	AdminRemark          any         // 后台备注
	PublishedAt          *gtime.Time // 发布时间
	Status               any         // 状态
	CreatedAt            *gtime.Time // 创建时间
	UpdatedAt            *gtime.Time // 更新时间
	DeletedAt            *gtime.Time // 删除时间
}
