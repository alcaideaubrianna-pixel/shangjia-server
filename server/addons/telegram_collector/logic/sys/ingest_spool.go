package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

const collectorIngestSpoolTable = "hg_tg_collector_ingest_spool"

// collectorIngestSpoolEnabled keeps the legacy PostgreSQL acknowledgement
// spool available for migration/replay, while allowing the normal webhook
// path to persist the business event once and enqueue it directly to Redis.
// The default is redis_first because persistEvent already has the idempotent
// event key and Asynq provides the durable execution handoff.
func collectorIngestSpoolEnabled(ctx context.Context) bool {
	mode := g.Cfg().MustGet(ctx, "telegramCollector.ingestSpoolMode", "redis_first").String()
	return normalizeCollectorIngestSpoolMode(mode) == "postgres_spool"
}

func normalizeCollectorIngestSpoolMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "postgres", "postgres_spool", "legacy":
		return "postgres_spool"
	default:
		return "redis_first"
	}
}

// persistIngestSpool is the legacy PostgreSQL acknowledgement boundary for
// Telegram updates. It is used only when postgres_spool mode is explicitly
// enabled; the normal webhook path uses the idempotent event row plus Redis.
func persistIngestSpool(ctx context.Context, event *sysin.RawUpdateEvent) (int64, error) {
	if event == nil || event.EventID == "" || event.TenantID <= 0 {
		return 0, fmt.Errorf("采集接收事件标识不完整")
	}
	if err := markCollectorRecoveryScope(ctx, collectorRecoveryScope{event.TenantID, event.AccountID, event.SourceID}); err != nil {
		return 0, err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("序列化采集接收事件失败: %w", err)
	}
	now := gtime.Now()
	row, err := g.DB().GetOne(ctx, `INSERT INTO `+collectorIngestSpoolTable+`
 (tenant_id,event_key,source_type,source_id,account_id,priority,payload,status,attempt_count,next_run_at,created_at,updated_at)
 VALUES (?,?,?,?,?,?,?, 'received',0,?,?,?)
 ON CONFLICT (tenant_id,event_key) DO UPDATE SET event_key=EXCLUDED.event_key
 RETURNING id`, event.TenantID, event.EventID, event.SourceType, event.SourceID,
		event.AccountID, event.Priority, payload, now, now, now)
	if err != nil {
		return 0, fmt.Errorf("持久化采集接收事件失败: %w", err)
	}
	if row.IsEmpty() || row["id"].Int64() <= 0 {
		return 0, fmt.Errorf("读取采集接收事件ID失败")
	}
	return row["id"].Int64(), nil
}

func completeIngestSpool(ctx context.Context, id int64) error {
	if id <= 0 {
		return nil
	}
	_, err := g.DB().Exec(ctx, "UPDATE "+collectorIngestSpoolTable+" SET status='completed' WHERE id=? AND status='received'", id)
	return err
}

// recoverIngestSpool replays accepted updates whose downstream Event handoff
// was interrupted. The unique event key makes replay idempotent.
func (s *sCollector) recoverIngestSpool(ctx context.Context, limit int, scopes ...collectorRecoveryScope) error {
	if len(scopes) != 1 || !scopes[0].valid() {
		return fmt.Errorf("接收 Spool 恢复必须指定唯一有效 scope，禁止全局扫描")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	for i := 0; i < limit; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		row, err := claimIngestSpool(ctx, scopes[0])
		if err != nil || row.IsEmpty() {
			return err
		}
		var event sysin.RawUpdateEvent
		decodeErr := json.Unmarshal([]byte(row["payload"].String()), &event)
		if decodeErr == nil && (event.EventID == "" || event.TenantID <= 0) {
			decodeErr = fmt.Errorf("采集接收事件标识不完整")
		}
		if decodeErr != nil {
			if updateErr := finishIngestSpool(ctx, row, "dead", decodeErr.Error()); updateErr != nil {
				return updateErr
			}
			continue
		}
		eventID, err := s.persistEvent(ctx, &event)
		if err == nil {
			err = s.enqueueEventTask(ctx, eventID, event.Priority)
		}
		if err == nil {
			if err = finishIngestSpool(ctx, row, "completed", ""); err != nil {
				return err
			}
			continue
		}
		if updateErr := finishIngestSpool(ctx, row, "failed_retry", err.Error()); updateErr != nil {
			return updateErr
		}
	}
	return nil
}

// Claim one row at a time so a slow handoff cannot expire the leases of an
// entire batch. attempt_count is the fencing token for late worker results.
func claimIngestSpool(ctx context.Context, scopes ...collectorRecoveryScope) (gdb.Record, error) {
	if len(scopes) != 1 || !scopes[0].valid() {
		return nil, fmt.Errorf("接收 Spool 领取必须指定唯一有效 scope，禁止全局扫描")
	}
	scope := scopes[0]
	return g.DB().GetOne(ctx, `WITH ready AS (
 SELECT id, priority FROM `+collectorIngestSpoolTable+`
 WHERE tenant_id=? AND account_id=? AND source_id=? AND status IN ('received','failed_retry','processing') AND next_run_at IS NULL
 ORDER BY priority DESC,id FOR UPDATE SKIP LOCKED LIMIT 1
 ), due AS (
 SELECT id, priority FROM `+collectorIngestSpoolTable+`
 WHERE tenant_id=? AND account_id=? AND source_id=? AND status IN ('received','failed_retry','processing') AND next_run_at <= CURRENT_TIMESTAMP
 ORDER BY priority DESC,next_run_at,id FOR UPDATE SKIP LOCKED LIMIT 1
 ), selected AS (
 SELECT * FROM ready UNION ALL SELECT * FROM due
 ), winner AS (
 SELECT id FROM selected ORDER BY priority DESC,id LIMIT 1
 ) UPDATE `+collectorIngestSpoolTable+` AS spool
 SET status='processing',attempt_count=attempt_count+1,
 next_run_at=CURRENT_TIMESTAMP + INTERVAL '2 minutes',updated_at=CURRENT_TIMESTAMP
 FROM winner WHERE spool.id=winner.id RETURNING spool.*`, scope.TenantID, scope.AccountID, scope.SourceID, scope.TenantID, scope.AccountID, scope.SourceID)
}

func finishIngestSpool(ctx context.Context, row gdb.Record, status, lastError string) error {
	if status == "completed" || status == "dead" {
		_, err := g.DB().Exec(ctx, `UPDATE `+collectorIngestSpoolTable+`
 SET status=?,last_error=?,next_run_at=?
 WHERE id=? AND status='processing' AND attempt_count=?`,
			status, lastError, gtime.Now().Add(30*time.Second), row["id"].Int64(), row["attempt_count"].Int())
		return err
	}
	_, err := g.DB().Exec(ctx, `UPDATE `+collectorIngestSpoolTable+`
 SET status=?,last_error=?,next_run_at=?,updated_at=?
 WHERE id=? AND status='processing' AND attempt_count=?`,
		status, lastError, gtime.Now().Add(30*time.Second), gtime.Now(), row["id"].Int64(), row["attempt_count"].Int())
	return err
}
