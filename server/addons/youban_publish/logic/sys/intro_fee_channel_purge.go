package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	pdo "hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const introFeeChannelPurgeMessage = "频道删除并下架"

func (s *sSysPublish) IntroFeeChannelPurge(ctx context.Context, in *sysin.IntroFeeChannelPurgeInp) (*sysin.IntroFeeChannelPurgeModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.introFeeChannelPurge(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) AdminIntroFeeChannelPurge(ctx context.Context, in *sysin.IntroFeeChannelPurgeInp) (*sysin.IntroFeeChannelPurgeModel, error) {
	admin, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || !s.publishAccountBelongsToTenant(ctx, in.AccountId, admin.TenantId) {
		return nil, gerror.New("采集账户不存在或无权访问")
	}
	return s.introFeeChannelPurge(ctx, in, admin.TenantId, in.AccountId)
}

func (s *sSysPublish) BotIntroFeeChannelPurge(ctx context.Context, in *sysin.IntroFeeChannelPurgeInp, tenantID, accountID int64) (*sysin.IntroFeeChannelPurgeModel, error) {
	if tenantID <= 0 || accountID <= 0 {
		return nil, gerror.New("频道下架参数不完整")
	}
	return s.introFeeChannelPurge(ctx, in, tenantID, accountID)
}

func (s *sSysPublish) introFeeChannelPurge(ctx context.Context, in *sysin.IntroFeeChannelPurgeInp, tenantID, accountID int64) (*sysin.IntroFeeChannelPurgeModel, error) {
	if in == nil || in.RuleId <= 0 || normalizeChannelProfileID(in.ChannelId) == "" {
		return nil, gerror.New("规则ID和频道ID不能为空")
	}
	if !s.introFeeRuleOwned(ctx, tenantID, accountID, in.RuleId) {
		return nil, gerror.New("介绍费规则不存在或无权操作")
	}
	channelID := normalizeChannelProfileID(in.ChannelId)
	profileIDs, err := collectChannelProfileIDs(ctx, tenantID, accountID, in.RuleId, channelID)
	if err != nil {
		return nil, err
	}
	preview, err := s.collectSourceDownPreview(ctx, 0, tenantID, accountID, profileIDs)
	if err != nil {
		return nil, err
	}
	result := &sysin.IntroFeeChannelPurgeModel{ProfileCount: len(profileIDs), JobCount: preview.JobCount, MessageCount: preview.MessageCount}
	if in.Preview {
		return result, nil
	}
	if err = s.disableCollectChannel(ctx, tenantID, accountID, in.BotId, channelID); err != nil {
		return nil, err
	}
	if err = s.supersedeCollectSourcePendingJobs(ctx, profileIDs, tenantID); err != nil {
		return nil, err
	}
	if len(profileIDs) > 0 {
		if err = s.enqueueCollectChannelPurge(ctx, collectChannelPurgeQueuePayload{TenantID: tenantID, AccountID: accountID, RuleID: in.RuleId, BotID: in.BotId, ChannelID: channelID}, 0); err != nil {
			return nil, gerror.Wrap(err, "投递频道下架任务失败")
		}
		result.Queued = 1
	} else if err = s.finishCollectChannelPurge(ctx, tenantID, accountID, in.RuleId, in.BotId, channelID); err != nil {
		return nil, err
	}
	s.refreshCollectSourceCache(ctx)
	return result, nil
}

func (s *sSysPublish) disableCollectChannel(ctx context.Context, tenantID, accountID, botID int64, channelID string) error {
	now := gtime.Now()
	var channels []struct {
		ID int64 `json:"id"`
	}
	loadChannels := func() error {
		channels = nil
		return pdao.YoubanPublishCollectChannel.Ctx(ctx).Fields("id").Where("tenant_id", tenantID).WhereIn("telegram_chat_id", tgChannelCacheLookupIds(channelID)).Scan(&channels)
	}
	if err := loadChannels(); err != nil {
		return gerror.Wrap(err, "读取采集频道失败")
	}
	if len(channels) == 0 {
		if err := s.upsertCollectChannel(ctx, tenantID, channelID, "channel", channelID, "", 0, 0, 0, now, now); err != nil {
			return gerror.Wrap(err, "创建禁用频道索引失败")
		}
		if err := loadChannels(); err != nil {
			return gerror.Wrap(err, "读取新建采集频道失败")
		}
	}
	channelIDs := make([]int64, 0, len(channels))
	for _, channel := range channels {
		channelIDs = append(channelIDs, channel.ID)
	}
	if len(channelIDs) > 0 {
		for _, id := range channelIDs {
			data := pdo.YoubanPublishCollectAccountChannel{
				TenantId: tenantID, AccountId: accountID, ChannelId: id, CollectEnabled: 0,
				Status: sysin.CollectAccountChannelDisabled, ApprovalStatus: "rejected",
				DisabledAt: now, DisabledReason: introFeeChannelPurgeMessage,
			}
			_, err := pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).Data(data).
				OnConflict("tenant_id,account_id,channel_id").
				OnDuplicate("collect_enabled,status,approval_status,disabled_at,disabled_reason,updated_at").Save()
			if err != nil {
				return gerror.Wrap(err, "禁用采集频道失败")
			}
		}
	}
	// Only channel-specific sources are disabled. Aggregate BOT sources must stay active.
	mod := g.DB().Model(publishCollectSourceTable).Ctx(ctx).Where("tenant_id", tenantID).Where("account_id", accountID).WhereIn("source_chat_id", tgChannelCacheLookupIds(channelID))
	if botID > 0 {
		mod = mod.Where("bot_id", botID)
	}
	_, err := mod.Data(g.Map{"channel_blacklisted": 1, "updated_at": now}).Update()
	return gerror.Wrap(err, "禁用频道采集源失败")
}

