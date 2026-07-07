package sys

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

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

func (s *sSysPublish) handleProfilesDown(ctx context.Context, ids []int64, tenantId int64) error {
	if err := s.deleteProfilesTelegramMessages(ctx, ids, tenantId); err != nil {
		return err
	}
	plan, err := s.prepareProfileDownPlan(ctx, tenantId)
	if err != nil {
		return err
	}
	if plan == nil || !plan.Notify {
		return nil
	}
	return s.notifyProfilesDown(ctx, ids, tenantId, plan.Channels)
}

func (s *sSysPublish) deleteProfilesTelegramMessages(ctx context.Context, ids []int64, tenantId int64) error {
	var jobs []telegramResubmitJob
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("profile_id", ids).
		Where("status", "sent").
		Scan(&jobs)
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
	return s.withTelegramChannelLock(ctx, job.TargetChatId, func() error {
		return s.sendDownChannelProfileLockedByChannel(ctx, job, taskId, channel.Id)
	})
}

func (s *sSysPublish) sendDownChannelProfileLockedByChannel(ctx context.Context, job telegramJobRecord, taskId int64, channelId int64) error {
	botToken, err := s.telegramJobBotToken(ctx, job.BotId, job.TenantId)
	if err != nil {
		return err
	}
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	caption, err := s.telegramJobText(ctx, taskId)
	if err != nil {
		return err
	}
	displayMedia, err := s.telegramJobMedia(ctx, job, "display")
	if err != nil {
		return err
	}
	verifyMedia, err := s.telegramJobMedia(ctx, job, "verify")
	if err != nil {
		return err
	}
	messages, err := s.sendTelegramDisplayPart(ctx, bot, job.TargetChatId, caption, displayMedia)
	if err != nil {
		return gerror.Wrapf(err, "推送下架频道展示资料失败，task:%d，channel:%d", taskId, channelId)
	}
	verifyMessages, err := s.sendTelegramVerifyPart(ctx, bot, job.TargetChatId, verifyMedia)
	if err != nil {
		return gerror.Wrapf(err, "推送下架频道验证资料失败，task:%d，channel:%d", taskId, channelId)
	}
	messages = append(messages, verifyMessages...)
	if err = s.updateTelegramMediaFileIds(ctx, messages); err != nil {
		return err
	}
	s.appendTelegramJobLog(ctx, job, "down_notify", "sent", fmt.Sprintf("资料下架通知已推送到下架频道，频道:%d", channelId))
	return nil
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
