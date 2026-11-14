package sys

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	iservice "hotgo/internal/service"
)

// profilePublishSource is the current profile projection used by both job
// creation and workers. It is deliberately loaded from the profile tables and
// is never persisted as a task snapshot.
func (s *sSysPublish) profilePublishSource(ctx context.Context, profileId, tenantId, accountId int64, requireOnline bool) (gdb.Record, error) {
	mod := g.DB().Model(dao.ContentProfile.Table()+" p").Safe().Ctx(ctx).
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		LeftJoin(publishAccountTable+" a", "a.id=ps.account_id AND a.deleted_at IS NULL").
		Fields("p.id AS profile_id,p.profile_no,p.title,p.province,p.city,p.plain_text,p.status,p.visibility,"+
			"ps.tenant_id,ps.account_id,ps.customer_remark,ps.anti_scan_enabled,"+
			"a.nickname AS account_nickname,"+
			"(SELECT COUNT(1) FROM "+publishProfileStateTable+" ps_seq "+
			"WHERE ps_seq.tenant_id=ps.tenant_id AND ps_seq.account_id=ps.account_id "+
			"AND ps_seq.id<=ps.id AND ps_seq.deleted_at IS NULL) AS account_sequence").
		Where("p.id", profileId).
		WhereNull("p.deleted_at")
	if tenantId > 0 {
		mod = mod.Where("ps.tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("ps.account_id", accountId)
	}
	row, err := mod.One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取待发布资料失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("资料不存在、已删除或账号归属已变化")
	}
	if requireOnline && (row["status"].Int() != 1 || row["visibility"].String() != consts.ContentVisibilityPublic) {
		return nil, errPublishProfileUnavailable
	}
	return row, nil
}

func (s *sSysPublish) submitProfilePublish(ctx context.Context, profileId, tenantId, accountId, operatorId int64, operationNo string, channelIds []int64, requireOnline bool) error {
	return s.submitProfilePublishWithMeta(ctx, profileId, tenantId, accountId, operatorId, operationNo, channelIds, requireOnline, telegramProfilePublishMeta{})
}

type telegramProfilePublishMeta struct {
	CollectEventId         int64
	CollectSourceId        int64
	CollectSourceChatId    string
	CollectSourceMessageId int64
	InitialSendPhase       string
	InitialPriority        int // 历史采集等入口已确认的调度优先级。
}

var errTelegramPublishPermissionPaused = gerror.New("目标频道推送已因权限错误暂停，资料已保留，请恢复频道权限后主动重新上架")

func profilePublishPreparationError(channelCount int, jobIds []int64) error {
	if channelCount > 0 && len(jobIds) == 0 {
		return errTelegramPublishPermissionPaused
	}
	return nil
}

