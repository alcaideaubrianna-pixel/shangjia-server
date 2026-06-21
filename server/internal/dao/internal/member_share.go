package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type MemberShareDao struct {
	table    string
	group    string
	columns  MemberShareColumns
	handlers []gdb.ModelHandler
}

type MemberShareColumns struct {
	Id            string
	MemberId      string
	ProfileId     string
	ShareToken    string
	VisitCount    string
	RegisterCount string
	LastVisitAt   string
	CreatedAt     string
	UpdatedAt     string
	DeletedAt     string
}

var memberShareColumns = MemberShareColumns{
	Id:            "id",
	MemberId:      "member_id",
	ProfileId:     "profile_id",
	ShareToken:    "share_token",
	VisitCount:    "visit_count",
	RegisterCount: "register_count",
	LastVisitAt:   "last_visit_at",
	CreatedAt:     "created_at",
	UpdatedAt:     "updated_at",
	DeletedAt:     "deleted_at",
}

func NewMemberShareDao(handlers ...gdb.ModelHandler) *MemberShareDao {
	return &MemberShareDao{
		group:    "default",
		table:    "hg_member_share",
		columns:  memberShareColumns,
		handlers: handlers,
	}
}

func (dao *MemberShareDao) DB() gdb.DB {
	return g.DB(dao.group)
}

func (dao *MemberShareDao) Table() string {
	return dao.table
}

func (dao *MemberShareDao) Columns() MemberShareColumns {
	return dao.columns
}

func (dao *MemberShareDao) Group() string {
	return dao.group
}

func (dao *MemberShareDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model.Safe().Ctx(ctx)
}

func (dao *MemberShareDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
