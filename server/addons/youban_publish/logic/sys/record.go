package sys

import (
	"context"
	"fmt"
	"strconv"
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

func (s *sSysPublish) AdminTgObserveQueueList(ctx context.Context, in *sysin.TgObserveQueueListInp) (list []*sysin.TgObserveQueueStatModel, totalCount int, err error) {
	if _, err = s.currentAdminAccount(ctx); err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.TgObserveQueueListInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return nil, 0, err
	}
	mod := g.DB().Model(publishTgQueueStatTable).Safe().Ctx(ctx)
	if in.QueueName != "" {
		mod = mod.Where("queue_name", in.QueueName)
	}
	if in.Status != "" {
		mod = mod.Where("status", in.Status)
	}
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取TG队列统计总数失败")
	}
	err = mod.Page(in.Page, in.PerPage).OrderDesc("job_count").OrderAsc("priority_level").Scan(&list)
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取TG队列统计失败")
	}
	return
}

func (s *sSysPublish) AdminTgObserveChannelList(ctx context.Context, in *sysin.TgObserveChannelListInp) (list []*sysin.TgObserveChannelStatModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.TgObserveChannelListInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return nil, 0, err
	}
	mod := g.DB().Model(publishTgChannelStatTable+" s").Safe().Ctx(ctx).
		LeftJoin(publishAccountTable+" a", "a.id=s.account_id").
		Where("s.tenant_id", account.TenantId)
	if in.AccountId > 0 {
		mod = mod.Where("s.account_id", in.AccountId)
	}
	if in.ChannelId > 0 {
		mod = mod.Where("s.channel_id", in.ChannelId)
	}
	if in.Keyword != "" {
		like := "%" + in.Keyword + "%"
		mod = mod.Where("(s.channel_title LIKE ? OR s.target_chat_id LIKE ? OR a.nickname LIKE ?)", like, like, like)
	}
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取TG频道统计总数失败")
	}
	err = mod.Fields("s.*,a.nickname AS account_name").Page(in.Page, in.PerPage).
		OrderDesc("s.sending_count").OrderDesc("s.queued_count").OrderDesc("s.pending_count").OrderDesc("s.updated_at").
		Scan(&list)
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取TG频道统计失败")
	}
	return
}

func (s *sSysPublish) AdminTgObserveBotList(ctx context.Context, in *sysin.TgObserveBotListInp) (list []*sysin.TgObserveBotStatModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.TgObserveBotListInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return nil, 0, err
	}
	mod := g.DB().Model(publishTgBotStatTable).Safe().Ctx(ctx).Where("tenant_id", account.TenantId)
	if in.BotId > 0 {
		mod = mod.Where("bot_id", in.BotId)
	}
	if in.Keyword != "" {
		like := "%" + in.Keyword + "%"
		mod = mod.Where("(bot_name LIKE ? OR bot_username LIKE ?)", like, like)
	}
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取TG Bot统计总数失败")
	}
	err = mod.Page(in.Page, in.PerPage).
		OrderDesc("sending_count").OrderDesc("queued_count").OrderDesc("pending_count").OrderDesc("updated_at").
		Scan(&list)
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取TG Bot统计失败")
	}
	return
}

func (s *sSysPublish) MyPublishRecordClear(ctx context.Context, in *sysin.PublishRecordClearInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	return s.publishRecordClear(ctx, account.TenantId, account.Id)
}

func (s *sSysPublish) publishRecordList(ctx context.Context, in *sysin.PublishRecordListInp, tenantId int64, accountId int64) (list []*sysin.PublishRecordModel, totalCount int, err error) {
	if err = ensurePublishSuccessRecordSchema(ctx); err != nil {
		return nil, 0, err
	}
	if in.Keyword == "" {
		return s.publishRecordListFast(ctx, in, tenantId, accountId)
	}
	base := s.publishRecordCountModel(ctx, in, tenantId, accountId)
	totalCount, err = base.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取发送记录总数失败")
	}
	mod := s.publishRecordBaseModel(ctx, in, tenantId, accountId).Fields(
		"l.id,l.job_id,l.task_id,l.tenant_id,l.account_id,l.profile_id,l.channel_id,l.bot_id,l.operation_no,l.target_chat_id,l.action,l.status,l.message,l.created_at",
		"t.client_request_id",
		"p.title",
		"a.nickname AS account_name",
		"COALESCE(NULLIF(owner.username, ''), NULLIF(tn.name, '')) AS tenant_name",
		"b.bot_name,b.bot_username",
		"COALESCE(NULLIF(c.channel_title, ''), NULLIF(tc.channel_title, '')) AS channel_title",
		"c.channel_username",
	)
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("l.id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取发送记录失败")
	}
	if err = s.enrichPublishRecordChannelDisplays(ctx, tenantId, list); err != nil {
		return nil, 0, err
	}
	if err = s.enrichPublishRecordFullPushProgress(ctx, list); err != nil {
		return nil, 0, err
	}
	normalizeCollectPublishRecordActions(list)
	return
}

