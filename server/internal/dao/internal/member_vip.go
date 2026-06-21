package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type MemberVipDao struct {
	table    string
	group    string
	columns  MemberVipColumns
	handlers []gdb.ModelHandler
}

type MemberVipColumns struct {
	Id        string
	MemberId  string
	Level     string
	Status    string
	OpenedAt  string
	ExpiredAt string
	Remark    string
	CreatedAt string
	UpdatedAt string
	DeletedAt string
}

var memberVipColumns = MemberVipColumns{
	Id:        "id",
	MemberId:  "member_id",
	Level:     "level",
	Status:    "status",
	OpenedAt:  "opened_at",
	ExpiredAt: "expired_at",
	Remark:    "remark",
	CreatedAt: "created_at",
	UpdatedAt: "updated_at",
	DeletedAt: "deleted_at",
}

func NewMemberVipDao(handlers ...gdb.ModelHandler) *MemberVipDao {
	return &MemberVipDao{
		group:    "default",
		table:    "hg_member_vip",
		columns:  memberVipColumns,
		handlers: handlers,
	}
}

func (dao *MemberVipDao) DB() gdb.DB {
	return g.DB(dao.group)
}

func (dao *MemberVipDao) Table() string {
	return dao.table
}

func (dao *MemberVipDao) Columns() MemberVipColumns {
	return dao.columns
}

func (dao *MemberVipDao) Group() string {
	return dao.group
}

func (dao *MemberVipDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model.Safe().Ctx(ctx)
}

func (dao *MemberVipDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