func (s *sSysPublish) ExecuteCollectChannelPurge(ctx context.Context, payload collectChannelPurgeQueuePayload) error {
	profileIDs, err := collectChannelProfileIDs(ctx, payload.TenantID, payload.AccountID, payload.RuleID, payload.ChannelID)
	if err != nil {
		return err
	}
	jobs, err := s.collectSourceDownJobs(ctx, profileIDs, payload.TenantID)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err = s.deleteTelegramMessageSet(ctx, job.telegramJobRecord(), introFeeChannelPurgeMessage); err != nil {
			g.Log().Warningf(ctx, "频道下架清理TG消息失败，继续删除资料 jobId:%d err:%v", job.Id, err)
		}
		_ = s.markTelegramJobSuperseded(ctx, job.Id)
	}
	if len(profileIDs) > 0 {
		if err = s.finishCollectSourceDownProfiles(ctx, profileIDs, payload.TenantID); err != nil {
			return err
		}
		_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where("tenant_id", payload.TenantID).Where("account_id", payload.AccountID).Where("rule_id", payload.RuleID).WhereIn("profile_id", profileIDs).Data(g.Map{
			"status": sysin.CollectDispatchStatusSkipped, "error_message": introFeeChannelPurgeMessage,
			"finished_at": gtime.Now(), "updated_at": gtime.Now(),
		}).Update()
		if err != nil {
			return gerror.Wrap(err, "更新频道采集分发状态失败")
		}
		for start := 0; start < len(profileIDs); start += collectSourceDownDeleteBatchSize {
			end := start + collectSourceDownDeleteBatchSize
			if end > len(profileIDs) {
				end = len(profileIDs)
			}
			if err = s.deleteProfiles(ctx, &sysin.ProfileDeleteInp{Ids: profileIDs[start:end]}, payload.TenantID, payload.AccountID); err != nil {
				return err
			}
		}
	}
	return s.finishCollectChannelPurge(ctx, payload.TenantID, payload.AccountID, payload.RuleID, payload.BotID, payload.ChannelID)
}

func (s *sSysPublish) finishCollectChannelPurge(ctx context.Context, tenantID, accountID, ruleID, botID int64, channelID string) error {
	now := gtime.Now()
	_, err := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("tenant_id", tenantID).WhereIn("account_id", []int64{accountID, 0}).Where("rule_id", ruleID).Where("bot_id", botID).WhereIn("channel_id", tgChannelCacheLookupIds(channelID)).WhereNull("deleted_at").Data(g.Map{"deleted_at": now, "updated_at": now}).Update()
	if err != nil {
		return gerror.Wrap(err, "删除频道后缀配置失败")
	}
	_, err = g.DB().Model(publishCollectChannelProfileTable).Ctx(ctx).Where("tenant_id", tenantID).Where("account_id", accountID).Where("rule_id", ruleID).WhereIn("channel_id", tgChannelCacheLookupIds(channelID)).Delete()
	return gerror.Wrap(err, "清空频道资料计数失败")
}
