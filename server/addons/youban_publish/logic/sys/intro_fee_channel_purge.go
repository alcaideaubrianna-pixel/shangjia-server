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
	"github.com/gogf/gf/v2/util/guid"

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
	jobs, err := s.collectChannelPurgeJobs(ctx, tenantID, accountID, profileIDs)
	if err != nil {
		return nil, err
	}
	jobIDs := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		jobIDs = append(jobIDs, job.Id)
	}
	pending, blocked, err := s.collectChannelPurgeRemainingMessages(ctx, tenantID, jobIDs)
	if err != nil {
		return nil, err
	}
	result := &sysin.IntroFeeChannelPurgeModel{ProfileCount: len(profileIDs), JobCount: len(jobs), MessageCount: pending + blocked}
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
		if err = s.enqueueCollectChannelPurge(ctx, collectChannelPurgeQueuePayload{OperationID: guid.S(), NotifyChatID: in.NotifyChatId, ProfileIDs: profileIDs, JobIDs: jobIDs, MessageCount: result.MessageCount, Title: channelTitle, Link: s.collectChannelDisplayLink(ctx, tenantID, channelID), Suffix: suffix, TenantID: tenantID, AccountID: accountID, RuleID: in.RuleId, BotID: in.BotId, ChannelID: channelID}, 0); err != nil {
			return nil, gerror.Wrap(err, "投递频道下架任务失败")
		}
		result.Queued = 1
	} else if err = s.finishCollectChannelPurge(ctx, tenantID, accountID, in.RuleId, in.BotId, channelID); err != nil {
		return nil, err
	}
	s.refreshCollectSourceCache(ctx)
	if err := s.enqueueCollectNotification(ctx, collectNotificationPayload{ID: "purge-start:" + guid.S(), BotID: in.BotId, Text: collectChannelPurgeNotifyText(channelTitle, channelID, suffix, result, s.collectChannelDisplayLink(ctx, tenantID, channelID))}); err != nil {
		return nil, gerror.Wrap(err, "删除任务已提交，但状态通知入队失败")
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
	// New tasks retain their original scope even after partial profile deletion.
	profileIDs := uniqueIds(payload.ProfileIDs)
	if payload.OperationID == "" {
		var err error
		profileIDs, err = collectChannelProfileIDs(ctx, payload.TenantID, payload.AccountID, payload.RuleID, payload.ChannelID)
		if err != nil {
			return err
		}
		payload.OperationID = fmt.Sprintf("legacy:%d:%d:%s", payload.TenantID, payload.AccountID, payload.ChannelID)
		payload.Title = s.collectChannelDisplayName(ctx, payload.TenantID, payload.ChannelID)
		payload.Link = s.collectChannelDisplayLink(ctx, payload.TenantID, payload.ChannelID)
		payload.Suffix, err = s.collectChannelPurgeSuffix(ctx, payload.TenantID, payload.AccountID, payload.RuleID, payload.BotID, payload.ChannelID)
		if err != nil {
			return err
		}
	}
	jobs, err := s.collectChannelPurgeJobs(ctx, payload.TenantID, payload.AccountID, profileIDs)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		payload.JobIDs = append(payload.JobIDs, job.Id)
	}
	payload.JobIDs = uniqueIds(payload.JobIDs)
	if payload.MessageCount == 0 {
		pending, blocked, err := s.collectChannelPurgeRemainingMessages(ctx, payload.TenantID, payload.JobIDs)
		if err != nil {
			return err
		}
		payload.MessageCount = pending + blocked
	}
	for _, job := range jobs {
		if err := s.deleteTelegramMessageSet(ctx, job.telegramJobRecord(), introFeeChannelPurgeMessage); err != nil {
			if queueErr := s.enqueueTelegramCleanupJob(ctx, job.Id, 0); queueErr != nil {
				return queueErr
			}
			g.Log().Warningf(ctx, "频道TG消息清理失败，已交由清理队列重试 jobId:%d err:%+v", job.Id, err)
		}
		if err := s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			return err
		}
	}
	if len(profileIDs) > 0 {
		// A retry must skip profiles already removed by a previous batch.
		model, err := s.profileBaseModel(ctx, payload.TenantID, payload.AccountID)
		if err != nil {
			return err
		}
		existingRows, err := model.Fields("p.id").WhereIn("p.id", profileIDs).All()
		if err != nil {
			return err
		}
		existingIDs := make([]int64, 0, len(existingRows))
		for _, row := range existingRows {
			existingIDs = append(existingIDs, row["id"].Int64())
		}
		if len(existingIDs) > 0 {
			if err := s.finishCollectSourceDownProfiles(ctx, existingIDs, payload.TenantID); err != nil {
				return err
			}
			for start := 0; start < len(existingIDs); start += collectSourceDownDeleteBatchSize {
				end := start + collectSourceDownDeleteBatchSize
				if end > len(existingIDs) {
					end = len(existingIDs)
				}
				if err := s.deleteProfiles(ctx, &sysin.ProfileDeleteInp{Ids: existingIDs[start:end]}, payload.TenantID, payload.AccountID); err != nil {
					return err
				}
			}
		}
		cols := pdao.YoubanPublishCollectDispatch.Columns()
		if _, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where(cols.TenantId, payload.TenantID).Where(cols.AccountId, payload.AccountID).
			WhereIn(cols.ProfileId, profileIDs).Data(pdo.YoubanPublishCollectDispatch{Status: sysin.CollectDispatchStatusSkipped, ErrorMessage: introFeeChannelPurgeMessage}).Update(); err != nil {
			return err
		}
	}
	if err := s.finishCollectChannelPurge(ctx, payload.TenantID, payload.AccountID, payload.RuleID, payload.BotID, payload.ChannelID); err != nil {
		return err
	}
	return s.enqueueCollectNotification(ctx, collectNotificationPayload{ID: "purge-completed:" + payload.OperationID, BotID: payload.BotID, ChatID: payload.NotifyChatID, TenantID: payload.TenantID, Purge: &collectChannelPurgeNotification{
		Title: payload.Title, ChannelID: payload.ChannelID, Link: payload.Link, Suffix: payload.Suffix,
		ProfileCount: len(profileIDs), MessageCount: payload.MessageCount, JobIDs: payload.JobIDs,
	}})
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
	return fmt.Sprintf("%s\n操作原因：管理员主动删除该频道的代理资料\n频道：%s\n频道 ID：%s%s\n原频道后缀：%s\n已删除代理资料：%d 条\n%s：%d 条\n已处理推送任务：%d 条\n频道采集：已禁用\n当前账户专属后缀和资料计数：已清理\n状态：%s",
		titleText, html.EscapeString(title), html.EscapeString(channelID), linkLine, html.EscapeString(suffix), profileCount, messageLabel, messageCount, jobCount, status)
}

