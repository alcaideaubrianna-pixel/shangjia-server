package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const materialImportMediaConcurrency = 3

func (s *sSysPublish) executeMaterialImportMedia(ctx context.Context, taskId int64) error {
	task, err := s.materialImportTaskById(ctx, taskId, 0)
	if err != nil {
		return err
	}
	if err = s.materialImportEnsureNotCanceled(ctx, task.Id); err != nil {
		return err
	}
	run := func(runCtx context.Context, client *telegram.Client) error {
		if _, selfErr := client.Self(runCtx); selfErr != nil {
			return selfErr
		}
		return s.materialImportDownloadGroups(runCtx, task, client)
	}
	usedRuntime, err := s.executeAccountCollectOperation(ctx, task.TgAccountId, 5*time.Hour, run)
	if err != nil || usedRuntime {
		return err
	}
	tgAccount, err := s.accountCollectTgAccount(ctx, task.TgAccountId)
	if err != nil {
		return err
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return err
	}
	client, err := s.newAccountCollectClient(ctx, conf, tgAccount, tg.NewUpdateDispatcher())
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Hour)
	defer cancel()
	return client.Run(runCtx, func(clientCtx context.Context) error {
		return run(clientCtx, client)
	})
}

func (s *sSysPublish) materialImportDownloadGroups(ctx context.Context, task *sysin.MaterialImportTaskModel, client *telegram.Client) error {
	groups, err := s.materialImportPendingMediaGroups(ctx, task.Id)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		return s.materialImportMarkSuccess(ctx, task.Id, task.UpdatedBy, g.Map{"message": "资料导入完成"})
	}
	jobs := make(chan *sysin.MaterialImportGroupModel)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	for i := 0; i < materialImportMediaConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for group := range jobs {
				if err := s.materialImportEnsureNotCanceled(ctx, task.Id); err != nil {
					materialImportSendErr(errCh, err)
					return
				}
				groupErr := s.materialImportDownloadGroup(ctx, task, group, client)
				if groupErr != nil {
					if retryErr := collectMediaRetryErrorFrom(groupErr); retryErr != nil {
						_ = s.refreshMaterialImportTaskStats(ctx, task.Id)
						materialImportSendErr(errCh, retryErr)
						return
					}
					_ = s.materialImportMarkGroupFailed(ctx, group.Id, groupErr.Error())
				}
				_ = s.refreshMaterialImportTaskStats(ctx, task.Id)
			}
		}()
	}
	for _, group := range groups {
		select {
		case jobs <- group:
		case err := <-errCh:
			close(jobs)
			wg.Wait()
			return s.materialImportHandleMediaRetry(ctx, task, err)
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-errCh:
		return s.materialImportHandleMediaRetry(ctx, task, err)
	default:
	}
	if err = s.refreshMaterialImportTaskStats(ctx, task.Id); err != nil {
		return err
	}
	failedCount, err := s.materialImportFailedGroupCount(ctx, task.Id)
	if err != nil {
		return err
	}
	if failedCount > 0 {
		return s.materialImportMarkFailed(ctx, task.Id, task.UpdatedBy, fmt.Sprintf("资料导入失败：%d组资料处理失败", failedCount))
	}
	return s.materialImportMarkSuccess(ctx, task.Id, task.UpdatedBy, g.Map{"message": "资料导入完成"})
}

func (s *sSysPublish) materialImportPendingMediaGroups(ctx context.Context, taskId int64) ([]*sysin.MaterialImportGroupModel, error) {
	rows, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).
		Where("task_id", taskId).
		WhereIn("status", []string{sysin.MaterialImportStatusPending, sysin.MaterialImportStatusFailed, sysin.MaterialImportStatusRunning}).
		OrderAsc("id").
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取待下载资料分组失败")
	}
	list := make([]*sysin.MaterialImportGroupModel, 0, len(rows))
	for _, row := range rows {
		item := materialImportGroupModelFromRecord(row)
		if item != nil && (item.MediaTotal > item.MediaDone || item.Status == sysin.MaterialImportStatusFailed || item.Status == sysin.MaterialImportStatusRunning) {
			list = append(list, item)
		}
	}
	return list, nil
}

