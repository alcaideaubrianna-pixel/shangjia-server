package sys

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/library/cache"
	hglock "hotgo/internal/library/hgrds/lock"
)

const telegramChannelDispatchLeaseTTL = 30 * time.Second
const telegramChannelActiveJobLimit = 1
const telegramChannelWakeDebounce = 200 * time.Millisecond

func (s *sSysPublish) telegramChannelHasEarlierActiveJob(ctx context.Context, job telegramJobRecord) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if job.Id <= 0 {
		return false, nil
	}
	createdAt := job.CreatedAt
	if createdAt == nil {
		current, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Master().
			Fields("created_at").
			Where("id", job.Id).
			One()
		if err != nil {
			return false, gerror.Wrap(err, "读取TG任务创建时间失败")
		}
		if current.IsEmpty() {
			return false, nil
		}
		createdAt = current["created_at"].GTime()
	}
	blocking := telegramChannelBlockingJobSQL("j")
	if s.accountPushEngine(ctx, job.TenantId, job.AccountId) == pushEngineUnifiedStream {
		blocking = telegramStreamChannelBlockingJobSQL("j")
	}
	// 队首判断必须读取主库，否则副本延迟会继续看到已终态的旧任务，
	// 让频道在数据库已释放后仍被旧队首阻塞。
	mod := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).Master().Unscoped().
		Where("j.id <> ?", job.Id).
		Where(telegramWebhookDetachedJobSQL("j.")).
		Where(blocking)
	// Once a display is visible in Telegram, finish that operation before
	// returning it behind untouched historical backlog. Earlier operations that
	// have also started still block it, preserving media/verification ordering.
	if telegramJobRequiresChannelCompletionPriority(job.SendPhase) {
		mod = mod.Where(`(
			j.status IN (?, ?) OR
			j.send_phase IN (?, ?, ?)
		)`, telegramJobStatusSending, telegramJobStatusConfirming,
			telegramSendPhaseDisplaySending, telegramSendPhaseDisplayConfirmed,
			telegramSendPhaseVerifySending)
	}
	if strings.TrimSpace(job.TargetChatId) != "" {
		mod = mod.WhereIn("j.target_chat_id", telegramChannelChatIDVariants(job.TargetChatId))
	} else if job.ChannelId > 0 {
		mod = mod.Where("j.channel_id", job.ChannelId)
	}
	// 统一流任务已经按账户和频道串行领取；使用轻量 FIFO 前序查询，避免
	// 复杂的 Bot/协议账户 EXISTS 和优先级 CASE 在高负载下阻塞发送队首。
	if s.accountPushEngine(ctx, job.TenantId, job.AccountId) == pushEngineUnifiedStream {
		fast := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).Master().Unscoped().
			Where("j.id <> ?", job.Id).
			Where("j.tenant_id", job.TenantId).
			Where("j.status IN (?)", []string{telegramJobStatusPending, telegramJobStatusRetryWait, telegramJobStatusConfirming, telegramJobStatusSending}).
			Where("j.created_at < ? OR (j.created_at = ? AND j.id < ?)", createdAt, createdAt, job.Id)
		// Outbox 已记录 Stream 入队失败且超过队首超时的旧任务不能继续
		// 阻塞频道。它们会由恢复扫描收口，当前判断先允许后续任务推进。
		fast = fast.Where("NOT (j.status='pending' AND COALESCE(j.dispatch_status,'')='idle' AND COALESCE(j.asynq_task_id,'')='' AND j.updated_at < CURRENT_TIMESTAMP - (? * INTERVAL '1 minute'))", telegramChannelHeadTimeoutMinutes())
		// 媒体尚未就绪的任务只应停留在媒体准备队列，不能占用频道发送队首；
		// 媒体完成后由唤醒逻辑重新进入 Ready 队列。
		fast = fast.Where("NOT (j.status IN ('pending','failed_retry') AND COALESCE(j.last_dispatch_error,'') LIKE '媒体仍在异步处理中%')")
		// 手动/交互任务可以越过尚未开始的循环、批次和历史任务，
		// 但不能越过已经入队、发送中或结果未知的任务，避免破坏同频道顺序。
		// Realtime/urgent work must not wait behind an untouched historical
		// backlog.  Keep the channel lease and already-started jobs as the
		// ordering boundary, but allow collect/quick-push jobs to overtake
		// pending idle bulk/history rows.  Otherwise a lost Stream event at
		// the head can stall every newer publication on the channel.
		// queued + pending only means the task is ready in Redis; it has not
		// started sending and must not form a multi-minute FIFO barrier.  The
		// actual ordering boundary is an execution that has claimed the job or
		// entered a send/confirmation phase.  This lets a newer publication
		// proceed while a stale queued task is waiting for a worker.
		fast = fast.Where("(j.dispatch_status = 'processing' OR j.status IN ('sending','confirming','unknown'))")
		if strings.TrimSpace(job.TargetChatId) != "" {
			// target_chat_id 是真实 Telegram 目标的串行边界。即使两个
			// 采集账户不同，只要租户和目标频道相同，也必须共享同一个队首。
			fast = fast.WhereIn("j.target_chat_id", telegramChannelChatIDVariants(job.TargetChatId))
		} else if job.ChannelId > 0 {
			// 没有目标 chat_id 时才退回内部频道和账户边界，避免不同
			// 账户恰好复用内部 channel_id 时互相阻塞。
			fast = fast.Where("j.channel_id", job.ChannelId).
				Where("j.account_id", job.AccountId)
		}
		// 使用与 Outbox 队首选择器完全相同的排序规则：已开始任务优先，
		// 其次按有效优先级、创建时间和主键排序，避免两套比较器互相打死。
		earlier, err := fast.Fields("j.id,j.created_at,j.priority,j.status,j.dispatch_status,j.operation_no").
			Order(telegramChannelScheduleOrderSQL("j")).Limit(1).One()
		if err != nil {
			return false, gerror.Wrap(err, "检查统一流频道前序TG任务失败")
		}
		// One 返回空记录表示没有前序任务，这是正常的队首状态，不应触发退避。
		if earlier.IsEmpty() {
			return false, nil
		}
		earlierId := earlier["id"].Int64()
		if earlierId > 0 {
			g.Log().Info(ctx, "TG统一流频道队首命中前序任务", g.Map{"currentJobId": job.Id, "earlierJobId": earlierId, "earlierStatus": earlier["status"].String()})
		}
		return earlierId > 0, nil
	}
	// A destination channel is a single ordered stream. Source message IDs are
	// only comparable within one source channel, so using them here lets jobs
	// from another collection source bypass an older job between its display
	// and verification phases.
	mod = mod.Where("EXISTS (SELECT 1 FROM "+publishTgJobTable+" current_job WHERE current_job.id=? AND "+telegramChannelSchedulePrecedesSQL("j", "current_job")+")", job.Id)
	// Interactive publications may overtake untouched work, but never a
	// started display/verification operation. Use the same ordering as Outbox.
	if err := ctx.Err(); err != nil {
		return false, err
	}
	earlier, err := mod.Fields("j.id,j.created_at,j.priority,j.status,j.dispatch_status,j.operation_no").Limit(1).One()
	if err != nil {
		return false, gerror.Wrap(err, "检查频道前序TG任务失败")
	}
	if !earlier.IsEmpty() {
		g.Log().Info(ctx, "TG频道队首判断命中前序任务", g.Map{
			"currentJobId":          job.Id,
			"currentChannelId":      job.ChannelId,
			"currentTargetChatId":   job.TargetChatId,
			"earlierJobId":          earlier["id"].Int64(),
			"earlierCreatedAt":      earlier["created_at"].GTime(),
			"earlierPriority":       earlier["priority"].Int(),
			"earlierStatus":         earlier["status"].String(),
			"earlierDispatchStatus": earlier["dispatch_status"].String(),
			"earlierOperationNo":    earlier["operation_no"].String(),
		})
	}
	return !earlier.IsEmpty(), nil
}

