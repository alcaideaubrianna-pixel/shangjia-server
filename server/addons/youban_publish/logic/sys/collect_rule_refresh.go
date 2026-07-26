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
			g.Log().Warningf(ctx, "刷新采集规则待推送任务失败 rule:%d tenant:%d account:%d err:%+v", ruleId, tenantId, accountId, err)
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
		Fields("id,event_id,task_id").
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("rule_id", ruleId).
		Where("status", sysin.CollectDispatchStatusPending).
		WhereGT("task_id", 0).
		OrderAsc("id").
		Limit(1000).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取待刷新采集分发失败")
	}
	for _, row := range rows {
		if err = s.refreshPendingCollectTaskForDispatch(ctx, row, rule); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) refreshCollectTaskBeforeTelegramSend(ctx context.Context, job telegramJobRecord) error {
	if job.Id <= 0 || job.TaskId <= 0 {
		return nil
	}
	dispatch, rule, err := s.collectDispatchRuleForTask(ctx, job.TaskId)
	if err != nil || dispatch.IsEmpty() {
		return err
	}
	return s.refreshPendingCollectTaskForDispatchMode(ctx, dispatch, rule, false, false)
}

func (s *sSysPublish) collectDispatchRuleForTask(ctx context.Context, taskId int64) (gdb.Record, gdb.Record, error) {
	dispatch, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("task_id", taskId).
		Where("status", sysin.CollectDispatchStatusPending).
		OrderDesc("id").
		Limit(1).
		One()
	if err != nil {
		return nil, nil, gerror.Wrap(err, "读取采集分发失败")
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
		return dispatch, nil, gerror.Wrap(err, "读取采集规则失败")
	}
	return dispatch, rule, nil
}

func (s *sSysPublish) refreshPendingCollectTaskForDispatch(ctx context.Context, dispatch gdb.Record, rule gdb.Record) error {
	return s.refreshPendingCollectTaskForDispatchMode(ctx, dispatch, rule, true, true)
}

func (s *sSysPublish) refreshPendingCollectTaskForDispatchMode(ctx context.Context, dispatch gdb.Record, rule gdb.Record, enqueueJobs bool, skipWhenSending bool) error {
	if dispatch.IsEmpty() || dispatch["task_id"].Int64() <= 0 || dispatch["event_id"].Int64() <= 0 {
		return nil
	}
	if rule.IsEmpty() {
		return s.skipPendingCollectDispatchAfterRuleRefresh(ctx, dispatch, nil, "规则已删除或已停用")
	}
	if skipWhenSending {
		sending, err := s.collectDispatchHasSendingJob(ctx, dispatch["task_id"].Int64())
		if err != nil {
			return err
		}
		if sending {
			s.appendCollectEventLog(ctx, dispatch["event_id"].Int64(), "rule", "refresh_skipped", "任务正在发送中，本次不刷新文案", "")
			return nil
		}
	}
	event, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("id", dispatch["event_id"].Int64()).
		Where("tenant_id", rule["tenant_id"].Int64()).
		Where("account_id", rule["account_id"].Int64()).
		One()
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
		return s.skipPendingCollectDispatchAfterRuleRefresh(ctx, dispatch, event, decision.Reason)
	}
	text := strings.TrimSpace(decision.Text)
	title := collectTitle(text)
	channelJSON := rule["target_channel_id_json"].String()
	now := gtime.Now()
	_, err = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", dispatch["task_id"].Int64()).
		WhereNull("deleted_at").
		Data(g.Map{
			"title":                     title,
			"plain_text":                text,
			"media_count":               content.MediaCount,
			"channel_id_json":           channelJSON,
			"collect_event_id":          event["id"].Int64(),
			"collect_source_id":         event["source_id"].Int64(),
			"collect_source_chat_id":    strings.TrimSpace(event["source_chat_id"].String()),
			"collect_source_message_id": event["source_message_id"].Int64(),
			"updated_at":                now,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "刷新采集任务文案失败")
	}
	_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("id", dispatch["id"].Int64()).
		Data(g.Map{
			"target_channel_id_json": channelJSON,
			"bot_id_json":            rule["bot_id_json"].String(),
			"match_json":             decision.MatchJSON,
			"error_message":          "",
			"updated_at":             now,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "刷新采集分发规则快照失败")
	}
	if err = s.refreshCollectTaskTelegramJobsAfterRuleChange(ctx, dispatch["task_id"].Int64(), rule, enqueueJobs); err != nil {
		return err
	}
	s.appendCollectEventLog(ctx, event["id"].Int64(), "rule", "refreshed", "已按最新规则刷新待推送文案", "")
	return nil
}

func (s *sSysPublish) refreshCollectTaskTelegramJobsAfterRuleChange(ctx context.Context, taskId int64, rule gdb.Record, enqueueJobs bool) error {
	if taskId <= 0 {
		return nil
	}
	if enqueueJobs {
		return s.ensureCollectTgJobs(ctx, taskId)
	}
	task, err := s.getPublishWorkflowTask(ctx, taskId, 0, 0)
	if err != nil {
		return err
	}
	channelIds := decodeInt64JSON(rule["target_channel_id_json"].String())
	if len(channelIds) == 0 {
		return s.supersedePendingCollectTaskJobs(ctx, taskId, "规则未配置目标频道")
	}
	return s.supersedeCollectTgJobsOutsideChannels(ctx, taskId, task["tg_operation_no"].String(), channelIds)
}

func (s *sSysPublish) collectDispatchHasSendingJob(ctx context.Context, taskId int64) (bool, error) {
	count, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		Where("status", "sending").
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查采集任务发送状态失败")
	}
	return count > 0, nil
}

func (s *sSysPublish) skipPendingCollectDispatchAfterRuleRefresh(ctx context.Context, dispatch gdb.Record, event gdb.Record, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "规则保存后不再匹配"
	}
	if err := s.supersedePendingCollectTaskJobs(ctx, dispatch["task_id"].Int64(), reason); err != nil {
		return err
	}
	now := gtime.Now()
	_, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("id", dispatch["id"].Int64()).
		Data(g.Map{
			"status":        sysin.CollectDispatchStatusSkipped,
			"error_message": reason,
			"finished_at":   now,
			"updated_at":    now,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新采集分发跳过状态失败")
	}
	_, _ = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", dispatch["task_id"].Int64()).
		Data(g.Map{
			"status":        sysin.PublishTaskStatusCanceled,
			"tg_status":     sysin.PublishTaskStatusCanceled,
			"error_message": reason,
			"updated_at":    now,
		}).
		Update()
	if err := s.refreshTaskNoteIndexes(ctx, []int64{dispatch["task_id"].Int64()}); err != nil {
		return err
	}
	eventId := dispatch["event_id"].Int64()
	if event != nil && !event.IsEmpty() {
		eventId = event["id"].Int64()
	}
	s.appendCollectEventLog(ctx, eventId, "rule", "skipped", "最新规则校验后任务已跳过："+reason, "")
	return nil
}

func (s *sSysPublish) supersedePendingCollectTaskJobs(ctx context.Context, taskId int64, reason string) error {
	if taskId <= 0 {
		return nil
	}
	var jobs []telegramResubmitJob
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		WhereIn("status", []string{"pending", "failed_retry"}).
		Scan(&jobs)
	if err != nil {
		return gerror.Wrap(err, "读取待废弃TG任务失败")
	}
	for _, job := range jobs {
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "publish", "superseded", "采集规则已修改，未发送任务已废弃："+reason)
		if err = s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			return err
		}
	}
	return nil
}
