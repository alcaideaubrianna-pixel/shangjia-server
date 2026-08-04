package sys

import (
	"context"
	"fmt"
	"strings"

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

func (s *sSysPublish) handleProfilesDown(ctx context.Context, ids []int64, tenantId int64, downAt string, operationNo string) error {
	ids, err := s.activeDownProfileIds(ctx, ids, tenantId)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	cutoffAt := strings.TrimSpace(downAt)
	operationNo = strings.TrimSpace(operationNo)
	if operationNo == "" {
		operationNo = fmt.Sprintf("down:%d:%s", tenantId, strings.NewReplacer(" ", "-", ":", "", ".", "").Replace(cutoffAt))
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
	return s.notifyProfilesDown(ctx, ids, tenantId, plan.Channels, operationNo, cutoffAt)
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
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		WhereIn("p."+columns.Id, ids).
		Where("ps.tenant_id", tenantId).
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
	jobs, err := s.telegramJobsWithActiveMessages(ctx, tenantId, ids, cutoffAt)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err = s.enqueueTelegramCleanupJob(ctx, job.Id, 0); err != nil {
			return gerror.Wrap(err, "加入TG下架清理队列失败")
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "down", "queued", "资料下架，TG历史消息已加入Redis清理队列")
	}
	return nil
}

func (s *sSysPublish) cleanupProfileDownMessagesBeforePublish(ctx context.Context, profileId int64, tenantId int64) error {
	if profileId <= 0 || tenantId <= 0 {
		return nil
	}
	var jobs []telegramResubmitJob
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("profile_id", profileId).
		Where("status", "sent").
		WhereLike("operation_no", "down:%").
		OrderAsc("id").
		Scan(&jobs)
	if err != nil {
		return gerror.Wrap(err, "读取下架频道历史消息失败")
	}
	for _, job := range jobs {
		if err = s.deleteTelegramMessageSet(ctx, job.telegramJobRecord(), "资料重新上架"); err != nil {
			return gerror.Wrap(err, "清理下架频道历史消息失败")
		}
		if err = s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			return err
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "republish", "deleted", "资料重新上架，下架频道历史消息已删除或不存在")
	}
	return nil
}

func (s *sSysPublish) notifyProfilesDown(ctx context.Context, ids []int64, tenantId int64, channels []telegramJobChannel, operationNo string, cutoffAt string) error {
	rows, err := s.profileDownRows(ctx, ids, tenantId)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	for _, row := range rows {
		for _, channel := range channels {
			if err = s.sendDownChannelProfile(ctx, tenantId, channel, row, operationNo, cutoffAt); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *sSysPublish) sendDownChannelProfile(ctx context.Context, tenantId int64, channel telegramJobChannel, row gdb.Record, operationNo string, cutoffAt string) error {
	profileId := row["profile_id"].Int64()
	accountId := row["account_id"].Int64()
	if profileId <= 0 {
		return nil
	}
	job := telegramJobRecord{
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
	return s.withTelegramChannelLock(ctx, job.TargetChatId, func() error {
		jobId, shouldSend, err := s.createDownChannelTelegramJob(ctx, job, operationNo, cutoffAt)
		if err != nil {
			return err
		}
		if !shouldSend {
			return nil
		}
		job.Id = jobId
		return s.sendDownChannelProfileLockedByChannel(ctx, job, channel.Id)
	})
}

func (s *sSysPublish) createDownChannelTelegramJob(ctx context.Context, job telegramJobRecord, operationNo string, cutoffAt string) (int64, bool, error) {
	existingMod := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("profile_id", job.ProfileId).
		Where("operation_no", operationNo).
		Where("channel_id", job.ChannelId)
	existing, err := existingMod.One()
	if err != nil {
		return 0, false, gerror.Wrap(err, "读取下架频道TG任务失败")
	}
	if existing.IsEmpty() && cutoffAt != "" {
		existing, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("profile_id", job.ProfileId).
			Where("channel_id", job.ChannelId).
			Where("status", "sent").
			WhereLike("operation_no", "down:%").
			WhereGTE("created_at", cutoffAt).
			OrderDesc("id").One()
		if err != nil {
			return 0, false, gerror.Wrap(err, "检查下架频道已发送任务失败")
		}
	}
	if !existing.IsEmpty() {
		if existing["status"].String() == "sent" {
			return existing["id"].Int64(), false, nil
		}
		_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", existing["id"].Int64()).Data(g.Map{
			"status": "sending", "dispatch_status": tgDispatchStatusProcessing,
			"error_message": "", "updated_at": gtime.Now(),
		}).Update()
		if err != nil {
			return 0, false, gerror.Wrap(err, "重置下架频道TG任务失败")
		}
		return existing["id"].Int64(), true, nil
	}
	now := gtime.Now()
	id, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
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
		return 0, false, gerror.Wrap(err, "创建下架频道TG任务失败")
	}
	return id, true, nil
}

func (s *sSysPublish) sendDownChannelProfileLockedByChannel(ctx context.Context, job telegramJobRecord, channelId int64) error {
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
	caption, err := s.telegramJobCaption(ctx, job)
	if err != nil {
		_ = s.markDownChannelTelegramJobFailed(ctx, job, err)
		return err
	}
	displayMedia, err := s.telegramJobMedia(ctx, job, "display")
	if err != nil {
		_ = s.markDownChannelTelegramJobFailed(ctx, job, err)
		return err
	}
	displayMedia = selectTelegramDisplayMedia(job, displayMedia, telegramMediaGroupMaxItems)
	verifyMedia, err := s.telegramJobMedia(ctx, job, "verify")
	if err != nil {
		_ = s.markDownChannelTelegramJobFailed(ctx, job, err)
		return err
	}
	caption, err = s.applyTelegramJobContentProtection(ctx, job, caption, displayMedia, verifyMedia)
	if err != nil {
		_ = s.markDownChannelTelegramJobFailed(ctx, job, err)
		return err
	}
	messages, err := s.sendTelegramDisplayPart(ctx, bot, job.TargetChatId, caption, displayMedia)
	if err != nil {
		_ = s.markDownChannelTelegramJobFailed(ctx, job, err)
		return gerror.Wrapf(err, "推送下架频道展示资料失败，profile:%d，channel:%d", job.ProfileId, channelId)
	}
	verifyMessages, err := s.sendTelegramVerifyPart(ctx, bot, job.TargetChatId, "", verifyMedia)
	if err != nil {
		_ = s.markDownChannelTelegramJobFailed(ctx, job, err)
		return gerror.Wrapf(err, "推送下架频道验证资料失败，profile:%d，channel:%d", job.ProfileId, channelId)
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
	s.appendTelegramJobLog(ctx, job, "down_notify", "sent", s.telegramJobPublishMessage(ctx, job, fmt.Sprintf("资料下架通知已推送到下架频道，频道:%d", channelId)))
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
	rows, err := g.DB().Model(publishProfileStateTable+" ps").Safe().Ctx(ctx).
		InnerJoin(dao.ContentProfile.Table()+" p", "p.id=ps.profile_id AND p.deleted_at IS NULL").
		Fields("ps.tenant_id,ps.account_id,p.id AS profile_id,p.title,p.province,p.city").
		Where("ps.tenant_id", tenantId).
		WhereIn("ps.profile_id", ids).
		WhereNull("ps.deleted_at").
		OrderAsc("p.id").
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取下架资料失败")
	}
	return rows, nil
}
