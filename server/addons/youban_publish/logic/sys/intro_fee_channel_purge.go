package sys

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	botService "hotgo/addons/youban_bot/service"
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
	channelTitle := s.collectChannelDisplayName(ctx, tenantID, channelID)
	suffix, err := s.collectChannelPurgeSuffix(ctx, tenantID, accountID, in.RuleId, in.BotId, channelID)
	if err != nil {
		return nil, err
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
	if notifyErr := botService.SysBot().NotifySuperAdmins(ctx, in.BotId, "collect_source", collectChannelPurgeNotifyText(channelTitle, channelID, suffix, result, s.collectChannelDisplayLink(ctx, tenantID, channelID))); notifyErr != nil {
		g.Log().Warningf(ctx, "发送频道删除下架BOT通知失败 tenantId:%d accountId:%d ruleId:%d channelId:%s err:%+v", tenantID, accountID, in.RuleId, channelID, notifyErr)
	}
	return result, nil
}

func (s *sSysPublish) collectChannelDisplayName(ctx context.Context, tenantID int64, channelID string) string {
	var channel struct {
		Title string `json:"title"`
	}
	_ = pdao.YoubanPublishCollectChannel.Ctx(ctx).Fields(pdao.YoubanPublishCollectChannel.Columns().Title).
		Where(pdao.YoubanPublishCollectChannel.Columns().TenantId, tenantID).
		WhereIn(pdao.YoubanPublishCollectChannel.Columns().TelegramChatId, tgChannelCacheLookupIds(channelID)).
		OrderDesc(pdao.YoubanPublishCollectChannel.Columns().Id).Limit(1).Scan(&channel)
	return strings.TrimSpace(channel.Title)
}

