package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/utility/telegrambudget"
)

// Keep this outside the processing-key prefix: the existing bounded claim
// recovery scanner must never mistake an ordering owner for a claim hash.
func telegramChannelOrderOwnerKey(channelID int64) string {
	return fmt.Sprintf("%s:order:%d", telegramChannelActiveSetKey, channelID)
}

// Wake only the existing owner and its existing lane. Cleanup completion must
// not wait for the cleanup claim's future retry time, and must not create a new
// owner or overwrite a different operation's ordering fence.
func wakeTelegramChannelOrderOwner(ctx context.Context, job telegramJobRecord) (bool, error) {
	if !telegramChannelQueueEnabled() || job.ChannelId <= 0 || job.Id <= 0 {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), telegrambudget.LeaseOperation(ctx))
	defer cancel()
	const script = `local owner=redis.call('GET',KEYS[1])
 if owner and owner~=ARGV[1] then return 0 end
 local t=redis.call('TIME'); local now=t[1]*1000+math.floor(t[2]/1000)
 if owner==ARGV[1] then
 for i=2,4 do local score=redis.call('ZSCORE',KEYS[i],ARGV[1])
 if score then
 local n=tonumber(score); local due=n%2000000000000
 if due>now then redis.call('ZADD',KEYS[i],n-due+now,ARGV[1]) end
 redis.call('SADD',KEYS[5],ARGV[2]); return 1
 end end end
 if redis.call('HGET',KEYS[6],'job')==ARGV[1] then
 redis.call('HSET',KEYS[6],'order_woken',ARGV[1]); return 1
 end return 0`
	value, err := telegramChannelRedisDo(ctx, "EVAL", script, 6,
		telegramChannelOrderOwnerKey(job.ChannelId),
		telegramChannelUrgentQueueKey(job.ChannelId), telegramChannelQueueKey(job.ChannelId),
		telegramChannelHistoryQueueKey(job.ChannelId), telegramChannelActiveSetKey,
		telegramChannelProcessingKey(job.ChannelId),
		job.Id, job.ChannelId)
	if err != nil {
		return false, gerror.Wrap(err, "唤醒频道清理完成的队首任务失败")
	}
	woken := value.Int() == 1
	if woken {
		logTelegramQueueEvent(ctx, "order_owner_wakeup", g.Map{
			"channelId": job.ChannelId, "jobId": job.Id, "reason": "cleanup_completed",
			"result": "success", "stage": "wake",
		}, nil)
	}
	return woken, nil
}

func telegramChannelPendingCleanupCondition(model *gdb.Model, alias string) *gdb.WhereBuilder {
	cleanupStarted := model.Builder().
		WherePrefixNotNull(alias, "timeout_cleanup_at").
		WherePrefixNotIn(alias, "status", []string{"sent", "superseded", "failed"})
	cleanupProcessing := model.Builder().
		WherePrefix(alias, "send_phase", "cleanup_processing").
		WherePrefixNotIn(alias, "status", []string{"sent", "failed", "superseded"})
	return model.Builder().Where(cleanupStarted).WhereOr(cleanupProcessing)
}

func telegramChannelStartedJobCondition(model *gdb.Model, alias string) *gdb.Model {
	phases := []string{
		"display_sending", "display_confirmed", "display_confirming",
		"verify_sending", "verify_confirming",
		"cleanup_sending", "cleanup_confirming",
	}
	statuses := []string{"sending", "confirming", "unknown", "reconciling"}
	cleanup := telegramChannelPendingCleanupCondition(model, alias)
	inFlight := model.Builder().WherePrefixIn(alias, "status", statuses)
	pending := model.Builder().
		WherePrefix(alias, "status", "pending").
		WherePrefixIn(alias, "send_phase", phases)
	retry := model.Builder().
		WherePrefix(alias, "status", "failed_retry").
		WherePrefixIn(alias, "dispatch_status", []string{"", "idle"}).
		WherePrefixIn(alias, "send_phase", phases)
	retry = retry.Where(
		retry.Builder().
			WherePrefixNull(alias, "next_retry_at").
			WhereOrPrefixLTE(alias, "next_retry_at", gdb.Raw("CURRENT_TIMESTAMP")),
	)
	return model.Where(cleanup).WhereOr(inFlight).WhereOr(pending).WhereOr(retry)
}

func telegramChannelApplyOrderScope(model *gdb.Model, alias string, job telegramJobRecord) *gdb.Model {
	if strings.TrimSpace(job.TargetChatId) != "" {
		return model.WherePrefixIn(alias, "target_chat_id", telegramChannelChatIDVariants(job.TargetChatId))
	}
	return model.
		WherePrefix(alias, "tenant_id", job.TenantId).
		WherePrefix(alias, "channel_id", job.ChannelId).
		WherePrefix(alias, "account_id", job.AccountId)
}

