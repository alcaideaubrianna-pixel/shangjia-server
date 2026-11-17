package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	publishconsts "hotgo/addons/youban_publish/consts"
	pdao "hotgo/addons/youban_publish/internal/dao"
	pdo "hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	hglock "hotgo/internal/library/hgrds/lock"
)

const (
	collectBotMediaRecoveryPageSize   = 12
	collectBotMediaRecoveryCursorTTL  = 24 * time.Hour
	collectBotMediaRecoveryTimeLayout = "2006-01-02 15:04:05.000000"
)

// Keep timestamp text from PostgreSQL: GTime.String() can discard the
// microseconds needed to distinguish rows at a keyset boundary.
type collectBotMediaRecoveryCursor struct {
	Fence     string `json:"fence"`
	UpdatedAt string `json:"updatedAt"`
	ID        int64  `json:"id"`
}

func (c *collectBotMediaRecoveryCursor) begin(now time.Time) {
	if c.Fence == "" {
		c.Fence = now.Add(-collectEventRecoverAfter).Format(collectBotMediaRecoveryTimeLayout)
	}
}

func (c *collectBotMediaRecoveryCursor) finishPage(candidates, pageSize int) bool {
	if candidates >= pageSize {
		return false
	}
	*c = collectBotMediaRecoveryCursor{}
	return true
}

func (c collectBotMediaRecoveryCursor) validate() error {
	if c.Fence == "" && c.UpdatedAt == "" && c.ID == 0 {
		return nil
	}
	if _, err := time.Parse(collectBotMediaRecoveryTimeLayout, c.Fence); err != nil {
		return gerror.New("BOT媒体恢复游标时间上界无效")
	}
	if c.UpdatedAt == "" && c.ID == 0 {
		return nil
	}
	if _, err := time.Parse(collectBotMediaRecoveryTimeLayout, c.UpdatedAt); err != nil || c.ID <= 0 || c.UpdatedAt > c.Fence {
		return gerror.New("BOT媒体恢复游标位置无效")
	}
	return nil
}

func parseCollectBotMediaRecoveryCursor(data string) (cursor collectBotMediaRecoveryCursor, err error) {
	if err = json.Unmarshal([]byte(data), &cursor); err == nil {
		err = cursor.validate()
	}
	if err != nil {
		// Only the cached scheduling hint is disposable. Query/transport
		// errors and malformed candidate rows must not take this path.
		cursor = collectBotMediaRecoveryCursor{}
	}
	return
}

func collectBotMediaRecoveryPageSQL(scope publishAccountScope, cursor collectBotMediaRecoveryCursor, limit int, retryAt string) (string, []interface{}) {
	e, m := pdao.YoubanPublishCollectEvent.Columns(), pdao.YoubanPublishCollectEventMedia.Columns()
	// Payload construction uses received_at/created_at for Bulk classification.
	// Keep raw_text/media_json and other wide event data out of the page.
	fields := "e." + strings.Join([]string{
		e.Id, e.TenantId, e.AccountId, e.SourceId, e.TgAccountId,
		e.ReceivedAt, e.CreatedAt, e.Status, e.SourceType, e.UpdatedAt,
	}, ",e.")
	// These trusted enum literals also let a generic PostgreSQL plan prove
	// the partial recovery index predicate, without knowing bind values.
	statuses := []string{
		sysin.CollectEventStatusMediaPending, sysin.CollectEventStatusMediaReady,
		sysin.CollectEventStatusGroupCollect, sysin.CollectEventStatusPending, sysin.CollectEventStatusFailed,
	}
	args := []interface{}{scope.TenantID, scope.AccountID, scope.SourceID, cursor.Fence}
	// MATERIALIZED makes LIMIT an actual candidate boundary. Source/media
	// predicates cannot be pushed into the potentially very large event scan.
	query := "WITH candidates AS MATERIALIZED (SELECT " + fields + " FROM " + pdao.YoubanPublishCollectEvent.Table() + " e WHERE e." +
		e.TenantId + "=? AND e." + e.AccountId + "=? AND e." + e.SourceId + "=? AND e." +
		e.ProcessedAt + " IS NULL AND e." + e.Status + " IN ('" + strings.Join(statuses, "','") + "') AND e." + e.UpdatedAt + "<=CAST(? AS timestamp)"
	if cursor.ID > 0 {
		query += " AND (e." + e.UpdatedAt + ",e." + e.Id + ")>(CAST(? AS timestamp),?)"
		args = append(args, cursor.UpdatedAt, cursor.ID)
	}
	query += " ORDER BY e." + e.UpdatedAt + ",e." + e.Id + " LIMIT ?) SELECT " + fields + ",to_char(e." +
		e.UpdatedAt + ",'YYYY-MM-DD HH24:MI:SS.US') AS recovery_updated_at,(e." + e.SourceType + "=? AND " +
		enabledCollectSourceExistsSQL("e") + " AND EXISTS (SELECT 1 FROM " + pdao.YoubanPublishCollectEventMedia.Table() +
		" m WHERE m." + m.EventId + "=e." + e.Id + " AND m." + m.TenantId + "=e." + e.TenantId +
		" AND m." + m.AccountId + "=e." + e.AccountId + " AND m." + m.SourceId + "=e." + e.SourceId +
		" AND m." + m.CacheStatus + "=? AND (m." + m.NextRetryAt + " IS NULL OR m." + m.NextRetryAt +
		"<=CAST(? AS timestamp)))) AS recovery_eligible FROM candidates e ORDER BY e." + e.UpdatedAt + ",e." + e.Id
	args = append(args, min(collectBotMediaRecoveryPageSize, max(1, limit)), sysin.CollectSourceTypeBot, collectMediaCachePending, retryAt)
	return query, args
}

