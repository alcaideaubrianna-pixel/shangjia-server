package sys

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"
	botin "hotgo/addons/youban_bot/model/input/sysin"
	botservice "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const channelPermissionCircuitTable = "hg_youban_publish_channel_permission_circuit"
const channelPermissionPauseMessage = "频道连续10次权限错误，推送已暂停，请在机器人检查权限后恢复"
const channelPermissionCancelledMessage = "频道权限暂停，历史未发送任务已终止；如需补投请主动重新上架"

// The same row lock is used by pause/resume, including the first pause.
// Returning zero from job creation deliberately does not persist deferred work.
func lockChannelPermissionCreationTx(ctx context.Context, tx gdb.TX, tenantId, channelId int64) (bool, error) {
	if _, err := tx.Model(channelPermissionCircuitTable).Safe().Ctx(ctx).InsertIgnore(g.Map{"tenant_id": tenantId, "channel_id": channelId}); err != nil {
		return false, gerror.Wrap(err, "初始化频道权限状态失败")
	}
	row, err := tx.Model(channelPermissionCircuitTable).Safe().Ctx(ctx).Where("tenant_id", tenantId).Where("channel_id", channelId).LockUpdate().One()
	return row["paused"].Int() == 1, err
}

// Only untouched jobs can be discarded without reconciling Telegram first.
// Keep all delivery evidence, including soft-deleted messages, out of pruning.
func permissionUntouchedJobSQL() string {
	return "NOT EXISTS (SELECT 1 FROM " + publishTgAttemptTable + " a WHERE a.job_id=" + publishTgJobTable + ".id) AND NOT EXISTS (SELECT 1 FROM " + publishTgMessageTable + " m WHERE m.job_id=" + publishTgJobTable + ".id) AND COALESCE(send_phase,'') IN ('','cleanup_confirmed')"
}

func cancelPermissionBacklogTx(ctx context.Context, tx gdb.TX, tenantId, channelId int64, onlyJob ...int64) error {
	mod := tx.Model(publishTgJobTable).Safe().Ctx(ctx).Where("tenant_id", tenantId).Where("channel_id", channelId).
		WhereIn("status", []string{telegramJobStatusPending, telegramJobStatusRetryWait}).Where(permissionUntouchedJobSQL()).
		Where("LOWER(TRIM(operation_no)) NOT LIKE ?", "down:%").
		Where("operation_no NOT LIKE ?", "message_push:%").Where("operation_no NOT LIKE ?", "message_push_plan:%")
	if len(onlyJob) > 0 {
		mod = mod.Where("id", onlyJob[0])
	}
	var jobs []telegramJobRecord
	if err := mod.Fields("id").OrderAsc("id").Limit(200).LockUpdate().Scan(&jobs); err != nil {
		return err
	}
	ids := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.Id)
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.Model(publishTgJobTable).Safe().Ctx(ctx).WhereIn("id", ids).
		Data(g.Map{"status": telegramJobStatusSuperseded, "dispatch_status": tgDispatchStatusDone, "next_retry_at": nil, "last_dispatch_error": channelPermissionCancelledMessage}).Update()
	if err != nil {
		return err
	}
	_, err = tx.Model(publishTgOutboxTable).Safe().Ctx(ctx).Where("event_type", telegramOutboxEventPublish).
		WhereIn("job_id", ids).
		Data(g.Map{"status": telegramOutboxDelivered, "lease_owner": "", "lease_expires_at": nil, "delivered_at": gtime.Now()}).Update()
	return err
}

type channelPermissionCircuit struct {
	TenantId   int64
	ChannelId  int64
	Paused     int
	Generation string
	AccountId  int64
	LastJobId  int64
	ResumedAt  *gtime.Time
	Notified   int
}

func isTelegramPermissionCircuitError(err error) bool {
	if err == nil || isTelegramAmbiguousDeliveryError(err) {
		return false
	}
	if isTelegramChannelPermissionError(err) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, part := range []string{"chat not found", "bot is not a member", "bot was kicked", "not enough rights to send", "chat_write_forbidden", "chat_admin_required", "user_banned_in_channel", "user banned in channel"} {
		if strings.Contains(message, part) {
			return true
		}
	}
	return false
}

func telegramChannelPermissionStreak(jobs []telegramJobRecord) bool {
	if len(jobs) < 10 {
		return false
	}
	for _, job := range jobs[:10] {
		if job.Status != telegramJobStatusFailed || telegramSendPhaseIsCleanup(job.SendPhase) || !isTelegramPermissionCircuitError(errors.New(job.ErrorMessage)) {
			return false
		}
	}
	return true
}