func (s *sSysPublish) submitProfilePublishWithMeta(ctx context.Context, profileId, tenantId, accountId, operatorId int64, operationNo string, channelIds []int64, requireOnline bool, meta telegramProfilePublishMeta) error {
	startedAt := time.Now()
	g.Log().Info(ctx, "资料发布准备开始", g.Map{
		"event":         "publish_prepare_start",
		"profileId":     profileId,
		"tenantId":      tenantId,
		"accountId":     accountId,
		"operatorId":    operatorId,
		"operationNo":   strings.TrimSpace(operationNo),
		"channelCount":  len(channelIds),
		"requireOnline": requireOnline,
	})
	if profileId > 0 {
		mediaCheckStartedAt := time.Now()
		ready, checkErr := s.profilePublishMediaReady(ctx, profileId)
		if checkErr != nil {
			return checkErr
		}
		// Bot creation persists the raw media operation before the worker writes
		// rows into hg_youban_publish_media. During that window the media table
		// is empty, which must mean "pending", not "text-only and ready".
		pending := false
		if ready {
			pending, checkErr = s.profileMediaOperationPending(ctx, tenantId, accountId, profileId)
			if checkErr != nil {
				return checkErr
			}
			ready = !pending
		}
		g.Log().Info(ctx, "资料发布媒体就绪检查完成", g.Map{
			"event":       "publish_media_ready",
			"profileId":   profileId,
			"tenantId":    tenantId,
			"accountId":   accountId,
			"operationNo": strings.TrimSpace(operationNo),
			"ready":       ready,
			"pending":     pending,
			"durationMs":  time.Since(mediaCheckStartedAt).Milliseconds(),
		})
		if !ready {
			failedMediaCount, countErr := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
				Where("profile_id", profileId).WhereNull("deleted_at").
				Where("processing_status", mediaProcessingFailed).Count()
			if countErr != nil {
				return gerror.Wrap(countErr, "检查资料媒体失败状态失败")
			}
			publishStatus := "waiting_media"
			if failedMediaCount > 0 {
				publishStatus = sysin.PublishTaskStatusFailed
			}
			if _, updateErr := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
				Where("tenant_id", tenantId).Where("account_id", accountId).Where("profile_id", profileId).WhereNull("deleted_at").
				Data(g.Map{"publish_task_status": publishStatus, "publish_operation_no": strings.TrimSpace(operationNo), "publish_task_updated_at": gtime.Now()}).Update(); updateErr != nil {
				return gerror.Wrap(updateErr, "记录资料媒体等待状态失败")
			}
			if failedMediaCount > 0 {
				g.Log().Warningf(ctx, "资料上架因媒体失败停止等待 profileId:%d operationNo:%s failed_media:%d", profileId, operationNo, failedMediaCount)
			} else {
				g.Log().Infof(ctx, "资料上架等待媒体完成 profileId:%d operationNo:%s", profileId, operationNo)
				if enqueueErr := s.enqueueWaitingMediaWake(ctx, profileId, tenantId, accountId); enqueueErr != nil {
					g.Log().Warningf(ctx, "等待媒体延迟唤醒任务入队失败 profileId:%d err:%+v", profileId, enqueueErr)
				}
				// Close the race where the media operation commits between the
				// readiness check above and this state update. The media worker may
				// already have attempted its wakeup before waiting_media existed;
				// re-check synchronously after recording the durable intent.
				if wakeErr := s.wakeProfileTelegramJobs(ctx, profileId); wakeErr != nil {
					g.Log().Warningf(ctx, "记录等待媒体状态后立即收敛失败 profileId:%d err:%+v", profileId, wakeErr)
				}
			}
			return nil
		}
	}
	var jobIds []int64
	var err error
	// 新引擎自动循环已有事务幂等和发送前资料校验，不再争用资料级Redis锁阻塞兄弟频道。
	if profileId <= 0 || (isCycleBatchOperation(operationNo) && s.accountPushEngine(ctx, tenantId, accountId) == pushEngineUnifiedStream) {
		jobIds, err = s.prepareProfilePublishJobs(ctx, profileId, tenantId, accountId, operationNo, channelIds, requireOnline, meta)
	} else {
		err = s.withProfileLifecycleLock(ctx, tenantId, profileId, func() error {
			jobIds, err = s.prepareProfilePublishJobs(ctx, profileId, tenantId, accountId, operationNo, channelIds, requireOnline, meta)
			return err
		})
	}
	if err != nil {
		return err
	}
	g.Log().Info(ctx, "资料发布任务创建完成", g.Map{
		"event":       "publish_job_created",
		"profileId":   profileId,
		"tenantId":    tenantId,
		"accountId":   accountId,
		"operationNo": strings.TrimSpace(operationNo),
		"jobCount":    len(jobIds),
		"jobIds":      jobIds,
		"durationMs":  time.Since(startedAt).Milliseconds(),
	})
	s.enqueuePreparedProfileJobs(ctx, jobIds, operatorId)
	return nil
}

