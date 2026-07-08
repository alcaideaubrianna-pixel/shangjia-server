package sys

import (
	"context"
	"fmt"
	"strings"

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

func (s *sSysPublish) AdminPublishRecordClear(ctx context.Context, in *sysin.PublishRecordClearInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.publishRecordClear(ctx, account.TenantId, 0)
}

func (s *sSysPublish) MyPublishRecordClear(ctx context.Context, in *sysin.PublishRecordClearInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	return s.publishRecordClear(ctx, account.TenantId, account.Id)
}

func (s *sSysPublish) publishRecordList(ctx context.Context, in *sysin.PublishRecordListInp, tenantId int64, accountId int64) (list []*sysin.PublishRecordModel, totalCount int, err error) {
	base := s.publishRecordBaseModel(ctx, in, tenantId, accountId)
	totalCount, err = base.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取发送记录总数失败")
	}
	mod := s.publishRecordBaseModel(ctx, in, tenantId, accountId).Fields(
		"l.id,l.job_id,l.task_id,l.tenant_id,l.account_id,l.profile_id,l.bot_id,l.action,l.status,l.message,l.created_at",
		"j.channel_id,j.target_chat_id,j.operation_no",
		"p.title",
		"a.nickname AS account_name",
		"b.bot_name,b.bot_username",
		"c.channel_title,c.channel_username",
	)
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("l.id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取发送记录失败")
	}
	if err = s.enrichPublishRecordFullPushProgress(ctx, list); err != nil {
		return nil, 0, err
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
	if in.Action == "full_push" {
		mod = mod.Where("j.operation_no LIKE ?", "full_push:%")
	} else if in.Action != "" {
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

func (s *sSysPublish) publishRecordClear(ctx context.Context, tenantId int64, accountId int64) error {
	mod := g.DB().Model(publishTgJobLogTable).Safe().Ctx(ctx).Where("tenant_id", tenantId)
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	_, err := mod.Delete()
	if err != nil {
		return gerror.Wrap(err, "清空发送记录失败")
	}
	return nil
}

func (s *sSysPublish) enrichPublishRecordFullPushProgress(ctx context.Context, list []*sysin.PublishRecordModel) error {
	batchKeys := make(map[string]struct{})
	for _, item := range list {
		if item == nil || !strings.HasPrefix(item.OperationNo, "full_push:") {
			continue
		}
		item.Action = "full_push"
		if key := fullPushOperationBatchKey(item.OperationNo); key != "" {
			batchKeys[key] = struct{}{}
		}
	}
	if len(batchKeys) == 0 {
		return nil
	}
	progress := make(map[string][2]int, len(batchKeys))
	for key := range batchKeys {
		total, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("operation_no LIKE ?", key+":%").Count()
		if err != nil {
			return gerror.Wrap(err, "统计全量推送进度失败")
		}
		done, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("operation_no LIKE ?", key+":%").
			WhereIn("status", []string{"sent", "failed", "superseded"}).
			Count()
		if err != nil {
			return gerror.Wrap(err, "统计全量推送完成数失败")
		}
		progress[key] = [2]int{done, total}
	}
	for _, item := range list {
		if item == nil || item.Action != "full_push" {
			continue
		}
		value := progress[fullPushOperationBatchKey(item.OperationNo)]
		item.ProgressDone = value[0]
		item.ProgressTotal = value[1]
		if value[1] > 0 {
			item.ProgressText = fmt.Sprintf("%d/%d", value[0], value[1])
		}
	}
	return nil
}

func fullPushOperationBatchKey(operationNo string) string {
	parts := strings.Split(operationNo, ":")
	if len(parts) < 3 || parts[0] != "full_push" {
		return ""
	}
	return strings.Join(parts[:3], ":")
}
