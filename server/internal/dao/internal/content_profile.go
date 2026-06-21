// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// ContentProfileDao is the data access object for the table hg_content_profile.
type ContentProfileDao struct {
	table    string                // table is the underlying table name of the DAO.
	group    string                // group is the database configuration group name of the current DAO.
	columns  ContentProfileColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler    // handlers for customized model modification.
}

// ContentProfileColumns defines and stores column names for the table hg_content_profile.
type ContentProfileColumns struct {
	Id                   string // ID
	ProfileNo            string // 资料编号
	SourceType           string // 来源类型
	SourceNoteId         string // FeiNiu笔记ID
	SourceNoteUuid       string // FeiNiu笔记UUID
	SourceKey            string // 来源唯一键
	SourceTextHash       string // 来源文本哈希
	ChannelId            string // 本地来源频道ID
	DuplicateOfId        string // 重复资料ID
	Title                string // 标题
	Summary              string // 摘要
	PlainText            string // 正文纯文本
	HtmlText             string // HTML正文
	SourceCategoryCode   string // FeiNiu分类编码
	DaysWithEscort       string // 陪伴天数
	ExpectedLivingCost   string // 期望生活费
	CanFlyToProvince     string // 可飞外省
	CanGoAbroad          string // 可出国
	CanOvernight         string // 可过夜
	CanCohabitate        string // 可同居
	HasHealthCheck       string // 有体检
	IsFullMonth          string // 满月
	IsVirgin             string // 是否处
	AcceptSm             string // 接受SM
	NoCondomAfterCheck   string // 体检后无套
	AllowCreampie        string // 可内射
	HasTattoo            string // 有纹身
	IsFavorite           string // 是否收藏
	SourceEditedAt       string // FeiNiu编辑时间
	GroupParams          string // 分组参数
	TagParams            string // 标签参数
	TextBlockCount       string // 文本块数
	StoragePolicy        string // 存储策略
	SourceRemark         string // FeiNiu备注
	SourceCreateBy       string // FeiNiu创建者
	SourceUpdateBy       string // FeiNiu更新者
	SourceCreatedAt      string // FeiNiu创建时间
	SourceUpdatedAt      string // FeiNiu更新时间
	Province             string // 省份
	City                 string // 城市
	Age                  string // 年龄
	Height               string // 身高
	Weight               string // 体重
	CupSize              string // 资料标签
	HasVerificationVideo string // 是否有验证视频
	MemberOnlyVideo      string // 视频是否仅会员可见
	CoverMediaId         string // 封面媒体ID
	ImageCount           string // 图片数
	VideoCount           string // 视频数
	Visibility           string // 可见性
	ReviewStatus         string // 审核状态
	ImportStatus         string // 导入状态
	AdminRemark          string // 后台备注
	PublishedAt          string // 发布时间
	Status               string // 状态
	CreatedAt            string // 创建时间
	UpdatedAt            string // 更新时间
	DeletedAt            string // 删除时间
}

// contentProfileColumns holds the columns for the table hg_content_profile.
var contentProfileColumns = ContentProfileColumns{
	Id:                   "id",
	ProfileNo:            "profile_no",
	SourceType:           "source_type",
	SourceNoteId:         "source_note_id",
	SourceNoteUuid:       "source_note_uuid",
	SourceKey:            "source_key",
	SourceTextHash:       "source_text_hash",
	ChannelId:            "channel_id",
	DuplicateOfId:        "duplicate_of_id",
	Title:                "title",
	Summary:              "summary",
	PlainText:            "plain_text",
	HtmlText:             "html_text",
	SourceCategoryCode:   "source_category_code",
	DaysWithEscort:       "days_with_escort",
	ExpectedLivingCost:   "expected_living_cost",
	CanFlyToProvince:     "can_fly_to_province",
	CanGoAbroad:          "can_go_abroad",
	CanOvernight:         "can_overnight",
	CanCohabitate:        "can_cohabitate",
	HasHealthCheck:       "has_health_check",
	IsFullMonth:          "is_full_month",
	IsVirgin:             "is_virgin",
	AcceptSm:             "accept_sm",
	NoCondomAfterCheck:   "no_condom_after_check",
	AllowCreampie:        "allow_creampie",
	HasTattoo:            "has_tattoo",
	IsFavorite:           "is_favorite",
	SourceEditedAt:       "source_edited_at",
	GroupParams:          "group_params",
	TagParams:            "tag_params",
	TextBlockCount:       "text_block_count",
	StoragePolicy:        "storage_policy",
	SourceRemark:         "source_remark",
	SourceCreateBy:       "source_create_by",
	SourceUpdateBy:       "source_update_by",
	SourceCreatedAt:      "source_created_at",
	SourceUpdatedAt:      "source_updated_at",
	Province:             "province",
	City:                 "city",
	Age:                  "age",
	Height:               "height",
	Weight:               "weight",
	CupSize:              "cup_size",
	HasVerificationVideo: "has_verification_video",
	MemberOnlyVideo:      "member_only_video",
	CoverMediaId:         "cover_media_id",
	ImageCount:           "image_count",
	VideoCount:           "video_count",
	Visibility:           "visibility",
	ReviewStatus:         "review_status",
	ImportStatus:         "import_status",
	AdminRemark:          "admin_remark",
	PublishedAt:          "published_at",
	Status:               "status",
	CreatedAt:            "created_at",
	UpdatedAt:            "updated_at",
	DeletedAt:            "deleted_at",
}

// NewContentProfileDao creates and returns a new DAO object for table data access.
func NewContentProfileDao(handlers ...gdb.ModelHandler) *ContentProfileDao {
	return &ContentProfileDao{
		group:    "default",
		table:    "hg_content_profile",
		columns:  contentProfileColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *ContentProfileDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *ContentProfileDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *ContentProfileDao) Columns() ContentProfileColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *ContentProfileDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *ContentProfileDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model.Safe().Ctx(ctx)
}

// Transaction wraps the transaction logic using function f.
// It rolls back the transaction and returns the error if function f returns a non-nil error.
// It commits the transaction and returns nil if function f returns nil.
//
// Note: Do not commit or roll back the transaction in function f,
// as it is automatically handled by this function.
func (dao *ContentProfileDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
