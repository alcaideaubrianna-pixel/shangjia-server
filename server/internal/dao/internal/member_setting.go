package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type MemberSettingDao struct {
	table    string
	group    string
	columns  MemberSettingColumns
	handlers []gdb.ModelHandler
}

type MemberSettingColumns struct {
	Id              string
	MemberId        string
	MessageEnabled  string
	HideOnline      string
	HideViewHistory string
	MatchChatOnly   string
	ProfileScope    string
	PhotoScope      string
	ThemeMode       string
	CreatedAt       string
	UpdatedAt       string
	DeletedAt       string
}

var memberSettingColumns = MemberSettingColumns{
	Id:              "id",
	MemberId:        "member_id",
	MessageEnabled:  "message_enabled",
	HideOnline:      "hide_online",
	HideViewHistory: "hide_view_history",
	MatchChatOnly:   "match_chat_only",
	ProfileScope:    "profile_scope",
	PhotoScope:      "photo_scope",
	ThemeMode:       "theme_mode",
	CreatedAt:       "created_at",
	UpdatedAt:       "updated_at",
	DeletedAt:       "deleted_at",
}

func NewMemberSettingDao(handlers ...gdb.ModelHandler) *MemberSettingDao {
	return &MemberSettingDao{
		group:    "default",
		table:    "hg_member_setting",
		columns:  memberSettingColumns,
		handlers: handlers,
	}
}

func (dao *MemberSettingDao) DB() gdb.DB {
	return g.DB(dao.group)
}

func (dao *MemberSettingDao) Table() string {
	return dao.table
}

func (dao *MemberSettingDao) Columns() MemberSettingColumns {
	return dao.columns
}

func (dao *MemberSettingDao) Group() string {
	return dao.group
}

func (dao *MemberSettingDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model.Safe().Ctx(ctx)
}

func (dao *MemberSettingDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