func telegramChannelBlockingJobStatuses() []string {
	return []string{
		telegramJobStatusPending,
		telegramJobStatusRetryWait,
		telegramJobStatusConfirming,
		telegramJobStatusSending,
	}
}

func telegramChannelBlockingJobSQL(alias string) string {
	// Unknown jobs are reconciled independently. They must not hold the normal
	// channel queue hostage while waiting for a webhook or account fallback.
	// Jobs tied to an unavailable protocol account are likewise left durable but
	// excluded until the account is authorized again.
	qualified := alias
	if qualified != "" {
		qualified += "."
	}
	// 已经进入发送队列的任务必须继续占住频道，即使其 Bot/账户信息
	// 在测试或授权切换期间暂时不可用；否则后续任务会越过已入队任务。
	return fmt.Sprintf(`(((%sdispatch_status IN ('queued','processing')) OR %s) AND %s AND NOT (
		%sstatus='pending' AND COALESCE(%sdispatch_status,'')='idle' AND COALESCE(%sasynq_task_id,'')='' AND %supdated_at < CURRENT_TIMESTAMP - (%d * INTERVAL '1 minute')
	))`,
		qualified, telegramChannelAvailableAccountSQL(alias), telegramChannelBlockingJobStateSQL(alias), qualified, qualified, qualified, qualified, telegramChannelHeadTimeoutMinutes())
}

