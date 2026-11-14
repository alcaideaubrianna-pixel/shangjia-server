package sys

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/hibiken/asynq"
	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/hgrds/lock"
)

// 统计纠偏属于低优先级维护任务，单进程只允许一个来源执行，避免与实时采集和发布争抢数据库 IO。
var collectStatsLowPriorityGate = make(chan struct{}, 1)

const tgTaskTypeCollectSourceStats = "youban_publish:collect:source_stats"
const collectSourceStatsWorkTable = "hg_youban_publish_collect_source_stats_work"

func collectSourceStatsRefreshDelay() time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(os.Getenv("YOUBAN_PUBLISH_COLLECT_SOURCE_STATS_DELAY_SECONDS")))
	if err != nil || seconds < 10 {
		seconds = 30
	}
	if seconds > 300 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

func collectSourceStatsBatchLimit() int {
	limit, err := strconv.Atoi(strings.TrimSpace(os.Getenv("YOUBAN_PUBLISH_COLLECT_SOURCE_STATS_BATCH_SIZE")))
	if err != nil || limit <= 0 {
		limit = 32
	}
	if limit > 128 {
		limit = 128
	}
	return limit
}

type collectSourceStatsPayload struct {
	SourceID int64 `json:"sourceId"`
}

// The database is the durable dirty set. One row per source coalesces all
// triggers, including triggers whose Redis enqueue fails. Do not reset an
// already pending source's deadline: busy sources must still make progress.
func markCollectSourceStatsDirty(ctx context.Context, sourceID int64) error {
	_, err := g.DB().Exec(ctx, `INSERT INTO `+collectSourceStatsWorkTable+` (source_id,generation,pending,available_at)
 VALUES (?, ?, 1, CURRENT_TIMESTAMP + (? * INTERVAL '1 second'))
 ON CONFLICT (source_id) DO UPDATE SET generation=EXCLUDED.generation,pending=1,
 available_at=CASE WHEN `+collectSourceStatsWorkTable+`.pending=1 THEN `+collectSourceStatsWorkTable+`.available_at ELSE EXCLUDED.available_at END,
	 updated_at=CURRENT_TIMESTAMP`, sourceID, guid.S(), int(collectSourceStatsRefreshDelay().Seconds()))
	return gerror.Wrap(err, "持久化采集源统计刷新请求失败")
}

func (s *sSysPublish) scheduleCollectSourceStats(ctx context.Context, sourceID int64) error {
	if sourceID <= 0 {
		return nil
	}
	if err := markCollectSourceStatsDirty(ctx, sourceID); err != nil {
		return err
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	return enqueueCollectSourceStatsTask(ctx, client, sourceID)
}

func enqueueCollectSourceStatsTask(ctx context.Context, client *asynq.Client, sourceID int64) error {
	body, err := json.Marshal(collectSourceStatsPayload{SourceID: sourceID})
	if err != nil {
		return err
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeCollectSourceStats, body),
		asynq.Queue(tgQueueNameBackground), asynq.ProcessIn(collectSourceStatsRefreshDelay()),
		asynq.Unique(30*time.Minute), asynq.Timeout(2*time.Minute), asynq.MaxRetry(1000))
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return gerror.Wrap(err, "提交采集源统计刷新失败")
}

func (s *sSysPublish) recoverCollectSourceStats(ctx context.Context, limit int) error {
	if limit <= 0 {
		return nil
	}
	rows, err := g.DB().Model(collectSourceStatsWorkTable).Ctx(ctx).Fields("source_id").Where("pending", 1).
		WhereLTE("available_at", time.Now()).OrderAsc("available_at").OrderAsc("source_id").Limit(limit).All()
	if err != nil {
		return gerror.Wrap(err, "读取待刷新采集源统计失败")
	}
	if len(rows) == 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		id := row["source_id"].Int64()
		if err := enqueueCollectSourceStatsTask(ctx, client, id); err != nil {
			return err
		}
		// Move only the dispatch deadline, never the pending generation. If a
		// task is lost or completion races ingress, this row remains recoverable.
		if _, err := g.DB().Model(collectSourceStatsWorkTable).Ctx(ctx).Where("source_id", id).Where("pending", 1).
			Data(g.Map{"available_at": time.Now().Add(30 * time.Second)}).Update(); err != nil {
			return err
		}
	}
	return nil
}

