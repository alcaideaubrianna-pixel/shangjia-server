package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
)

func (s *sSysPublish) MyPublishRecordList(ctx context.Context, in *sysin.PublishRecordListInp) (list []*sysin.PublishRecordModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.PublishRecordListInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return nil, 0, err
	}
	return s.publishRecordList(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) AdminPublishRecordList(ctx context.Context, in *sysin.PublishRecordListInp) (list []*sysin.PublishRecordModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.PublishRecordListInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return nil, 0, err
	}
	return s.publishRecordList(ctx, in, account.TenantId, 0)
}

func (s *sSysPublish) publishRecordList(ctx context.Context, in *sysin.PublishRecordListInp, tenantId int64, accountId int64) (list []*sysin.PublishRecordModel, totalCount int, err error) {
	base := s.publishRecordBaseModel(ctx, in, tenantId, accountId)
	totalCount, err = base.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取发送记录总数失败")
	}
	mod := s.publishRecordBaseModel(ctx, in, tenantId, accountId).Fields(
		"l.id,l.job_id,l.task_id,l.tenant_id,l.account_id,l.profile_id,l.bot_id,l.action,l.status,l.message,l.created_at",
		"j.channel_id,j.target_chat_id",
		"p.title",
		"a.nickname AS account_name",
		"b.bot_name,b.bot_username",
		"c.channel_title,c.channel_username",
	)
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("l.id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取发送记录失败")
	}
	return
}

func (s *sSysPublish) publishRecordBaseModel(ctx context.Context, in *sysin.PublishRecordListInp, tenantId int64, accountId int64) *gdb.Model {
	mod := g.DB().Model(publishTgJobLogTable+" l").Safe().Ctx(ctx).
		LeftJoin(publishTgJobTable+" j", "j.id=l.job_id").
		LeftJoin(dao.ContentProfile.Table()+" p", "p.id=l.profile_id").
		LeftJoin(publishAccountTable+" a", "a.id=l.account_id").
		LeftJoin(publishBotTable+" b", "b.id=l.bot_id").
		LeftJoin(publishChannelTable+" c", "c.id=j.channel_id").
		Where("l.tenant_id", tenantId)
	if accountId > 0 {
		mod = mod.Where("l.account_id", accountId)
	}
	if in.ProfileId > 0 {
		mod = mod.Where("l.profile_id", in.ProfileId)
	}
	if in.TaskId > 0 {
		mod = mod.Where("l.task_id", in.TaskId)
	}
	if in.Action != "" {
		mod = mod.Where("l.action", in.Action)
	}
	if in.Status != "" {
		mod = mod.Where("l.status", in.Status)
	}
	if in.Keyword != "" {
		like := "%" + in.Keyword + "%"
		mod = mod.Where("(p.title LIKE ? OR l.message LIKE ? OR c.channel_title LIKE ? OR b.bot_name LIKE ?)", like, like, like, like)
	}
	return mod
}