func (s *sSysPublish) profileMediaOperationPending(ctx context.Context, tenantId, accountId, profileId int64) (bool, error) {
	if tenantId <= 0 || accountId <= 0 || profileId <= 0 {
		return false, nil
	}
	var profileNo string
	// Tenant/account ownership is stored in profile_state; content_profile
	// only contains the profile identity and must not be filtered by columns it
	// does not own.
	row, err := g.DB().Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).
		Fields("profile_no").Where("id", profileId).WhereNull("deleted_at").One()
	if err != nil {
		return false, gerror.Wrap(err, "读取资料媒体任务编号失败")
	}
	profileNo = strings.TrimSpace(row["profile_no"].String())
	if profileNo == "" {
		return false, nil
	}
	count, err := g.DB().Model(profileMediaOperationTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).Where("account_id", accountId).Where("profile_no", profileNo).
		WhereIn("status", profileMediaOperationPendingStatuses()).Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查资料媒体任务状态失败")
	}
	return count > 0, nil
}

func profileMediaOperationPendingStatuses() []string {
	return []string{"pending", "processing"}
}

func (s *sSysPublish) profilePublishMediaReady(ctx context.Context, profileID int64) (bool, error) {
	// Keep the submit path and the worker on one readiness predicate. Divergent
	// checks previously let submit create jobs while the media rows were still
	// being inserted, and the worker could then mistake the empty snapshot for
	// an intentional text-only profile.
	return s.profileMediaReady(ctx, profileID)
}

