package sys

import (
	"context"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/utility/excel"
)

func (s *sSysPublish) AdminChannelMemberList(ctx context.Context, in *sysin.TgChannelMemberListInp) ([]*sysin.TgChannelMemberModel, int, error) {
	var err error
	if err := ensureTgChannelMemberSchema(ctx); err != nil {
		return nil, 0, err
	}
	if err := s.requireSystemSuperAdmin(ctx); err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.TgChannelMemberListInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return nil, 0, err
	}
	tenantId, err := s.tenantIdForTgAccount(ctx, in.TgAccountId)
	if err != nil {
		return nil, 0, err
	}
	return s.channelMemberList(ctx, in, tenantId)
}

func (s *sSysPublish) AdminChannelMemberExport(ctx context.Context, in *sysin.TgChannelMemberListInp) error {
	var err error
	if err := ensureTgChannelMemberSchema(ctx); err != nil {
		return err
	}
	if err := s.requireSystemSuperAdmin(ctx); err != nil {
		return err
	}
	if in == nil {
		in = &sysin.TgChannelMemberListInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	tenantId, err := s.tenantIdForTgAccount(ctx, in.TgAccountId)
	if err != nil {
		return err
	}
	in.Page = 1
	in.PerPage = 50000
	rows, _, err := s.channelMemberList(ctx, in, tenantId)
	if err != nil {
		return err
	}
	exports := make([]channelMemberExportRow, 0, len(rows))
	for _, row := range rows {
		exports = append(exports, channelMemberExportRow{
			UserId:      strconv.FormatInt(row.UserId, 10),
			DisplayName: row.DisplayName,
			Username:    row.Username,
			Role:        row.ParticipantRoleText,
			IsBot:       yesNoText(row.IsBot == 1),
			Status:      yesNoStatus(row.Status),
			SyncedAt:    gtimeString(row.LastSyncedAt),
		})
	}
	tags := []string{"用户ID", "昵称", "用户名", "角色", "机器人", "状态", "最后同步时间"}
	return excel.ExportByStructs(ctx, tags, exports, "TG频道成员列表", "成员列表")
}

func (s *sSysPublish) channelMemberList(ctx context.Context, in *sysin.TgChannelMemberListInp, tenantId int64) ([]*sysin.TgChannelMemberModel, int, error) {
	base := g.DB().Model(publishTgChannelMemberTable+" m").Safe().Ctx(ctx).
		Where("m.tenant_id", tenantId).
		Where("m.tg_account_id", in.TgAccountId)
	if in.ChannelId != "" {
		base = base.WhereIn("m.channel_id", tgChannelCacheLookupIds(in.ChannelId))
	}
	if len(in.Roles) > 0 {
		base = base.WhereIn("m.participant_role", in.Roles)
	}
	if in.Status > 0 {
		base = base.Where("m.status", in.Status)
	}
	if in.Keyword != "" {
		like := "%" + in.Keyword + "%"
		base = base.Where("(m.display_name LIKE ? OR m.username LIKE ? OR CAST(m.user_id AS CHAR) LIKE ?)", like, like, like)
	}
	total, err := base.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取频道成员总数失败")
	}
	mod := base.LeftJoin(publishTgChannelTable+" c", "c.tenant_id=m.tenant_id AND c.tg_account_id=m.tg_account_id AND c.channel_id=m.channel_id").
		Fields(channelMemberListFields())
	var list []*sysin.TgChannelMemberModel
	err = mod.Page(in.Page, in.PerPage).
		OrderAsc("m.participant_role").
		OrderDesc("m.id").
		Scan(&list)
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取频道成员列表失败")
	}
	if list == nil {
		list = []*sysin.TgChannelMemberModel{}
	}
	for _, item := range list {
		item.ParticipantRoleText = tgChannelMemberRoleText(item.ParticipantRole)
		item.Username = strings.TrimPrefix(item.Username, "@")
	}
	return list, total, nil
}

func channelMemberListFields() string {
	return strings.Join([]string{
		"m.id",
		"m.tenant_id",
		"m.tg_account_id",
		"m.channel_id",
		"c.channel_title AS channel_title",
		"m.user_id",
		"m.display_name",
		"m.username",
		"m.phone",
		"m.participant_role",
		"m.is_bot",
		"m.is_premium",
		"m.status",
		"m.last_sync_task_id",
		"m.last_synced_at",
		"m.created_at",
		"m.updated_at",
	}, ",")
}

type channelMemberExportRow struct {
	UserId      string
	DisplayName string
	Username    string
	Role        string
	IsBot       string
	Status      string
	SyncedAt    string
}

func tgChannelMemberRoleText(role string) string {
	switch strings.TrimSpace(role) {
	case "creator", "owner":
		return "创建者"
	case "admin":
		return "管理员"
	case "member":
		return "成员"
	default:
		return ""
	}
}

func yesNoText(ok bool) string {
	if ok {
		return "是"
	}
	return "否"
}

func yesNoStatus(status int) string {
	if status == 1 {
		return "有效"
	}
	return "失效"
}

func gtimeString(value *gtime.Time) string {
	if value == nil {
		return ""
	}
	return value.String()
}
