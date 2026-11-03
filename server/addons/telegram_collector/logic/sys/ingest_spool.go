package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

const collectorIngestSpoolTable = "hg_tg_collector_ingest_spool"

// persistIngestSpool is the acknowledgement boundary for Telegram updates.
// The event table remains the business projection; this table is the durable
// handoff that can be replayed independently when a downstream worker fails.
func persistIngestSpool(ctx context.Context, event *sysin.RawUpdateEvent) (int64, error) {
	if event == nil || event.EventID == "" || event.TenantID <= 0 {
		return 0, fmt.Errorf("采集接收事件标识不完整")
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("序列化采集接收事件失败: %w", err)
	}
	now := gtime.Now()
	row, err := g.DB().GetOne(ctx, `INSERT INTO `+collectorIngestSpoolTable+`
 (tenant_id,event_key,source_type,source_id,account_id,payload,status,attempt_count,next_run_at,created_at,updated_at)
 VALUES (?,?,?,?,?,?, 'received',0,?,?,?)
 ON CONFLICT (tenant_id,event_key) DO UPDATE SET event_key=EXCLUDED.event_key
 RETURNING id`, event.TenantID, event.EventID, event.SourceType, event.SourceID,
		event.AccountID, payload, now, now, now)
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
	_, err := g.DB().Exec(ctx, "UPDATE "+collectorIngestSpoolTable+" SET status='completed',updated_at=? WHERE id=? AND status='received'", gtime.Now(), id)
	return err
}

// recoverIngestSpool replays accepted updates whose downstream Event handoff
// was interrupted. The unique event key makes replay idempotent.
func (s *sCollector) recoverIngestSpool(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	rows, err := g.DB().Model(collectorIngestSpoolTable).Ctx(ctx).
		WhereIn("status", []string{"received", "failed_retry"}).
		Where("next_run_at IS NULL OR next_run_at <= ?", gtime.Now()).
		OrderAsc("id").Limit(limit).All()
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		var event sysin.RawUpdateEvent
		decodeErr := json.Unmarshal([]byte(row["payload"].String()), &event)
		if decodeErr == nil && (event.EventID == "" || event.TenantID <= 0) {
			decodeErr = fmt.Errorf("采集接收事件标识不完整")
		}
		if decodeErr != nil {
			_, updateErr := g.DB().Exec(ctx, "UPDATE "+collectorIngestSpoolTable+" SET status='dead',last_error=?,updated_at=? WHERE id=?", decodeErr.Error(), gtime.Now(), row["id"].Int64())
			if updateErr != nil {
				return updateErr
			}
			continue
		}
		eventID, err := s.persistEvent(ctx, &event)
		if err == nil {
			err = s.enqueueEventTask(ctx, eventID, event.Priority)
		}
		if err == nil {
			_, err = g.DB().Model(collectorIngestSpoolTable).Ctx(ctx).Where("id", row["id"].Int64()).Data(g.Map{"status": "completed", "last_error": "", "updated_at": gtime.Now()}).Update()
			continue
		}
		_, _ = g.DB().Model(collectorIngestSpoolTable).Ctx(ctx).Where("id", row["id"].Int64()).Data(g.Map{"status": "failed_retry", "attempt_count": gdb.Raw("attempt_count + 1"), "next_run_at": gtime.Now().Add(30 * time.Second), "last_error": err.Error(), "updated_at": gtime.Now()}).Update()
	}
	return nil
}
