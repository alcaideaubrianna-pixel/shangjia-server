package sys

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/cache"
	hglock "hotgo/internal/library/hgrds/lock"
)

const telegramOutboxFairScanPages = 4
const telegramOutboxFairSweepTTL = 24 * time.Hour

type telegramOutboxFairCursor struct {
	Fence string `json:"fence"`
	At    string `json:"at"`
	ID    int64  `json:"id"`
}

type telegramOutboxFairCandidate struct {
	telegramJobOutboxRecord
	// Read the database calendar fields without a timezone conversion or
	// precision loss. PostgreSQL timestamps can share the same microsecond.
	CursorAt string `json:"cursor_at"`
}

type telegramOutboxFairPage func(context.Context, telegramOutboxFairCursor, int) ([]telegramOutboxFairCandidate, error)

// scanTelegramOutboxFairPages has no database/cache dependency. Progress includes
// rejected candidates; quotas reset per invocation, not per page or sweep.
// An error discards all progress, so callers never persist a partial scan.
func scanTelegramOutboxFairPages(ctx context.Context, cursor telegramOutboxFairCursor, limit int, page telegramOutboxFairPage) ([]telegramJobOutboxRecord, telegramOutboxFairCursor, error) {
	original := cursor
	if limit <= 0 {
		return nil, original, gerror.New("Outbox公平扫描批次必须为正数")
	}
	type shard struct{ tenant, id int64 }
	accounts := make(map[shard]int)
	channels := make(map[shard]bool)
	var selected []telegramJobOutboxRecord
	pageSize := limit * 2
	for n := 0; n < telegramOutboxFairScanPages; n++ {
		if err := ctx.Err(); err != nil {
			return nil, original, err
		}
		rows, err := page(ctx, cursor, pageSize)
		if err != nil {
			return nil, original, err
		}
		for i, row := range rows {
			cursor.At, cursor.ID = row.CursorAt, row.Id
			account := shard{row.TenantId, row.AccountId}
			channel := shard{row.TenantId, row.ChannelId}
			if !channels[channel] && accounts[account] < max(1, limit/8) {
				channels[channel] = true
				accounts[account]++
				selected = append(selected, row.telegramJobOutboxRecord)
			}
			if len(selected) == limit {
				if i == len(rows)-1 && len(rows) < pageSize {
					cursor = telegramOutboxFairCursor{}
				}
				return selected, cursor, nil
			}
		}
		if len(rows) < pageSize {
			// Start a new fenced sweep on the NEXT invocation. Do not spend
			// the remainder of this invocation rereading skipped candidates.
			return selected, telegramOutboxFairCursor{}, nil
		}
	}
	return selected, cursor, nil
}

func (s *sSysPublish) fairTelegramStreamOutboxRows(ctx context.Context, limit int, now *gtime.Time) ([]telegramJobOutboxRecord, error) {
	startedAt := time.Now()
	if now == nil {
		now = gtime.Now()
	}
	ctx, cancel := context.WithTimeout(ctx, telegramOutboxQueryTimeout)
	defer cancel()
	if !cache.Initialized() || g.Cfg().MustGet(ctx, "cache.adapter").String() != "redis" {
		return nil, gerror.New("Outbox公平扫描要求共享Redis缓存")
	}
	lease := hglock.NewConfig(telegramOutboxLeaseTTL, 100*time.Millisecond).
		WithoutWatchDog().Mutex(publishconsts.TelegramOutboxFairScanLockKey)
	if err := lease.TryLock(ctx); err != nil {
		if gerror.Is(err, hglock.ErrLockFailed) {
			return nil, nil
		}
		return nil, err
	}
	defer func() {
		unlockCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer stop()
		if err := lease.Unlock(unlockCtx); err != nil {
			g.Log().Warningf(unlockCtx, "释放Outbox公平扫描租约失败: %+v", err)
		}
	}()
	var cursor telegramOutboxFairCursor
	value, err := cache.Instance().Get(ctx, publishconsts.TelegramOutboxFairScanCursorKey)
	if err != nil {
		return nil, err
	}
	if !value.IsNil() {
		if err := json.Unmarshal([]byte(value.String()), &cursor); err != nil {
			g.Log().Warning(ctx, "Outbox公平扫描游标损坏，重新开始有界扫描", g.Map{
				"event": "outbox_fair_cursor_reset", "reason": "invalid_json",
			})
			cursor = telegramOutboxFairCursor{}
		}
	}
	// Keep a live sweep's fence until reaching the tail. Resetting it by age
	// would repeatedly revisit a large prefix and starve the same tail again.
	nowText := now.Time.Format("2006-01-02 15:04:05.000000")
	if cursor.Fence != "" {
		fence, err := time.ParseInLocation("2006-01-02 15:04:05.000000", cursor.Fence, now.Time.Location())
		valid := err == nil && !fence.After(now.Time) && cursor.ID >= 0
		if cursor.ID > 0 {
			at, atErr := time.ParseInLocation("2006-01-02 15:04:05.000000", cursor.At, now.Time.Location())
			valid = valid && atErr == nil && !at.After(fence)
		} else {
			valid = valid && cursor.At == ""
		}
		if !valid {
			cursor = telegramOutboxFairCursor{}
		}
	}
	if cursor.Fence == "" {
		cursor = telegramOutboxFairCursor{Fence: nowText}
	}
	pages, scanned := 0, 0
	rows, next, err := scanTelegramOutboxFairPages(ctx, cursor, limit,
		func(ctx context.Context, after telegramOutboxFairCursor, pageSize int) ([]telegramOutboxFairCandidate, error) {
			pages++
			query, args := telegramOutboxFairScanSQL(after, nowText, pageSize, g.DB().GetConfig().Type)
			// The candidate embeds an unexported record type. Decode database
			// columns explicitly: reflective scanning can leave that embedded
			// record zero-valued while still populating cursor_at.
			records, err := g.DB().GetAll(ctx, query, args...)
			if err != nil {
				return nil, err
			}
			candidates, err := telegramOutboxFairCandidatesFromRecords(records)
			scanned += len(records)
			return candidates, err
		})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Reject a stale lease holder before committing progress. Renewal is bounded
	// to this invocation (no watchdog); delivery still owns job idempotency.
	if owned, err := lease.Refresh(ctx); err != nil || !owned {
		if err != nil {
			return nil, err
		}
		return nil, gerror.New("Outbox公平扫描租约已失效")
	}
	data, err := json.Marshal(next)
	if err != nil {
		return nil, err
	}
	if err := cache.Instance().Set(ctx, publishconsts.TelegramOutboxFairScanCursorKey, string(data), telegramOutboxFairSweepTTL); err != nil {
		return nil, err
	}
	if scanned > 0 {
		g.Log().Info(ctx, "TG Outbox公平扫描完成", g.Map{
			"event": "outbox_fair_scan", "pages": pages, "scanned": scanned,
			"selected": len(rows), "limit": limit, "cursorFromId": cursor.ID,
			"cursorToId": next.ID, "sweepFence": cursor.Fence,
			"wrapped": next.Fence == "", "durationMs": time.Since(startedAt).Milliseconds(),
		})
	}
	return rows, nil
}