// prepareProfilePublishJobs 一次读取资料与频道，事务内写入任务和Outbox；Redis投递在提交后执行。
func (s *sSysPublish) prepareProfilePublishJobs(ctx context.Context, profileId, tenantId, accountId int64, operationNo string, channelIds []int64, requireOnline bool, meta telegramProfilePublishMeta) ([]int64, error) {
	source, err := s.profilePublishSource(ctx, profileId, tenantId, accountId, requireOnline)
	if err != nil {
		return nil, err
	}
	requestedChannelIds := uniqueIds(channelIds)
	var channels []telegramJobChannel
	if len(requestedChannelIds) == 0 {
		channels, err = s.telegramJobChannels(ctx, source)
	} else {
		channels, err = s.telegramJobChannels(ctx, source, requestedChannelIds)
	}
	if err != nil {
		return nil, err
	}
	resolvedChannelIds := make([]int64, 0, len(channels))
	for _, channel := range channels {
		resolvedChannelIds = append(resolvedChannelIds, channel.Id)
	}
	// 频道解析发生在每次资料上架入口，批量/采集场景使用 Debug 避免日志成为吞吐瓶颈。
	g.Log().Debugf(ctx, "资料发布频道解析 profileId:%d tenantId:%d accountId:%d requestedChannelIds:%v resolvedChannelIds:%v explicit:%t", profileId, tenantId, accountId, requestedChannelIds, resolvedChannelIds, len(channelIds) > 0)
	operationNo = strings.TrimSpace(operationNo)
	if operationNo == "" {
		operationNo = newTelegramOperationNo("profile", profileId)
	}
	jobIds := make([]int64, 0, len(channels))
	streamEngine := s.accountPushEngine(ctx, source["tenant_id"].Int64(), source["account_id"].Int64()) == pushEngineUnifiedStream
	// Creation locks channel circuit rows; always acquire them in ID order.
	sort.Slice(channels, func(i, j int) bool { return channels[i].Id < channels[j].Id })
	if streamEngine {
		var existing []struct {
			ID        int64 `orm:"id"`
			ChannelID int64 `orm:"channel_id"`
		}
		if err := g.DB().Model(publishTgJobTable).Ctx(ctx).Where("tenant_id", source["tenant_id"].Int64()).Where("account_id", source["account_id"].Int64()).Where("profile_id", profileId).Where("operation_no", operationNo).Fields("id,channel_id").Scan(&existing); err != nil {
			return nil, err
		}
		byChannel := make(map[int64]int64, len(existing))
		for _, job := range existing {
			byChannel[job.ChannelID] = job.ID
		}
		for _, channel := range channels {
			if id := byChannel[channel.Id]; id > 0 {
				jobIds = append(jobIds, id)
			}
		}
		// 同一操作完整重试不重开资料状态，也不把已经成功的记录改回pending。
		if len(channels) > 0 && len(jobIds) == len(channels) {
			return jobIds, nil
		}
		jobIds = jobIds[:0]
	}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if txErr := s.beginProfilePublishOperationTx(ctx, tx, source["tenant_id"].Int64(), source["account_id"].Int64(), profileId, operationNo); txErr != nil {
			return txErr
		}
		for _, channel := range channels {
			var jobID int64
			var createErr error
			if streamEngine {
				// 使用框架嵌套事务隔离单频道配置/约束错误，源资料和操作状态只准备一次。
				createErr = g.DB().Transaction(ctx, func(ctx context.Context, channelTx gdb.TX) error {
					var err error
					jobID, err = s.prepareProfileChannelJobTx(ctx, channelTx, source, channel, operationNo, meta)
					return err
				})
				if createErr != nil {
					g.Log().Warningf(ctx, "准备频道任务失败，其他频道继续 profile:%d channel:%d err:%+v", profileId, channel.Id, createErr)
					jobID, createErr = s.recordProfileChannelPreparationFailureTx(ctx, tx, source, channel, operationNo, meta, createErr)
				}
			} else {
				jobID, createErr = s.prepareProfileChannelJobTx(ctx, tx, source, channel, operationNo, meta)
			}
			if createErr != nil {
				return createErr
			}
			if jobID > 0 {
				jobIds = append(jobIds, jobID)
			}
		}
		if len(channels) > 0 && len(jobIds) == 0 {
			return s.writeProfilePublishOperationStateTx(ctx, tx, source["tenant_id"].Int64(), source["account_id"].Int64(), profileId, operationNo, sysin.PublishTaskStatusFailed, true)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Report the skip only after committing the failed UI projection. Returning
	// this from the transaction would roll that projection back to pending.
	return jobIds, profilePublishPreparationError(len(channels), jobIds)
}

// enqueuePreparedProfileJobs 在事务提交后投递待执行任务，终态重试保持原状态并合并资料投影。
func (s *sSysPublish) enqueuePreparedProfileJobs(ctx context.Context, jobIds []int64, operatorId int64) {
	// A large manual publish batch can contain thousands of jobs. Persist all
	// jobs in the transaction, then wake only one head per channel; completion
	// callbacks continue the channel. This avoids one DB/Redis/lease round trip
	// per item while preserving strict per-channel ordering.
	wokenChannels := make(map[string]struct{})
	for _, jobId := range jobIds {
		job, readErr := s.telegramJobById(ctx, jobId)
		if readErr != nil {
			g.Log().Warningf(ctx, "读取资料待发送记录失败 jobId:%d err:%+v", jobId, readErr)
		} else if job.Status == telegramJobStatusSent || job.Status == telegramJobStatusSuperseded {
			continue
		} else if job.Status == telegramJobStatusFailed && s.accountPushEngine(ctx, job.TenantId, job.AccountId) == pushEngineUnifiedStream {
			if recordErr := s.upsertPublishJobRecord(ctx, job, "failed", job.ErrorMessage); recordErr != nil {
				g.Log().Warningf(ctx, "保存频道准备失败记录失败 job:%d err:%+v", job.Id, recordErr)
			}
			// 同操作重试可能曾创建通知；终态失败只收口通知，不进入发送队列。
			_, _ = g.DB().Model(publishTgOutboxTable).Ctx(ctx).Where("job_id", job.Id).Where("event_type", telegramOutboxEventPublish).Data(g.Map{"status": telegramOutboxDelivered, "last_error": "任务准备失败，无需投递"}).Update()
			if projectionErr := s.enqueueStreamProfileProjection(ctx, job); projectionErr != nil {
				g.Log().Warningf(ctx, "提交失败任务投影失败 job:%d err:%+v", job.Id, projectionErr)
			}
			continue
		} else if recordErr := s.upsertPublishJobRecord(ctx, job, "pending", ""); recordErr != nil {
			g.Log().Warningf(ctx, "保存资料待发送记录失败 jobId:%d err:%+v", jobId, recordErr)
		}
		channelKey := fmt.Sprintf("%d:%s", job.TenantId, telegramChannelDispatchKey(job))
		if _, ok := wokenChannels[channelKey]; ok {
			continue
		}
		wokenChannels[channelKey] = struct{}{}
		g.Log().Debug(ctx, "资料发布任务开始入队", g.Map{
			"event":       "publish_enqueue_start",
			"profileId":   job.ProfileId,
			"tenantId":    job.TenantId,
			"accountId":   job.AccountId,
			"channelId":   job.ChannelId,
			"jobId":       job.Id,
			"operationNo": job.OperationNo,
			"priority":    job.Priority,
			"queue":       telegramChannelQueueKey(job.ChannelId),
			"score":       telegramChannelQueueScore(job.Priority, time.Now(), job.Id),
		})
		enqueueStartedAt := time.Now()
		if err := s.dispatchTelegramJobOutboxByJob(ctx, jobId); err != nil {
			message := "Redis调度失败，等待数据库调度器恢复：" + err.Error()
			_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobId).
				Data(g.Map{"last_dispatch_error": message, "updated_at": gtime.Now()}).Update()
			g.Log().Warning(ctx, "资料TG任务入队失败，等待数据库调度器恢复", g.Map{
				"event":       "publish_enqueue_failed",
				"profileId":   job.ProfileId,
				"tenantId":    job.TenantId,
				"accountId":   job.AccountId,
				"channelId":   job.ChannelId,
				"jobId":       job.Id,
				"operationNo": job.OperationNo,
				"priority":    job.Priority,
				"queue":       telegramChannelQueueKey(job.ChannelId),
				"score":       telegramChannelQueueScore(job.Priority, time.Now(), job.Id),
				"durationMs":  time.Since(enqueueStartedAt).Milliseconds(),
				"operatorId":  operatorId,
				"reason":      telegramPublishLogError(err),
			})
		} else {
			g.Log().Info(ctx, "资料发布任务入队完成", g.Map{
				"event":       "publish_enqueue_success",
				"profileId":   job.ProfileId,
				"tenantId":    job.TenantId,
				"accountId":   job.AccountId,
				"channelId":   job.ChannelId,
				"jobId":       job.Id,
				"operationNo": job.OperationNo,
				"priority":    job.Priority,
				"queue":       telegramChannelQueueKey(job.ChannelId),
				"score":       telegramChannelQueueScore(job.Priority, time.Now(), job.Id),
				"durationMs":  time.Since(enqueueStartedAt).Milliseconds(),
				"reason":      "queued",
			})
		}
	}
}

