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

const (
	telegramJobStatusPending    = "pending"
	telegramJobStatusSending    = "sending"
	telegramJobStatusRetryWait  = "failed_retry"
	telegramJobStatusConfirming = "unknown"
	telegramJobStatusSent       = "sent"
	telegramJobStatusFailed     = "failed"
	telegramJobStatusSuperseded = "superseded"
)

var telegramJobAllowedTransitions = map[string]map[string]struct{}{
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
	JobId         int64
	From          []string
	To            string
	RetryDelay    time.Duration
	ErrorMessage  string
	Additional    g.Map
	SendPhases    []string
	UpdatedBefore *gtime.Time
	NotExpiredAt  *gtime.Time
}

func validateTelegramJobTransition(from []string, to string) error {
	to = strings.TrimSpace(to)
	if to == "" || len(from) == 0 {
		return gerror.New("TG任务状态迁移参数不完整")
	}
	for _, source := range from {
		source = strings.TrimSpace(source)
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
		return false, gerror.Wrapf(err, "TG任务状态迁移失败：%s -> %s", strings.Join(from, ","), in.To)
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
		return false, gerror.Wrapf(err, "事务内TG任务状态迁移失败：%s -> %s", strings.Join(from, ","), in.To)
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
		return 0, gerror.Wrapf(err, "事务内批量TG任务状态迁移失败：%s -> %s", strings.Join(from, ","), in.To)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, gerror.Wrap(err, "确认事务内批量TG任务状态迁移结果失败")
	}
	return affected, nil
}

func transitionTelegramJobModel(model *gdb.Model, in telegramJobTransition, data g.Map) (sql.Result, error) {
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
	if strings.TrimSpace(in.To) == telegramJobStatusSuperseded {
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
		return 0, gerror.Wrapf(err, "批量TG任务状态迁移失败：%s -> %s", strings.Join(from, ","), in.To)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, gerror.Wrap(err, "确认批量TG任务状态迁移结果失败")
	}
	if affected == 0 || strings.TrimSpace(in.To) != telegramJobStatusSuperseded {
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

func telegramJobTransitionUpdate(in telegramJobTransition) ([]string, g.Map, error) {
	if err := validateTelegramJobTransition(in.From, in.To); err != nil {
		return nil, nil, err
	}
	from := append([]string(nil), in.From...)
	for index := range from {
		from[index] = strings.TrimSpace(from[index])
	}
	sort.Strings(from)
	now := gtime.Now()
	data := telegramJobStateUpdateData(strings.TrimSpace(in.To), in.RetryDelay, now)
	if in.ErrorMessage != "" || in.To == telegramJobStatusSent {
		data["error_message"] = strings.TrimSpace(in.ErrorMessage)
	}
	for field, value := range in.Additional {
		if strings.EqualFold(strings.TrimSpace(field), "status") {
			return nil, nil, gerror.New("TG任务附加更新字段不能包含status")
		}
		data[field] = value
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
