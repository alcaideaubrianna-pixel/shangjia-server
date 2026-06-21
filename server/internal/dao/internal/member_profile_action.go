package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type MemberProfileActionDao struct {
	table    string
	group    string
	columns  MemberProfileActionColumns
	handlers []gdb.ModelHandler
}

type MemberProfileActionColumns struct {
	Id         string
	MemberId   string
	ProfileId  string
	ActionType string
	CreatedAt  string
	UpdatedAt  string
	DeletedAt  string
}

var memberProfileActionColumns = MemberProfileActionColumns{
	Id:         "id",
	MemberId:   "member_id",
	ProfileId:  "profile_id",
	ActionType: "action_type",
	CreatedAt:  "created_at",
	UpdatedAt:  "updated_at",
	DeletedAt:  "deleted_at",
}

func NewMemberProfileActionDao(handlers ...gdb.ModelHandler) *MemberProfileActionDao {
	return &MemberProfileActionDao{
		group:    "default",
		table:    "hg_member_profile_action",
		columns:  memberProfileActionColumns,
		handlers: handlers,
	}
}

func (dao *MemberProfileActionDao) DB() gdb.DB {
	return g.DB(dao.group)
}

func (dao *MemberProfileActionDao) Table() string {
	return dao.table
}

func (dao *MemberProfileActionDao) Columns() MemberProfileActionColumns {
	return dao.columns
}

func (dao *MemberProfileActionDao) Group() string {
	return dao.group
}

func (dao *MemberProfileActionDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model.Safe().Ctx(ctx)
}

func (dao *MemberProfileActionDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