func (s *sSysPublish) ensureTelegramProfileJob(ctx context.Context, source gdb.Record, channel telegramJobChannel, operationNo string) (int64, error) {
	return s.ensureTelegramProfileJobWithMeta(ctx, source, channel, operationNo, telegramProfilePublishMeta{})
}

func (s *sSysPublish) ensureTelegramProfileJobWithMeta(ctx context.Context, source gdb.Record, channel telegramJobChannel, operationNo string, meta telegramProfilePublishMeta) (int64, error) {
	var jobID int64
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var txErr error
		jobID, txErr = s.createPublishJobTx(ctx, tx, publishJobCreateRequest{
			Source:      source,
			Channel:     channel,
			OperationNo: operationNo,
			Meta:        meta,
		})
		return txErr
	})
	return jobID, err
}

// ensureTelegramProfileJobWithMetaTx 按账户、操作和频道幂等创建任务；新操作不复用另一操作的优先级。
func (s *sSysPublish) ensureTelegramProfileJobWithMetaTx(ctx context.Context, tx gdb.TX, source gdb.Record, channel telegramJobChannel, operationNo string, meta telegramProfilePublishMeta) (int64, error) {
	// Resolve collection source per target publish channel. A profile can be
	// matched by multiple rules/BOTs; using the profile-level latest source here
	// leaks one channel's suffix into another channel's message.
	var channelSource struct {
		SourceId int64 `orm:"source_id"`
	}
	if err := tx.Model(publishCollectDispatchTable+" d").Safe().Ctx(ctx).
		InnerJoin(collectDispatchChannelTable+" dc", "dc.dispatch_id=d.id AND dc.tenant_id=d.tenant_id AND dc.account_id=d.account_id").
		Where("d.profile_id", source["profile_id"].Int64()).Where("dc.channel_id", channel.Id).
		Fields("d.source_id").OrderDesc("d.id").Scan(&channelSource); err == nil && channelSource.SourceId > 0 {
		meta.CollectSourceId = channelSource.SourceId
	}
	paused, err := lockChannelPermissionCreationTx(ctx, tx, source["tenant_id"].Int64(), channel.Id)
	if err != nil || paused {
		return 0, err
	}
	existing, err := tx.Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", source["tenant_id"].Int64()).Where("account_id", source["account_id"].Int64()).
		Where("profile_id", source["profile_id"].Int64()).
		Where("operation_no", operationNo).
		Where("channel_id", channel.Id).
		Fields("id,status,bot_id,target_chat_id").One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取资料TG频道任务失败")
	}
	if jobId := existing["id"].Int64(); jobId > 0 {
		return jobId, nil
	}
	// A newer manual/full-push request replaces an older active operation for
	// the same profile/channel. The partial unique index intentionally allows
	// only one active job, so returning a raw duplicate-key error would make a
	// user retry appear to do nothing. Promote the existing job to the latest
	// operation; the caller will rearm its Outbox and enqueue it on the proper
	// realtime lane.
	if !isCycleBatchOperation(operationNo) {
		active, readErr := tx.Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("tenant_id", source["tenant_id"].Int64()).Where("account_id", source["account_id"].Int64()).
			Where("profile_id", source["profile_id"].Int64()).Where("channel_id", channel.Id).
			WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
			Fields("id,status").OrderDesc("id").One()
		if readErr != nil {
			return 0, gerror.Wrap(readErr, "读取资料活动TG频道任务失败")
		}
		if activeID := active["id"].Int64(); activeID > 0 {
			priority := meta.InitialPriority
			if priority <= 0 {
				priority = tgJobPriorityInteractive
			}
			if _, updateErr := tx.Model(publishTgJobTable).Safe().Ctx(ctx).
				Where("id", activeID).
				Data(g.Map{
					"operation_no":        operationNo,
					"operation_type":      string(profileOperationTypeFromNo(operationNo)),
					"priority":            priority,
					"queue_name":          telegramQueueNameByPriorityAndChannel(ctx, priority, channel.Id),
					"dispatch_status":     tgDispatchStatusIdle,
					"asynq_task_id":       "",
					"next_retry_at":       nil,
					"last_dispatch_error": "",
					"updated_at":          gtime.Now(),
				}).Update(); updateErr != nil {
				return 0, gerror.Wrap(updateErr, "提升资料活动TG任务失败")
			}
			return activeID, nil
		}
	}
	// Cycle scheduling must be idempotent across manual/full-push operations.
	// The active partial unique index is enforced by PostgreSQL, so checking
	// before INSERT avoids surfacing a raw duplicate-key error when a manual
	// push already owns this profile/channel. The sender/lease still serializes
	// any task that is actually executed.
	if isCycleBatchOperation(operationNo) {
		active, readErr := tx.Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("tenant_id", source["tenant_id"].Int64()).Where("account_id", source["account_id"].Int64()).
			Where("profile_id", source["profile_id"].Int64()).Where("channel_id", channel.Id).
			WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
			Fields("id").OrderDesc("id").One()
		if readErr != nil {
			return 0, gerror.Wrap(readErr, "读取资料活动TG频道任务失败")
		}
		if jobID := active["id"].Int64(); jobID > 0 {
			return jobID, nil
		}
	}
	// 新引擎按操作号幂等：手工与编辑必须保留独立操作和优先级，频道执行租约负责串行。
	// 旧引擎保留原来的活动任务合并，便于灰度回退。
	if s.accountPushEngine(ctx, source["tenant_id"].Int64(), source["account_id"].Int64()) != pushEngineUnifiedStream {
		active, err := tx.Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("tenant_id", source["tenant_id"].Int64()).Where("account_id", source["account_id"].Int64()).
			Where("profile_id", source["profile_id"].Int64()).Where("channel_id", channel.Id).
			WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
			Fields("id").OrderDesc("id").One()
		if err != nil {
			return 0, gerror.Wrap(err, "读取资料活动TG频道任务失败")
		}
		if jobID := active["id"].Int64(); jobID > 0 {
			return jobID, nil
		}
	}
	// The caller's snapshot may predate a Bot removal or channel edit. Read
	// transport configuration only after acquiring the shared creation gate.
	var current *telegramJobChannel
	if err := tx.Model(publishChannelTable).Safe().Ctx(ctx).Where("id", channel.Id).
		Where("tenant_id", source["tenant_id"].Int64()).Where("status", 1).WhereNull("deleted_at").
		Fields("id,bot_id_json,target_chat_id").Scan(&current); err != nil {
		return 0, gerror.Wrap(err, "读取当前TG频道发送配置失败")
	}
	if current == nil {
		return 0, gerror.New("目标频道已停用或删除，请刷新后重试")
	}
	channel = *current
	botId, err := telegramChannelSenderBotId(channel)
	if err != nil {
		return 0, err
	}
	now := gtime.Now()
	cycleData := g.Map{}
	if isCycleBatchOperation(operationNo) {
		var cycleConfig struct {
			Enabled     int    `orm:"cycle_publish_enabled"`
			Days        int    `orm:"cycle_publish_days"`
			PublishTime string `orm:"cycle_publish_time"`
		}
		if err := tx.Model(publishChannelTable).Safe().Ctx(ctx).
			Fields("cycle_publish_enabled,cycle_publish_days,cycle_publish_time").Where("id", channel.Id).Where("tenant_id", source["tenant_id"].Int64()).Scan(&cycleConfig); err != nil {
			return 0, gerror.Wrap(err, "读取当前频道循环配置失败")
		}
		cycleData = g.Map{"cycle_enabled": cycleConfig.Enabled, "cycle_days": defaultCycleDays(cycleConfig.Days), "cycle_publish_time": strings.TrimSpace(cycleConfig.PublishTime)}
	}
	job := telegramJobRecord{
		OperationNo: operationNo, TenantId: source["tenant_id"].Int64(), AccountId: source["account_id"].Int64(),
		ProfileId: source["profile_id"].Int64(), ChannelId: channel.Id, BotId: botId,
		TargetChatId: normalizeTelegramChannelChatID(channel.TargetChatId), Status: "pending", Priority: meta.InitialPriority,
		CollectEventId: meta.CollectEventId, CollectSourceId: meta.CollectSourceId,
		CollectSourceChatId: strings.TrimSpace(meta.CollectSourceChatId), CollectSourceMessageId: meta.CollectSourceMessageId,
	}
	jobData := g.Map{
		"operation_no":      operationNo,
		"operation_type":    string(profileOperationTypeFromNo(operationNo)),
		"notify_account_id": job.AccountId,
		"tenant_id":         job.TenantId, "merchant_id": job.TenantId, "account_id": job.AccountId,
		"profile_id": job.ProfileId, "channel_id": job.ChannelId, "bot_id": job.BotId,
		"target_chat_id": job.TargetChatId, "status": "pending",
		"collect_event_id": job.CollectEventId, "collect_source_id": job.CollectSourceId,
		"collect_source_chat_id": job.CollectSourceChatId, "collect_source_message_id": job.CollectSourceMessageId,
		"priority": s.telegramJobPriority(job), "queue_name": telegramQueueNameByPriorityAndChannel(ctx, s.telegramJobPriority(job), channel.Id),
		"dispatch_status": tgDispatchStatusIdle, "created_at": now, "updated_at": now,
	}
	for key, value := range cycleData {
		jobData[key] = value
	}
	jobID, err := tx.Model(publishTgJobTable).Safe().Ctx(ctx).Data(jobData).InsertAndGetId()
	if err != nil {
		if isUniqueConstraintError(err) {
			active, readErr := tx.Model(publishTgJobTable).Safe().Ctx(ctx).
				Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
				Where("profile_id", job.ProfileId).Where("channel_id", job.ChannelId).
				WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
				Fields("id").OrderDesc("id").One()
			if readErr == nil && active["id"].Int64() > 0 {
				return active["id"].Int64(), nil
			}
		}
		// Keep database details out of the API response. The framework would
		// otherwise serialize the wrapped PostgreSQL error (including SQL and
		// constraint names), which is an information disclosure risk.
		g.Log().Errorf(ctx, "创建资料TG频道任务失败 tenant:%d account:%d profile:%d channel:%d err:%+v", job.TenantId, job.AccountId, job.ProfileId, job.ChannelId, err)
		return 0, gerror.New("创建资料TG频道任务失败，请稍后重试")
	}
	return jobID, nil
}

