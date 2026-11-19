package sys

import (
	"context"
	"errors"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
)

const telegramBotNotMemberMessage = "bot is not a member of the channel chat"

// Only this explicit Bot API error disables a channel's cycle configuration.
// Other permission failures retain their existing retry/circuit policy.
func isTelegramBotNotMemberError(cause error) bool {
	return cause != nil && strings.Contains(strings.ToLower(cause.Error()), telegramBotNotMemberMessage)
}

// Historical message deletion can use a different Bot from the new job.
// Preserve the actual failing sender through the wrapped deletion error.
type telegramBotMembershipError struct {
	botID int64
	cause error
}

func (e *telegramBotMembershipError) Error() string { return e.cause.Error() }
func (e *telegramBotMembershipError) Unwrap() error { return e.cause }

func telegramMembershipFailureActor(job telegramJobRecord, cause error) telegramJobRecord {
	var failure *telegramBotMembershipError
	if errors.As(cause, &failure) && failure.botID > 0 {
		job.BotId = failure.botID
	}
	return job
}

// The job's CAS and the channel stop commit together. A process crash must
// never leave a terminal job without the channel's durable permission fence.
func (s *sSysPublish) failTelegramBotNotMember(ctx context.Context, job telegramJobRecord, transition telegramJobTransition, cause error) (changed bool, resultErr error) {
	if !isTelegramBotNotMemberError(cause) || job.TenantId <= 0 || job.ChannelId <= 0 {
		return false, gerror.New("停止频道循环需要明确的机器人非频道成员错误")
	}
	var generation string
	resultErr = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := lockChannelPermissionCreationTx(ctx, tx, job.TenantId, job.ChannelId); err != nil {
			return err
		}
		var err error
		changed, err = s.transitionTelegramJobTx(ctx, tx, transition)
		if err != nil || !changed {
			return err
		}
		generation, err = s.stopTelegramBotNotMemberChannelTx(ctx, tx, job, cause)
		return err
	})
	if resultErr != nil {
		return false, resultErr
	}
	if !changed {
		return false, nil
	}
	invalidateProfileCycleChannelConfigCache(ctx, job.ChannelId)
	actor := telegramMembershipFailureActor(job, cause)
	g.Log().Warning(ctx, "机器人不在频道，任务已失败并关闭频道循环", g.Map{
		"event": "channel_bot_not_member", "tenantId": job.TenantId, "accountId": job.AccountId,
		"channelId": job.ChannelId, "jobId": job.Id, "botId": actor.BotId,
		"operationNo": job.OperationNo, "sendPhase": job.SendPhase.String(),
		"cycleEnabled": false, "reason": telegramBotNotMemberMessage,
	})
	if generation != "" {
		if err := s.enqueueCollectNotification(ctx, collectNotificationPayload{
			ID: "permission:" + generation,
			PermissionCircuit: &channelPermissionCircuit{
				TenantId: job.TenantId, ChannelId: job.ChannelId, Generation: generation,
			},
		}); err != nil {
			// The existing notification recovery reads notified=0.
			g.Log().Warning(ctx, "频道停止通知入队失败，等待现有恢复器", g.Map{
				"event":     "channel_permission_notification_deferred",
				"channelId": job.ChannelId, "jobId": job.Id, "error": err.Error(),
			})
		}
	}
	return true, nil
}

// Called with the permission row locked, shared with job creation and resume.
// The initial cancellation remains bounded; existing paused-channel gates and
// recovery batches drain the rest without a global scan or Redis queue purge.
func (s *sSysPublish) stopTelegramBotNotMemberChannelTx(ctx context.Context, tx gdb.TX, job telegramJobRecord, cause error) (string, error) {
	if !isTelegramBotNotMemberError(cause) {
		return "", gerror.New("关闭频道循环需要机器人非频道成员错误")
	}
	channel, err := tx.Model(publishChannelTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).Where("id", job.ChannelId).
		Fields("created_by").LockUpdate().One()
	if err != nil {
		return "", err
	}
	if channel.IsEmpty() {
		return "", gerror.New("停止循环失败：频道不存在")
	}
	ownerID := channel["created_by"].Int64()
	if ownerID <= 0 {
		ownerID = job.AccountId
	}
	circuit, err := tx.Model(channelPermissionCircuitTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).Where("channel_id", job.ChannelId).One()
	if err != nil {
		return "", err
	}
	generation := ""
	if circuit["paused"].Int() != 1 {
		generation = guid.S()
		if _, err = tx.Model(channelPermissionCircuitTable).Safe().Ctx(ctx).
			Where("tenant_id", job.TenantId).Where("channel_id", job.ChannelId).
			Data(g.Map{"paused": 1, "generation": generation, "account_id": ownerID, "last_job_id": job.Id, "notified": 0}).Update(); err != nil {
			return "", err
		}
	}
	if _, err = tx.Model(publishChannelTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).Where("id", job.ChannelId).
		Data(g.Map{
			"cycle_publish_enabled": 0, "cycle_next_run_at": nil, "cycle_active_run_id": 0,
			"cycle_config_version":     gdb.Raw("COALESCE(cycle_config_version,0)+1"),
			"cycle_last_error_message": telegramBotNotMemberMessage,
		}).Update(); err != nil {
		return "", err
	}
	if err = s.projectChannelPermissionFailure(ctx, tx, telegramMembershipFailureActor(job, cause), cause); err != nil {
		return "", err
	}
	if err = cancelPermissionBacklogTx(ctx, tx, job.TenantId, job.ChannelId); err != nil {
		return "", err
	}
	return generation, nil
}
