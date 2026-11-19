package sys

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type telegramJobLifecycle string
type telegramSendPhaseState string
type telegramDispatchState string
type telegramJobSource string

const (
	telegramLifecyclePending    telegramJobLifecycle  = "pending"
	telegramLifecycleSending    telegramJobLifecycle  = "sending"
	telegramLifecycleRetryWait  telegramJobLifecycle  = "failed_retry"
	telegramLifecycleConfirming telegramJobLifecycle  = "unknown"
	telegramLifecycleUnknown    telegramJobLifecycle  = telegramLifecycleConfirming
	telegramLifecycleSent       telegramJobLifecycle  = "sent"
	telegramLifecycleFailed     telegramJobLifecycle  = "failed"
	telegramLifecycleSuperseded telegramJobLifecycle  = "superseded"
	telegramDispatchIdle        telegramDispatchState = "idle"
	telegramDispatchQueued      telegramDispatchState = "queued"
	telegramDispatchProcessing  telegramDispatchState = "processing"
	telegramDispatchDone        telegramDispatchState = "done"
	telegramSourceManual        telegramJobSource     = "manual"
	telegramSourceInteractive   telegramJobSource     = "interactive"
	telegramSourceCycle         telegramJobSource     = "cycle"
	telegramSourceFullPush      telegramJobSource     = "full_push"
	telegramSourceCollect       telegramJobSource     = "collect"
	telegramSourceHistory       telegramJobSource     = "history"
	telegramSourceCleanup       telegramJobSource     = "cleanup"
	telegramSourceUnknown       telegramJobSource     = "unknown"
)

func telegramLifecycleState(value string) telegramJobLifecycle {
	return telegramJobLifecycle(strings.TrimSpace(value))
}
func telegramDispatchStateOf(value string) telegramDispatchState {
	return telegramDispatchState(strings.TrimSpace(value))
}
func telegramJobLifecycleTerminal(value string) bool {
	switch telegramLifecycleState(value) {
	case telegramLifecycleSent, telegramLifecycleFailed, telegramLifecycleSuperseded:
		return true
	}
	return false
}
func telegramJobLifecycleOccupiesChannel(value string) bool {
	switch telegramLifecycleState(value) {
	case telegramLifecycleSending, telegramLifecycleConfirming:
		return true
	}
	return false
}
func telegramJobLifecycleCanBeOvertaken(value string) bool {
	switch telegramLifecycleState(value) {
	case telegramLifecyclePending, telegramLifecycleRetryWait:
		return true
	}
	return false
}
func telegramJobLifecycleNeedsRetry(value string) bool {
	switch telegramLifecycleState(value) {
	case telegramLifecyclePending, telegramLifecycleRetryWait:
		return true
	}
	return false
}
func telegramJobLifecycleNeedsReconcile(value string) bool {
	return telegramLifecycleState(value) == telegramLifecycleUnknown
}

func telegramJobSourceOf(operationNo string, priority int) telegramJobSource {
	value := strings.ToLower(strings.TrimSpace(operationNo))
	switch {
	case strings.HasPrefix(value, "collect:"):
		return telegramSourceCollect
	case strings.HasPrefix(value, "cycle_batch:"), strings.HasPrefix(value, "profile_cycle:"):
		return telegramSourceCycle
	case strings.HasPrefix(value, "full_push:"):
		return telegramSourceFullPush
	case strings.HasPrefix(value, "history:"):
		return telegramSourceHistory
	case strings.HasPrefix(value, "down:"), strings.HasPrefix(value, "delete:"):
		return telegramSourceCleanup
	case isInteractiveProfilePublishOperation(value):
		return telegramSourceInteractive
	case priority == tgJobPriorityInteractive:
		return telegramSourceManual
	default:
		return telegramSourceUnknown
	}
}

func telegramJobRetryableStatuses() []JobStatus {
	return []JobStatus{telegramJobStatusPending, telegramJobStatusRetryWait}
}

func telegramJobRecoverableStatuses() []JobStatus {
	return []JobStatus{telegramJobStatusPending, telegramJobStatusRetryWait, telegramJobStatusConfirming}
}