// A successful skip is progress too. On an enqueue error leave the cursor
// immediately before that event, even when earlier rows were skipped.
func walkCollectBotMediaRecoveryPage(cursor *collectBotMediaRecoveryCursor, rows gdb.Result, visit func(gdb.Record) error) (int, error) {
	advanced := 0
	for _, row := range rows {
		next := collectBotMediaRecoveryCursor{
			Fence: cursor.Fence, UpdatedAt: row["recovery_updated_at"].String(), ID: row[pdao.YoubanPublishCollectEvent.Columns().Id].Int64(),
		}
		if next.ID <= 0 || next.UpdatedAt == "" {
			return advanced, gerror.New("BOT媒体候选页缺少事件ID或更新时间")
		}
		if err := next.validate(); err != nil {
			return advanced, err
		}
		if cursor.ID > 0 && (next.UpdatedAt < cursor.UpdatedAt || (next.UpdatedAt == cursor.UpdatedAt && next.ID <= cursor.ID)) {
			return advanced, gerror.New("BOT媒体候选页游标未正向推进")
		}
		if row["recovery_eligible"].Bool() {
			if err := visit(row); err != nil {
				return advanced, err
			}
		}
		*cursor = next
		advanced++
	}
	return advanced, nil
}

func (s *sSysPublish) recoverBotCollectMediaPages(ctx context.Context, limit int, scope publishAccountScope) (resultErr error) {
	if !cache.Initialized() || g.Cfg().MustGet(ctx, "cache.adapter").String() != "redis" {
		return gerror.New("BOT媒体分页恢复需要共享Redis缓存")
	}
	member := fmt.Sprintf("%d:%d:%d", scope.TenantID, scope.AccountID, scope.SourceID)
	lease := hglock.NewConfig(15*time.Second, 50*time.Millisecond).WithoutWatchDog().
		Mutex(publishconsts.CollectBotMediaRecoveryLockKeyPrefix + member)
	if err := lease.TryLock(ctx); err != nil {
		if errors.Is(err, hglock.ErrLockFailed) {
			return nil
		}
		return gerror.Wrap(err, "获取BOT媒体分页恢复锁失败")
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		if err := lease.Unlock(cleanupCtx); err != nil {
			g.Log().Warning(cleanupCtx, "释放BOT媒体分页恢复锁失败", g.Map{"scope": member, "error": err.Error()})
		}
	}()
	key := publishconsts.CollectBotMediaRecoveryCursorKeyPrefix + member
	value, err := cache.Instance().Get(ctx, key)
	if err != nil {
		return gerror.Wrap(err, "读取BOT媒体恢复游标失败")
	}
	var cursor collectBotMediaRecoveryCursor
	if value != nil && !value.IsNil() {
		if cursor, err = parseCollectBotMediaRecoveryCursor(value.String()); err != nil {
			g.Log().Warning(ctx, "BOT媒体恢复缓存游标损坏，已重置调度位置", g.Map{
				"event": "collect.recovery.bot_media_cursor_reset", "tenantId": scope.TenantID,
				"accountId": scope.AccountID, "sourceId": scope.SourceID, "error": err.Error(),
			})
		}
	}
	cursor.begin(gtime.Now().Time)
	// Cleanup is independent of the exhausted query/enqueue budget. Refresh
	// checks the token before saving; a stale owner must never overwrite it.
	defer func() {
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		owned, saveErr := lease.Refresh(saveCtx)
		if saveErr == nil && !owned {
			saveErr = gerror.New("BOT媒体恢复游标租约已失效")
		}
		if saveErr == nil {
			var data []byte
			data, saveErr = json.Marshal(cursor)
			if saveErr == nil {
				saveErr = cache.Instance().Set(saveCtx, key, string(data), collectBotMediaRecoveryCursorTTL)
			}
		}
		if saveErr != nil {
			resultErr = errors.Join(resultErr, gerror.Wrap(saveErr, "保存BOT媒体恢复游标失败"))
		}
	}()
	for scanned := 0; scanned < limit; {
		if err = ctx.Err(); err != nil {
			return gerror.Wrap(err, "BOT媒体恢复预算已耗尽，保留下一轮处理")
		}
		pageSize := min(collectBotMediaRecoveryPageSize, limit-scanned)
		query, args := collectBotMediaRecoveryPageSQL(scope, cursor, pageSize, gtime.Now().Time.Format(collectBotMediaRecoveryTimeLayout))
		started := time.Now()
		rows, queryErr := g.DB().GetAll(ctx, query, args...)
		if queryErr != nil {
			return gerror.Wrap(queryErr, "读取待恢复BOT媒体候选页失败")
		}
		var eligible, skippedIneligible, reserved, skippedReserved, requeued int
		for _, row := range rows {
			if row["recovery_eligible"].Bool() {
				eligible++
			}
		}
		advanced, pageErr := walkCollectBotMediaRecoveryPage(&cursor, rows, func(row gdb.Record) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			payload := collectMediaQueuePayloadFromEvent(row)
			payload.Reason = "bot_recovery"
			if payload.EventId <= 0 || payload.TenantId <= 0 || payload.SourceId <= 0 {
				return nil
			}
			if !reserveCollectMediaRecovery(ctx, payload.EventId) {
				skippedReserved++
				return nil
			}
			// This is the existing reservation gate's allow count; the gate
			// deliberately also allows work when its Redis operation fails.
			reserved++
			queued, enqueueErr := s.enqueueCollectMediaCacheDeferred(ctx, payload, 0)
			if enqueueErr != nil {
				releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
				defer cancel()
				releaseCollectMediaRecovery(releaseCtx, payload.EventId)
				return gerror.Wrapf(enqueueErr, "BOT媒体事件重新入队失败，暂停本轮来源恢复 eventId:%d", payload.EventId)
			}
			if queued {
				e := pdao.YoubanPublishCollectEvent.Columns()
				status := row[e.Status].String()
				if _, updateErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).
					Where(e.Id, payload.EventId).Where(e.Status, status).
					WhereLTE(e.UpdatedAt, cursor.Fence).
					Data(pdo.YoubanPublishCollectEvent{Status: status}).Update(); updateErr != nil {
					g.Log().Warningf(ctx, "更新BOT媒体事件恢复心跳失败 eventId:%d err:%+v", payload.EventId, updateErr)
				}
				requeued++
			}
			return nil
		})
		for _, row := range rows[:advanced] {
			if !row["recovery_eligible"].Bool() {
				skippedIneligible++
			}
		}
		wrapped := pageErr == nil && len(rows) < pageSize
		g.Log().Info(ctx, "BOT媒体恢复候选页完成", g.Map{
			"event": "collect.recovery.bot_media_page", "tenantId": scope.TenantID, "accountId": scope.AccountID,
			"sourceId": scope.SourceID, "candidates": len(rows), "advanced": advanced, "requeued": requeued,
			"eligible": eligible, "skipped_ineligible": skippedIneligible, "reserved": reserved, "skipped_reserved": skippedReserved,
			"fence": cursor.Fence, "cursorUpdatedAt": cursor.UpdatedAt, "cursorId": cursor.ID,
			"wrapped": wrapped, "failed": pageErr != nil, "durationMs": time.Since(started).Milliseconds(),
		})
		if pageErr != nil {
			return pageErr
		}
		scanned += len(rows)
		if cursor.finishPage(len(rows), pageSize) {
			// Revisit skipped media/source checks (including future retries)
			// on the next invocation, under a newly fixed upper fence.
			return nil
		}
	}
	return nil
}
