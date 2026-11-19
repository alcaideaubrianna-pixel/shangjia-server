package sys

import (
	"context"
	"fmt"
	"strings"

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

func telegramChannelPendingCleanupSQL(alias string) string {
	p := ""
	if alias != "" {
		p = strings.TrimSuffix(alias, ".") + "."
	}
	return fmt.Sprintf("(%stimeout_cleanup_at IS NOT NULL OR %ssend_phase='cleanup_processing')", p, p)
}

func telegramChannelStartedJobSQL(alias string) string {
	p := ""
	if alias != "" {
		p = strings.TrimSuffix(alias, ".") + "."
	}
	return fmt.Sprintf(`(%s OR %sstatus IN ('sending','confirming','unknown','reconciling') OR (%sstatus IN ('pending','failed_retry') AND %ssend_phase IN ('display_sending','display_confirmed','display_confirming','verify_sending','verify_confirming','cleanup_sending','cleanup_confirming')))`, telegramChannelPendingCleanupSQL(alias), p, p, p)
}

func telegramChannelOrderScopeSQL(alias string, job telegramJobRecord) (string, []interface{}) {
	p := ""
	if alias != "" {
		p = alias + "."
	}
	if strings.TrimSpace(job.TargetChatId) != "" {
		return p + "target_chat_id IN (?)", []interface{}{telegramChannelChatIDVariants(job.TargetChatId)}
	}
	return fmt.Sprintf("(%stenant_id=? AND %schannel_id=? AND %saccount_id=?)", p, p, p),
		[]interface{}{job.TenantId, job.ChannelId, job.AccountId}
}

func telegramChannelJobPendingCleanup(job telegramJobRecord) bool {
	return job.TimeoutCleanupAt != nil || job.SendPhase == "cleanup_processing"
}

func telegramChannelJobOwnsOrder(job telegramJobRecord) bool {
	if telegramChannelJobPendingCleanup(job) {
		return true
	}
	if isTelegramChannelJobTerminal(job) {
		return false
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
	scope, args := telegramChannelOrderScopeSQL("j", job)
	model := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).Master().Unscoped().
		Where("j.id <> ?", job.Id).Where(scope, args...).
		Where(telegramChannelStartedJobSQL("j"))
	if telegramChannelJobOwnsOrder(job) {
		model = model.Where("("+telegramChannelPendingCleanupSQL("j")+" OR j.id < ?)", job.Id)
	}
	row, err := model.Fields("j.id,j.status,j.send_phase,j.timeout_cleanup_at").Limit(1).One()
	if err != nil {
		return false, err
	}
	if !row.IsEmpty() {
		g.Log().Warning(ctx, "频道保留已开始资料的发送顺序", g.Map{
			"event": "channel_queue_order_blocked", "jobId": job.Id, "channelId": job.ChannelId,
			"targetChatId": job.TargetChatId, "blockingJobId": row["id"].Int64(),
			"blockingStatus": row["status"].String(), "blockingPhase": row["send_phase"].String(),
			"timeoutCleanupAt": row["timeout_cleanup_at"].String(),
		})
	}
	return !row.IsEmpty(), nil
}