// telegramChannelHeadTimeoutMinutes 返回频道队首最大等待时间，默认10分钟。
// 与循环队首超时共用环境变量，保证旧队首和循环队首不会使用不同策略。
func telegramChannelHeadTimeoutMinutes() int {
	value := os.Getenv("YOUBAN_PUBLISH_CYCLE_HEAD_TIMEOUT_MINUTES")
	minutes, err := strconv.Atoi(value)
	if err != nil || minutes <= 0 {
		return 10
	}
	if minutes > 24*60 {
		return 24 * 60
	}
	return minutes
}

func telegramChannelBlockingJobStateSQL(alias string) string {
	if alias != "" {
		alias += "."
	}
	// Terminal jobs must never become unschedulable channel heads. Retrying a
	// failed display later would also send verification apart from its display.
	return fmt.Sprintf(`(
		(%[1]sstatus IN ('pending','confirming','sending') AND NOT (
			%[1]sstatus='pending' AND COALESCE(%[1]sdispatch_status,'')='idle'
			AND COALESCE(%[1]slast_dispatch_error,'') LIKE '就绪通知或执行任务缺失%%'
			AND %[1]supdated_at < CURRENT_TIMESTAMP - INTERVAL '5 minutes'
		)) OR
		(%[1]sstatus='failed_retry' AND (%[1]snext_retry_at IS NULL OR %[1]snext_retry_at<=CURRENT_TIMESTAMP)) OR
		(%[1]sstatus='failed' AND %[1]ssend_phase='display_confirmed' AND EXISTS (
			SELECT 1 FROM %[2]s failed_display_message
			WHERE failed_display_message.job_id=%[1]sid
			  AND failed_display_message.status IN ('sent','undeletable')
			  AND failed_display_message.deleted_at IS NULL
		))
	)`, alias, publishTgMessageTable)
}

func telegramChannelAvailableAccountSQL(alias string) string {
	botAvailable := telegramJobAvailableBotSQL(alias)
	missingAccount := telegramChannelMissingAccountSQL(alias)
	if alias != "" {
		alias += "."
	}
	return fmt.Sprintf(`(%[5]s OR %[6]s OR EXISTS (SELECT 1 FROM %[2]s c INNER JOIN %[3]s a ON a.id=c.tg_account_id AND a.tenant_id=c.tenant_id AND a.status='authorized' AND NULLIF(BTRIM(a.session_key), '') IS NOT NULL AND a.deleted_at IS NULL WHERE c.id=%[1]schannel_id AND c.tenant_id=%[1]stenant_id AND c.status=1 AND c.deleted_at IS NULL) OR EXISTS (SELECT 1 FROM %[4]s tc INNER JOIN %[3]s a ON a.id=tc.tg_account_id AND a.tenant_id=tc.tenant_id AND a.status='authorized' AND NULLIF(BTRIM(a.session_key), '') IS NOT NULL AND a.deleted_at IS NULL WHERE tc.id=%[1]schannel_id AND tc.tenant_id=%[1]stenant_id))`, alias, publishChannelTable, publishTgAccountTable, publishTgChannelTable, botAvailable, missingAccount)
}

// Untouched jobs with broken references must reach the gate once, so it can
// expose a terminal configuration error instead of silently filtering them out.
func telegramChannelMissingAccountSQL(alias string) string {
	if alias != "" {
		alias += "."
	}
	return fmt.Sprintf(`(COALESCE(%[1]ssend_phase,'')='' AND %[1]sstatus IN ('pending','failed_retry') AND EXISTS (SELECT 1 FROM %[2]s missing_channel WHERE missing_channel.id=%[1]schannel_id AND missing_channel.tenant_id=%[1]stenant_id AND missing_channel.deleted_at IS NULL AND missing_channel.status=1 AND NOT EXISTS (SELECT 1 FROM %[3]s missing_account WHERE missing_account.id=missing_channel.tg_account_id AND missing_account.tenant_id=missing_channel.tenant_id AND missing_account.deleted_at IS NULL)))`, alias, publishChannelTable, publishTgAccountTable)
}

