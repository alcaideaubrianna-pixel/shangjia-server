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
		limit = 128
	}
	if limit > 256 {
		limit = 256
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
	// 来源统计不参与实时采集链路，禁止触发事件表聚合扫描。
	return nil
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
	return nil
}
