package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/utility/dbtime"
)

const collectProcessOutboxTable = "hg_youban_publish_collect_process_outbox"

const collectProcessOutboxLease = 5 * time.Minute
const collectProcessProcessingCap = 100

func collectProcessOutboxRecoveryBatchSize(ctx context.Context) int {
	batchSize := g.Cfg().MustGet(ctx, "youbanPublish.collect.processOutboxRecoveryBatchSize", 5000).Int()
	if batchSize < 100 {
		return 100
	}
	if batchSize > 10000 {
		return 10000
	}
	return batchSize
}

func ensureCollectProcessOutbox(ctx context.Context, payload collectProcessQueuePayload, delay time.Duration) error {
	if payload.EventId <= 0 {
		return nil
	}
	availableAt := gtime.Now().Add(delay)
	var history any = payload.History
	if payload.Recovery {
		history = nil
	}
	args := []any{payload.EventId, payload.TenantId, payload.AccountId, payload.SourceId, payload.SourceChatId, availableAt, gtime.Now(), gtime.Now(), history}
	var query string
	if strings.EqualFold(strings.TrimSpace(g.DB().GetConfig().Type), "pgsql") {
		query = `INSERT INTO ` + collectProcessOutboxTable + `
            (event_id,tenant_id,account_id,source_id,source_chat_id,status,available_at,created_at,updated_at,history)
            VALUES ($1,$2,$3,$4,$5,'pending',$6,$7,$8,$9)
            ON CONFLICT (event_id) DO UPDATE SET tenant_id=EXCLUDED.tenant_id,account_id=EXCLUDED.account_id,
            source_id=EXCLUDED.source_id,source_chat_id=EXCLUDED.source_chat_id,status='pending',
            available_at=EXCLUDED.available_at,lease_owner='',lease_expires_at=NULL,last_error='',updated_at=EXCLUDED.updated_at,
             history=COALESCE(` + collectProcessOutboxTable + `.history,EXCLUDED.history)`
		if payload.Recovery && payload.OutboxOwner == "" {
			query += ` WHERE ` + collectProcessOutboxTable + `.status='delivered'`
		} else if !payload.Recovery {
			// 正常入队必须保持已完成事件的终态。重复收到媒体完成、验证完成或
			// webhook 通知时，不能把 delivered Outbox 重新改成 pending，
			// 否则会造成重复处理并放大采集压力。
			query += ` WHERE ` + collectProcessOutboxTable + `.status<>'delivered'`
		}
	} else {
		query = `INSERT INTO ` + collectProcessOutboxTable + `
            (event_id,tenant_id,account_id,source_id,source_chat_id,status,available_at,created_at,updated_at,history)
            VALUES (?,?,?,?,?,'pending',?,?,?,?) ON DUPLICATE KEY UPDATE tenant_id=VALUES(tenant_id),
			account_id=VALUES(account_id),source_id=VALUES(source_id),source_chat_id=VALUES(source_chat_id),
			history=COALESCE(history,VALUES(history)),
			status=IF(status='delivered',status,'pending'),
			available_at=IF(status='delivered',available_at,VALUES(available_at)),
			lease_owner=IF(status='delivered',lease_owner,''),
			lease_expires_at=IF(status='delivered',lease_expires_at,NULL),
			last_error=IF(status='delivered',last_error,''),updated_at=VALUES(updated_at)`
	}
	if _, err := g.DB().Exec(ctx, query, args...); err != nil {
		return gerror.Wrap(err, "保存采集处理Outbox失败")
	}
	return nil
}

func isMissingCollectProcessOutboxError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, collectProcessOutboxTable) && !strings.Contains(message, "column ") &&
		(strings.Contains(message, "does not exist") || strings.Contains(message, "doesn't exist") || strings.Contains(message, "undefined table") || strings.Contains(message, "no such table"))
}

