package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/consts"
	"hotgo/internal/dao"
	iservice "hotgo/internal/service"
)

// profilePublishSource is the current profile projection used by both job
// creation and workers. It is deliberately loaded from the profile tables and
// is never persisted as a task snapshot.
func (s *sSysPublish) profilePublishSource(ctx context.Context, profileId, tenantId, accountId int64, requireOnline bool) (gdb.Record, error) {
	mod := g.DB().Model(dao.ContentProfile.Table()+" p").Safe().Ctx(ctx).
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		LeftJoin(publishAccountTable+" a", "a.id=ps.account_id AND a.deleted_at IS NULL").
		Fields("p.id AS profile_id,p.profile_no,p.title,p.province,p.city,p.plain_text,p.status,p.visibility,"+
			"ps.tenant_id,ps.account_id,ps.channel_id_json,ps.customer_remark,ps.anti_scan_enabled,"+
			"a.nickname AS account_nickname,"+
			"(SELECT COUNT(1) FROM "+publishProfileStateTable+" ps_seq "+
			"WHERE ps_seq.tenant_id=ps.tenant_id AND ps_seq.account_id=ps.account_id "+
			"AND ps_seq.id<=ps.id AND ps_seq.deleted_at IS NULL) AS account_sequence").
		Where("p.id", profileId).
		WhereNull("p.deleted_at")
	if tenantId > 0 {
		mod = mod.Where("ps.tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("ps.account_id", accountId)
	}
	row, err := mod.One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取待发布资料失败")
	}
	if row.IsEmpty() {
		return nil, errPublishProfileUnavailable
	}
	if requireOnline && (row["status"].Int() != 1 || row["visibility"].String() != consts.ContentVisibilityPublic) {
		return nil, errPublishProfileUnavailable
	}
	return row, nil
}

func (s *sSysPublish) submitProfilePublish(ctx context.Context, profileId, tenantId, accountId, operatorId int64, operationNo string, channelIds []int64, requireOnline bool) error {
	return s.submitProfilePublishWithMeta(ctx, profileId, tenantId, accountId, operatorId, operationNo, channelIds, requireOnline, telegramProfilePublishMeta{})
}

type telegramProfilePublishMeta struct {
	CollectEventId         int64
	CollectSourceId        int64
	CollectSourceChatId    string
	CollectSourceMessageId int64
}

func (s *sSysPublish) submitProfilePublishWithMeta(ctx context.Context, profileId, tenantId, accountId, operatorId int64, operationNo string, channelIds []int64, requireOnline bool, meta telegramProfilePublishMeta) error {
	source, err := s.profilePublishSource(ctx, profileId, tenantId, accountId, requireOnline)
	if err != nil {
		return err
	}
	channels, err := s.telegramJobChannels(ctx, source, channelIds)
	if err != nil {
		return err
	}
	operationNo = strings.TrimSpace(operationNo)
	if operationNo == "" {
		operationNo = newTelegramOperationNo("profile", profileId)
	}
	jobIds := make([]int64, 0, len(channels))
	for _, channel := range channels {
		jobId, createErr := s.ensureTelegramProfileJobWithMeta(ctx, source, channel, operationNo, meta)
		if createErr != nil {
			return createErr
		}
		jobIds = append(jobIds, jobId)
	}
	for _, jobId := range jobIds {
		if err = s.enqueueTelegramJob(ctx, jobId, 0); err != nil {
			message := "Redis调度失败，等待数据库调度器恢复：" + err.Error()
			_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobId).Data(g.Map{
				"dispatch_status": tgDispatchStatusIdle, "last_dispatch_error": message, "updated_at": gtime.Now(),
			}).Update()
			g.Log().Warningf(ctx, "资料TG任务入队失败，等待数据库调度器恢复 jobId:%d operatorId:%d err:%+v", jobId, operatorId, err)
		}
	}
	return nil
}

func (s *sSysPublish) ensureTelegramProfileJob(ctx context.Context, source gdb.Record, channel telegramJobChannel, operationNo string) (int64, error) {
	return s.ensureTelegramProfileJobWithMeta(ctx, source, channel, operationNo, telegramProfilePublishMeta{})
}

