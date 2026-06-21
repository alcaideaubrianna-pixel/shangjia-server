package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type MemberProfileViewDao struct {
	table    string
	group    string
	columns  MemberProfileViewColumns
	handlers []gdb.ModelHandler
}

type MemberProfileViewColumns struct {
	Id         string
	MemberId   string
	ProfileId  string
	ViewCount  string
	LastViewAt string
	CreatedAt  string
	UpdatedAt  string
	DeletedAt  string
}

var memberProfileViewColumns = MemberProfileViewColumns{
	Id:         "id",
	MemberId:   "member_id",
	ProfileId:  "profile_id",
	ViewCount:  "view_count",
	LastViewAt: "last_view_at",
	CreatedAt:  "created_at",
	UpdatedAt:  "updated_at",
	DeletedAt:  "deleted_at",
}

func NewMemberProfileViewDao(handlers ...gdb.ModelHandler) *MemberProfileViewDao {
	return &MemberProfileViewDao{
		group:    "default",
		table:    "hg_member_profile_view",
		columns:  memberProfileViewColumns,
		handlers: handlers,
	}
}

func (dao *MemberProfileViewDao) DB() gdb.DB {
	return g.DB(dao.group)
}

func (dao *MemberProfileViewDao) Table() string {
	return dao.table
}

func (dao *MemberProfileViewDao) Columns() MemberProfileViewColumns {
	return dao.columns
}

func (dao *MemberProfileViewDao) Group() string {
	return dao.group
}

func (dao *MemberProfileViewDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model.Safe().Ctx(ctx)
}

func (dao *MemberProfileViewDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
