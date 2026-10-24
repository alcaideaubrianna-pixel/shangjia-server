package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func materialImportSendErr(ch chan error, err error) {
	select {
	case ch <- err:
	default:
	}
}

func (s *sSysPublish) materialImportHandleMediaRetry(ctx context.Context, task *sysin.MaterialImportTaskModel, err error) error {
	if retryErr, ok := err.(*collectMediaRetryError); ok {
		delay := int(retryErr.delay.Seconds())
		if delay <= 0 {
			delay = 30
		}
		_ = s.materialImportMarkWaiting(ctx, task.Id, task.UpdatedBy, delay, retryErr.message, sysin.MaterialImportStageMedia)
		return s.enqueueMaterialImportTask(ctx, task.Id, retryErr.delay)
	}
	return err
}

func (s *sSysPublish) materialImportMarkGroupRunning(ctx context.Context, id int64) error {
	_, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where("id", id).Data(g.Map{
		"status": sysin.MaterialImportStatusRunning, "error_message": "", "updated_at": gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) materialImportUpdateGroupProgress(ctx context.Context, id int64, done, failed int) error {
	_, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where("id", id).Data(g.Map{
		"media_done": gdb.Raw(fmt.Sprintf("GREATEST(media_done,%d)", done)), "media_failed": gdb.Raw(fmt.Sprintf("GREATEST(media_failed,%d)", failed)), "updated_at": gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) materialImportMarkGroupFailed(ctx context.Context, id int64, message string) error {
	_, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where("id", id).Data(g.Map{
		"status": sysin.MaterialImportStatusFailed, "error_message": strings.TrimSpace(message), "updated_at": gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) materialImportMarkGroupDone(ctx context.Context, id, profileId, taskProfileId int64) error {
	_, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where("id", id).Data(g.Map{
		"status": sysin.MaterialImportStatusSuccess, "media_done": gdb.Raw("media_total"), "media_failed": 0,
		"profile_id": profileId, "task_profile_id": taskProfileId, "updated_at": gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) materialImportMarkGroupDiscarded(ctx context.Context, id int64, mediaTotal int, reason string) error {
	_, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where("id", id).Data(g.Map{
		"status": sysin.MaterialImportStatusSuccess, "media_done": mediaTotal, "media_failed": 0,
		"profile_id": 0, "task_profile_id": 0, "error_message": "已跳过：" + strings.TrimSpace(reason), "updated_at": gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) materialImportTaskByPrimary(ctx context.Context, taskId int64) (*sysin.MaterialImportTaskModel, error) {
	row, err := s.materialImportTaskBaseQuery(ctx, 0, nil).Master().
		Fields("t.*,ta.display_name AS tg_account_nickname,ta.telegram_username AS tg_account_username,a.nickname AS account_name,tn.name AS tenant_name").
		Where("t.id", taskId).Limit(1).One()
	if err != nil {
		return nil, gerror.Wrap(err, "从主库读取资料导入任务失败")
	}
	task := materialImportTaskModelFromRecord(row)
	if task == nil {
		return nil, gerror.New("资料导入任务不存在")
	}
	return task, nil
}

func (s *sSysPublish) materialImportEnsureNotCanceled(ctx context.Context, taskId int64) error {
	cols := pdao.YoubanPublishMaterialImportTask.Columns()
	value, err := pdao.YoubanPublishMaterialImportTask.Ctx(ctx).Fields(cols.Status).Where(cols.Id, taskId).Value()
	if err != nil {
		return gerror.Wrap(err, "读取资料导入状态失败")
	}
	if strings.TrimSpace(value.String()) == sysin.MaterialImportStatusCanceled {
		return gerror.New("资料导入已取消")
	}
	return nil
}

func (s *sSysPublish) materialImportAddPulledMessages(ctx context.Context, taskId int64, count int) error {
	if count <= 0 {
		return nil
	}
	cols := pdao.YoubanPublishMaterialImportTask.Columns()
	_, err := pdao.YoubanPublishMaterialImportTask.Ctx(ctx).Where(cols.Id, taskId).Data(g.Map{
		cols.MessageDone: gdb.Raw(fmt.Sprintf("%s+%d", cols.MessageDone, count)), cols.MessageTotal: gdb.Raw(fmt.Sprintf("%s+%d", cols.MessageTotal, count)), cols.UpdatedAt: gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) materialImportMarkFailed(ctx context.Context, taskId, operatorId int64, message string) error {
	_, err := pdao.YoubanPublishMaterialImportTask.Ctx(ctx).Where("id", taskId).Data(g.Map{
		"status": sysin.MaterialImportStatusFailed, "stage": sysin.MaterialImportStageFinished, "error_message": strings.TrimSpace(message),
		"updated_by": operatorId, "updated_at": gtime.Now(), "finished_at": gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) refreshMaterialImportTaskStats(ctx context.Context, taskId int64) error {
	groupCols := pdao.YoubanPublishMaterialImportGroup.Columns()
	taskCols := pdao.YoubanPublishMaterialImportTask.Columns()
	stats, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where(groupCols.TaskId, taskId).Fields(
		"COUNT(*) AS group_total",
		fmt.Sprintf("SUM(CASE WHEN %s='%s' THEN 1 ELSE 0 END) AS group_done", groupCols.Status, sysin.MaterialImportStatusSuccess),
		"COALESCE(SUM(media_total),0) AS media_total", "COALESCE(SUM(media_done),0) AS media_done", "COALESCE(SUM(media_failed),0) AS media_failed",
	).One()
	if err != nil {
		return err
	}
	_, err = pdao.YoubanPublishMaterialImportTask.Ctx(ctx).Where(taskCols.Id, taskId).Data(g.Map{
		taskCols.GroupTotal: stats["group_total"].Int(), taskCols.GroupDone: stats["group_done"].Int(), taskCols.MediaTotal: stats["media_total"].Int(),
		taskCols.MediaDone: stats["media_done"].Int(), taskCols.MediaFailed: stats["media_failed"].Int(),
	}).Update()
	return err
}