func telegramJobInFlightStatuses() []JobStatus {
	return []JobStatus{telegramJobStatusSending, telegramJobStatusConfirming}
}

// telegramJobActiveStatuses 是唯一的“任务仍占用资料/频道”状态集合。
// 所有循环、批次和手动发布的幂等检查都必须复用它，禁止自行拼接字符串。
func telegramJobActiveStatuses() []JobStatus {
	return []JobStatus{
		telegramJobStatusPending,
		telegramJobStatusSending,
		telegramJobStatusRetryWait,
		telegramJobStatusConfirming,
	}
}

func telegramJobReconcileStatuses() []JobStatus {
	return []JobStatus{telegramJobStatusConfirming}
}

func telegramJobTerminalStatuses() []JobStatus {
	return []JobStatus{telegramJobStatusSent, telegramJobStatusFailed, telegramJobStatusSuperseded}
}

const (
	telegramJobStatusPending    = "pending"
	telegramJobStatusSending    = "sending"
	telegramJobStatusRetryWait  = "failed_retry"
	telegramJobStatusConfirming = "unknown"
	telegramJobStatusSent       = "sent"
	telegramJobStatusFailed     = "failed"
	telegramJobStatusSuperseded = "superseded"
)

var telegramJobAllowedTransitions = map[JobStatus]map[JobStatus]struct{}{
	telegramJobStatusPending: {
		telegramJobStatusSending: {}, telegramJobStatusRetryWait: {}, telegramJobStatusConfirming: {}, telegramJobStatusSent: {},
		telegramJobStatusFailed: {}, telegramJobStatusSuperseded: {},
	},
	telegramJobStatusSending: {
		telegramJobStatusPending: {}, telegramJobStatusRetryWait: {}, telegramJobStatusConfirming: {},
		telegramJobStatusSent: {}, telegramJobStatusFailed: {}, telegramJobStatusSuperseded: {},
	},
	telegramJobStatusRetryWait: {
		telegramJobStatusPending: {}, telegramJobStatusSending: {}, telegramJobStatusRetryWait: {}, telegramJobStatusConfirming: {},
		telegramJobStatusSent: {}, telegramJobStatusFailed: {}, telegramJobStatusSuperseded: {},
	},
	telegramJobStatusConfirming: {
		telegramJobStatusSending: {}, telegramJobStatusRetryWait: {}, telegramJobStatusSent: {},
		telegramJobStatusFailed: {}, telegramJobStatusSuperseded: {},
	},
	telegramJobStatusFailed: {
		telegramJobStatusPending: {}, telegramJobStatusSending: {}, telegramJobStatusRetryWait: {},
		telegramJobStatusSent: {}, telegramJobStatusSuperseded: {},
	},
	telegramJobStatusSent: {
		telegramJobStatusSuperseded: {},
	},
}

type telegramJobTransition struct {
	JobId          int64
	From           []JobStatus
	To             JobStatus
	RetryDelay     time.Duration
	ErrorMessage   string
	Additional     g.Map
	SendPhases     []SendPhase
	DispatchStatus *DispatchStatus
	SendPhase      *SendPhase
	// Atomically fence a failed started send until its existing cleanup runs.
	// The SQL reads the persisted phase; callers cannot supply raw phase writes.
	CleanupStartedSend bool
	UpdatedBefore      *gtime.Time
	NotExpiredAt       *gtime.Time
}

func validateTelegramJobTransition(from []JobStatus, to JobStatus) error {
	to = JobStatus(strings.TrimSpace(to.String()))
	if to == "" || len(from) == 0 {
		return gerror.New("TG任务状态迁移参数不完整")
	}
	for _, source := range from {
		source = JobStatus(strings.TrimSpace(source.String()))
		if source == "" {
			return gerror.New("TG任务来源状态不能为空")
		}
		if _, ok := telegramJobAllowedTransitions[source][to]; !ok {
			return gerror.Newf("TG任务状态不允许从 %s 迁移到 %s", source, to)
		}
	}
	return nil
}

