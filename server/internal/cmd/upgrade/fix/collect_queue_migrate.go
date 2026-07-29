package fix

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
)

const (
	legacyCollectMediaQueue = "youban_publish_collect_media_source"
	collectMediaTaskType    = "youban_publish:collect:media_cache"
)

type collectMediaMigrationPayload struct {
	EventID  int64 `json:"eventId"`
	SourceID int64 `json:"sourceId"`
}

// MigrateYoubanPublishCollectQueue moves legacy collection media tasks into
// source-specific queues. A task is deleted from the legacy queue only after
// it has been accepted by the destination queue.
func MigrateYoubanPublishCollectQueue(ctx context.Context) error {
	redisOpt := asynq.RedisClientOpt{
		Addr:     g.Cfg().MustGet(ctx, "redis.default.address", "127.0.0.1:6379").String(),
		Password: g.Cfg().MustGet(ctx, "redis.default.pass", "").String(),
		DB:       g.Cfg().MustGet(ctx, "redis.default.db", 0).Int(),
	}
	inspector := asynq.NewInspector(redisOpt)
	defer inspector.Close()
	client := asynq.NewClient(redisOpt)
	defer client.Close()

	states := []struct {
		name string
		list func(string, ...asynq.ListOption) ([]*asynq.TaskInfo, error)
	}{
		{name: "pending", list: inspector.ListPendingTasks},
		{name: "scheduled", list: inspector.ListScheduledTasks},
		{name: "retry", list: inspector.ListRetryTasks},
	}
	var migrated, skipped, failed int
	for _, state := range states {
		for {
			tasks, err := state.list(legacyCollectMediaQueue, asynq.PageSize(500), asynq.Page(1))
			if err != nil {
				return fmt.Errorf("读取旧采集媒体队列 %s 失败: %w", state.name, err)
			}
			if len(tasks) == 0 {
				break
			}
			for _, info := range tasks {
				if err := migrateCollectMediaTask(ctx, client, inspector, info); err != nil {
					failed++
					g.Log().Errorf(ctx, "迁移采集媒体任务失败 taskId:%s state:%s err:%+v", info.ID, state.name, err)
					return fmt.Errorf("采集媒体队列迁移中断，已成功迁移%d条，失败任务%s: %w", migrated, info.ID, err)
				}
				if info.Type != collectMediaTaskType {
					skipped++
				} else {
					migrated++
				}
			}
		}
	}
	g.Log().Infof(ctx, "采集媒体队列迁移完成 migrated=%d skipped=%d failed=%d", migrated, skipped, failed)
	if failed > 0 {
		return fmt.Errorf("采集媒体队列迁移失败 %d 条", failed)
	}
	return nil
}

func migrateCollectMediaTask(ctx context.Context, client *asynq.Client, inspector *asynq.Inspector, info *asynq.TaskInfo) error {
	if info == nil || info.ID == "" {
		return fmt.Errorf("任务信息为空")
	}
	if info.Type != collectMediaTaskType {
		return fmt.Errorf("不支持的任务类型: %s", info.Type)
	}
	var payload collectMediaMigrationPayload
	if err := json.Unmarshal(info.Payload, &payload); err != nil {
		return fmt.Errorf("解析任务 payload 失败: %w", err)
	}
	if payload.SourceID <= 0 && payload.EventID > 0 {
		row, err := g.DB().Model("hg_youban_publish_collect_event").Safe().Ctx(ctx).
			Fields("source_id").Where("id", payload.EventID).One()
		if err != nil {
			return fmt.Errorf("按 eventId 查询 sourceId 失败: %w", err)
		}
		if row.IsEmpty() {
			if err = inspector.DeleteTask(info.Queue, info.ID); err != nil {
				return fmt.Errorf("清理孤儿采集媒体任务失败: %w", err)
			}
			g.Log().Warningf(ctx, "清理孤儿采集媒体任务 taskId:%s eventId:%d", info.ID, payload.EventID)
			return nil
		}
		payload.SourceID = row["source_id"].Int64()
	}
	if payload.SourceID <= 0 {
		return fmt.Errorf("任务缺少 sourceId")
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("重编码任务 payload 失败: %w", err)
	}

	destinationQueue := fmt.Sprintf("%s_%d", legacyCollectMediaQueue, payload.SourceID)
	options := []asynq.Option{
		asynq.Queue(destinationQueue),
		asynq.TaskID(info.ID),
		asynq.MaxRetry(info.MaxRetry),
	}
	if info.Timeout > 0 {
		options = append(options, asynq.Timeout(info.Timeout))
	}
	if !info.NextProcessAt.IsZero() {
		options = append(options, asynq.ProcessAt(info.NextProcessAt))
	}
	if _, err := client.EnqueueContext(ctx, asynq.NewTask(info.Type, payloadBytes), options...); err != nil {
		existing, getErr := inspector.GetTaskInfo(destinationQueue, info.ID)
		if getErr != nil || existing.Type != info.Type || string(existing.Payload) != string(payloadBytes) {
			if getErr == nil {
				if deleteErr := inspector.DeleteTask(destinationQueue, info.ID); deleteErr != nil {
					return fmt.Errorf("清理目标队列冲突任务失败: %w", deleteErr)
				}
				if _, enqueueErr := client.EnqueueContext(ctx, asynq.NewTask(info.Type, payloadBytes), options...); enqueueErr != nil {
					return fmt.Errorf("写入目标队列失败: %w", enqueueErr)
				}
			} else {
				return fmt.Errorf("写入目标队列失败: %w", err)
			}
		}
	}
	if err := inspector.DeleteTask(info.Queue, info.ID); err != nil {
		return fmt.Errorf("删除旧队列任务失败: %w", err)
	}
	return nil
}