// Read the generation before counting and clear only that generation. Ingress
// concurrent with counting or ACK leaves the source dirty for recovery.
func refreshCollectSourceStatsWork(ctx context.Context, sourceID int64, refresh func(context.Context, int64) error) error {
	row, err := g.DB().Model(collectSourceStatsWorkTable).Ctx(ctx).Where("source_id", sourceID).Where("pending", 1).One()
	if err != nil {
		return err
	}
	if row.IsEmpty() {
		return nil
	}
	if err := refresh(ctx, sourceID); err != nil {
		return err
	}
	_, err = g.DB().Model(collectSourceStatsWorkTable).Ctx(ctx).Where("source_id", sourceID).
		Where("generation", row["generation"].String()).Data(g.Map{"pending": 0}).Update()
	return err
}

func (s *sSysPublish) handleCollectSourceStatsTask(ctx context.Context, task *asynq.Task) error {
	if heavyRecoveryDisabled("YOUBAN_DISABLE_COLLECT_STATS_RECONCILIATION") {
		return nil
	}
	select {
	case collectStatsLowPriorityGate <- struct{}{}:
		defer func() { <-collectStatsLowPriorityGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	var payload collectSourceStatsPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	if payload.SourceID <= 0 {
		return gerror.New("采集源统计任务缺少来源ID")
	}
	mutex := lock.NewConfig(2*time.Minute, time.Second).Mutex(publishconsts.CollectSourceStatsLockKey)
	if err := mutex.TryLock(ctx); err != nil {
		if errors.Is(err, lock.ErrLockFailed) {
			return newCollectProcessRetryError(30*time.Second, "采集源统计正在刷新，稍后重试")
		}
		return err
	}
	defer mutex.Unlock(context.WithoutCancel(ctx))
	// 一次领取一批 dirty source，统一聚合明细表，避免每个 source 单独
	// 执行 COUNT/SUM/MAX 扫描。批量有上限，避免低频纠偏任务争抢实时采集连接。
	rows, err := g.DB().Model(collectSourceStatsWorkTable).Ctx(ctx).
		Fields("source_id,generation").Where("pending", 1).
		WhereLTE("available_at", time.Now()).OrderAsc("available_at").
		OrderAsc("source_id").Limit(collectSourceStatsBatchLimit()).All()
	if err != nil || len(rows) == 0 {
		// 没有待对账记录时不要回退到单来源全表 COUNT/SUM/MAX。
		// 实时路径只负责写入 dirty work，低频批量任务负责真正聚合；
		// 旧任务到达这里也直接结束，避免再次触发大表扫描。
		if err != nil {
			return gerror.Wrap(err, "读取采集源统计待对账任务失败")
		}
		return nil
	}
	ids := make([]int64, 0, len(rows))
	generations := make(map[int64]string, len(rows))
	for _, row := range rows {
		id := row["source_id"].Int64()
		if id > 0 {
			ids = append(ids, id)
			generations[id] = row["generation"].String()
		}
	}
	if err := s.syncCollectSourceStatsBatch(ctx, ids); err != nil {
		return err
	}
	for id, generation := range generations {
		_, err = g.DB().Model(collectSourceStatsWorkTable).Ctx(ctx).
			Where("source_id", id).Where("generation", generation).
			Data(g.Map{"pending": 0, "updated_at": time.Now()}).Update()
		if err != nil {
			return err
		}
	}
	return nil
}
