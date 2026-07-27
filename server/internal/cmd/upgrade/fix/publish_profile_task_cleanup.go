package fix

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

// CleanupYoubanPublishProfileTasks removes the obsolete publish snapshot table.
// Current profile data must already live in content_profile/profile_state/media.
func CleanupYoubanPublishProfileTasks(ctx context.Context) error {
	count, err := g.DB().Model("hg_youban_publish_task").Safe().Ctx(ctx).Count()
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "does not exist") || strings.Contains(message, "doesn't exist") || strings.Contains(message, "unknown table") {
			g.Log().Info(ctx, "历史发布Task表已不存在，无需清理")
			return nil
		}
		return gerror.Wrap(err, "检查历史发布Task表失败")
	}
	if count > 0 {
		missingStateCount, checkErr := g.DB().Model("hg_youban_publish_task t").Safe().Ctx(ctx).
			Where("t.profile_id > 0").
			Where("NOT EXISTS (SELECT 1 FROM hg_youban_publish_profile_state ps WHERE ps.profile_id=t.profile_id)").
			Count()
		if checkErr != nil {
			return gerror.Wrap(checkErr, "校验历史资料归属迁移状态失败")
		}
		if missingStateCount > 0 {
			return gerror.Newf("仍有%d个历史发布Task未迁移到ProfileState，已停止删表", missingStateCount)
		}
	}
	if err := BackfillYoubanPublishProfileMedia(ctx); err != nil {
		return err
	}
	for _, table := range []string{
		"hg_youban_publish_note_index",
		"hg_youban_publish_tg_message",
		"hg_youban_publish_tg_job_log",
		"hg_youban_publish_success_record",
	} {
		if _, err := g.DB().Exec(ctx, "UPDATE "+table+" SET task_id=0 WHERE task_id<>0"); err != nil {
			return gerror.Wrapf(err, "清理历史发布Task引用失败 table:%s", table)
		}
	}
	if _, err := g.DB().Exec(ctx, "UPDATE hg_youban_publish_tg_job SET task_id=NULL WHERE task_id IS NOT NULL"); err != nil {
		return gerror.Wrap(err, "清理TG发送任务的历史发布Task引用失败")
	}
	if _, err := g.DB().Exec(ctx, "UPDATE hg_youban_publish_media SET task_id=NULL WHERE task_id IS NOT NULL"); err != nil {
		return gerror.Wrap(err, "清理媒体的历史发布Task引用失败")
	}
	if _, err := g.DB().Exec(ctx, "DROP TABLE IF EXISTS hg_youban_publish_task"); err != nil {
		return gerror.Wrap(err, "删除历史发布Task表失败")
	}
	g.Log().Info(ctx, "历史发布Task引用和数据表已清理")
	return nil
}
