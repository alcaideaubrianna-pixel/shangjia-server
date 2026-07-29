package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) refreshPendingCollectTasksForRuleAsync(ruleId int64, tenantId int64, accountId int64) {
	if ruleId <= 0 || tenantId <= 0 || accountId <= 0 {
		return
	}
	go func() {
		ctx := context.Background()
		if err := s.refreshPendingCollectTasksForRule(ctx, ruleId, tenantId, accountId); err != nil {
			g.Log().Warningf(ctx, "刷新采集规则待推送资料失败 rule:%d tenant:%d account:%d err:%+v", ruleId, tenantId, accountId, err)
		}
	}()
}

func (s *sSysPublish) refreshPendingCollectTasksForRule(ctx context.Context, ruleId int64, tenantId int64, accountId int64) error {
	if ruleId <= 0 || tenantId <= 0 || accountId <= 0 {
		return nil
	}
	rule, err := pdao.YoubanPublishCollectRule.Ctx(ctx).
		Where("id", ruleId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return gerror.Wrap(err, "读取采集规则失败")
	}
	if rule.IsEmpty() {
		return nil
	}
	rows, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("rule_id", ruleId).
		Where("status", sysin.CollectDispatchStatusPending).
		WhereGT("profile_id", 0).
		OrderAsc("id").
		Limit(1000).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取待刷新采集分发失败")
	}
	for _, row := range rows {
		if err = s.refreshCollectProfileDispatch(ctx, row, rule, true, true); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) refreshCollectProfileBeforeTelegramSend(ctx context.Context, job telegramJobRecord) error {
	if job.ProfileId <= 0 || job.CollectEventId <= 0 {
		return nil
	}
	dispatch, rule, err := s.collectDispatchRuleForProfile(ctx, job.ProfileId, job.CollectEventId)
	if err != nil || dispatch.IsEmpty() {
		return err
	}
	return s.refreshCollectProfileDispatch(ctx, dispatch, rule, true, false)
}

func (s *sSysPublish) collectDispatchRuleForProfile(ctx context.Context, profileId, eventId int64) (gdb.Record, gdb.Record, error) {
	dispatch, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("profile_id", profileId).
		Where("event_id", eventId).
		Where("status", sysin.CollectDispatchStatusPending).
		OrderDesc("id").
		Limit(1).
		One()
	if err != nil {
		return nil, nil, gerror.Wrap(err, "读取采集资料分发失败")
	}
	if dispatch.IsEmpty() {
		return dispatch, nil, nil
	}
	rule, err := pdao.YoubanPublishCollectRule.Ctx(ctx).
		Where("id", dispatch["rule_id"].Int64()).
		Where("tenant_id", dispatch["tenant_id"].Int64()).
		Where("account_id", dispatch["account_id"].Int64()).
		Where("status", 1).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return dispatch, nil, gerror.Wrap(err, "读取采集资料规则失败")
	}
	return dispatch, rule, nil
}

func (s *sSysPublish) refreshCollectProfileDispatch(ctx context.Context, dispatch gdb.Record, rule gdb.Record, enqueueJobs, skipWhenSending bool) error {
	if dispatch.IsEmpty() || dispatch["profile_id"].Int64() <= 0 || dispatch["event_id"].Int64() <= 0 {
		return nil
	}
	if rule.IsEmpty() {
		return s.skipCollectProfileDispatch(ctx, dispatch, "规则已删除或已停用")
	}
	if skipWhenSending {
		count, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("profile_id", dispatch["profile_id"].Int64()).
			Where("collect_event_id", dispatch["event_id"].Int64()).
			Where("status", "sending").
			Count()
		if err != nil {
			return gerror.Wrap(err, "检查采集资料发送状态失败")
		}
		if count > 0 {
			return nil
		}
	}
	event, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", dispatch["event_id"].Int64()).One()
	if err != nil {
		return gerror.Wrap(err, "读取采集事件失败")
	}
	if event.IsEmpty() {
		return nil
	}
	content, err := s.collectContentSnapshot(ctx, event)
	if err != nil {
		return err
	}
	decision, err := s.evaluateCollectRule(ctx, event, content, rule)
	if err != nil {
		return err
	}
	if decision.Skipped || !decision.Matched {
		return s.skipCollectProfileDispatch(ctx, dispatch, decision.Reason)
	}
	profileId, err := s.upsertCollectProfile(ctx, event, content, rule, strings.TrimSpace(decision.Text))
	if err != nil {
		return err
	}
	now := gtime.Now()
	_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where("id", dispatch["id"].Int64()).Data(g.Map{
		"profile_id": profileId, "target_channel_id_json": rule["target_channel_id_json"].String(),
		"bot_id_json": rule["bot_id_json"].String(), "match_json": decision.MatchJSON, "error_message": "", "updated_at": now,
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "刷新采集资料分发规则失败")
	}
	if err = s.supersedeRemovedCollectProfileJobs(ctx, dispatch["profile_id"].Int64(), dispatch["event_id"].Int64(), decodeInt64JSON(rule["target_channel_id_json"].String())); err != nil {
		return err
	}
	if enqueueJobs {
		return s.submitCollectProfileDispatch(ctx, dispatch["id"].Int64(), profileId, event, rule)
	}
	return nil
}

func (s *sSysPublish) supersedeRemovedCollectProfileJobs(ctx context.Context, profileId, eventId int64, channelIds []int64) error {
	if profileId <= 0 || eventId <= 0 {
		return nil
	}
	allowed := make(map[int64]struct{}, len(channelIds))
	for _, channelId := range uniqueIds(channelIds) {
		allowed[channelId] = struct{}{}
	}
	var jobs []telegramResubmitJob
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).
		Where("collect_event_id", eventId).
		WhereIn("status", []string{"pending", "failed_retry"}).
		OrderAsc("id").Scan(&jobs); err != nil {
		return gerror.Wrap(err, "读取采集资料待发送任务失败")
	}
	for _, job := range jobs {
		if _, ok := allowed[job.ChannelId]; ok {
			continue
		}
		if err := s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			return gerror.Wrap(err, "废弃已移除频道的采集任务失败")
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "publish", "superseded", "采集规则已修改，目标频道已移除")
	}
	return nil
}

func (s *sSysPublish) skipCollectProfileDispatch(ctx context.Context, dispatch gdb.Record, reason string) error {
	if strings.TrimSpace(reason) == "" {
		reason = "规则保存后不再匹配"
	}
	var pending []telegramResubmitJob
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("profile_id", dispatch["profile_id"].Int64()).
		Where("collect_event_id", dispatch["event_id"].Int64()).
		WhereIn("status", []string{"pending", "failed_retry"}).
		Scan(&pending)
	if err == nil {
		for _, job := range pending {
			_ = s.markTelegramJobSuperseded(ctx, job.Id)
		}
	}
	_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where("id", dispatch["id"].Int64()).Data(g.Map{
		"status": sysin.CollectDispatchStatusSkipped, "error_message": reason, "finished_at": gtime.Now(), "updated_at": gtime.Now(),
	}).Update()
	return gerror.Wrap(err, "更新采集资料跳过状态失败")
}