func collectChannelPurgeNotifyText(title, channelID, suffix string, result *sysin.IntroFeeChannelPurgeModel, links ...string) string {
	if result != nil && result.Queued == 0 {
		return collectChannelPurgeCompletedNotifyText(title, channelID, suffix, result.ProfileCount, result.MessageCount, result.JobCount, 0, links...)
	}
	linkLine := ""
	if len(links) > 0 {
		linkLine = collectChannelLinkLine(links[0])
	}
	if strings.TrimSpace(title) == "" {
		title = channelID
	}
	status := "已完成；频道采集已禁用，后缀和计数已清理，无关联资料或已推送消息需要删除"
	if result != nil && result.Queued == 1 {
		status = "任务已提交；频道采集已禁用，后台正在删除上述资料及其已推送消息，完成后会再次通知"
	}
	profileCount, messageCount := 0, 0
	if result != nil {
		profileCount, messageCount = result.ProfileCount, result.MessageCount
	}
	return fmt.Sprintf("删除频道代理资料并下架\n操作原因：管理员主动删除该频道的代理资料\n频道：%s\n频道 ID：%s%s\n频道后缀：%s\n关联代理资料：%d 条\n已推送消息：%d 条\n状态：%s",
		html.EscapeString(title), html.EscapeString(channelID), linkLine, html.EscapeString(suffix), profileCount, messageCount, status)
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
	channelTitle := s.collectChannelDisplayName(ctx, payload.TenantID, payload.ChannelID)
	profileIDs, err := collectChannelProfileIDs(ctx, payload.TenantID, payload.AccountID, payload.RuleID, payload.ChannelID)
	if err != nil {
		return err
	}
	suffix, err := s.collectChannelPurgeSuffix(ctx, payload.TenantID, payload.AccountID, payload.RuleID, payload.BotID, payload.ChannelID)
	if err != nil {
		return err
	}
	preview, err := s.collectSourceDownPreview(ctx, 0, payload.TenantID, payload.AccountID, profileIDs)
	if err != nil {
		return err
	}
	jobs, err := s.collectSourceDownJobs(ctx, profileIDs, payload.TenantID)
	if err != nil {
		return err
	}
	failedJobs := 0
	for _, job := range jobs {
		if err = s.deleteTelegramMessageSet(ctx, job.telegramJobRecord(), introFeeChannelPurgeMessage); err != nil {
			failedJobs++
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
	if err = s.finishCollectChannelPurge(ctx, payload.TenantID, payload.AccountID, payload.RuleID, payload.BotID, payload.ChannelID); err != nil {
		return err
	}
	if notifyErr := botService.SysBot().NotifySuperAdmins(ctx, payload.BotID, "collect_source", collectChannelPurgeCompletedNotifyText(channelTitle, payload.ChannelID, suffix, len(profileIDs), preview.MessageCount, len(jobs), failedJobs, s.collectChannelDisplayLink(ctx, payload.TenantID, payload.ChannelID))); notifyErr != nil {
		g.Log().Warningf(ctx, "发送频道删除下架完成BOT通知失败 tenantId:%d accountId:%d ruleId:%d channelId:%s err:%+v", payload.TenantID, payload.AccountID, payload.RuleID, payload.ChannelID, notifyErr)
	}
	return nil
}

func collectChannelPurgeCompletedNotifyText(title, channelID, suffix string, profileCount, messageCount, jobCount, failedJobs int, links ...string) string {
	linkLine := ""
	if len(links) > 0 {
		linkLine = collectChannelLinkLine(links[0])
	}
	if strings.TrimSpace(title) == "" {
		title = channelID
	}
	status := "删除并下架成功"
	if failedJobs > 0 {
		status = fmt.Sprintf("资料删除已完成；%d 个任务的 Telegram 消息清理失败，需进一步处理", failedJobs)
	}
	titleText := "频道代理资料删除成功"
	messageLabel := "已清理推送消息"
	if failedJobs > 0 {
		titleText = "频道代理资料删除完成（消息清理异常）"
		messageLabel = "关联已推送消息"
	}
	return fmt.Sprintf("%s\n操作原因：管理员主动删除该频道的代理资料\n频道：%s\n频道 ID：%s%s\n原频道后缀：%s\n已删除代理资料：%d 条\n%s：%d 条\n已处理推送任务：%d 条\n频道采集：已禁用\n频道后缀和资料计数：已清理\n状态：%s",
		titleText, html.EscapeString(title), html.EscapeString(channelID), linkLine, html.EscapeString(suffix), profileCount, messageLabel, messageCount, jobCount, status)
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

func (s *sSysPublish) collectChannelPurgeSuffix(ctx context.Context, tenantID, accountID, ruleID, botID int64, channelID string) (string, error) {
	rule, err := pdao.YoubanPublishCollectRule.Ctx(ctx).WherePri(ruleID).Where(pdao.YoubanPublishCollectRule.Columns().TenantId, tenantID).One()
	if err != nil {
		return "", gerror.Wrap(err, "读取频道后缀失败")
	}
	event := gdb.Record{"tenant_id": gvar.New(tenantID), "account_id": gvar.New(accountID), "bot_id": gvar.New(botID), "source_chat_id": gvar.New(channelID)}
	suffix, found, err := s.resolveIntroFeeChannelSuffix(ctx, event, rule)
	if err != nil {
		return "", err
	}
	if !found {
		suffix = strings.TrimSpace(rule["intro_fee_suffix"].String())
	}
	if suffix == "" {
		suffix = "未配置"
	}
	return suffix, nil
}

func collectChannelPublicLink(username, inviteLink string) string {
	if username = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(username), "@")); username != "" {
		return "https://t.me/" + username
	}
	return strings.TrimSpace(inviteLink)
}

func collectChannelLinkLine(link string) string {
	if strings.TrimSpace(link) == "" {
		return ""
	}
	return "\n频道链接：" + html.EscapeString(strings.TrimSpace(link))
}

func (s *sSysPublish) collectChannelDisplayLink(ctx context.Context, tenantID int64, channelID string) string {
	cols := pdao.YoubanPublishCollectChannel.Columns()
	var channel struct {
		Username   string
		InviteLink string
	}
	if err := pdao.YoubanPublishCollectChannel.Ctx(ctx).Fields(cols.Username, cols.InviteLink).
		Where(cols.TenantId, tenantID).WhereIn(cols.TelegramChatId, tgChannelCacheLookupIds(channelID)).
		OrderDesc(cols.Id).Limit(1).Scan(&channel); err != nil {
		g.Log().Warningf(ctx, "读取通知频道链接失败 tenantId:%d channelId:%s err:%+v", tenantID, channelID, err)
	}
	return collectChannelPublicLink(channel.Username, channel.InviteLink)
}
