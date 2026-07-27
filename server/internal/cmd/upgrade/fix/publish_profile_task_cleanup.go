package fix

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

const publishProfileTaskCleanupBatchSize = 500

const retainedPublishTaskCondition = `(
	collect_source_id > 0 OR
	collect_event_id > 0 OR
	client_request_id LIKE 'collect:%' OR
	client_request_id LIKE 'legacy:%' OR
	tg_operation_no LIKE 'collect:%'
)`

// CleanupYoubanPublishProfileTasks removes the obsolete Task snapshot layer
// after current profile media has been materialized. Collection and legacy
// import tasks remain available to their dedicated workflows.
func CleanupYoubanPublishProfileTasks(ctx context.Context) error {
	if err := BackfillYoubanPublishProfileMedia(ctx); err != nil {
		return err
	}
	if err := ensurePublishProfileTaskMediaReady(ctx); err != nil {
		return err
	}

	processed := 0
	for {
		ids, err := nextObsoletePublishTaskIds(ctx, publishProfileTaskCleanupBatchSize)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			break
		}
		if err = cleanupObsoletePublishTaskBatch(ctx, ids); err != nil {
			return err
		}
		processed += len(ids)
		g.Log().Infof(ctx, "普通资料历史Task清理进度：processed=%d", processed)
	}
	g.Log().Infof(ctx, "普通资料历史Task清理完成：processed=%d", processed)
	return nil
}

func ensurePublishProfileTaskMediaReady(ctx context.Context) error {
	count, err := g.DB().Model("hg_youban_publish_task t").Safe().Ctx(ctx).
		Where("NOT " + retainedPublishTaskCondition).
		Where("t.profile_id > 0").
		Where("EXISTS (SELECT 1 FROM hg_youban_publish_media tm WHERE tm.task_id=t.id AND tm.deleted_at IS NULL)").
		Where("NOT EXISTS (SELECT 1 FROM hg_youban_publish_media pm WHERE pm.profile_id=t.profile_id AND pm.task_id IS NULL AND pm.deleted_at IS NULL)").
		Count()
	if err != nil {
		return gerror.Wrap(err, "校验资料当前媒体失败")
	}
	if count > 0 {
		return gerror.Newf("仍有%d个资料未生成当前媒体，已停止清理历史Task", count)
	}
	return nil
}

func nextObsoletePublishTaskIds(ctx context.Context, limit int) ([]int64, error) {
	var rows []struct {
		Id int64 `orm:"id"`
	}
	err := g.DB().Model("hg_youban_publish_task").Safe().Ctx(ctx).Unscoped().
		Fields("id").
		Where("NOT " + retainedPublishTaskCondition).
		OrderAsc("id").
		Limit(limit).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取待清理普通资料Task失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.Id > 0 {
			ids = append(ids, row.Id)
		}
	}
	return ids, nil
}

func cleanupObsoletePublishTaskBatch(ctx context.Context, taskIds []int64) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var mediaRows []struct {
			Id int64 `orm:"id"`
		}
		if err := tx.Model("hg_youban_publish_media").Ctx(ctx).Unscoped().
			Fields("id").WhereIn("task_id", taskIds).Scan(&mediaRows); err != nil {
			return gerror.Wrap(err, "读取历史Task媒体失败")
		}
		mediaIds := make([]int64, 0, len(mediaRows))
		for _, row := range mediaRows {
			if row.Id > 0 {
				mediaIds = append(mediaIds, row.Id)
			}
		}
		if len(mediaIds) > 0 {
			for _, table := range []string{
				"hg_youban_publish_media_face",
				"hg_youban_publish_media_phash_bucket",
				"hg_youban_publish_media_phash_lsh",
			} {
				if _, err := tx.Model(table).Ctx(ctx).WhereIn("media_id", mediaIds).Delete(); err != nil {
					return gerror.Wrapf(err, "清理历史媒体索引失败 table:%s", table)
				}
			}
		}
		if _, err := tx.Model("hg_youban_publish_media").Ctx(ctx).Unscoped().WhereIn("task_id", taskIds).Delete(); err != nil {
			return gerror.Wrap(err, "清理历史Task媒体失败")
		}
		var jobs []struct {
			Id     int64 `orm:"id"`
			TaskId int64 `orm:"task_id"`
		}
		if err := tx.Model("hg_youban_publish_tg_job").Ctx(ctx).
			Fields("id,task_id").WhereIn("task_id", taskIds).Scan(&jobs); err != nil {
			return gerror.Wrap(err, "读取历史TG任务失败")
		}
		for _, job := range jobs {
			operationNo := fmt.Sprintf("legacy-task:%d:%d", job.TaskId, job.Id)
			if _, err := tx.Model("hg_youban_publish_tg_job").Ctx(ctx).
				Where("id", job.Id).
				Data(g.Map{"operation_no": operationNo, "task_id": nil}).
				Update(); err != nil {
				return gerror.Wrapf(err, "迁移历史TG任务归属失败 job:%d", job.Id)
			}
		}
		return cleanupObsoletePublishTaskReferences(ctx, tx, taskIds)
	})
}

func cleanupObsoletePublishTaskReferences(ctx context.Context, tx gdb.TX, taskIds []int64) error {
	if len(taskIds) == 0 {
		return nil
	}
	for _, table := range []string{
		"hg_youban_publish_note_index",
		"hg_youban_publish_tg_message",
		"hg_youban_publish_tg_job_log",
		"hg_youban_publish_success_record",
	} {
		if _, err := tx.Model(table).Ctx(ctx).WhereIn("task_id", taskIds).Data(g.Map{"task_id": 0}).Update(); err != nil {
			return gerror.Wrapf(err, "清理历史Task引用失败 table:%s", table)
		}
	}
	if _, err := tx.Model("hg_youban_publish_task").Ctx(ctx).Unscoped().WhereIn("id", taskIds).Delete(); err != nil {
		return gerror.Wrap(err, "删除普通资料历史Task失败")
	}
	return nil
}
