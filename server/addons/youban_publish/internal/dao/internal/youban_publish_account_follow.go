// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishAccountFollowDao is the data access object for the table hg_youban_publish_account_follow.
type YoubanPublishAccountFollowDao struct {
	table    string                            // table is the underlying table name of the DAO.
	group    string                            // group is the database configuration group name of the current DAO.
	columns  YoubanPublishAccountFollowColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                // handlers for customized model modification.
}

// YoubanPublishAccountFollowColumns defines and stores column names for the table hg_youban_publish_account_follow.
type YoubanPublishAccountFollowColumns struct {
	Id                       string // 主键
	TenantId                 string // 租户ID
	FollowerAccountId        string // 关注人账号ID
	FollowingAccountId       string // 被关注账号ID
	Status                   string // 状态
	ApprovalRequiredSnapshot string // 申请时是否需要审批
	Remark                   string // 备注
	BlockedBy                string // 拉黑人
	ApprovedAt               string // 通过时间
	CreatedAt                string // 创建时间
	UpdatedAt                string // 更新时间
	DeletedAt                string // 删除时间
}

// youbanPublishAccountFollowColumns holds the columns for the table hg_youban_publish_account_follow.
var youbanPublishAccountFollowColumns = YoubanPublishAccountFollowColumns{
	Id:                       "id",
	TenantId:                 "tenant_id",
	FollowerAccountId:        "follower_account_id",
	FollowingAccountId:       "following_account_id",
	Status:                   "status",
	ApprovalRequiredSnapshot: "approval_required_snapshot",
	Remark:                   "remark",
	BlockedBy:                "blocked_by",
	ApprovedAt:               "approved_at",
	CreatedAt:                "created_at",
	UpdatedAt:                "updated_at",
	DeletedAt:                "deleted_at",
}

// NewYoubanPublishAccountFollowDao creates and returns a new DAO object for table data access.
func NewYoubanPublishAccountFollowDao(handlers ...gdb.ModelHandler) *YoubanPublishAccountFollowDao {
	return &YoubanPublishAccountFollowDao{
		group:    "default",
		table:    "hg_youban_publish_account_follow",
		columns:  youbanPublishAccountFollowColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishAccountFollowDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishAccountFollowDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishAccountFollowDao) Columns() YoubanPublishAccountFollowColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishAccountFollowDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishAccountFollowDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishAccountFollowDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
