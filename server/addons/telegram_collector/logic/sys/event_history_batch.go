package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/microbatch"
)

type historyEventMutationKind uint8

const (
	historyEventClaim historyEventMutationKind = iota + 1
	historyEventReady
)

type historyEventMutation struct {
	Kind     historyEventMutationKind
	ID       int64
	TenantID int64
	Owner    string
	Attempt  int
}

func historyEventBatchEnabled(ctx context.Context) bool {
	return eventQueueIsHistory(collectorQueueName(ctx)) &&
		g.Cfg().MustGet(ctx, "telegramCollector.worker.historyBatchEnabled", true).Bool() &&
		g.DB().GetConfig().Type == "pgsql"
}

func historyEventBatchConfig(ctx context.Context) (int, time.Duration) {
	size := g.Cfg().MustGet(ctx, "telegramCollector.worker.historyBatchSize", 20).Int()
	if size < 1 {
		size = 1
	}
	if size > 50 {
		size = 50
	}
	wait := g.Cfg().MustGet(ctx, "telegramCollector.worker.historyBatchWaitMilliseconds", 10).Int()
	if wait < 0 {
		wait = 0
	}
	if wait > 50 {
		wait = 50
	}
	return size, time.Duration(wait) * time.Millisecond
}

func (s *sCollector) historyEventWriter(ctx context.Context) *microbatch.Batcher[historyEventMutation, entity.TgCollectorEvent] {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	if s.historyEventBatch == nil {
		size, wait := historyEventBatchConfig(ctx)
		parent := s.runtimeCtx
		if parent == nil {
			parent = context.Background()
		}
		s.historyEventBatch = microbatch.New(parent, size, wait, func(ctx context.Context, rows []historyEventMutation) []microbatch.Result[entity.TgCollectorEvent] {
			s.historyBatchSQLMu.Lock()
			defer s.historyBatchSQLMu.Unlock()
			return writeHistoryEventMutations(ctx, rows)
		})
	}
	return s.historyEventBatch
}

// Each statement uses explicit, bounded primary keys. No scope discovery or
// historical table scan is performed here. One writer serializes batch SQL.
func writeHistoryEventMutations(ctx context.Context, mutations []historyEventMutation) []microbatch.Result[entity.TgCollectorEvent] {
	out := make([]microbatch.Result[entity.TgCollectorEvent], len(mutations))
	for _, kind := range []historyEventMutationKind{historyEventClaim, historyEventReady} {
		var group []historyEventMutation
		var positions []int
		seen := make(map[int64]bool)
		for i, m := range mutations {
			if m.Kind != historyEventClaim && m.Kind != historyEventReady {
				out[i].Err = gerror.New("历史采集批次操作无效")
				continue
			}
			if m.Kind != kind {
				continue
			}
			if m.ID <= 0 || m.Owner == "" || (kind == historyEventReady && (m.TenantID <= 0 || m.Attempt <= 0)) {
				out[i].Err = gerror.New("历史采集批次归属无效")
				continue
			}
			// Exactly one caller may receive a successful claim for a duplicate ID.
			if seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			group = append(group, m)
			positions = append(positions, i)
		}
		if len(group) == 0 {
			continue
		}
		rows, err := executeHistoryEventMutation(ctx, kind, group)
		byID := make(map[int64]entity.TgCollectorEvent, len(rows))
		for _, row := range rows {
			byID[row.Id] = row
		}
		for j, pos := range positions {
			out[pos] = microbatch.Result[entity.TgCollectorEvent]{Value: byID[group[j].ID], Err: err}
		}
	}
	return out
}

func executeHistoryEventMutation(ctx context.Context, kind historyEventMutationKind, rows []historyEventMutation) ([]entity.TgCollectorEvent, error) {
	if len(rows) == 0 || len(rows) > 50 {
		return nil, gerror.New("历史采集批次必须包含1到50条记录")
	}
	c := dao.TgCollectorEvent.Columns()
	table := dao.TgCollectorEvent.Table()
	var values []string
	var args []any
	var query string
	now := "(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai')"
	switch kind {
	case historyEventClaim:
		for _, row := range rows {
			values = append(values, "(CAST(? AS bigint),CAST(? AS text))")
			args = append(args, row.ID, row.Owner)
		}
		query = fmt.Sprintf(`WITH input(event_id,owner) AS (VALUES %s), locked AS (
 SELECT e.%s FROM %s e JOIN input i ON e.%s=i.event_id
 WHERE e.%s IN ('%s','%s') ORDER BY e.%s FOR UPDATE OF e SKIP LOCKED)
 UPDATE %s e SET %s='%s', %s=i.owner, %s=%s+INTERVAL '%d seconds',
 %s=e.%s+1, %s=%s FROM input i, locked l
 WHERE e.%s=i.event_id AND e.%s=l.%s RETURNING e.*`,
			strings.Join(values, ","), c.Id, table, c.Id, c.Status, sysin.EventStatusReceived, sysin.EventStatusFailedRetry, c.Id,
			table, c.Status, sysin.EventStatusProcessing, c.LeaseOwner, c.LeaseUntil, now, int64(eventLeaseDuration/time.Second), c.AttemptCount, c.AttemptCount, c.UpdatedAt, now, c.Id, c.Id, c.Id)
	case historyEventReady:
		for _, row := range rows {
			values = append(values, "(CAST(? AS bigint),CAST(? AS bigint),CAST(? AS text),CAST(? AS integer))")
			args = append(args, row.ID, row.TenantID, row.Owner, row.Attempt)
		}
		query = fmt.Sprintf(`UPDATE %s e SET %s='%s', %s=%s, %s='', %s=NULL, %s=NULL, %s='', %s=%s
 FROM (VALUES %s) AS i(event_id,tenant_id,owner,attempt)
 WHERE e.%s=i.event_id AND e.%s=i.tenant_id AND e.%s=i.owner AND e.%s=i.attempt AND e.%s='%s' RETURNING e.%s`,
			table, c.Status, sysin.EventStatusReady, c.ProcessedAt, now, c.LeaseOwner, c.LeaseUntil, c.NextRunAt, c.ErrorMessage, c.UpdatedAt, now,
			strings.Join(values, ","), c.Id, c.TenantId, c.LeaseOwner, c.AttemptCount, c.Status, sysin.EventStatusProcessing, c.Id)
	default:
		return nil, gerror.New("历史采集批次操作无效")
	}
	started := time.Now()
	result, err := g.DB().GetAll(ctx, query, args...)
	if err != nil {
		return nil, gerror.Wrap(err, "历史采集批次写入失败")
	}
	var records []entity.TgCollectorEvent
	if len(result) > 0 {
		if err = result.Structs(&records); err != nil {
			return nil, gerror.Wrap(err, "解析历史采集批次结果失败")
		}
	}
	g.Log().Info(ctx, "历史采集批次写入", g.Map{"event": "collector.history.batch", "operation": kind, "requested": len(rows), "affected": len(records), "durationMs": time.Since(started).Milliseconds()})
	return records, nil
}