func telegramJobAvailableBotSQL(alias string) string {
	if alias != "" {
		alias += "."
	}
	return fmt.Sprintf(`(COALESCE(%[1]spush_mode,'') <> 'account' AND EXISTS (SELECT 1 FROM %[2]s available_bot WHERE available_bot.id=%[1]sbot_id AND available_bot.tenant_id IN (0,%[1]stenant_id) AND available_bot.status=1 AND available_bot.deleted_at IS NULL AND NULLIF(BTRIM(available_bot.bot_token),'') IS NOT NULL))`, alias, publishBotTable)
}

func telegramChannelOrderChatKey(chatID string) string {
	return strings.TrimPrefix(normalizeTelegramChannelChatID(chatID), "-100")
}

// Match both legacy raw IDs and normalized IDs without applying a function to
// target_chat_id, allowing PostgreSQL to use the existing B-Tree index.
func telegramChannelChatIDVariants(chatID string) []string {
	raw := strings.TrimSpace(chatID)
	if raw == "" {
		return nil
	}
	normalized := normalizeTelegramChannelChatID(raw)
	variants := []string{raw}
	if normalized != "" && normalized != raw {
		variants = append(variants, normalized)
	}
	key := telegramChannelOrderChatKey(raw)
	if key != "" && key != raw && key != normalized {
		variants = append(variants, key)
	}
	return variants
}

func telegramJobRequiresChannelCompletionPriority(sendPhase string) bool {
	return telegramSendPhaseHasDisplay(sendPhase) &&
		sendPhase != telegramSendPhaseVerifyConfirmed &&
		sendPhase != telegramSendPhaseCompletedNoVerify
}

// telegramChannelWakeDispatchKey 标记完成回调允许在短期限内等待调度锁，避免一次竞争丢掉下一条唤醒。
type telegramChannelWakeDispatchKey struct{}