// completeProfileTelegramOperation 汇总当前操作的频道结果，部分失败保持失败提示，不误报全部完成。
func (s *sSysPublish) completeProfileTelegramOperation(ctx context.Context, job telegramJobRecord, isCycle bool) (bool, error) {
	total, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).
		Where("operation_no", job.OperationNo).Count()
	if err != nil {
		return false, gerror.Wrap(err, "统计资料TG任务失败")
	}
	if total == 0 {
		return false, nil
	}
	streamEngine := s.accountPushEngine(ctx, job.TenantId, job.AccountId) == pushEngineUnifiedStream
	terminal := []string{"sent", "superseded"}
	if streamEngine {
		terminal = append(terminal, telegramJobStatusFailed)
	}
	pending, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).
		Where("operation_no", job.OperationNo).
		WhereNotIn("status", terminal).Count()
	if err != nil {
		return false, gerror.Wrap(err, "统计资料未完成TG任务失败")
	}
	if pending > 0 {
		return false, nil
	}
	sent, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).
		Where("operation_no", job.OperationNo).
		Where("status", telegramJobStatusSent).Count()
	if err != nil {
		return false, gerror.Wrap(err, "统计资料成功TG任务失败")
	}
	if sent == 0 {
		return false, s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed)
	}
	isCurrent, err := s.profilePublishOperationIsCurrent(ctx, job)
	if err != nil {
		return false, err
	}
	if !isCurrent {
		return false, nil
	}
	if !isCycle {
		now := gtime.Now()
		if _, err = s.syncProfilePublishState(ctx, job.ProfileId, 1, consts.ContentVisibilityPublic, now); err != nil {
			return false, gerror.Wrap(err, "同步资料上架状态失败")
		}
		iservice.SysContent().ClearHomeProfileCardsCache(ctx)
		if err = s.incrementDailyPublishStat(ctx, job); err != nil {
			return false, err
		}
	}
	if isCycle && s.accountPushEngine(ctx, job.TenantId, job.AccountId) != pushEngineUnifiedStream {
		s.cleanupPreviousCycleMessages(ctx, job)
	}
	if streamEngine {
		failed, err := g.DB().Model(publishTgJobTable).Ctx(ctx).Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).Where("profile_id", job.ProfileId).Where("operation_no", job.OperationNo).Where("status", telegramJobStatusFailed).Count()
		if err != nil {
			return false, err
		}
		if failed > 0 {
			return false, s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed)
		}
	}
	if err = s.clearProfilePublishOperationState(ctx, job); err != nil {
		return false, err
	}
	if err = s.syncProfileNoteIndex(ctx, job.ProfileId); err != nil {
		return false, err
	}
	return true, nil
}