func (s *sSysPublish) materialImportFailedGroupCount(ctx context.Context, taskId int64) (int, error) {
	value, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).
		Where("task_id", taskId).
		Where("status", sysin.MaterialImportStatusFailed).
		Count()
	if err != nil {
		return 0, gerror.Wrap(err, "统计导入失败分组失败")
	}
	return value, nil
}

func (s *sSysPublish) materialImportDownloadGroup(ctx context.Context, task *sysin.MaterialImportTaskModel, group *sysin.MaterialImportGroupModel, client *telegram.Client) error {
	if !materialImportProfileText(firstNonEmpty(group.ProfileText, group.RawText)) {
		return s.materialImportMarkGroupDone(ctx, group.Id, 0, 0, group.MediaJson)
	}
	items := make([]collectMediaItem, 0)
	_ = json.Unmarshal([]byte(group.MediaJson), &items)
	if len(items) == 0 {
		profileId, err := s.saveMaterialImportGroupProfile(ctx, task, group, "[]")
		if err != nil {
			return err
		}
		return s.materialImportMarkGroupDone(ctx, group.Id, profileId, 0, "[]")
	}
	profileId, err := s.materialImportExistingProfile(ctx, group)
	if err != nil {
		return err
	}
	if profileId > 0 && (group.MediaTotal == 0 || s.materialImportProfileHasMedia(ctx, profileId)) {
		if err = s.ensureMaterialImportTelegramIndex(ctx, task, group, profileId); err != nil {
			return err
		}
		if err = s.syncProfileNoteIndex(ctx, profileId); err != nil {
			return err
		}
		_ = s.appendMaterialImportPublishLog(ctx, task, profileId, "reused", fmt.Sprintf("资料已存在，跳过重复导入：%s", strings.TrimSpace(group.Title)))
		return s.materialImportMarkGroupDone(ctx, group.Id, profileId, 0, group.MediaJson)
	}
	_ = s.materialImportMarkGroupRunning(ctx, group.Id)
	done := 0
	failed := 0
	for index, item := range items {
		item = normalizeCollectMediaItem(item)
		if strings.TrimSpace(item.StoragePath) != "" || strings.TrimSpace(item.FileUrl) != "" {
			done++
			continue
		}
		meta, ok := gotdCollectMediaMetaFromItem(item)
		if !ok {
			failed++
			continue
		}
		down, err := s.cachedGotdCollectMediaFileWithClient(ctx, task.TenantId, task.TgAccountId, item, meta, client)
		if err != nil {
			return err
		}
		if strings.TrimSpace(down.Path) != "" {
			items[index].StoragePath = down.Path
		}
		done++
		_ = s.materialImportUpdateGroupProgress(ctx, group.Id, done, failed)
	}
	data, _ := json.Marshal(items)
	profileId, err = s.saveMaterialImportGroupProfile(ctx, task, group, string(data))
	if err != nil {
		return err
	}
	return s.materialImportMarkGroupDone(ctx, group.Id, profileId, 0, string(data))
}