// transitionTelegramJob is the only write primitive for business status.
// Callers may still update lease/phase metadata directly when status is unchanged.
func (s *sSysPublish) transitionTelegramJob(ctx context.Context, in telegramJobTransition) (changed bool, resultErr error) {
	stage := beginTelegramPublishStage(ctx, "job.state_transition", g.Map{"jobId": in.JobId, "fromStatus": in.From, "toStatus": in.To, "requiredPhases": in.SendPhases})
	defer stage.Finish(&resultErr)
	// The result is logged at the write boundary, including CAS misses.
	if in.JobId <= 0 {
		return false, gerror.New("TG任务ID不能为空")
	}
	if err := validateTelegramJobTransition(in.From, in.To); err != nil {
		return false, err
	}
	from, data, err := telegramJobTransitionUpdate(in)
	if err != nil {
		return false, err
	}
	model := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", in.JobId).WhereIn("status", from)
	result, err := transitionTelegramJobModel(model, in, data)
	stage.End(err)
	if err != nil {
		return false, gerror.Wrapf(err, "TG任务状态迁移失败：%s -> %s", strings.Join(jobStatusStrings(from), ","), in.To)
	}
	affected, err := result.RowsAffected()
	beginTelegramPublishStage(ctx, "job.state_transition_result").End(err, g.Map{"jobId": in.JobId, "fromStatus": in.From, "toStatus": in.To, "affectedRows": affected})
	if err != nil {
		return false, gerror.Wrap(err, "确认TG任务状态迁移结果失败")
	}
	return affected > 0, nil
}

func (s *sSysPublish) transitionTelegramJobTx(ctx context.Context, tx gdb.TX, in telegramJobTransition) (bool, error) {
	if in.JobId <= 0 {
		return false, gerror.New("TG任务ID不能为空")
	}
	from, data, err := telegramJobTransitionUpdate(in)
	if err != nil {
		return false, err
	}
	model := tx.Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", in.JobId).WhereIn("status", from)
	result, err := transitionTelegramJobModel(model, in, data)
	if err != nil {
		return false, gerror.Wrapf(err, "事务内TG任务状态迁移失败：%s -> %s", strings.Join(jobStatusStrings(from), ","), in.To)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, gerror.Wrap(err, "确认事务内TG任务状态迁移结果失败")
	}
	return affected > 0, nil
}

func (s *sSysPublish) transitionTelegramJobsTx(ctx context.Context, tx gdb.TX, jobIds []int64, in telegramJobTransition) (int64, error) {
	jobIds = uniqueIds(jobIds)
	if len(jobIds) == 0 {
		return 0, nil
	}
	from, data, err := telegramJobTransitionUpdate(in)
	if err != nil {
		return 0, err
	}
	model := tx.Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("id", jobIds).WhereIn("status", from)
	result, err := transitionTelegramJobModel(model, in, data)
	if err != nil {
		return 0, gerror.Wrapf(err, "事务内批量TG任务状态迁移失败：%s -> %s", strings.Join(jobStatusStrings(from), ","), in.To)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, gerror.Wrap(err, "确认事务内批量TG任务状态迁移结果失败")
	}
	return affected, nil
}

func transitionTelegramJobModel(model *gdb.Model, in telegramJobTransition, data g.Map) (sql.Result, error) {
	if profilePublishCancelEnabled() && in.To != JobStatusSuperseded {
		model = model.Where("NOT EXISTS (SELECT 1 FROM " + publishProfileCancelTable + " pc WHERE pc.tenant_id=" + publishTgJobTable + ".tenant_id AND pc.account_id=" + publishTgJobTable + ".account_id AND pc.profile_id=" + publishTgJobTable + ".profile_id AND pc.operation_no=" + publishTgJobTable + ".operation_no)")
	}
	if len(in.SendPhases) > 0 {
		model = model.WhereIn("send_phase", in.SendPhases)
	}
	if in.UpdatedBefore != nil {
		model = model.WhereLTE("updated_at", in.UpdatedBefore)
	}
	if in.NotExpiredAt != nil {
		model = model.Where("send_deadline_at IS NULL OR send_deadline_at > ?", in.NotExpiredAt)
	}
	return model.Data(data).Update()
}