func (s *sSysPublish) channelPermissionCircuit(ctx context.Context, job telegramJobRecord) (*channelPermissionCircuit, error) {
	if job.ChannelId <= 0 || job.TenantId <= 0 || isMessagePushOperationNo(job.OperationNo) || isDownTelegramOperationNo(job.OperationNo) {
		return nil, nil
	}
	var row *channelPermissionCircuit
	err := g.DB().Model(channelPermissionCircuitTable).Safe().Ctx(ctx).Where("tenant_id", job.TenantId).Where("channel_id", job.ChannelId).Scan(&row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return row, err
}

func (s *sSysPublish) postponePermissionPausedJob(ctx context.Context, job telegramJobRecord) (bool, error) {
	// Unknown/confirmed attempts must still reconcile; pausing new sends must
	// never prevent local projection or duplicate-message cleanup.
	if job.Status != telegramJobStatusPending && job.Status != telegramJobStatusRetryWait {
		return false, nil
	}
	row, err := s.channelPermissionCircuit(ctx, job)
	if err != nil {
		return false, gerror.Wrap(err, "读取频道权限暂停状态失败，请确认数据库迁移已执行")
	}
	if row == nil || (row.Paused != 1 && row.ResumedAt == nil) {
		return false, nil
	}
	parked := false
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var parkErr error
		parked, parkErr = parkPermissionPausedJobTx(ctx, tx, job, row.Generation)
		return parkErr
	})
	return parked, err
}

func parkPermissionPausedJobTx(ctx context.Context, tx gdb.TX, job telegramJobRecord, generation string) (bool, error) {
	// Serialize parking with resume. Otherwise a stale worker can move an
	// Outbox back into the future immediately after the owner resumes it.
	row, err := tx.Model(channelPermissionCircuitTable).Safe().Ctx(ctx).Where("tenant_id", job.TenantId).Where("channel_id", job.ChannelId).Where("generation", generation).LockUpdate().One()
	if err != nil || row.IsEmpty() {
		return false, err
	}
	if row["paused"].Int() != 1 {
		if row["resumed_at"].IsNil() {
			return false, nil
		}
		old, err := tx.Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Where("created_at <= ?", row["resumed_at"]).Where(permissionUntouchedJobSQL()).Count()
		if err != nil || old == 0 {
			return false, err
		}
		return true, cancelPermissionBacklogTx(ctx, tx, job.TenantId, job.ChannelId, job.Id)
	}
	if err := cancelPermissionBacklogTx(ctx, tx, job.TenantId, job.ChannelId, job.Id); err != nil {
		return false, err
	}
	current, err := tx.Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Fields("status,last_dispatch_error").One()
	if err != nil {
		return false, err
	}
	if current["status"].String() == telegramJobStatusSuperseded && current["last_dispatch_error"].String() == channelPermissionCancelledMessage {
		return true, nil
	}
	result, err := tx.Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Where("tenant_id", job.TenantId).Where("channel_id", job.ChannelId).
		WhereIn("status", []string{telegramJobStatusPending, telegramJobStatusRetryWait}).
		Data(g.Map{"next_retry_at": gtime.Now().Add(time.Hour), "dispatch_status": tgDispatchStatusIdle, "last_dispatch_error": channelPermissionPauseMessage}).Update()
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 0 {
		return false, err
	}
	_, err = tx.Model(publishTgOutboxTable).Safe().Ctx(ctx).Where("job_id", job.Id).Where("event_type", telegramOutboxEventPublish).WhereIn("status", []string{telegramOutboxPending, telegramOutboxDelivered}).
		Data(g.Map{"status": telegramOutboxPending, "available_at": gtime.Now().Add(time.Hour), "lease_owner": "", "lease_expires_at": nil, "delivered_at": nil}).Update()
	if err != nil {
		return true, err
	}
	return true, (&sSysPublish{}).ensureTelegramJobOutboxTx(ctx, tx, job.Id, time.Hour)
}

