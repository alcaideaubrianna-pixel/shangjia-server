package sys

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/consts"
	"hotgo/internal/dao"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type profileDownPlan struct {
	Notify   bool
	Channels []telegramJobChannel
}

func (s *sSysPublish) prepareProfileDownPlan(ctx context.Context, tenantId int64) (*profileDownPlan, error) {
	res, err := NewSysConfig().PublishConfigView(ctx, &sysin.PublishConfigViewInp{})
	if err != nil {
		return nil, err
	}
	plan := &profileDownPlan{}
	if res != nil && res.PublishConfig.SkipDownChannelEnabled == 1 {
		return plan, nil
	}
	channels, err := s.telegramDownChannels(ctx, tenantId)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return plan, nil
	}
	plan.Notify = true
	plan.Channels = channels
	return plan, nil
}

func (s *sSysPublish) handleProfilesDown(ctx context.Context, ids []int64, tenantId int64, downAt ...string) error {
	ids, err := s.activeDownProfileIds(ctx, ids, tenantId)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	cutoffAt := ""
	if len(downAt) > 0 {
		cutoffAt = downAt[0]
	}
	if err := s.deleteProfilesTelegramMessages(ctx, ids, tenantId, cutoffAt); err != nil {
		return err
	}
	ids, err = s.activeDownProfileIds(ctx, ids, tenantId)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	plan, err := s.prepareProfileDownPlan(ctx, tenantId)
	if err != nil {
		return err
	}
	if plan == nil || !plan.Notify {
		return nil
	}
	ids, err = s.activeDownProfileIds(ctx, ids, tenantId)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return s.notifyProfilesDown(ctx, ids, tenantId, plan.Channels)
}

func (s *sSysPublish) activeDownProfileIds(ctx context.Context, ids []int64, tenantId int64) ([]int64, error) {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return []int64{}, nil
	}
	columns := dao.ContentProfile.Columns()
	var rows []struct {
		Id int64 `json:"id"`
	}
	err := g.DB().Model(dao.ContentProfile.Table()+" p").Safe().Ctx(ctx).
		Fields("p."+columns.Id+" AS id").
		LeftJoin(publishTaskTable+" t", "t.profile_id=p.id AND t.deleted_at IS NULL").
		WhereIn("p."+columns.Id, ids).
		Where("t.tenant_id", tenantId).
		Where("t.status", sysin.PublishTaskStatusCanceled).
		Where("p."+columns.Status, 2).
		Where("p."+columns.Visibility, consts.ContentVisibilityPrivate).
		WhereNull("p." + columns.DeletedAt).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取下架资料状态失败")
	}
	activeIds := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.Id > 0 {
			activeIds = append(activeIds, row.Id)
		}
	}
	return uniqueIds(activeIds), nil
}

func (s *sSysPublish) deleteProfilesTelegramMessages(ctx context.Context, ids []int64, tenantId int64, cutoffAt string) error {
	var jobs []telegramResubmitJob
	mod := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("profile_id", ids).
		Where("status", "sent")
	if cutoffAt != "" {
		mod = mod.WhereLTE("created_at", cutoffAt)
	}
	err := mod.Scan(&jobs)
	if err != nil {
		return gerror.Wrap(err, "读取TG下架消息失败")
	}
	for _, job := range jobs {
		if err = s.deleteTelegramJobMessagesForResubmit(ctx, job); err != nil {
			return err
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "down", "deleted", "资料下架，TG历史消息已删除")
	}
	return nil
}

func (s *sSysPublish) notifyProfilesDown(ctx context.Context, ids []int64, tenantId int64, channels []telegramJobChannel) error {
	rows, err := s.profileDownRows(ctx, ids, tenantId)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	for _, row := range rows {
		for _, channel := range channels {
			if err = s.sendDownChannelProfile(ctx, tenantId, channel, row); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *sSysPublish) sendDownChannelProfile(ctx context.Context, tenantId int64, channel telegramJobChannel, row gdb.Record) error {
	taskId := row["id"].Int64()
	profileId := row["profile_id"].Int64()
	accountId := row["account_id"].Int64()
	if taskId <= 0 || profileId <= 0 {
		return nil
	}
	job := telegramJobRecord{
		TaskId:       taskId,
		TenantId:     tenantId,
		AccountId:    accountId,
		ProfileId:    profileId,
		ChannelId:    channel.Id,
		BotId:        firstPositiveId(decodeBotIds(channel.BotIdJson)),
		TargetChatId: normalizeTelegramChannelChatID(channel.TargetChatId),
	}
	if job.TargetChatId == "" {
		return gerror.New("下架频道Chat ID未配置")
	}
	jobId, err := s.createDownChannelTelegramJob(ctx, job)
	if err != nil {
		return err
	}
	job.Id = jobId
	return s.withTelegramChannelLock(ctx, job.TargetChatId, func() error {
		return s.sendDownChannelProfileLockedByChannel(ctx, job, taskId, channel.Id)
	})
}

func (s *sSysPublish) createDownChannelTelegramJob(ctx context.Context, job telegramJobRecord) (int64, error) {
	now := gtime.Now()
	operationNo := fmt.Sprintf("down:%d:%d", job.TaskId, now.TimestampNano())
	id, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
		"task_id":         job.TaskId,
		"operation_no":    operationNo,
		"tenant_id":       job.TenantId,
		"account_id":      job.AccountId,
		"profile_id":      job.ProfileId,
		"channel_id":      job.ChannelId,
		"bot_id":          job.BotId,
		"target_chat_id":  job.TargetChatId,
		"status":          "sending",
		"priority":        tgJobPriorityUrgent,
		"queue_name":      tgQueueNameUrgent,
		"dispatch_status": tgDispatchStatusProcessing,
		"dispatched_at":   now,
		"dispatch_count":  1,
		"created_at":      now,
		"updated_at":      now,
	}).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建下架频道TG任务失败")
	}
	return id, nil
}