func (s *sSysPublish) wakeNextTelegramChannelJob(ctx context.Context, job telegramJobRecord) error {
	// Completion callbacks often run while the worker task context is being
	// cancelled. The channel lease has already been released at this point, so
	// waking the durable successor must outlive that task context; otherwise a
	// cancelled callback leaves the next Outbox row idle until periodic recovery.
	wakeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	ctx = wakeCtx
	if s.accountPushEngine(ctx, job.TenantId, job.AccountId) == pushEngineUnifiedStream {
		ctx = context.WithValue(ctx, telegramChannelWakeDispatchKey{}, true)
	}
	if job.TenantId <= 0 || (job.ChannelId <= 0 && strings.TrimSpace(job.TargetChatId) == "") {
		return nil
	}
	// 完成回调、Stream 通知、Outbox 恢复可能同时唤醒同一频道。
	// 只保留一个短窗口内的唤醒，避免重复执行账户/队首查询和 Asynq 入队。
	// debounce 过期后仍由持久化 Outbox 继续保证可靠恢复。
	if cache.Initialized() {
		wakeKey := fmt.Sprintf("youban_publish:tg:wake:%d:%s", job.TenantId, telegramChannelDispatchKey(job))
		ok, err := cache.Instance().SetIfNotExist(ctx, wakeKey, 1, telegramChannelWakeDebounce)
		if err != nil {
			g.Log().Warningf(ctx, "频道唤醒去重缓存失败，继续执行唤醒 channelId:%d err:%+v", job.ChannelId, err)
		} else if !ok {
			return nil
		}
	}
	// 优先从频道 ZSET 读取队首，避免在 tg_job/outbox 历史记录上排序扫描。
	// ZSET 只做加速索引，命中后仍用状态条件校验；过期成员立即移除，
	// 再回退到 PostgreSQL 事实查询，保证 Redis 丢数据不会漏发任务。
	if job.ChannelId > 0 && cache.Initialized() {
		if headID, headErr := (tgChannelPendingIndex{}).head(ctx, job.TenantId, job.ChannelId); headErr == nil && headID > 0 {
			var head telegramJobRecord
			headErr = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
				Where("id", headID).
				WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
				Where("dispatch_status", tgDispatchStatusIdle).
				Where("next_retry_at IS NULL OR next_retry_at <= ?", gtime.Now()).
				Scan(&head)
			if headErr == nil && head.Id > 0 {
				if enqueueErr := s.enqueueTelegramJob(ctx, head.Id, 0); enqueueErr != nil {
					return gerror.Wrapf(enqueueErr, "通过频道索引唤醒TG任务失败 jobId:%d channelId:%d", head.Id, job.ChannelId)
				}
				return nil
			}
			_ = (tgChannelPendingIndex{}).remove(ctx, job.TenantId, job.ChannelId, headID)
		}
	}
	now := gtime.Now()
	var nextJobs []telegramJobRecord
	model := g.DB().Model(publishTgJobTable).As("j").Safe().Ctx(ctx).
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		Where(telegramWebhookDetachedJobSQL("")).
		Where("(dispatch_status = ? OR dispatch_status = '')", tgDispatchStatusIdle).
		Where("(next_retry_at IS NULL OR next_retry_at <= ?)", now).
		Order(telegramChannelScheduleOrderSQL("j")).
		Limit(telegramChannelActiveJobLimit)
	if strings.TrimSpace(job.TargetChatId) != "" {
		model = model.WhereIn("j.target_chat_id", telegramChannelChatIDVariants(job.TargetChatId))
	} else {
		model = model.Where("j.tenant_id", job.TenantId).Where("j.channel_id", job.ChannelId)
	}
	err := model.Scan(&nextJobs)
	if err != nil {
		return gerror.Wrap(err, "读取频道待入队TG任务失败")
	}
	for _, next := range nextJobs {
		if next.Id <= 0 {
			continue
		}
		if err = s.enqueueTelegramJob(ctx, next.Id, 0); err != nil {
			return gerror.Wrapf(err, "唤醒频道待入队TG任务失败 nextJobId:%d channelId:%d", next.Id, job.ChannelId)
		}
	}
	return nil
}

func telegramChannelDispatchKey(job telegramJobRecord) string {
	if key := telegramChannelOrderChatKey(job.TargetChatId); key != "" {
		return fmt.Sprintf("youban_publish:tg_destination_dispatch:%s", key)
	}
	return fmt.Sprintf("youban_publish:tg_channel_dispatch:%d:%d", job.TenantId, job.ChannelId)
}

func (s *sSysPublish) withTelegramChannelDispatchLease(ctx context.Context, job telegramJobRecord, reserve func(context.Context) (bool, error)) (bool, error) {
	if job.TenantId <= 0 || job.ChannelId <= 0 {
		return reserve(ctx)
	}
	reserveCtx, cancelReserve := context.WithTimeout(ctx, telegramChannelDispatchLeaseTTL-5*time.Second)
	defer cancelReserve()
	lease := hglock.NewConfig(telegramChannelDispatchLeaseTTL, 100*time.Millisecond).
		WithoutWatchDog().
		Mutex(telegramChannelDispatchKey(job))
	var lockErr error
	if wait, _ := ctx.Value(telegramChannelWakeDispatchKey{}).(bool); wait {
		// 只让新引擎完成回调等待；普通 Stream 事件仍快速失败并保留 pending。
		// reserveCtx 继承回调五秒预算，不能无限占用发送 Worker。
		lockErr = lease.Lock(reserveCtx)
	} else {
		lockErr = lease.TryLock(reserveCtx)
	}
	if err := lockErr; err != nil {
		if gerror.Is(err, hglock.ErrLockFailed) {
			// Lock contention or an unavailable Redis lock backend is not a
			// successful enqueue. Returning nil here silently stranded the
			// Outbox in pending/idle forever. Let the caller record the error,
			// release the lease, and retry with backoff.
			return false, gerror.Wrap(err, "获取频道任务调度租约失败，稍后重试")
		}
		return false, gerror.Wrap(err, "获取频道任务调度租约失败")
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if err := lease.Unlock(unlockCtx); err != nil && !gerror.Is(err, hglock.ErrNotExist) {
			g.Log().Warningf(unlockCtx, "释放频道任务调度租约失败 tenantId:%d channelId:%d err:%+v", job.TenantId, job.ChannelId, err)
		}
	}()
	return reserve(reserveCtx)
}

