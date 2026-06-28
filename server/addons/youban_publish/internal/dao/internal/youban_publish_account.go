// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishAccountDao is the data access object for the table hg_youban_publish_account.
type YoubanPublishAccountDao struct {
	table    string                      // table is the underlying table name of the DAO.
	group    string                      // group is the database configuration group name of the current DAO.
	columns  YoubanPublishAccountColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler          // handlers for customized model modification.
}

// YoubanPublishAccountColumns defines and stores column names for the table hg_youban_publish_account.
type YoubanPublishAccountColumns struct {
	Id                 string // 主键
	MerchantId         string // 商家ID
	AdminMemberId      string // 绑定系统账号ID
	ParentId           string // 父账号ID
	AccountType        string // 账号类型
	Nickname           string // 昵称
	Username           string // 用户名
	TelegramUserId     string // TG用户ID
	TelegramUsername   string // TG用户名
	DailyPublishLimit  string // 每日上架额度
	CanDirectPublish   string // 是否可直接发布
	AllowedChannelJson string // 可发布频道JSON
	AllowedRegionJson  string // 可发布地区JSON
	Remark             string // 备注
	Status             string // 状态
	CreatedBy          string // 创建人
	UpdatedBy          string // 更新人
	DeletedBy          string // 删除人
	CreatedAt          string // 创建时间
	UpdatedAt          string // 更新时间
	DeletedAt          string // 删除时间
}

// youbanPublishAccountColumns holds the columns for the table hg_youban_publish_account.
var youbanPublishAccountColumns = YoubanPublishAccountColumns{
	Id:                 "id",
	MerchantId:         "merchant_id",
	AdminMemberId:      "admin_member_id",
	ParentId:           "parent_id",
	AccountType:        "account_type",
	Nickname:           "nickname",
	Username:           "username",
	TelegramUserId:     "telegram_user_id",
	TelegramUsername:   "telegram_username",
	DailyPublishLimit:  "daily_publish_limit",
	CanDirectPublish:   "can_direct_publish",
	AllowedChannelJson: "allowed_channel_json",
	AllowedRegionJson:  "allowed_region_json",
	Remark:             "remark",
	Status:             "status",
	CreatedBy:          "created_by",
	UpdatedBy:          "updated_by",
	DeletedBy:          "deleted_by",
	CreatedAt:          "created_at",
	UpdatedAt:          "updated_at",
	DeletedAt:          "deleted_at",
}

// NewYoubanPublishAccountDao creates and returns a new DAO object for table data access.
func NewYoubanPublishAccountDao(handlers ...gdb.ModelHandler) *YoubanPublishAccountDao {
	return &YoubanPublishAccountDao{
		group:    "default",
		table:    "hg_youban_publish_account",
		columns:  youbanPublishAccountColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishAccountDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishAccountDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishAccountDao) Columns() YoubanPublishAccountColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishAccountDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishAccountDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishAccountDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