func (s *sSysPublish) sendDownChannelProfileLockedByChannel(ctx context.Context, job telegramJobRecord, taskId int64, channelId int64) error {
	activeIds, err := s.activeDownProfileIds(ctx, []int64{job.ProfileId}, job.TenantId)
	if err != nil {
		_ = s.markDownChannelTelegramJobFailed(ctx, job, err)
		return err
	}
	if len(activeIds) == 0 {
		_ = s.markTelegramJobSuperseded(ctx, job.Id)
		s.appendTelegramJobLog(ctx, job, "down_notify", "superseded", "资料已被新操作覆盖，跳过下架频道推送")
		return nil
	}
	botToken, err := s.telegramJobBotToken(ctx, job.BotId, job.TenantId)
	if err != nil {
		_ = s.markDownChannelTelegramJobFailed(ctx, job, err)
		return err
	}
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		_ = s.markDownChannelTelegramJobFailed(ctx, job, err)
		return err
	}
	caption, err := s.telegramJobText(ctx, taskId)
	if err != nil {
		_ = s.markDownChannelTelegramJobFailed(ctx, job, err)
		return err
	}
	displayMedia, err := s.telegramJobMedia(ctx, job, "display")
	if err != nil {
		_ = s.markDownChannelTelegramJobFailed(ctx, job, err)
		return err
	}
	verifyMedia, err := s.telegramJobMedia(ctx, job, "verify")
	if err != nil {
		_ = s.markDownChannelTelegramJobFailed(ctx, job, err)
		return err
	}
	messages, err := s.sendTelegramDisplayPart(ctx, bot, job.TargetChatId, caption, displayMedia)
	if err != nil {
		_ = s.markDownChannelTelegramJobFailed(ctx, job, err)
		return gerror.Wrapf(err, "推送下架频道展示资料失败，task:%d，channel:%d", taskId, channelId)
	}
	verifyMessages, err := s.sendTelegramVerifyPart(ctx, bot, job.TargetChatId, verifyMedia)
	if err != nil {
		_ = s.markDownChannelTelegramJobFailed(ctx, job, err)
		return gerror.Wrapf(err, "推送下架频道验证资料失败，task:%d，channel:%d", taskId, channelId)
	}
	messages = append(messages, verifyMessages...)
	if err = s.saveTelegramSentMessages(ctx, job, messages); err != nil {
		_ = s.markDownChannelTelegramJobFailed(ctx, job, err)
		return err
	}
	if err = s.updateTelegramMediaFileIds(ctx, messages); err != nil {
		_ = s.markDownChannelTelegramJobFailed(ctx, job, err)
		return err
	}
	_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Data(g.Map{
		"status":          "sent",
		"dispatch_status": tgDispatchStatusDone,
		"sent_at":         gtime.Now(),
		"updated_at":      gtime.Now(),
	}).Update()
	s.appendTelegramJobLog(ctx, job, "down_notify", "sent", fmt.Sprintf("资料下架通知已推送到下架频道，频道:%d", channelId))
	return nil
}

func (s *sSysPublish) markDownChannelTelegramJobFailed(ctx context.Context, job telegramJobRecord, cause error) error {
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Data(g.Map{
		"status":          "failed",
		"dispatch_status": tgDispatchStatusDone,
		"error_message":   message,
		"updated_at":      gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) telegramDownChannels(ctx context.Context, tenantId int64) ([]telegramJobChannel, error) {
	var channels []telegramJobChannel
	err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,target_chat_id,bot_id_json").
		Where("tenant_id", tenantId).
		Where("publish_direction", "down").
		Where("status", 1).
		WhereNull("deleted_at").
		OrderAsc("id").
		Scan(&channels)
	if err != nil {
		return nil, gerror.Wrap(err, "读取下架频道失败")
	}
	return channels, nil
}

func (s *sSysPublish) profileDownRows(ctx context.Context, ids []int64, tenantId int64) ([]gdb.Record, error) {
	rows, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,account_id,profile_id,title,province,city").
		Where("tenant_id", tenantId).
		WhereIn("profile_id", ids).
		WhereNull("deleted_at").
		OrderDesc("id").
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取下架资料失败")
	}
	return rows, nil
}