func telegramOutboxFairScanSQL(after telegramOutboxFairCursor, nowText string, pageSize int, dbType string) (string, []interface{}) {
	cursorExpr := "DATE_FORMAT(o.available_at,'%Y-%m-%d %H:%i:%s.%f')"
	if strings.EqualFold(dbType, "pgsql") {
		cursorExpr = "to_char(o.available_at,'YYYY-MM-DD HH24:MI:SS.US')"
	}
	query := `SELECT o.id,o.job_id,o.attempt_count,o.status,o.available_at,` + cursorExpr + ` AS cursor_at,
		j.account_id,j.tenant_id,j.channel_id,j.priority
		FROM ` + publishTgOutboxTable + ` o JOIN ` + publishTgJobTable + ` j ON j.id=o.job_id
		WHERE j.tenant_id>0 AND j.channel_id>0
			AND o.event_type=? AND o.available_at<=?
			AND (o.status=? OR (o.status=? AND o.lease_expires_at<=?))
			AND j.status IN ('pending','failed_retry','unknown')
			AND (COALESCE(j.dispatch_status,'') IN ('','idle') OR (j.dispatch_status='queued' AND COALESCE(j.asynq_task_id,'')=''))
			AND (j.next_retry_at IS NULL OR j.next_retry_at<=? OR (j.status='pending' AND COALESCE(j.dispatch_status,'')='idle'))`
	args := []interface{}{telegramOutboxEventPublish, after.Fence, telegramOutboxPending, telegramOutboxProcessing, nowText, nowText}
	if after.ID > 0 {
		query += ` AND (o.available_at,o.id)>(?,?)`
		args = append(args, after.At, after.ID)
	}
	query += ` ORDER BY o.available_at,o.id LIMIT ?`
	args = append(args, pageSize)
	return query, args
}

// Decode the existing bounded query result without relying on reflection into
// an unexported embedded struct. Reject malformed rows before cursor progress
// is saved, so a mapping failure cannot skip work or dispatch job zero.
func telegramOutboxFairCandidatesFromRecords(records gdb.Result) ([]telegramOutboxFairCandidate, error) {
	candidates := make([]telegramOutboxFairCandidate, 0, len(records))
	for _, record := range records {
		row := telegramJobOutboxRecordFromRecord(record)
		cursorAt := record["cursor_at"].String()
		if row.Id <= 0 || row.JobId <= 0 || row.TenantId <= 0 || row.ChannelId <= 0 || cursorAt == "" {
			return nil, gerror.Newf("TG Outbox候选记录身份或游标不完整 outboxId:%d jobId:%d", row.Id, row.JobId)
		}
		candidates = append(candidates, telegramOutboxFairCandidate{telegramJobOutboxRecord: row, CursorAt: cursorAt})
	}
	return candidates, nil
}