func (s *sSysPublish) transitionTelegramJobs(ctx context.Context, jobIds []int64, in telegramJobTransition) (int64, error) {
	jobIds = uniqueIds(jobIds)
	if len(jobIds) == 0 {
		return 0, nil
	}
	from, data, err := telegramJobTransitionUpdate(in)
	if err != nil {
		return 0, err
	}
	// 批量废弃任务也必须收口 Outbox、清理频道索引并唤醒下一条，
	// 不能只更新 tg_job 状态后留下 Redis 队首和 pending Outbox 垃圾。
	var targets []struct {
		Id           int64  `orm:"id"`
		TenantId     int64  `orm:"tenant_id"`
		AccountId    int64  `orm:"account_id"`
		ChannelId    int64  `orm:"channel_id"`
		TargetChatId string `orm:"target_chat_id"`
	}
	if strings.TrimSpace(in.To.String()) == telegramJobStatusSuperseded {
		if scanErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			WhereIn("id", jobIds).
			WhereIn("status", from).
			Fields("id,tenant_id,account_id,channel_id,target_chat_id").
			Scan(&targets); scanErr != nil {
			return 0, gerror.Wrap(scanErr, "读取批量废弃TG任务上下文失败")
		}
	}
	model := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("id", jobIds).WhereIn("status", from)
	result, err := transitionTelegramJobModel(model, in, data)
	if err != nil {
		return 0, gerror.Wrapf(err, "批量TG任务状态迁移失败：%s -> %s", strings.Join(jobStatusStrings(from), ","), in.To)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, gerror.Wrap(err, "确认批量TG任务状态迁移结果失败")
	}
	if affected == 0 || strings.TrimSpace(in.To.String()) != telegramJobStatusSuperseded {
		return affected, nil
	}
	// 状态迁移与后处理之间可能有并发 Worker 改写同一批 Job；
	// 只对当前仍为 superseded 的记录收口，避免误清理已经成功发送的任务。
	settledIDs := make([]int64, 0, len(targets))
	for _, target := range targets {
		settledIDs = append(settledIDs, target.Id)
	}
	var settledTargets []struct {
		Id           int64  `orm:"id"`
		TenantId     int64  `orm:"tenant_id"`
		AccountId    int64  `orm:"account_id"`
		ChannelId    int64  `orm:"channel_id"`
		TargetChatId string `orm:"target_chat_id"`
	}
	if scanErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("id", settledIDs).
		Where("status", telegramJobStatusSuperseded).
		Fields("id,tenant_id,account_id,channel_id,target_chat_id").
		Scan(&settledTargets); scanErr != nil {
		return affected, gerror.Wrap(scanErr, "确认批量废弃TG任务状态失败")
	}

	now := gtime.Now()
	channels := make(map[[3]int64]telegramJobRecord)
	for _, target := range settledTargets {
		// 已废弃 Job 的 Outbox 不得继续占用 pending/processing 状态，
		// 否则恢复扫描会反复读取一条永远不会发送的事件。
		if _, cleanupErr := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
			Where("job_id", target.Id).
			WhereIn("status", []string{"pending", "processing"}).
			Data(g.Map{
				"status":           "delivered",
				"lease_owner":      "",
				"lease_expires_at": nil,
				"delivered_at":     now,
				"last_error":       "TG任务已废弃",
				"updated_at":       now,
			}).Update(); cleanupErr != nil {
			return affected, gerror.Wrapf(cleanupErr, "收口废弃TG任务Outbox失败 jobId:%d", target.Id)
		}
		if target.ChannelId <= 0 {
			continue
		}
		if removeErr := (tgChannelPendingIndex{}).remove(ctx, target.TenantId, target.ChannelId, target.Id); removeErr != nil {
			g.Log().Warningf(ctx, "移除批量废弃TG任务频道索引失败 jobId:%d channelId:%d err:%+v", target.Id, target.ChannelId, removeErr)
		}
		key := [3]int64{target.TenantId, target.AccountId, target.ChannelId}
		if _, exists := channels[key]; !exists {
			channels[key] = telegramJobRecord{
				TenantId: target.TenantId, AccountId: target.AccountId,
				ChannelId: target.ChannelId, TargetChatId: target.TargetChatId,
			}
		}
	}
	for _, channelJob := range channels {
		if wakeErr := s.wakeNextTelegramChannelJob(ctx, channelJob); wakeErr != nil {
			return affected, gerror.Wrapf(wakeErr, "批量废弃TG任务后唤醒频道下一条任务失败 channelId:%d", channelJob.ChannelId)
		}
	}
	return affected, nil
}

