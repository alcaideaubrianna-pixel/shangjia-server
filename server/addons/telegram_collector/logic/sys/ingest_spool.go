package sys

import (
	"context"
	"encoding/json"
	"fmt"

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
	result, err := g.DB().Exec(ctx, `INSERT INTO `+collectorIngestSpoolTable+`
 (tenant_id,event_key,source_type,source_id,account_id,payload,status,attempt_count,next_run_at,created_at,updated_at)
 VALUES (?,?,?,?,?,?, 'received',0,?,?,?)
 ON CONFLICT (tenant_id,event_key) DO UPDATE SET payload=EXCLUDED.payload,
 source_type=EXCLUDED.source_type,source_id=EXCLUDED.source_id,account_id=EXCLUDED.account_id,
 status=CASE WHEN `+collectorIngestSpoolTable+`.status='completed' THEN `+collectorIngestSpoolTable+`.status ELSE 'received' END,
 updated_at=EXCLUDED.updated_at`, event.TenantID, event.EventID, event.SourceType, event.SourceID,
		event.AccountID, payload, now, now, now)
	if err != nil {
		return 0, fmt.Errorf("持久化采集接收事件失败: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil || id <= 0 {
		row, readErr := g.DB().GetOne(ctx, "SELECT id FROM "+collectorIngestSpoolTable+" WHERE tenant_id=? AND event_key=?", event.TenantID, event.EventID)
		if readErr != nil || row.IsEmpty() {
			return 0, fmt.Errorf("读取采集接收事件ID失败: %w", readErr)
		}
		id = row["id"].Int64()
	}
	return id, nil
}

func completeIngestSpool(ctx context.Context, id int64) error {
	if id <= 0 {
		return nil
	}
	_, err := g.DB().Exec(ctx, "UPDATE "+collectorIngestSpoolTable+" SET status='completed',updated_at=? WHERE id=? AND status='received'", gtime.Now(), id)
	return err
}