func completeCollectProcessOutbox(ctx context.Context, payload collectProcessQueuePayload) {
	eventID := payload.EventId
	if eventID <= 0 || payload.OutboxOwner == "" {
		return
	}
	// Outbox 完成只代表处理函数返回，必须同时核对事件终态和分发记录，
	// 及时暴露“Outbox delivered 但事件仍停在中间状态”的采集链路故障。
	var event struct {
		Status string `json:"status"`
	}
	if err := g.DB().Model(publishCollectEventTable).Ctx(ctx).Fields("status").Where("id", eventID).Scan(&event); err == nil {
		dispatchCount, countErr := g.DB().Model(publishCollectDispatchTable).Ctx(ctx).Where("event_id", eventID).Count()
		if countErr == nil {
			terminal := event.Status == sysin.CollectEventStatusProcessed || event.Status == sysin.CollectEventStatusDispatched || event.Status == sysin.CollectEventStatusIgnored || event.Status == sysin.CollectEventStatusFailed
			if !terminal && dispatchCount == 0 {
				g.Log().Warningf(ctx, "采集Outbox完成但事件未进入终态 eventId:%d status:%s dispatchCount:%d sourceId:%d accountId:%d sourceChatId:%s", eventID, event.Status, dispatchCount, payload.SourceId, payload.AccountId, payload.SourceChatId)
				// 处理函数提前返回但没有写入终态时，保留 Outbox 作为恢复入口，
				// 避免 delivered 掩盖业务事件永久停在 media_ready/pending。
				_, retryErr := g.DB().Model(collectProcessOutboxTable).Ctx(ctx).
					Where("event_id", eventID).Where("status", "processing").Where("lease_owner", payload.OutboxOwner).
					Data(g.Map{"status": "pending", "lease_owner": "", "lease_expires_at": nil,
						"available_at": gtime.Now().Add(10 * time.Second), "last_error": "业务事件未进入终态，自动恢复重试",
						"attempt_count": gdbRawIncrement(), "updated_at": gtime.Now()}).Update()
				if retryErr != nil {
					g.Log().Warningf(ctx, "采集Outbox异常收口后重新入队失败 eventId:%d err:%+v", eventID, retryErr)
				}
				return
			}
		}
	}
	if _, err := g.DB().Model(collectProcessOutboxTable).Ctx(ctx).Where("event_id", eventID).Where("status", "processing").Where("lease_owner", payload.OutboxOwner).Data(g.Map{
		"status": "delivered", "lease_owner": "", "lease_expires_at": nil,
		"delivered_at": gtime.Now(), "updated_at": gtime.Now(),
	}).Update(); err != nil {
		g.Log().Warningf(ctx, "确认采集处理Outbox完成失败 eventId:%d err:%+v", eventID, err)
		return
	}
	identity := currentCollectWorkerIdentity(ctx)
	if _, err := g.DB().Model(collectProcessOutboxTable).Ctx(ctx).Where("event_id", eventID).Data(g.Map{
		"worker_role": identity.Role, "worker_instance_id": identity.InstanceID,
		"worker_revision": identity.Revision, "worker_image_digest": identity.ImageDigest,
		"worker_finished_at": gtime.Now(),
	}).Update(); err != nil {
		g.Log().Warningf(ctx, "记录采集处理Worker身份失败，不影响Outbox完成 eventId:%d err:%+v", eventID, err)
	}
}