func (s *sSysPublish) publishRecordCountModel(ctx context.Context, in *sysin.PublishRecordListInp, tenantId int64, accountId int64) *gdb.Model {
	mod := g.DB().Model(publishSuccessRecordTable+" l").Safe().Ctx(ctx).
		LeftJoin(publishTgJobTable+" j", "j.id=l.job_id").
		LeftJoin(publishTaskTable+" t", "t.id=l.task_id").
		LeftJoin(dao.ContentProfile.Table()+" p", "p.id=l.profile_id").
		LeftJoin(publishAccountTable+" a", "a.id=l.account_id").
		LeftJoin(publishTenantTable+" tn", "tn.id=l.tenant_id").
		LeftJoin(publishAccountTable+" owner", "owner.tenant_id=l.tenant_id AND owner.account_type='admin' AND owner.deleted_at IS NULL").
		LeftJoin(publishBotTable+" b", "b.id=l.bot_id").
		LeftJoin(publishChannelTable+" c", "c.id=j.channel_id").
		Where("l.tenant_id", tenantId)
	return s.applyPublishRecordFilters(mod, in, accountId)
}

func (s *sSysPublish) publishRecordBaseModel(ctx context.Context, in *sysin.PublishRecordListInp, tenantId int64, accountId int64) *gdb.Model {
	mod := g.DB().Model(publishSuccessRecordTable+" l").Safe().Ctx(ctx).
		LeftJoin(publishTgJobTable+" j", "j.id=l.job_id").
		LeftJoin(publishTaskTable+" t", "t.id=l.task_id").
		LeftJoin(dao.ContentProfile.Table()+" p", "p.id=l.profile_id").
		LeftJoin(publishAccountTable+" a", "a.id=l.account_id").
		LeftJoin(publishBotTable+" b", "b.id=l.bot_id").
		LeftJoin(publishChannelTable+" c", "c.id=j.channel_id").
		LeftJoin(publishTgChannelTable+" tc", "tc.tenant_id=j.tenant_id AND tc.tg_account_id=j.account_id AND REPLACE(tc.channel_id, '-100', '')=REPLACE(j.target_chat_id, '-100', '')").
		Where("l.tenant_id", tenantId)
	return s.applyPublishRecordFilters(mod, in, accountId)
}

func (s *sSysPublish) applyPublishRecordFilters(mod *gdb.Model, in *sysin.PublishRecordListInp, accountId int64) *gdb.Model {
	if accountId > 0 {
		mod = mod.Where("l.account_id", accountId)
	} else if in.AccountId > 0 {
		mod = mod.Where("l.account_id", in.AccountId)
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
	if in.Status != "" && in.Status != "success" && in.Status != "sent" {
		mod = mod.Where("1", 0)
	} else {
		mod = mod.Where("l.status", "success")
	}
	if in.Keyword != "" {
		like := "%" + in.Keyword + "%"
		if id, err := strconv.ParseInt(in.Keyword, 10, 64); err == nil && id > 0 {
			mod = mod.Where("(p.title LIKE ? OR l.message LIKE ? OR c.channel_title LIKE ? OR b.bot_name LIKE ? OR b.bot_username LIKE ? OR a.nickname LIKE ? OR l.target_chat_id LIKE ? OR l.id=? OR l.job_id=? OR l.task_id=? OR l.profile_id=?)", like, like, like, like, like, like, like, id, id, id, id)
		} else {
			mod = mod.Where("(p.title LIKE ? OR l.message LIKE ? OR c.channel_title LIKE ? OR b.bot_name LIKE ? OR b.bot_username LIKE ? OR a.nickname LIKE ? OR l.target_chat_id LIKE ?)", like, like, like, like, like, like, like)
		}
	}
	return mod
}

func (s *sSysPublish) publishRecordClear(ctx context.Context, tenantId int64, accountId int64) error {
	if err := ensurePublishSuccessRecordSchema(ctx); err != nil {
		return err
	}
	mod := g.DB().Model(publishSuccessRecordTable).Safe().Ctx(ctx).Where("tenant_id", tenantId)
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

func normalizeCollectPublishRecordActions(list []*sysin.PublishRecordModel) {
	for _, item := range list {
		if item == nil || item.Action != "publish" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(item.ClientRequestId), "collect:") {
			item.Action = "collect_publish"
		}
	}
}
