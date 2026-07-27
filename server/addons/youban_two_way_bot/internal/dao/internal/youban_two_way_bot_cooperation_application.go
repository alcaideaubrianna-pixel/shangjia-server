// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanTwoWayBotCooperationApplicationDao is the data access object for the table hg_youban_two_way_bot_cooperation_application.
type YoubanTwoWayBotCooperationApplicationDao struct {
	table    string                                       // table is the underlying table name of the DAO.
	group    string                                       // group is the database configuration group name of the current DAO.
	columns  YoubanTwoWayBotCooperationApplicationColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                           // handlers for customized model modification.
}

// YoubanTwoWayBotCooperationApplicationColumns defines and stores column names for the table hg_youban_two_way_bot_cooperation_application.
type YoubanTwoWayBotCooperationApplicationColumns struct {
	Id                   string //
	TenantId             string //
	ConfigId             string //
	ApplicantTgUserId    string //
	ApplicantUsername    string //
	ApplicantFirstName   string //
	ApplicantLastName    string //
	SubmittedBotUserId   string //
	SubmittedBotUsername string //
	SubmittedBotName     string //
	ReviewStatus         string //
	JoinStatus           string //
	TopicThreadId        string //
	ReviewedBy           string //
	ReviewRemark         string //
	ErrorMessage         string //
	SubmittedAt          string //
	ReviewedAt           string //
	CreatedAt            string //
	UpdatedAt            string //
	DeletedAt            string //
}

// youbanTwoWayBotCooperationApplicationColumns holds the columns for the table hg_youban_two_way_bot_cooperation_application.
var youbanTwoWayBotCooperationApplicationColumns = YoubanTwoWayBotCooperationApplicationColumns{
	Id:                   "id",
	TenantId:             "tenant_id",
	ConfigId:             "config_id",
	ApplicantTgUserId:    "applicant_tg_user_id",
	ApplicantUsername:    "applicant_username",
	ApplicantFirstName:   "applicant_first_name",
	ApplicantLastName:    "applicant_last_name",
	SubmittedBotUserId:   "submitted_bot_user_id",
	SubmittedBotUsername: "submitted_bot_username",
	SubmittedBotName:     "submitted_bot_name",
	ReviewStatus:         "review_status",
	JoinStatus:           "join_status",
	TopicThreadId:        "topic_thread_id",
	ReviewedBy:           "reviewed_by",
	ReviewRemark:         "review_remark",
	ErrorMessage:         "error_message",
	SubmittedAt:          "submitted_at",
	ReviewedAt:           "reviewed_at",
	CreatedAt:            "created_at",
	UpdatedAt:            "updated_at",
	DeletedAt:            "deleted_at",
}

// NewYoubanTwoWayBotCooperationApplicationDao creates and returns a new DAO object for table data access.
func NewYoubanTwoWayBotCooperationApplicationDao(handlers ...gdb.ModelHandler) *YoubanTwoWayBotCooperationApplicationDao {
	return &YoubanTwoWayBotCooperationApplicationDao{
		group:    "default",
		table:    "hg_youban_two_way_bot_cooperation_application",
		columns:  youbanTwoWayBotCooperationApplicationColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanTwoWayBotCooperationApplicationDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanTwoWayBotCooperationApplicationDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanTwoWayBotCooperationApplicationDao) Columns() YoubanTwoWayBotCooperationApplicationColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanTwoWayBotCooperationApplicationDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanTwoWayBotCooperationApplicationDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanTwoWayBotCooperationApplicationDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
