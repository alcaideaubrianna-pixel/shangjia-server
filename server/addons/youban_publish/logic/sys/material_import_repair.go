package sys

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// RepairMaterialImportMissingMedia requeues completed TG import groups whose
// stored profile has fewer display or verify media than the source group.
func (s *sSysPublish) RepairMaterialImportMissingMedia(ctx context.Context, accountId int64, groupIds []int64) error {
	if accountId <= 0 {
		return gerror.New("资料账号ID不能为空")
	}
	selectedGroups := make(map[int64]struct{}, len(groupIds))
	for _, groupId := range groupIds {
		if groupId > 0 {
			selectedGroups[groupId] = struct{}{}
		}
	}
	rows, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).
		Where("account_id", accountId).
		WhereIn("status", []string{
			sysin.MaterialImportStatusSuccess,
			sysin.MaterialImportStatusPending,
			sysin.MaterialImportStatusFailed,
		}).
		OrderAsc("id").
		All()
	if err != nil {
		return err
	}

	taskIds := make(map[int64]struct{})
	requeuedGroups := 0
	for _, row := range rows {
		group := materialImportGroupModelFromRecord(row)
		if group == nil || group.Id <= 0 {
			continue
		}
		if len(selectedGroups) > 0 {
			if _, ok := selectedGroups[group.Id]; !ok {
				continue
			}
		}
		if group.Status == sysin.MaterialImportStatusFailed && materialImportFileReferenceExpired(group.ErrorMessage) {
			if len(selectedGroups) == 0 {
				g.Log().Warningf(ctx, "跳过FILE_REFERENCE_EXPIRED资料删除，必须通过a3明确指定 groupId:%d profileId:%d", group.Id, group.ProfileId)
				continue
			}
			if err = s.deleteMaterialImportExpiredProfile(ctx, group); err != nil {
				return err
			}
			continue
		}
		if group.Status != sysin.MaterialImportStatusSuccess && group.Status != sysin.MaterialImportStatusPending {
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
			WhereIn(taskCols.Status, []string{sysin.MaterialImportStatusSuccess, sysin.MaterialImportStatusFailed}).
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

func materialImportFileReferenceExpired(message string) bool {
	return strings.Contains(strings.ToUpper(strings.TrimSpace(message)), "FILE_REFERENCE_EXPIRED")
}

func (s *sSysPublish) deleteMaterialImportExpiredProfile(ctx context.Context, group *sysin.MaterialImportGroupModel) error {
	if group == nil || group.Id <= 0 {
		return nil
	}
	now := gtime.Now()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if group.ProfileId > 0 {
			profile, err := tx.Model("hg_content_profile").Safe().Ctx(ctx).
				Fields("id,tenant_id,account_id").
				Where("id", group.ProfileId).
				Where("tenant_id", group.TenantId).
				Where("account_id", group.AccountId).
				WhereNull("deleted_at").
				One()
			if err != nil {
				return gerror.Wrap(err, "校验待删除TG导入资料归属失败")
			}
			if profile.IsEmpty() {
				return gerror.Newf("待删除TG导入资料不存在或归属不匹配 profileId:%d", group.ProfileId)
			}
			for _, table := range []string{
				"hg_content_media",
				"hg_youban_publish_media_phash_bucket",
				"hg_youban_publish_media_phash_lsh",
				"hg_youban_publish_media",
				"hg_youban_publish_note_index",
				"hg_youban_publish_profile_state",
				"hg_youban_publish_channel_profile",
				"hg_content_source_map",
			} {
				if _, err = tx.Model(table).Safe().Ctx(ctx).Where("profile_id", group.ProfileId).Delete(); err != nil {
					return gerror.Wrapf(err, "清理FILE_REFERENCE_EXPIRED资料关联失败 table:%s", table)
				}
			}
			if _, err = tx.Model("hg_content_profile").Safe().Ctx(ctx).
				Where("id", group.ProfileId).
				Data(g.Map{"status": 0, "deleted_at": now, "updated_at": now}).Update(); err != nil {
				return gerror.Wrap(err, "删除FILE_REFERENCE_EXPIRED资料失败")
			}
		}
		_, err := tx.Model(pdao.YoubanPublishMaterialImportGroup.Table()).Safe().Ctx(ctx).
			Where("id", group.Id).
			Data(g.Map{
				"status":        sysin.MaterialImportStatusFailed,
				"error_message": "FILE_REFERENCE_EXPIRED，恢复脚本已删除资料",
				"updated_at":    now,
			}).Update()
		return err
	})
	if err != nil {
		return err
	}
	g.Log().Warningf(ctx, "恢复脚本已处理FILE_REFERENCE_EXPIRED分组 groupId:%d profileId:%d tenantId:%d accountId:%d action:%s", group.Id, group.ProfileId, group.TenantId, group.AccountId, materialImportExpiredAction(group.ProfileId))
	return nil
}

func materialImportExpiredAction(profileId int64) string {
	if profileId > 0 {
		return "delete_profile"
	}
	return "discard_group_without_profile"
}
