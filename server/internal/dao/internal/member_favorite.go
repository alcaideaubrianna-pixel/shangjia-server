package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type MemberFavoriteDao struct {
	table    string
	group    string
	columns  MemberFavoriteColumns
	handlers []gdb.ModelHandler
}

type MemberFavoriteColumns struct {
	Id        string
	MemberId  string
	ProfileId string
	CreatedAt string
	UpdatedAt string
	DeletedAt string
}

var memberFavoriteColumns = MemberFavoriteColumns{
	Id:        "id",
	MemberId:  "member_id",
	ProfileId: "profile_id",
	CreatedAt: "created_at",
	UpdatedAt: "updated_at",
	DeletedAt: "deleted_at",
}

func NewMemberFavoriteDao(handlers ...gdb.ModelHandler) *MemberFavoriteDao {
	return &MemberFavoriteDao{
		group:    "default",
		table:    "hg_member_favorite",
		columns:  memberFavoriteColumns,
		handlers: handlers,
	}
}

func (dao *MemberFavoriteDao) DB() gdb.DB {
	return g.DB(dao.group)
}

func (dao *MemberFavoriteDao) Table() string {
	return dao.table
}

func (dao *MemberFavoriteDao) Columns() MemberFavoriteColumns {
	return dao.columns
}

func (dao *MemberFavoriteDao) Group() string {
	return dao.group
}

func (dao *MemberFavoriteDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model.Safe().Ctx(ctx)
}

func (dao *MemberFavoriteDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
