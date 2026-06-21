package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type MemberVipLogDao struct {
	table    string
	group    string
	columns  MemberVipLogColumns
	handlers []gdb.ModelHandler
}

type MemberVipLogColumns struct {
	Id              string
	MemberId        string
	OperatorId      string
	Source          string
	Action          string
	BeforeStatus    string
	AfterStatus     string
	BeforeLevel     string
	AfterLevel      string
	BeforeExpiredAt string
	AfterExpiredAt  string
	Remark          string
	CreatedAt       string
}

var memberVipLogColumns = MemberVipLogColumns{
	Id:              "id",
	MemberId:        "member_id",
	OperatorId:      "operator_id",
	Source:          "source",
	Action:          "action",
	BeforeStatus:    "before_status",
	AfterStatus:     "after_status",
	BeforeLevel:     "before_level",
	AfterLevel:      "after_level",
	BeforeExpiredAt: "before_expired_at",
	AfterExpiredAt:  "after_expired_at",
	Remark:          "remark",
	CreatedAt:       "created_at",
}

func NewMemberVipLogDao(handlers ...gdb.ModelHandler) *MemberVipLogDao {
	return &MemberVipLogDao{
		group:    "default",
		table:    "hg_member_vip_log",
		columns:  memberVipLogColumns,
		handlers: handlers,
	}
}

func (dao *MemberVipLogDao) DB() gdb.DB {
	return g.DB(dao.group)
}

func (dao *MemberVipLogDao) Table() string {
	return dao.table
}

func (dao *MemberVipLogDao) Columns() MemberVipLogColumns {
	return dao.columns
}

func (dao *MemberVipLogDao) Group() string {
	return dao.group
}

func (dao *MemberVipLogDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model.Safe().Ctx(ctx)
}

func (dao *MemberVipLogDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