func (s *sSysPublish) materialImportProfileHasMedia(ctx context.Context, profileId int64) bool {
	if profileId <= 0 {
		return false
	}
	mod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).WhereNull("deleted_at")
	count, err := mod.Count()
	return err == nil && count > 0
}

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
		"status":        sysin.MaterialImportStatusRunning,
		"error_message": "",
		"updated_at":    gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) materialImportUpdateGroupProgress(ctx context.Context, id int64, done int, failed int) error {
	_, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where("id", id).Data(g.Map{
		"media_done":   done,
		"media_failed": failed,
		"updated_at":   gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) materialImportMarkGroupFailed(ctx context.Context, id int64, message string) error {
	_, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where("id", id).Data(g.Map{
		"status":        sysin.MaterialImportStatusFailed,
		"error_message": strings.TrimSpace(message),
		"updated_at":    gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) materialImportMarkGroupDone(ctx context.Context, id int64, profileId int64, taskProfileId int64, mediaJson string) error {
	_, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where("id", id).Data(g.Map{
		"status":          sysin.MaterialImportStatusSuccess,
		"media_json":      strings.TrimSpace(mediaJson),
		"media_done":      gdb.Raw("media_total"),
		"media_failed":    0,
		"profile_id":      profileId,
		"task_profile_id": taskProfileId,
		"updated_at":      gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) materialImportTaskByPrimary(ctx context.Context, taskId int64) (*sysin.MaterialImportTaskModel, error) {
	return s.materialImportTaskById(ctx, taskId, 0)
}

func (s *sSysPublish) materialImportEnsureNotCanceled(ctx context.Context, taskId int64) error {
	cols := pdao.YoubanPublishMaterialImportTask.Columns()
	value, err := pdao.YoubanPublishMaterialImportTask.Ctx(ctx).
		Fields(cols.Status).
		Where(cols.Id, taskId).
		Value()
	if err != nil {
		return gerror.Wrap(err, "读取资料导入状态失败")
	}
	status := value.String()
	if strings.TrimSpace(status) == sysin.MaterialImportStatusCanceled {
		return gerror.New("资料导入已取消")
	}
	return nil
}

func (s *sSysPublish) materialImportAddPulledMessages(ctx context.Context, taskId int64, count int) error {
	if count <= 0 {
		return nil
	}
	cols := pdao.YoubanPublishMaterialImportTask.Columns()
	_, err := pdao.YoubanPublishMaterialImportTask.Ctx(ctx).
		Where(cols.Id, taskId).
		Data(g.Map{
			cols.MessageDone:  gdb.Raw(fmt.Sprintf("%s+%d", cols.MessageDone, count)),
			cols.MessageTotal: gdb.Raw(fmt.Sprintf("%s+%d", cols.MessageTotal, count)),
			cols.UpdatedAt:    gtime.Now(),
		}).
		Update()
	return err
}

func (s *sSysPublish) materialImportMarkFailed(ctx context.Context, taskId int64, operatorId int64, message string) error {
	_, err := pdao.YoubanPublishMaterialImportTask.Ctx(ctx).Where("id", taskId).Data(g.Map{
		"status":        sysin.MaterialImportStatusFailed,
		"stage":         sysin.MaterialImportStageFinished,
		"error_message": strings.TrimSpace(message),
		"updated_by":    operatorId,
		"updated_at":    gtime.Now(),
		"finished_at":   gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) refreshMaterialImportTaskStats(ctx context.Context, taskId int64) error {
	groupCols := pdao.YoubanPublishMaterialImportGroup.Columns()
	taskCols := pdao.YoubanPublishMaterialImportTask.Columns()
	total, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where(groupCols.TaskId, taskId).Count()
	if err != nil {
		return err
	}
	done, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where(groupCols.TaskId, taskId).Where(groupCols.Status, sysin.MaterialImportStatusSuccess).Count()
	if err != nil {
		return err
	}
	mediaTotalValue, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).
		Where(groupCols.TaskId, taskId).
		Fields("COALESCE(SUM(media_total),0)").
		Value()
	if err != nil {
		return err
	}
	mediaDoneValue, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).
		Where(groupCols.TaskId, taskId).
		Fields("COALESCE(SUM(media_done),0)").
		Value()
	if err != nil {
		return err
	}
	mediaFailedValue, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).
		Where(groupCols.TaskId, taskId).
		Fields("COALESCE(SUM(media_failed),0)").
		Value()
	if err != nil {
		return err
	}
	_, err = pdao.YoubanPublishMaterialImportTask.Ctx(ctx).Where(taskCols.Id, taskId).Data(g.Map{
		taskCols.GroupTotal:  total,
		taskCols.GroupDone:   done,
		taskCols.MediaTotal:  mediaTotalValue.Int(),
		taskCols.MediaDone:   mediaDoneValue.Int(),
		taskCols.MediaFailed: mediaFailedValue.Int(),
		taskCols.UpdatedAt:   gtime.Now(),
	}).Update()
	return err
}

func gotdCollectMediaMetaFromItem(item collectMediaItem) (gotdCollectMediaMeta, bool) {
	var meta gotdCollectMediaMeta
	if strings.TrimSpace(item.MetaJson) == "" {
		return meta, false
	}
	if err := json.Unmarshal([]byte(item.MetaJson), &meta); err != nil || meta.Id <= 0 {
		return gotdCollectMediaMeta{}, false
	}
	return meta, true
}
