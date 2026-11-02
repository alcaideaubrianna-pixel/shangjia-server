package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/hibiken/asynq"
	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/hgrds/lock"
)

const tgTaskTypeCollectSourceStats = "youban_publish:collect:source_stats"

type collectSourceStatsPayload struct {
	SourceID int64 `json:"sourceId"`
	Window   int64 `json:"window"`
}

// Each source has one delayed task per closed 30-second window. New events
// cannot be swallowed by the completion/ACK of an older window. Counts no
// longer hold up delivery workers; the dirty generation detects concurrent
// changes during a refresh and keeps the task retryable until caught up.
func (s *sSysPublish) scheduleCollectSourceStats(ctx context.Context, sourceID int64) error {
	if sourceID <= 0 {
		return nil
	}
	if !cache.Initialized() {
		return s.syncCollectSourceStats(ctx, sourceID)
	}
	key := fmt.Sprintf("%s%d", publishconsts.CollectSourceStatsDirtyKeyPrefix, sourceID)
	if err := cache.Instance().Set(ctx, key, guid.S(), 24*time.Hour); err != nil {
		return gerror.Wrap(err, "标记采集源统计刷新失败")
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	return enqueueCollectSourceStatsTask(ctx, client, sourceID)
}

func enqueueCollectSourceStatsTask(ctx context.Context, client *asynq.Client, sourceID int64) error {
	return enqueueCollectSourceStatsTaskAt(ctx, client, sourceID, time.Now())
}

func enqueueCollectSourceStatsTaskAt(ctx context.Context, client *asynq.Client, sourceID int64, now time.Time) error {
	window := now.Unix() / 30
	body, err := json.Marshal(collectSourceStatsPayload{SourceID: sourceID, Window: window})
	if err != nil {
		return err
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeCollectSourceStats, body),
		asynq.Queue(tgQueueNameBackground), asynq.ProcessAt(time.Unix((window+1)*30+5, 0)),
		asynq.Unique(30*time.Minute), asynq.Timeout(2*time.Minute), asynq.MaxRetry(1000))
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return gerror.Wrap(err, "提交采集源统计刷新失败")
}

func (s *sSysPublish) handleCollectSourceStatsTask(ctx context.Context, task *asynq.Task) error {
	var payload collectSourceStatsPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	if payload.SourceID <= 0 {
		return gerror.New("采集源统计任务缺少来源ID")
	}
	// One cluster-wide statistics scan at a time, even with multiple workers.
	mutex := lock.NewConfig(2*time.Minute, time.Second).Mutex(publishconsts.CollectSourceStatsLockKey)
	if err := mutex.TryLock(ctx); err != nil {
		if errors.Is(err, lock.ErrLockFailed) {
			return newCollectProcessRetryError(30*time.Second, "采集源统计正在刷新，稍后重试")
		}
		return err
	}
	defer mutex.Unlock(context.WithoutCancel(ctx))
	key := fmt.Sprintf("%s%d", publishconsts.CollectSourceStatsDirtyKeyPrefix, payload.SourceID)
	before, err := cache.Instance().Get(ctx, key)
	if err != nil {
		return err
	}
	if err = s.syncCollectSourceStats(ctx, payload.SourceID); err != nil {
		return err
	}
	after, err := cache.Instance().Get(ctx, key)
	if err != nil {
		return err
	}
	if before.String() != after.String() {
		return newCollectProcessRetryError(30*time.Second, "采集源统计发生变化，合并后重试")
	}
	return nil
}