func (s *sSysPublish) dispatchDueCollectProcessOutbox(ctx context.Context, limit int, scopes ...publishAccountScope) error {
	if limit <= 0 {
		limit = 500
	}
	now := gtime.Now()
	// 账户已有大量处理中任务时暂停继续领取，避免 Dispatcher 速度超过
	// Worker，导致所有 Outbox 长时间停在 processing，事件无法进入分发终态。
	if len(scopes) > 0 {
		processing, countErr := g.DB().Model(collectProcessOutboxTable).Ctx(ctx).
			Where("tenant_id", scopes[0].TenantID).Where("account_id", scopes[0].AccountID).
			Where("status", "processing").WhereGT("lease_expires_at", now).Count()
		if countErr != nil {
			return gerror.Wrap(countErr, "读取账户采集处理中任务数量失败")
		}
		if processing >= collectProcessProcessingCap {
			return nil
		}
		limit = min(limit, collectProcessProcessingCap-processing)
	}
	model := g.DB().Model(collectProcessOutboxTable).Ctx(ctx)
	if len(scopes) > 0 {
		model = model.Where("tenant_id", scopes[0].TenantID).Where("account_id", scopes[0].AccountID)
	}
	if strings.EqualFold(g.DB().GetConfig().Type, "pgsql") {
		model = model.Fields("*, updated_at::text AS claim_version")
	}
	// Reserve capacity for realtime first, then history and expired processing
	// work. The explicit history marker is authoritative: an old realtime
	// event must not be silently demoted merely because it waited in a backlog.
	cutoff := now.Add(-10 * time.Minute)
	freshLimit := max(1, limit*3/5)
	historyLimit := min(max(1, limit/5), limit-freshLimit)
	expiredLimit := limit - freshLimit - historyLimit
	load := func(status, dueColumn string, count int, condition string, args ...any) ([]gdb.Record, error) {
		if count <= 0 {
			return nil, nil
		}
		base := model.Clone().Where("status", status).WhereLTE(dueColumn, now)
		if condition != "" {
			base = base.Where(condition, args...)
		}
		return base.OrderAsc(dueColumn).OrderAsc("id").Limit(count).All()
	}
	// 账户内再按来源分片，避免一个来源的历史积压占满账户本轮配额。
	loadFair := func(status, dueColumn string, count int, condition string, args ...any) ([]gdb.Record, error) {
		if len(scopes) == 0 || count <= 1 {
			return load(status, dueColumn, count, condition, args...)
		}
		var sources []struct {
			SourceID int64 `orm:"source_id"`
		}
		// 不能在已带有 SELECT * 的 model 上追加 GROUP BY；PostgreSQL 会拒绝
		// SELECT * 与仅按 source_id 分组的查询，导致整个账户分片停止灌入队列。
		sourceModel := g.DB().Model(collectProcessOutboxTable).Ctx(ctx)
		if len(scopes) > 0 {
			sourceModel = sourceModel.Where("tenant_id", scopes[0].TenantID).Where("account_id", scopes[0].AccountID)
		}
		if err := sourceModel.Fields("source_id").Where("status", status).WhereLTE(dueColumn, now).
			Where("source_id > 0").Distinct().OrderAsc("source_id").Limit(min(32, count)).Scan(&sources); err != nil {
			g.Log().Warning(ctx, "采集Outbox来源分片读取失败", g.Map{"tenantId": func() int64 {
				if len(scopes) > 0 {
					return scopes[0].TenantID
				}
				return 0
			}(), "accountId": func() int64 {
				if len(scopes) > 0 {
					return scopes[0].AccountID
				}
				return 0
			}(), "status": status, "error": err.Error()})
			return nil, err
		}
		if len(scopes) > 0 && scopes[0].AccountID == 615 {
			g.Log().Info(ctx, "采集Outbox来源分片读取完成", g.Map{"tenantId": scopes[0].TenantID, "accountId": scopes[0].AccountID, "status": status, "sourceCount": len(sources), "limit": count})
		}
		if len(sources) <= 1 {
			return load(status, dueColumn, count, condition, args...)
		}
		perSource := max(1, count/len(sources))
		rows := make([]gdb.Record, 0, count)
		for _, source := range sources {
			base := model.Clone().Where("status", status).Where("source_id", source.SourceID).WhereLTE(dueColumn, now)
			if condition != "" {
				base = base.Where(condition, args...)
			}
			part, err := base.OrderAsc(dueColumn).OrderAsc("id").Limit(perSource).All()
			if err != nil {
				return nil, err
			}
			rows = append(rows, part...)
		}
		return rows, nil
	}
	rows, err := loadFair("pending", "available_at", freshLimit, "COALESCE(history,FALSE)=FALSE")
	if err == nil {
		var more []gdb.Record
		more, err = loadFair("pending", "available_at", historyLimit, "history=TRUE OR (history IS NULL AND created_at<?)", cutoff)
		rows = append(rows, more...)
	}
	if err == nil {
		var more []gdb.Record
		more, err = loadFair("processing", "lease_expires_at", expiredLimit, "")
		rows = append(rows, more...)
	}
	if err != nil {
		if isMissingCollectProcessOutboxError(err) {
			return nil
		}
		return gerror.Wrap(err, "读取到期采集处理Outbox失败")
	}
	// Resolve legacy classification in one bounded query, not one DB round
	// trip per queued event. This map is only a transient query projection.
	legacyIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row["history"].IsNil() {
			legacyIDs = append(legacyIDs, row["event_id"].Int64())
		}
	}
	legacyHistory := make(map[int64]bool, len(legacyIDs))
	if len(legacyIDs) > 0 {
		events, readErr := g.DB().Model(publishCollectEventTable).Ctx(ctx).Fields("id,received_at,created_at").WhereIn("id", legacyIDs).All()
		if readErr != nil {
			return readErr
		}
		for _, event := range events {
			received, created := event["received_at"].GTime(), event["created_at"].GTime()
			legacyHistory[event["id"].Int64()] = received != nil && created != nil && received.Before(created.Add(-5*time.Minute))
		}
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		history := row["history"].Bool()
		createdAt := dbtime.FromTimestamp(row["created_at"].GTime())
		old := createdAt != nil && time.Since(createdAt.Time) > 10*time.Minute
		// Legacy rows have no history marker. Compare database wall clocks, not
		// raw RFC3339 offsets; new ingress always persists the explicit marker.
		if row["history"].IsNil() {
			history = legacyHistory[row["event_id"].Int64()]
		}
		owner := row["lease_owner"].String()
		if owner == "" {
			owner = "realtime-" + guid.S()
			if history || old {
				owner = "recovery-" + guid.S()
			}
		}
		claimModel := g.DB().Model(collectProcessOutboxTable).Ctx(ctx).Where("id", row["id"].Int64())
		if version, ok := row["claim_version"]; ok {
			claimModel = claimModel.Where("updated_at::text = ?", version.String())
		}
		claimed, claimErr := claimModel.
			Where("status = ? OR (status = ? AND lease_expires_at <= ?)", "pending", "processing", now).
			Data(g.Map{"status": "processing", "lease_owner": owner, "lease_expires_at": now.Add(collectProcessOutboxLease), "updated_at": gtime.Now()}).Update()
		if claimErr != nil {
			g.Log().Warningf(ctx, "领取采集处理Outbox失败 id:%d err:%+v", row["id"].Int64(), claimErr)
			continue
		}
		affected, _ := claimed.RowsAffected()
		if affected == 0 {
			continue
		}
		payload := collectProcessQueuePayload{
			OutboxOwner: owner,
			History:     history,
			Recovery:    strings.HasPrefix(owner, "recovery-") || old,
			EventId:     row["event_id"].Int64(), TenantId: row["tenant_id"].Int64(), AccountId: row["account_id"].Int64(),
			SourceId: row["source_id"].Int64(), SourceChatId: row["source_chat_id"].String(),
		}
		queued, queueErr := s.enqueueCollectProcessTaskWithOutbox(ctx, payload, 0, false, false)
		if row["account_id"].Int64() == 615 {
			queueError := ""
			if queueErr != nil {
				queueError = queueErr.Error()
			}
			g.Log().Info(ctx, "采集Outbox入队结果", g.Map{"outboxId": row["id"].Int64(), "eventId": payload.EventId, "tenantId": payload.TenantId, "accountId": payload.AccountId, "sourceId": payload.SourceId, "queued": queued, "error": queueError, "queue": collectProcessQueueName(payload)})
		}
		if queueErr == nil && queued {
			// Keep the row leased until the worker marks it delivered. Updating it
			// back to pending here can overwrite a fast worker's delivered state
			// and cause a duplicate recovery dispatch.
			continue
		}
		if queueErr == nil && !queued {
			// Stable task ID already exists. Keep its owner valid while queued.
			continue
		}
		message := "任务未入队"
		if queueErr != nil {
			message = queueErr.Error()
		}
		_, _ = g.DB().Model(collectProcessOutboxTable).Ctx(ctx).Where("id", row["id"].Int64()).Where("lease_owner", owner).
			Data(g.Map{"status": "pending", "lease_owner": "", "lease_expires_at": nil, "attempt_count": gdbRawIncrement(), "last_error": message, "available_at": gtime.Now().Add(time.Minute), "updated_at": gtime.Now()}).Update()
	}
	return nil
}

func gdbRawIncrement() any { return gdb.Raw("attempt_count + 1") }