func telegramChannelJobPendingCleanup(job telegramJobRecord) bool {
	return (job.TimeoutCleanupAt != nil && job.Status != telegramJobStatusSent && job.Status != telegramJobStatusSuperseded) ||
		(job.SendPhase == "cleanup_processing" && !job.Status.IsTerminal())
}

func telegramChannelJobOwnsOrder(job telegramJobRecord) bool {
	if isTelegramChannelJobTerminal(job) {
		return false
	}
	if telegramChannelJobPendingCleanup(job) {
		return true
	}
	switch job.Status {
	case telegramJobStatusSending, telegramJobStatusConfirming, "confirming", "reconciling":
		return true
	}
	return job.SendPhase == telegramSendPhaseDisplayConfirmed ||
		job.SendPhase == telegramSendPhaseDisplaySending ||
		job.SendPhase == telegramSendPhaseVerifySending ||
		job.SendPhase == "display_confirming" || job.SendPhase == "verify_confirming" ||
		job.SendPhase == "cleanup_sending" || job.SendPhase == "cleanup_confirming"
}

// The Redis queue orders untouched work. A bounded exact-destination lookup
// protects already-visible media across channel aliases and queue retries.
// It never restores the legacy pending/idle DB head gate.
func (s *sSysPublish) telegramChannelHasStartedOwner(ctx context.Context, job telegramJobRecord) (bool, error) {
	if job.Id <= 0 {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, telegrambudget.LeaseOperation(ctx))
	defer cancel()
	model := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).Master().Unscoped().
		WhereNot("j.id", job.Id)
	model = telegramChannelApplyOrderScope(model, "j", job)
	if telegramJobPriorityValue(job) <= tgJobPriorityInteractive {
		// Manual, AI and bot publishing is already admitted to the foreground
		// Redis lane. Historical/cycle pending work must not turn that lane into
		// a channel-wide wait queue; only a real in-flight send or cleanup for
		// the same profile can block it.
		sameProfile := model.Builder().
			WherePrefix("j", "profile_id", job.ProfileId).
			WherePrefixNotIn("j", "status", []string{"sent", "failed", "superseded"}).
			Where(telegramChannelPendingCleanupCondition(model, "j"))
		sameProfileOrPhase := model.Builder().
			WherePrefix("j", "profile_id", job.ProfileId).
			WherePrefixNotIn("j", "status", []string{"sent", "failed", "superseded"}).
			WherePrefixIn("j", "send_phase", []string{
				"display_sending", "display_confirmed", "display_confirming",
				"verify_sending", "verify_confirming",
				"cleanup_sending", "cleanup_confirming",
			})
		model = model.
			WherePrefixIn("j", "status", []string{"sending", "confirming", "unknown", "reconciling"}).
			WhereOr(sameProfile).
			WhereOr(sameProfileOrPhase)
	} else {
		model = telegramChannelStartedJobCondition(model, "j").
			Where(
				model.Builder().
					WherePrefixNotIn("j", "status", []string{"sent", "failed", "superseded"}).
					WhereOr(
						model.Builder().
							WherePrefixGT("j", "profile_id", 0).
							WherePrefix("j", "profile_id", job.ProfileId),
					),
			)
	}
	if telegramChannelJobOwnsOrder(job) {
		model = model.
			Where(telegramChannelPendingCleanupCondition(model, "j")).
			WhereLT("j.id", job.Id)
	}
	row, err := model.Fields("j.id,j.status,j.send_phase,j.timeout_cleanup_at").Limit(1).One()
	if err != nil {
		g.Log().Error(ctx, "频道发送闸门查询失败", g.Map{
			"event":        "channel_order_gate_query_failed",
			"jobId":        job.Id,
			"profileId":    job.ProfileId,
			"channelId":    job.ChannelId,
			"targetChatId": job.TargetChatId,
			"tenantId":     job.TenantId,
			"accountId":    job.AccountId,
			"status":       job.Status,
			"sendPhase":    job.SendPhase,
			"queueName":    job.QueueName,
			"priority":     telegramJobPriorityValue(job),
			"error":        err.Error(),
			"scope":        scope,
		})
		return false, err
	}
	if !row.IsEmpty() {
		if claim, ok := ctx.Value(telegramChannelClaimContextKey{}).(*telegramChannelClaimContext); ok && claim.jobID == job.Id {
			claim.predecessorID = row["id"].Int64()
		}
		g.Log().Warning(ctx, "频道保留已开始资料的发送顺序", g.Map{
			"event": "channel_queue_order_blocked", "jobId": job.Id, "channelId": job.ChannelId,
			"targetChatId": job.TargetChatId, "blockingJobId": row["id"].Int64(),
			"blockingStatus": row["status"].String(), "blockingPhase": row["send_phase"].String(),
			"timeoutCleanupAt": row["timeout_cleanup_at"].String(),
		})
	}
	return !row.IsEmpty(), nil
}