func (s *sSysPublish) telegramChannelHasDispatchCapacity(ctx context.Context, job telegramJobRecord) (bool, error) {
	model := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id <> ?", job.Id).
		Where(telegramWebhookDetachedJobSQL("")).
		Where(`(
			status = 'sending' OR
			(status IN ('pending', 'failed_retry', 'unknown') AND dispatch_status IN (?, ?))
		)`, tgDispatchStatusQueued, tgDispatchStatusProcessing).
		Fields("id").Limit(telegramChannelActiveJobLimit)
	// 新引擎手工任务可越过尚未开始的低优先级排队项；发送中或部分确认任务仍占用执行权。
	if telegramJobPriorityValue(job) == tgJobPriorityInteractive && s.accountPushEngine(ctx, job.TenantId, job.AccountId) == pushEngineUnifiedStream {
		model = model.Where("("+telegramChannelScheduleRankSQL("")+") <= ?", tgJobPriorityInteractive)
	}
	if strings.TrimSpace(job.TargetChatId) != "" {
		model = model.WhereIn("target_chat_id", telegramChannelChatIDVariants(job.TargetChatId))
	} else {
		model = model.Where("tenant_id", job.TenantId).Where("channel_id", job.ChannelId)
	}
	active, err := model.Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查频道活动TG任务失败")
	}
	return active < telegramChannelActiveJobLimit, nil
}

func (s *sSysPublish) postponeTelegramJobForChannelOrder(ctx context.Context, job telegramJobRecord) error {
	// 前序任务等待不是异常重试。使用固定短延迟，避免同频道正常串行发送
	// 因历史 dispatch_count 被放大到 30 秒，造成资料之间出现长时间空洞。
	// 锁竞争和 Redis/数据库错误仍使用独立的指数退避策略。
	retryDelay := telegramChannelOrderRetryDelay()
	nextRetryAt := gtime.Now().Add(retryDelay)
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		WhereIn("status", []string{"pending", "failed_retry"}).
		Data(g.Map{
			"dispatch_status": tgDispatchStatusIdle,
			// 顺序延期意味着本次 Asynq 执行已经结束；清除旧任务引用，允许恢复通知创建新的执行任务。
			"asynq_task_id": "",
			// task_id 是 PostgreSQL BIGINT。普通资料推送没有协议任务 ID，
			// 必须写 NULL，不能写空字符串，否则会触发
			// invalid input syntax for type bigint 并让频道队首反复重试。
			"task_id":             nil,
			"next_retry_at":       nextRetryAt,
			"last_dispatch_error": "等待同频道前序推送任务完成",
			"updated_at":          gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "延后TG频道顺序任务失败")
	}
	return s.rearmTelegramJobOutbox(ctx, job.Id, retryDelay)
}

// telegramChannelOrderRetryDelay 只用于等待同频道前序任务完成。
// 前序任务一旦完成，完成回调会立即唤醒下一条；定时重试仅作为兜底。
func telegramChannelOrderRetryDelay() time.Duration {
	return time.Second
}

func (s *sSysPublish) postponeTelegramJobForCollectPushPause(ctx context.Context, job telegramJobRecord) error {
	nextRetryAt := gtime.Now().Add(30 * time.Second)
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		WhereIn("status", []string{"pending", "failed_retry"}).
		Data(g.Map{
			"dispatch_status":     tgDispatchStatusIdle,
			"next_retry_at":       nextRetryAt,
			"last_dispatch_error": "采集推送已暂停，等待重新开启",
			"updated_at":          gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "延后采集推送任务失败")
	}
	if err = s.rearmTelegramJobOutbox(ctx, job.Id, 30*time.Second); err != nil {
		return err
	}
	s.appendTelegramJobLog(ctx, job, "publish", "paused", "采集推送已暂停，任务保留等待恢复")
	if wakeErr := s.wakeNextTelegramChannelJob(ctx, job); wakeErr != nil {
		g.Log().Warningf(ctx, "采集推送暂停后唤醒频道下一条TG任务失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, wakeErr)
	}
	return nil
}

// Ambiguous sends remain an ordering barrier until Webhook or account
// reconciliation confirms the message or moves the job to a terminal state.
func telegramWebhookDetachedJobSQL(_ string) string {
	return "1=1"
}