// Called only after a successful sending -> failed transition, so redelivery
// of the same worker task cannot count as another permission failure.
func (s *sSysPublish) recordChannelPermissionFailure(ctx context.Context, job telegramJobRecord, cause error) error {
	if !isTelegramPermissionCircuitError(cause) || telegramSendPhaseIsCleanup(job.SendPhase) || isMessagePushOperationNo(job.OperationNo) || isDownTelegramOperationNo(job.OperationNo) {
		return nil
	}
	row, err := s.channelPermissionCircuit(ctx, job)
	if err != nil {
		return err
	}
	if row != nil && row.Paused == 1 {
		return nil
	}
	jobs, err := channelPermissionRecentJobs(ctx, g.DB().Model(publishTgJobTable), job, row)
	if err != nil {
		return err
	}
	if !telegramChannelPermissionStreak(jobs) {
		return nil
	}
	owner, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Fields("created_by").Where("id", job.ChannelId).Where("tenant_id", job.TenantId).WhereNull("deleted_at").Value()
	if err != nil {
		return err
	}
	if owner.Int64() <= 0 {
		return gerror.New("频道缺少可通知的归属账号")
	}
	ownerAccountId := owner.Int64()
	generation := guid.S()
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		return pauseChannelPermissionTx(ctx, tx, job, ownerAccountId, generation, row)
	})
	if err != nil {
		return err
	}
	g.Log().Warning(ctx, "频道权限熔断，已暂停推送", g.Map{"tenantId": job.TenantId, "channelId": job.ChannelId, "jobId": job.Id})
	return s.enqueueCollectNotification(ctx, collectNotificationPayload{ID: "permission:" + generation, PermissionCircuit: &channelPermissionCircuit{TenantId: job.TenantId, ChannelId: job.ChannelId, Generation: generation}})
}

func channelPermissionRecentJobs(ctx context.Context, mod *gdb.Model, job telegramJobRecord, row *channelPermissionCircuit) ([]telegramJobRecord, error) {
	mod = mod.Safe().Ctx(ctx).
		Fields("id,status,error_message,send_phase").Where("tenant_id", job.TenantId).Where("channel_id", job.ChannelId).
		WhereIn("status", []string{"sent", "failed", "failed_retry", "unknown"}).
		Where("dispatch_count > 0").Where("LOWER(TRIM(operation_no)) NOT LIKE ?", "down:%").Where("operation_no NOT LIKE ?", "message_push:%").Where("operation_no NOT LIKE ?", "message_push_plan:%")
	if row != nil && row.ResumedAt != nil {
		mod = mod.Where("updated_at > ?", row.ResumedAt)
	}
	var jobs []telegramJobRecord
	err := mod.OrderDesc("updated_at").OrderDesc("id").Limit(10).Scan(&jobs)
	return jobs, err
}

func pauseChannelPermissionTx(ctx context.Context, tx gdb.TX, job telegramJobRecord, ownerAccountId int64, generation string, observed *channelPermissionCircuit) error {
	if _, err := tx.Model(channelPermissionCircuitTable).Safe().Ctx(ctx).InsertIgnore(g.Map{"tenant_id": job.TenantId, "channel_id": job.ChannelId}); err != nil {
		return err
	}
	mod := tx.Model(channelPermissionCircuitTable).Safe().Ctx(ctx).Where("tenant_id", job.TenantId).Where("channel_id", job.ChannelId).Where("paused", 0)
	if observed != nil && observed.ResumedAt != nil {
		mod = mod.Where("resumed_at", observed.ResumedAt)
	} else {
		mod = mod.WhereNull("resumed_at")
	}
	result, err := mod.
		Data(g.Map{"paused": 1, "generation": generation, "account_id": ownerAccountId, "last_job_id": job.Id, "notified": 0}).Update()
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 0 {
		return err
	}
	// Bound the initial transaction too. Remaining backlog is handled by the
	// enqueue/send gates and the recovery batch, never an unbounded update.
	return cancelPermissionBacklogTx(ctx, tx, job.TenantId, job.ChannelId)
}

func (s *sSysPublish) notifyChannelPermissionCircuit(ctx context.Context, expected *channelPermissionCircuit) error {
	row, err := s.channelPermissionCircuit(ctx, telegramJobRecord{TenantId: expected.TenantId, ChannelId: expected.ChannelId})
	if err != nil || row == nil || row.Paused != 1 || row.Generation != expected.Generation || row.Notified == 1 {
		return err
	}
	channel, err := s.channelById(ctx, row.TenantId, row.ChannelId)
	if err != nil {
		return err
	}
	if channel == nil {
		return gerror.New("频道不存在")
	}
	err = botservice.SysBot().NotifyAccount(ctx, &botin.NotifyAccountInp{BotStrategy: "official", FallbackBoundBot: true, RequireDelivery: true, IgnoreFeatureSwitch: true,
		App: "api", AccountId: row.AccountId,
		Text:    fmt.Sprintf("频道推送已暂停\n频道：%s\n连续10次发送出现频道不存在或权限不足错误。请先将推送 Bot 添加为管理员并开启发布消息权限，再点击恢复。采集与资料保留，暂停期间不新增发送任务；历史未发送任务不会自动补投，调度记录最长保留一个月。", html.EscapeString(channel.ChannelTitle)),
		Buttons: [][]botin.NotifyButton{{{Label: "检查权限并恢复推送", Data: fmt.Sprintf("ch:pr:%d:%s", row.ChannelId, row.Generation)}}},
	})
	if err != nil {
		return err
	}
	_, err = g.DB().Model(channelPermissionCircuitTable).Safe().Ctx(ctx).Where("tenant_id", row.TenantId).Where("channel_id", row.ChannelId).Where("generation", row.Generation).Where("paused", 1).Data(g.Map{"notified": 1}).Update()
	return err
}

