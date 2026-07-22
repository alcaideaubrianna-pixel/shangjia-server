// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// ContentProfile is the golang structure for table content_profile.
type ContentProfile struct {
	Id                   int64       `json:"id"                   orm:"id"                     description:"ID"`
	ProfileNo            string      `json:"profileNo"            orm:"profile_no"             description:"资料编号"`
	SourceType           string      `json:"sourceType"           orm:"source_type"            description:"来源类型"`
	SourceNoteId         int64       `json:"sourceNoteId"         orm:"source_note_id"         description:"FeiNiu笔记ID"`
	SourceNoteUuid       string      `json:"sourceNoteUuid"       orm:"source_note_uuid"       description:"FeiNiu笔记UUID"`
	SourceKey            string      `json:"sourceKey"            orm:"source_key"             description:"来源唯一键"`
	SourceTextHash       string      `json:"sourceTextHash"       orm:"source_text_hash"       description:"来源文本哈希"`
	ChannelId            int64       `json:"channelId"            orm:"channel_id"             description:"本地来源频道ID"`
	DuplicateOfId        int64       `json:"duplicateOfId"        orm:"duplicate_of_id"        description:"重复资料ID"`
	Title                string      `json:"title"                orm:"title"                  description:"标题"`
	Summary              string      `json:"summary"              orm:"summary"                description:"摘要"`
	PlainText            string      `json:"plainText"            orm:"plain_text"             description:"正文纯文本"`
	HtmlText             string      `json:"htmlText"             orm:"html_text"              description:"HTML正文"`
	SourceCategoryCode   string      `json:"sourceCategoryCode"   orm:"source_category_code"   description:"FeiNiu分类编码"`
	DaysWithEscort       int         `json:"daysWithEscort"       orm:"days_with_escort"       description:"陪伴天数"`
	ExpectedLivingCost   int         `json:"expectedLivingCost"   orm:"expected_living_cost"   description:"期望生活费"`
	CanFlyToProvince     int         `json:"canFlyToProvince"     orm:"can_fly_to_province"    description:"可飞外省"`
	CanGoAbroad          int         `json:"canGoAbroad"          orm:"can_go_abroad"          description:"可出国"`
	CanOvernight         int         `json:"canOvernight"         orm:"can_overnight"          description:"可过夜"`
	CanCohabitate        int         `json:"canCohabitate"        orm:"can_cohabitate"         description:"可同居"`
	HasHealthCheck       int         `json:"hasHealthCheck"       orm:"has_health_check"       description:"有体检"`
	IsFullMonth          int         `json:"isFullMonth"          orm:"is_full_month"          description:"满月"`
	IsVirgin             int         `json:"isVirgin"             orm:"is_virgin"              description:"是否处"`
	AcceptSm             int         `json:"acceptSm"             orm:"accept_sm"              description:"接受SM"`
	NoCondomAfterCheck   int         `json:"noCondomAfterCheck"   orm:"no_condom_after_check"  description:"体检后无套"`
	AllowCreampie        int         `json:"allowCreampie"        orm:"allow_creampie"         description:"可内射"`
	HasTattoo            int         `json:"hasTattoo"            orm:"has_tattoo"             description:"有纹身"`
	IsFavorite           int         `json:"isFavorite"           orm:"is_favorite"            description:"是否收藏"`
	SourceEditedAt       *gtime.Time `json:"sourceEditedAt"       orm:"source_edited_at"       description:"FeiNiu编辑时间"`
	GroupParams          string      `json:"groupParams"          orm:"group_params"           description:"分组参数"`
	TagParams            string      `json:"tagParams"            orm:"tag_params"             description:"标签参数"`
	TextBlockCount       int         `json:"textBlockCount"       orm:"text_block_count"       description:"文本块数"`
	StoragePolicy        string      `json:"storagePolicy"        orm:"storage_policy"         description:"存储策略"`
	SourceRemark         string      `json:"sourceRemark"         orm:"source_remark"          description:"FeiNiu备注"`
	SourceCreateBy       string      `json:"sourceCreateBy"       orm:"source_create_by"       description:"FeiNiu创建者"`
	SourceUpdateBy       string      `json:"sourceUpdateBy"       orm:"source_update_by"       description:"FeiNiu更新者"`
	SourceCreatedAt      *gtime.Time `json:"sourceCreatedAt"      orm:"source_created_at"      description:"FeiNiu创建时间"`
	SourceUpdatedAt      *gtime.Time `json:"sourceUpdatedAt"      orm:"source_updated_at"      description:"FeiNiu更新时间"`
	Province             string      `json:"province"             orm:"province"               description:"省份"`
	City                 string      `json:"city"                 orm:"city"                   description:"城市"`
	Age                  int         `json:"age"                  orm:"age"                    description:"年龄"`
	Height               int         `json:"height"               orm:"height"                 description:"身高"`
	Weight               int         `json:"weight"               orm:"weight"                 description:"体重"`
	CupSize              string      `json:"cupSize"              orm:"cup_size"               description:"资料标签"`
	HasVerificationVideo int         `json:"hasVerificationVideo" orm:"has_verification_video" description:"是否有验证视频"`
	MemberOnlyVideo      int         `json:"memberOnlyVideo"      orm:"member_only_video"      description:"视频是否仅会员可见"`
	CoverMediaId         int64       `json:"coverMediaId"         orm:"cover_media_id"         description:"封面媒体ID"`
	ImageCount           int         `json:"imageCount"           orm:"image_count"            description:"图片数"`
	VideoCount           int         `json:"videoCount"           orm:"video_count"            description:"视频数"`
	Visibility           string      `json:"visibility"           orm:"visibility"             description:"可见性"`
	ReviewStatus         string      `json:"reviewStatus"         orm:"review_status"          description:"审核状态"`
	ImportStatus         string      `json:"importStatus"         orm:"import_status"          description:"导入状态"`
	AdminRemark          string      `json:"adminRemark"          orm:"admin_remark"           description:"后台备注"`
	PublishedAt          *gtime.Time `json:"publishedAt"          orm:"published_at"           description:"发布时间"`
	Status               int         `json:"status"               orm:"status"                 description:"状态"`
	CreatedAt            *gtime.Time `json:"createdAt"            orm:"created_at"             description:"创建时间"`
	UpdatedAt            *gtime.Time `json:"updatedAt"            orm:"updated_at"             description:"更新时间"`
	DeletedAt            *gtime.Time `json:"deletedAt"            orm:"deleted_at"             description:"删除时间"`
}
