package sys

import (
	"context"
	"encoding/json"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// RepairMaterialImportMissingMedia requeues completed TG import groups whose
// stored profile has fewer display or verify media than the source group.
func (s *sSysPublish) RepairMaterialImportMissingMedia(ctx context.Context, accountId int64) error {
	if accountId <= 0 {
		return gerror.New("资料账号ID不能为空")
	}
	rows, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).
		Where("account_id", accountId).
		Where("status", sysin.MaterialImportStatusSuccess).
		WhereGT("profile_id", 0).
		OrderAsc("id").
		All()
	if err != nil {
		return err
	}

	taskIds := make(map[int64]struct{})
	requeuedGroups := 0
	for _, row := range rows {
		group := materialImportGroupModelFromRecord(row)
		if group == nil || group.Id <= 0 || group.ProfileId <= 0 {
			continue
		}
		var items []collectMediaItem
		if err = json.Unmarshal([]byte(group.MediaJson), &items); err != nil {
			g.Log().Warningf(ctx, "解析TG导入媒体失败 groupId:%d err:%+v", group.Id, err)
			continue
		}
		counts, countErr := s.materialImportProfileMediaCounts(ctx, group.ProfileId)
		if countErr != nil {
			return countErr
		}
		missing, _ := materialImportMissingMediaItems(items, counts)
		if len(missing) == 0 {
			continue
		}
		_, err = pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).
			Where("id", group.Id).
			Data(g.Map{
				"status":        sysin.MaterialImportStatusPending,
				"media_done":    0,
				"media_failed":  0,
				"error_message": "",
				"updated_at":    gtime.Now(),
			}).Update()
		if err != nil {
			return err
		}
		taskIds[group.TaskId] = struct{}{}
		requeuedGroups++
		g.Log().Infof(ctx, "发现TG导入资料缺失媒体，已重新排队 groupId:%d profileId:%d display:%d verify:%d missing:%d", group.Id, group.ProfileId, counts.Display, counts.Verify, len(missing))
	}

	for taskId := range taskIds {
		if taskId <= 0 {
			continue
		}
		taskCols := pdao.YoubanPublishMaterialImportTask.Columns()
		result, updateErr := pdao.YoubanPublishMaterialImportTask.Ctx(ctx).
			Where(taskCols.Id, taskId).
			Where(taskCols.Status, sysin.MaterialImportStatusSuccess).
			Data(g.Map{
				taskCols.Status:       sysin.MaterialImportStatusRunning,
				taskCols.Stage:        sysin.MaterialImportStageMedia,
				taskCols.ErrorMessage: "",
				taskCols.NextRunAt:    nil,
				taskCols.UpdatedAt:    gtime.Now(),
			}).Update()
		if updateErr != nil {
			return updateErr
		}
		affected, affectedErr := result.RowsAffected()
		if affectedErr != nil {
			return affectedErr
		}
		if affected == 0 {
			continue
		}
		if err = s.enqueueMaterialImportTask(ctx, taskId, 0); err != nil {
			return err
		}
	}
	g.Log().Infof(ctx, "TG导入历史媒体修复完成 accountId:%d requeuedGroups:%d tasks:%d", accountId, requeuedGroups, len(taskIds))
	return nil
}
