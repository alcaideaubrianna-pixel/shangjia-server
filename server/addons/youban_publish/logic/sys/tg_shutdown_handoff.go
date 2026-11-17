package sys

import (
	"context"
	"strconv"
	"strings"
	"time"

	"hotgo/utility/telegrambudget"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

const telegramShutdownHandoffPrefix = "runtime_shutdown_handoff:"

func telegramShutdownHandoffDeadline(marker string) (time.Time, bool) {
	if !strings.HasPrefix(marker, telegramShutdownHandoffPrefix) {
		return time.Time{}, false
	}
	ms, err := strconv.ParseInt(strings.TrimPrefix(marker, telegramShutdownHandoffPrefix), 10, 64)
	if err != nil || ms <= 0 {
		return time.Time{}, false
	}
	return time.UnixMilli(ms), true
}

func telegramShutdownHandoffExpired(job telegramJobRecord, now time.Time) bool {
	deadline, ok := telegramShutdownHandoffDeadline(job.LastDispatchError)
	return ok && !deadline.After(now)
}

// A bounded, operation-local checkpoint survives process exit. The existing
// Redis claim/requeue supplies the due work; no extra queue or scanner is added.
func (s *sSysPublish) persistTelegramShutdownHandoff(ctx context.Context, claim telegramChannelQueueClaim) {
	ctx, cancel := telegramFinalizeContextWithTimeout(ctx, "shutdown.handoff", telegrambudget.LeaseOperation(ctx))
	defer cancel()
	owned, err := renewTelegramChannelClaim(ctx, claim)
	if err != nil || !owned {
		logTelegramFinalize(ctx, "shutdown.handoff", "claim_recovery_required", err)
		return
	}
	deadline := time.Now().Add(telegrambudget.Webhook(ctx))
	marker := telegramShutdownHandoffPrefix + strconv.FormatInt(deadline.UnixMilli(), 10)
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", claim.JobID).Where("channel_id", claim.ChannelID).
		WhereIn("status", []JobStatus{telegramJobStatusSending, telegramJobStatusConfirming}).
		Where("(last_dispatch_error IS NULL OR (last_dispatch_error NOT LIKE ? AND last_dispatch_error NOT LIKE ?))",
			telegramShutdownHandoffPrefix+"%", "awaiting_account_fallback:%").
		Data(g.Map{"last_dispatch_error": marker}).Update()
	if err != nil {
		logTelegramFinalize(ctx, "shutdown.handoff", "checkpoint_failed", err)
		return
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logTelegramFinalize(ctx, "shutdown.handoff", "checkpoint_failed", err)
		return
	}
	if affected == 0 {
		logTelegramFinalize(ctx, "shutdown.handoff", "checkpoint_unchanged", nil)
		return
	}
	logTelegramQueueEvent(ctx, "shutdown_handoff_persisted", g.Map{
		"jobId": claim.JobID, "channelId": claim.ChannelID,
		"claimId": telegramQueueClaimID(claim.Token), "confirmationDeadline": deadline.UTC().Format(time.RFC3339Nano),
		"nextStep": "reconcile_before_retry",
	}, nil)
}

// Called only by the existing per-job leased reconciliation/Attempt paths.
// Known API success is repaired before reaching this branch. Unknown sends
// enter the existing fenced cleanup, whose unresolved-Attempt check forbids
// automatic replacement while remote delivery is still ambiguous.
func (s *sSysPublish) expireTelegramShutdownHandoff(ctx context.Context, job telegramJobRecord) (bool, error) {
	if !telegramShutdownHandoffExpired(job, time.Now()) {
		return false, nil
	}
	expired := false
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		_, _, err := s.expireTelegramSendTx(ctx, tx, job.Id)
		if err != nil {
			return err
		}
		phase, err := tx.Model(publishTgJobTable).Where("id", job.Id).Fields("send_phase").Value()
		if err == nil {
			expired = phase.String() == telegramSendPhaseTimeoutCancelled
		}
		return err
	})
	if err == nil && expired {
		logTelegramQueueEvent(ctx, "shutdown_handoff_expired", g.Map{
			"jobId": job.Id, "channelId": job.ChannelId, "nextStep": "cleanup_known_messages_before_retry",
		}, nil)
	}
	return expired, err
}

func telegramSendExpirationText(ctx context.Context, job telegramJobRecord) string {
	if _, ok := telegramShutdownHandoffDeadline(job.LastDispatchError); ok {
		return "发布实例停机后发送结果未确认，已隔离旧任务并等待清理已知消息；旧Attempt仍不确定时禁止自动补发"
	}
	return telegramSendTimeoutText(ctx)
}
