package sys

import (
	"context"
	"github.com/gogf/gf/v2/util/guid"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const collectProcessOutboxTable = "hg_youban_publish_collect_process_outbox"

const collectProcessOutboxLease = 5 * time.Minute

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
	args := []any{payload.EventId, payload.TenantId, payload.AccountId, payload.SourceId, payload.SourceChatId, availableAt, gtime.Now(), gtime.Now()}
	var query string
	if strings.EqualFold(strings.TrimSpace(g.DB().GetConfig().Type), "pgsql") {
		query = `INSERT INTO ` + collectProcessOutboxTable + `
            (event_id,tenant_id,account_id,source_id,source_chat_id,status,available_at,created_at,updated_at)
            VALUES ($1,$2,$3,$4,$5,'pending',$6,$7,$8)
            ON CONFLICT (event_id) DO UPDATE SET tenant_id=EXCLUDED.tenant_id,account_id=EXCLUDED.account_id,
            source_id=EXCLUDED.source_id,source_chat_id=EXCLUDED.source_chat_id,status='pending',
            available_at=EXCLUDED.available_at,lease_owner='',lease_expires_at=NULL,last_error='',updated_at=EXCLUDED.updated_at`
		if payload.Recovery && payload.OutboxOwner == "" {
			query += ` WHERE ` + collectProcessOutboxTable + `.status='delivered'`
		}
	} else {
		query = `INSERT INTO ` + collectProcessOutboxTable + `
            (event_id,tenant_id,account_id,source_id,source_chat_id,status,available_at,created_at,updated_at)
            VALUES (?,?,?,?,?,'pending',?,?,?) ON DUPLICATE KEY UPDATE tenant_id=VALUES(tenant_id),
            account_id=VALUES(account_id),source_id=VALUES(source_id),source_chat_id=VALUES(source_chat_id),
            status='pending',available_at=VALUES(available_at),lease_owner='',lease_expires_at=NULL,last_error='',updated_at=VALUES(updated_at)`
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
	return strings.Contains(message, "does not exist") || strings.Contains(message, "doesn't exist") ||
		strings.Contains(message, "undefined table") || strings.Contains(message, "no such table")
}

func completeCollectProcessOutbox(ctx context.Context, payload collectProcessQueuePayload) {
	eventID := payload.EventId
	if eventID <= 0 || payload.OutboxOwner == "" {
		return
	}
	if _, err := g.DB().Model(collectProcessOutboxTable).Ctx(ctx).Where("event_id", eventID).Where("status", "processing").Where("lease_owner", payload.OutboxOwner).Data(g.Map{
		"status": "delivered", "lease_owner": "", "lease_expires_at": nil,
		"delivered_at": gtime.Now(), "updated_at": gtime.Now(),
	}).Update(); err != nil {
		g.Log().Warningf(ctx, "确认采集处理Outbox完成失败 eventId:%d err:%+v", eventID, err)
	}
}

func (s *sSysPublish) dispatchDueCollectProcessOutbox(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 500
	}
	now := gtime.Now()
	model := g.DB().Model(collectProcessOutboxTable).Ctx(ctx)
	if strings.EqualFold(g.DB().GetConfig().Type, "pgsql") {
		model = model.Fields("*, updated_at::text AS claim_version")
	}
	model = model.Where("available_at <= ?", now).
		Where("status = ? OR (status = ? AND lease_expires_at <= ?)", "pending", "processing", now)
	// Keep most dispatch capacity for fresh events while a historical backlog is
	// being recovered. This protects live collection latency; the remaining
	// capacity still guarantees bounded progress for older rows.
	cutoff := now.Add(-10 * time.Minute)
	realtimeLimit := limit * 4 / 5
	if realtimeLimit < 1 {
		realtimeLimit = 1
	}
	rows, err := model.Clone().WhereGTE("created_at", cutoff).OrderAsc("available_at").OrderAsc("id").Limit(realtimeLimit).All()
	if err == nil && len(rows) < limit {
		older, readErr := model.Clone().WhereLT("created_at", cutoff).OrderAsc("available_at").OrderAsc("id").Limit(limit - len(rows)).All()
		err = readErr
		rows = append(rows, older...)
	}
	if err != nil {
		if isMissingCollectProcessOutboxError(err) {
			return nil
		}
		return gerror.Wrap(err, "读取到期采集处理Outbox失败")
	}
	for _, row := range rows {
		owner := row["lease_owner"].String()
		if owner == "" {
			owner = "realtime-" + guid.S()
			if row["created_at"].GTime() != nil && time.Since(row["created_at"].Time()) > 10*time.Minute {
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
			Recovery:    strings.HasPrefix(owner, "recovery-") || (!strings.HasPrefix(owner, "realtime-") && row["created_at"].GTime() != nil && time.Since(row["created_at"].Time()) > 10*time.Minute),
			EventId:     row["event_id"].Int64(), TenantId: row["tenant_id"].Int64(), AccountId: row["account_id"].Int64(),
			SourceId: row["source_id"].Int64(), SourceChatId: row["source_chat_id"].String(),
		}
		queued, queueErr := s.enqueueCollectProcessTaskWithOutbox(ctx, payload, 0, false, false)
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