func (s *sSysPublish) retryChannelPermissionNotifications(ctx context.Context) error {
	var rows []*channelPermissionCircuit
	if err := g.DB().Model(channelPermissionCircuitTable).Safe().Ctx(ctx).Where("paused", 1).Where("notified", 0).OrderAsc("updated_at").Limit(20).Scan(&rows); err != nil {
		return err
	}
	for _, row := range rows {
		if err := s.enqueueCollectNotification(ctx, collectNotificationPayload{ID: "permission:" + row.Generation, PermissionCircuit: row}); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) BotChannelPermissionResume(ctx context.Context, in *sysin.BotChannelActionInp, generation string) error {
	if in == nil || in.TenantId <= 0 || in.AccountId <= 0 || in.ChannelId <= 0 || generation == "" {
		return gerror.New("恢复参数无效")
	}
	row, err := s.channelPermissionCircuit(ctx, telegramJobRecord{TenantId: in.TenantId, ChannelId: in.ChannelId})
	if err != nil {
		return err
	}
	if row == nil || row.Paused != 1 || row.Generation != generation {
		return gerror.New("按钮已失效或频道已恢复")
	}
	if row.AccountId != in.AccountId {
		return gerror.New("无权恢复其他账号的频道")
	}
	channel, err := s.channelById(ctx, in.TenantId, in.ChannelId)
	if err != nil {
		return err
	}
	if channel == nil || channel.Status != 1 {
		return gerror.New("频道已禁用或删除，请先恢复频道配置")
	}
	ids := decodeBotIds(channel.BotIdJson)
	bots, err := s.channelCheckBots(ctx, ids, in.TenantId)
	if err != nil {
		return err
	}
	if len(bots) == 0 {
		return gerror.New("频道没有可用推送Bot")
	}
	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, bot := range bots {
		canSend, _, inChannel, message := s.checkBotChannelMember(checkCtx, bot, normalizeTelegramChannelChatID(channel.TargetChatId))
		if !canSend || !inChannel {
			return gerror.New("权限检查未通过：" + message)
		}
	}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		return resumeChannelPermissionTx(ctx, tx, in, generation)
	})
	if err != nil {
		return err
	}
	if wakeErr := s.wakeNextTelegramChannelJob(ctx, telegramJobRecord{TenantId: in.TenantId, ChannelId: in.ChannelId}); wakeErr != nil {
		g.Log().Warningf(ctx, "权限恢复后唤醒频道失败，等待Outbox调度 channelId:%d err:%+v", in.ChannelId, wakeErr)
	}
	return nil
}

func resumeChannelPermissionTx(ctx context.Context, tx gdb.TX, in *sysin.BotChannelActionInp, generation string) error {
	result, err := tx.Model(channelPermissionCircuitTable).Safe().Ctx(ctx).Where("tenant_id", in.TenantId).Where("channel_id", in.ChannelId).Where("account_id", in.AccountId).Where("generation", generation).Where("paused", 1).
		Data(g.Map{"paused": 0, "resumed_at": gtime.Now()}).Update()
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return gerror.New("频道状态已变化，请重新检查")
	}
	// Rearmed Outbox rows were delayed too; releasing only next_retry_at
	// would leave the channel asleep until their one-hour backoff elapsed.
	_, err = tx.Model(publishTgOutboxTable).Safe().Ctx(ctx).Where("status", telegramOutboxPending).
		Where("job_id IN (SELECT id FROM "+publishTgJobTable+" WHERE tenant_id=? AND channel_id=? AND status IN ('pending','failed_retry') AND last_dispatch_error=?)", in.TenantId, in.ChannelId, channelPermissionPauseMessage).
		Data(g.Map{"available_at": gtime.Now()}).Update()
	if err != nil {
		return err
	}
	_, err = tx.Model(publishTgJobTable).Safe().Ctx(ctx).Where("tenant_id", in.TenantId).Where("channel_id", in.ChannelId).
		WhereIn("status", []string{"pending", "failed_retry"}).Where("last_dispatch_error", channelPermissionPauseMessage).
		Data(g.Map{"next_retry_at": nil, "last_dispatch_error": ""}).Update()
	return err
}