func (s *sSysPublish) ensureTelegramProfileJobWithMeta(ctx context.Context, source gdb.Record, channel telegramJobChannel, operationNo string, meta telegramProfilePublishMeta) (int64, error) {
	existing, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("profile_id", source["profile_id"].Int64()).
		Where("operation_no", operationNo).
		Where("channel_id", channel.Id).
		Fields("id,status,bot_id,target_chat_id").One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取资料TG频道任务失败")
	}
	if jobId := existing["id"].Int64(); jobId > 0 {
		return jobId, nil
	}
	botId, err := telegramChannelSenderBotId(channel)
	if err != nil {
		return 0, err
	}
	now := gtime.Now()
	job := telegramJobRecord{
		OperationNo: operationNo, TenantId: source["tenant_id"].Int64(), AccountId: source["account_id"].Int64(),
		ProfileId: source["profile_id"].Int64(), ChannelId: channel.Id, BotId: botId,
		TargetChatId: normalizeTelegramChannelChatID(channel.TargetChatId), Status: "pending",
		CollectEventId: meta.CollectEventId, CollectSourceId: meta.CollectSourceId,
		CollectSourceChatId: strings.TrimSpace(meta.CollectSourceChatId), CollectSourceMessageId: meta.CollectSourceMessageId,
	}
	jobId, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
		"operation_no": operationNo,
		"tenant_id":    job.TenantId, "merchant_id": job.TenantId, "account_id": job.AccountId,
		"profile_id": job.ProfileId, "channel_id": job.ChannelId, "bot_id": job.BotId,
		"target_chat_id": job.TargetChatId, "status": "pending",
		"collect_event_id": job.CollectEventId, "collect_source_id": job.CollectSourceId,
		"collect_source_chat_id": job.CollectSourceChatId, "collect_source_message_id": job.CollectSourceMessageId,
		"priority": s.telegramJobPriority(job), "queue_name": telegramQueueNameByPriority(s.telegramJobPriority(job)),
		"dispatch_status": tgDispatchStatusIdle, "created_at": now, "updated_at": now,
	}).InsertAndGetId()
	if err != nil {
		if isDuplicateKeyError(err) {
			value, readErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
				Where("profile_id", job.ProfileId).
				Where("operation_no", operationNo).Where("channel_id", channel.Id).Fields("id").Value()
			if readErr == nil && value.Int64() > 0 {
				return value.Int64(), nil
			}
		}
		return 0, gerror.Wrap(err, "创建资料TG频道任务失败")
	}
	job.Id = jobId
	if recordErr := s.upsertPublishJobRecord(ctx, job, "pending", ""); recordErr != nil {
		g.Log().Warningf(ctx, "保存资料待发送记录失败 jobId:%d err:%+v", jobId, recordErr)
	}
	return jobId, nil
}

func (s *sSysPublish) completeProfileTelegramOperation(ctx context.Context, job telegramJobRecord, isCycle bool) error {
	total, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("profile_id", job.ProfileId).
		Where("operation_no", job.OperationNo).Count()
	if err != nil {
		return gerror.Wrap(err, "统计资料TG任务失败")
	}
	if total == 0 {
		return nil
	}
	pending, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("profile_id", job.ProfileId).
		Where("operation_no", job.OperationNo).
		WhereNotIn("status", []string{"sent", "superseded"}).Count()
	if err != nil {
		return gerror.Wrap(err, "统计资料未完成TG任务失败")
	}
	if pending > 0 {
		return nil
	}
	if !isCycle {
		now := gtime.Now()
		if _, err = s.syncProfilePublishState(ctx, job.ProfileId, 1, consts.ContentVisibilityPublic, now); err != nil {
			return gerror.Wrap(err, "同步资料上架状态失败")
		}
		if err = s.syncProfileNoteIndex(ctx, job.ProfileId); err != nil {
			return err
		}
		iservice.SysContent().ClearHomeProfileCardsCache(ctx)
		if err = s.incrementDailyPublishStat(ctx, job); err != nil {
			return err
		}
	}
	if isCycle {
		s.cleanupPreviousCycleMessages(ctx, job)
	}
	return nil
}
