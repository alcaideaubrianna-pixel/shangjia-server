package sys

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"go.opentelemetry.io/otel/attribute"

	pdao "hotgo/addons/youban_publish/internal/dao"
	pdo "hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/collectobserve"
)

type collectDispatchResumeAction int

const (
	collectDispatchResumeInvalid collectDispatchResumeAction = iota
	collectDispatchResumeNoop
	collectDispatchResumeSubmit
)

func collectDispatchResumeActionForStatus(status string) collectDispatchResumeAction {
	switch strings.TrimSpace(status) {
	case sysin.CollectDispatchStatusFailed:
		return collectDispatchResumeSubmit
	case sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing, sysin.CollectDispatchStatusSent, sysin.CollectDispatchStatusSkipped:
		return collectDispatchResumeNoop
	default:
		return collectDispatchResumeInvalid
	}
}

func (s *sSysPublish) resumeCollectProfileDispatch(ctx context.Context, dispatch, event gdb.Record, content *collectContentResult, rule gdb.Record, text string) error {
	dispatchID := dispatch["id"].Int64()
	if dispatchID <= 0 || event.IsEmpty() || rule.IsEmpty() {
		return gerror.New("恢复采集分发参数不完整")
	}
	profileID, err := s.commitObservedCollectMaterial(ctx, event, content, rule, text)
	if err != nil {
		var duplicate *profileFingerprintDuplicateError
		if errors.As(err, &duplicate) {
			return err
		}
		_ = s.markCollectDispatchFailed(ctx, dispatchID, err.Error())
		return err
	}
	channelMap, err := collectDispatchChannelMap(ctx, []int64{dispatchID})
	if err != nil {
		return err
	}
	channelIDs := channelMap[dispatchID]
	if len(channelIDs) == 0 {
		return gerror.New("采集规则未配置目标频道")
	}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, txErr := tx.Model(pdao.YoubanPublishCollectDispatch.Table()).Ctx(ctx).
			Where("id", dispatchID).
			Data(g.Map{
				"profile_id": profileID, "status": sysin.CollectDispatchStatusPending,
				"error_message": "", "finished_at": nil, "updated_at": gtime.Now(),
			}).Update(); txErr != nil {
			return gerror.Wrap(txErr, "恢复采集分发状态失败")
		}
		jobIDValues, txErr := tx.Model(publishTgJobTable).Ctx(ctx).
			Fields("id").
			Where("profile_id", profileID).
			Where("operation_no", fmt.Sprintf("collect:%d", dispatchID)).
			Where("status", "failed").
			Array()
		if txErr != nil {
			return gerror.Wrap(txErr, "读取待恢复采集TG任务失败")
		}
		jobIDs := make([]int64, 0, len(jobIDValues))
		for _, value := range jobIDValues {
			jobIDs = append(jobIDs, value.Int64())
		}
		if _, txErr := s.transitionTelegramJobsTx(ctx, tx, jobIDs, telegramJobTransition{
			From: []JobStatus{telegramJobStatusFailed},
			To:   telegramJobStatusPending,
			Additional: g.Map{
				"retry_count": 0, "next_retry_at": nil, "error_message": "", "last_dispatch_error": "",
			},
		}); txErr != nil {
			return gerror.Wrap(txErr, "恢复采集TG发送任务失败")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err = s.submitCollectProfileDispatch(ctx, dispatchID, profileID, event); err != nil {
		return err
	}
	g.Log().Infof(ctx, "采集分发已恢复并重新提交 profileId:%d eventId:%d dispatchId:%d", profileID, event["id"].Int64(), dispatchID)
	return nil
}

func (s *sSysPublish) skipCollectProfileDispatchDuplicate(ctx context.Context, dispatchID int64, event gdb.Record, content *collectContentResult, rule gdb.Record, duplicate *profileFingerprintDuplicateError) (string, error) {
	if dispatchID <= 0 || event.IsEmpty() || rule.IsEmpty() || duplicate == nil {
		return "", gerror.New("跳过重复采集分发参数不完整")
	}
	filled, err := s.fillDuplicateCollectVerification(ctx, event, rule, content, duplicate)
	if err != nil {
		return "", err
	}
	replaced := false
	if !filled {
		replaced, err = s.replaceDuplicateCollectProfileMedia(ctx, event, content, duplicate.ProfileID)
	}
	if err != nil {
		return "", err
	}
	reason := collectDuplicateReason(duplicate, replaced)
	if filled {
		reason += "，已补全缺失验证媒体并提交发布"
	}
	if _, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where("id", dispatchID).Data(g.Map{
		"status": sysin.CollectDispatchStatusSkipped, "error_message": reason,
		"finished_at": gtime.Now(), "updated_at": gtime.Now(),
	}).Update(); err != nil {
		return "", gerror.Wrap(err, "更新重复采集分发状态失败")
	}
	s.appendCollectEventLogForRecord(ctx, event, "dedupe", "skipped", reason, fmt.Sprintf("rule=%d", rule["id"].Int64()))
	g.Log().Infof(ctx, "采集资料指纹命中，已跳过分发 eventId:%d ruleId:%d dispatchId:%d profileId:%d channelId:%d layer:%s cacheHit:%t mediaReplaced:%t", event["id"].Int64(), rule["id"].Int64(), dispatchID, duplicate.ProfileID, duplicate.ChannelID, duplicate.Layer, duplicate.CacheHit, replaced)
	return reason, nil
}

func (s *sSysPublish) submitCollectProfileDispatch(ctx context.Context, dispatchId, profileId int64, event gdb.Record) (submitErr error) {
	ctx = collectBusinessEventContext(ctx, event, nil, 0)
	ctx = collectobserve.With(ctx, collectobserve.Metadata{ProfileID: profileId})
	ctx, finish := startCollectPipelineStage(ctx, collectPipelineDispatch)
	outcome := collectPipelineUnconfirmed
	defer func() {
		// A nil business return is not a Telegram or queue receipt.
		finish(collectBusinessStepResult(submitErr, outcome), submitErr,
			attribute.Int64("collect.rule_id", collectobserve.From(ctx).RuleID),
			attribute.Int64("collect.dispatch_id", dispatchId), attribute.Int64("profile.id", profileId))
	}()
	if dispatchId <= 0 || profileId <= 0 || event.IsEmpty() {
		return gerror.New("采集分发参数不完整")
	}
	// Existing pending dispatches can be resumed without the raw-message or
	// event-processing ingress. They must pass the same actual-chat boundary.
	ctx, blocked, guardErr := s.checkCollectEventPublishTarget(ctx, event)
	if guardErr != nil {
		return guardErr
	}
	if blocked {
		outcome = collectPipelineIgnored
		cols := pdao.YoubanPublishCollectDispatch.Columns()
		_, updateErr := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
			Where(cols.Id, dispatchId).Where(cols.TenantId, event["tenant_id"].Int64()).
			Where(cols.SourceId, event["source_id"].Int64()).
			Data(pdo.YoubanPublishCollectDispatch{Status: sysin.CollectDispatchStatusSkipped, ErrorMessage: "采集来源是发布目标频道，禁止回流分发"}).Update()
		return gerror.Wrap(updateErr, "终止发布频道回流分发失败")
	}
	channelMap, err := collectDispatchChannelMap(ctx, []int64{dispatchId})
	if err != nil {
		return err
	}
	channelIds := channelMap[dispatchId]
	if len(channelIds) == 0 {
		return gerror.New("采集规则未配置目标频道")
	}
	operationNo := fmt.Sprintf("collect:%d", dispatchId)
	if _, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where("id", dispatchId).Data(g.Map{
		"profile_id": profileId,
		"status":     sysin.CollectDispatchStatusPending, "error_message": "", "updated_at": gtime.Now(),
	}).Update(); err != nil {
		return gerror.Wrap(err, "更新采集分发资料失败")
	}
	dispatch, queryErr := pdao.YoubanPublishCollectDispatch.Ctx(ctx).Fields("rule_id,source_id").Where("id", dispatchId).One()
	if queryErr != nil {
		return gerror.Wrap(queryErr, "读取采集分发索引参数失败")
	}
	ctx = collectobserve.With(ctx, collectobserve.Metadata{RuleID: dispatch["rule_id"].Int64()})
	if disabled, gateErr := collectRuleChannelDisabled(ctx, event["tenant_id"].Int64(), event["account_id"].Int64(), dispatch["rule_id"].Int64(), event["bot_id"].Int64(), event["source_chat_id"].String()); gateErr != nil {
		return gateErr
	} else if disabled {
		outcome = collectPipelineIgnored
		_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where("id", dispatchId).Data(g.Map{"status": sysin.CollectDispatchStatusSkipped, "error_message": "当前规则的来源频道已下架禁用"}).Update()
		return err
	}
	if err = indexCollectChannelProfile(ctx, event["tenant_id"].Int64(), event["account_id"].Int64(), dispatch["source_id"].Int64(), dispatch["rule_id"].Int64(), profileId, event["source_chat_id"].String()); err != nil {
		return err
	}
	meta := telegramProfilePublishMeta{
		CollectEventId:         event["id"].Int64(),
		CollectSourceId:        event["source_id"].Int64(),
		CollectSourceChatId:    strings.TrimSpace(event["source_chat_id"].String()),
		CollectSourceMessageId: event["source_message_id"].Int64(),
	}
	if s.accountPushEngine(ctx, event["tenant_id"].Int64(), event["account_id"].Int64()) == pushEngineUnifiedStream {
		history := false
		if cache.Initialized() {
			if value, historyErr := cache.Instance().Get(ctx, collectProcessHistoryMarkerKey(event["id"].Int64())); historyErr == nil && !value.IsNil() {
				history = value.Int() == 1
			}
		}
		if history {
			meta.InitialPriority = tgJobPriorityHistory
		}
	}
	if err := s.submitProfilePublishWithMeta(ctx, profileId, event["tenant_id"].Int64(), event["account_id"].Int64(), event["account_id"].Int64(), operationNo, channelIds, false, meta); err != nil {
		if errors.Is(err, errTelegramPublishPermissionPaused) {
			outcome = collectPipelineIgnored
			_, updateErr := pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where("id", dispatchId).Data(g.Map{
				"status": sysin.CollectDispatchStatusSkipped, "error_message": err.Error(), "finished_at": gtime.Now(),
			}).Update()
			if updateErr != nil {
				return gerror.Wrap(updateErr, "记录采集分发权限暂停失败")
			}
			s.appendCollectEventLogForRecord(ctx, event, "dispatch", "skipped", err.Error(), fmt.Sprintf("profileId=%d", profileId))
			return nil
		}
		_ = s.markCollectDispatchFailed(ctx, dispatchId, err.Error())
		return gerror.Wrap(err, "创建采集TG发送任务失败")
	}
	return nil
}

// reconcileCollectOperationDispatch uses only an already-known operation ID.
// It does not discover work or scan historical dispatches. The existing channel
// manifest predicate remains the authority for acknowledging delivery.
func (s *sSysPublish) reconcileCollectOperationDispatch(ctx context.Context, job telegramJobRecord) error {
	if !strings.HasPrefix(job.OperationNo, "collect:") {
		return nil
	}
	dispatchID, err := strconv.ParseInt(strings.TrimPrefix(job.OperationNo, "collect:"), 10, 64)
	if err != nil || dispatchID <= 0 {
		return gerror.New("采集分发操作编号无效")
	}
	row, err := g.DB().Model(publishCollectDispatchTable).Ctx(ctx).
		Fields("id,event_id").Where("id", dispatchID).
		Where("tenant_id", job.TenantId).Where("profile_id", job.ProfileId).
		WhereIn("account_id", []int64{0, job.AccountId}).One()
	if err != nil {
		return gerror.Wrap(err, "读取采集操作收尾范围失败")
	}
	if row.IsEmpty() {
		return nil // A stale operation must never acknowledge another scope.
	}
	return s.markCollectDispatchSentByProfile(ctx, job.ProfileId, row["event_id"].Int64(), dispatchID)
}

func (s *sSysPublish) markCollectDispatchSentByProfile(ctx context.Context, profileId, eventId int64, dispatchIDs ...int64) error {
	if profileId <= 0 || eventId <= 0 {
		return nil
	}
	scope := func(model *gdb.Model) *gdb.Model {
		model = model.Where("profile_id", profileId).Where("event_id", eventId)
		if len(dispatchIDs) > 0 {
			model = model.WhereIn("id", dispatchIDs)
		}
		return model
	}
	// Include already-sent rows: a previous call may have committed dispatch
	// before event finalization failed. Retrying must still repair that event.
	rows, err := scope(pdao.YoubanPublishCollectDispatch.Ctx(ctx)).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing, sysin.CollectDispatchStatusFailed, sysin.CollectDispatchStatusSent}).
		Fields("id,event_id").All()
	if err != nil {
		return gerror.Wrap(err, "读取采集分发事件失败")
	}
	if len(rows) == 0 {
		return nil
	}
	_, err = scope(pdao.YoubanPublishCollectDispatch.Ctx(ctx)).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing, sysin.CollectDispatchStatusFailed}).
		Where(collectDispatchAllJobsSentCondition(pdao.YoubanPublishCollectDispatch.Table())).
		Data(g.Map{"status": sysin.CollectDispatchStatusSent, "error_message": "", "finished_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新采集分发发送状态失败")
	}
	for _, row := range rows {
		_, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", row["event_id"].Int64()).
			WhereIn("status", []string{sysin.CollectEventStatusDispatched, sysin.CollectEventStatusFailed}).
			Where(collectEventAllDispatchesFinishedCondition(pdao.YoubanPublishCollectEvent.Table())).
			Data(g.Map{"status": sysin.CollectEventStatusProcessed, "error_message": "", "processed_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
		if err != nil {
			return gerror.Wrap(err, "更新采集事件完成状态失败")
		}
		s.syncCollectSourceStatsForEvent(ctx, row["event_id"].Int64())
	}
	return nil
}

func (s *sSysPublish) markCollectDispatchFailedByProfile(ctx context.Context, profileId, eventId int64, message string) error {
	if profileId <= 0 || eventId <= 0 {
		return nil
	}
	rows, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields("id,event_id").Where("profile_id", profileId).Where("event_id", eventId).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing}).All()
	if err != nil {
		return gerror.Wrap(err, "读取采集失败分发事件失败")
	}
	_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("profile_id", profileId).Where("event_id", eventId).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing}).
		Data(g.Map{"status": sysin.CollectDispatchStatusFailed, "error_message": message, "finished_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新采集分发失败状态失败")
	}
	if err = s.markCollectEventsFailedByDispatchRows(ctx, rows, message); err != nil {
		return err
	}
	return nil
}