func (s *sSysPublish) finishCollectChannelPurge(ctx context.Context, tenantID, accountID, ruleID, botID int64, channelID string) error {
	now := gtime.Now()
	_, err := g.DB().Model(introFeeChannelRuleTable).Ctx(ctx).Where("tenant_id", tenantID).Where("account_id", accountID).WhereIn("channel_id", tgChannelCacheLookupIds(channelID)).WhereNull("deleted_at").Data(g.Map{"deleted_at": now, "updated_at": now}).Update()
	if err != nil {
		return gerror.Wrap(err, "删除频道后缀配置失败")
	}
	_, err = g.DB().Model(publishCollectChannelProfileTable).Ctx(ctx).Where("tenant_id", tenantID).Where("account_id", accountID).WhereIn("channel_id", tgChannelCacheLookupIds(channelID)).Delete()
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

func (s *sSysPublish) collectChannelPurgeJobs(ctx context.Context, tenantID, accountID int64, profileIDs []int64) ([]telegramResubmitJob, error) {
	if len(profileIDs) == 0 {
		return nil, nil
	}
	var jobs []telegramResubmitJob
	err := g.DB().Model(publishTgJobTable).Ctx(ctx).Where("tenant_id", tenantID).Where("account_id", accountID).
		WhereIn("profile_id", profileIDs).OrderAsc("id").Scan(&jobs)
	return jobs, err
}

func (s *sSysPublish) collectChannelPurgeRemainingMessages(ctx context.Context, tenantID int64, jobIDs []int64) (pending, blocked int, err error) {
	if len(jobIDs) == 0 {
		return
	}
	var rows []struct {
		Status string
		Total  int
	}
	err = g.DB().Model(publishTgMessageTable+" m").Ctx(ctx).
		InnerJoin(publishTgJobTable+" j", "j.id=m.job_id").Fields("m.status,COUNT(*) AS total").
		Where("j.tenant_id", tenantID).WhereIn("m.job_id", jobIDs).WhereIn("m.status", []string{"sent", "undeletable"}).Group("m.status").Scan(&rows)
	for _, row := range rows {
		if row.Status == "sent" {
			pending = row.Total
		} else {
			blocked = row.Total
		}
	}
	return
}