// Preserve the destination order atomically with terminal failure until the
// existing watchdog has removed all messages and resolved outstanding attempts.
func telegramTerminalFailureCleanupFields() g.Map {
	const started = "send_phase IN ('display_sending','display_confirmed','verify_sending')"
	return g.Map{
		"send_phase":         gdb.Raw("CASE WHEN " + started + " THEN 'timeout_cancelled' ELSE send_phase END"),
		"timeout_cleanup_at": gdb.Raw("CASE WHEN " + started + " THEN CURRENT_TIMESTAMP ELSE timeout_cleanup_at END"),
	}
}

func telegramJobTransitionUpdate(in telegramJobTransition) ([]JobStatus, g.Map, error) {
	if err := validateTelegramJobTransition(in.From, in.To); err != nil {
		return nil, nil, err
	}
	for _, phase := range in.SendPhases {
		if !phase.Valid() {
			return nil, nil, gerror.New("TG来源发送阶段不合法")
		}
	}
	from := append([]JobStatus(nil), in.From...)
	for i := range from {
		from[i] = JobStatus(strings.TrimSpace(from[i].String()))
	}
	sort.Slice(from, func(i, j int) bool { return from[i] < from[j] })
	now := gtime.Now()
	data := telegramJobStateUpdateData(strings.TrimSpace(in.To.String()), in.RetryDelay, now)
	if in.ErrorMessage != "" || in.To == telegramJobStatusSent {
		data["error_message"] = strings.TrimSpace(in.ErrorMessage)
	}
	for field, value := range in.Additional {
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "status", "dispatch_status", "send_phase":
			return nil, nil, gerror.Newf("TG任务附加更新字段不能包含受保护状态列 %s", field)
		}
		data[field] = value
	}
	if in.DispatchStatus != nil {
		if !in.DispatchStatus.Valid() {
			return nil, nil, gerror.New("TG调度状态不合法")
		}
		data["dispatch_status"] = in.DispatchStatus.String()
	}
	if in.SendPhase != nil {
		if !in.SendPhase.Valid() {
			return nil, nil, gerror.New("TG发送阶段不合法")
		}
		data["send_phase"] = in.SendPhase.String()
	}
	if in.CleanupStartedSend {
		if in.To != JobStatusFailed || in.SendPhase != nil {
			return nil, nil, gerror.New("发送清理栅栏只能用于失败终态，且不能同时指定发送阶段")
		}
		for field, value := range telegramTerminalFailureCleanupFields() {
			data[field] = value
		}
	}
	dispatch, ok := data["dispatch_status"].(string)
	if !ok {
		return nil, nil, gerror.New("TG调度状态写入类型不合法")
	}
	if in.To.IsTerminal() && DispatchStatus(dispatch) != DispatchStatusDone {
		return nil, nil, gerror.New("TG终态任务必须完成调度")
	}
	// This summary is varchar(512), unlike the full error_message text field.
	// An oversized driver error must never prevent a sending job from settling.
	if message, ok := data["last_dispatch_error"].(string); ok {
		characters := []rune(message)
		if len(characters) > 512 {
			data["last_dispatch_error"] = string(characters[:509]) + "..."
		}
	}
	return from, data, nil
}
