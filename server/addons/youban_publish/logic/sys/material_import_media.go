package sys

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"golang.org/x/sync/errgroup"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const materialImportMediaItemTimeout = 10 * time.Minute

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
	return s.executeTelegramAccountMediaOperation(ctx, task.TgAccountId, 5*time.Hour, run)
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
	for i := 0; i < accountCollectMediaConcurrency(ctx); i++ {
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
	if !profileMessageHasProfileText(firstNonEmpty(group.ProfileText, group.RawText)) {
		return s.materialImportMarkGroupDone(ctx, group.Id, 0, 0)
	}
	items, err := s.materialImportGroupMediaItems(ctx, group.Id)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		profileId, err := s.saveMaterialImportGroupProfile(ctx, task, group, nil)
		if err != nil {
			return err
		}
		return s.materialImportMarkGroupDone(ctx, group.Id, profileId, 0)
	}
	profileId, err := s.materialImportExistingProfile(ctx, group)
	if err != nil {
		return err
	}
	if profileId > 0 && group.MediaTotal == 0 {
		if err = s.refreshMaterialImportProfileMetadata(ctx, task, group, profileId); err != nil {
			return err
		}
		if err = s.ensureMaterialImportTelegramIndex(ctx, task, group, profileId); err != nil {
			return err
		}
		_ = s.appendMaterialImportPublishLog(ctx, task, profileId, "reused", fmt.Sprintf("资料已存在，跳过重复导入：%s", strings.TrimSpace(group.Title)))
		return s.materialImportMarkGroupDone(ctx, group.Id, profileId, 0)
	}
	missingItems := items
	missingIndexes := make([]int, len(items))
	for index := range items {
		missingIndexes[index] = index
	}
	if profileId > 0 {
		counts, countErr := s.materialImportProfileMediaCounts(ctx, profileId)
		if countErr != nil {
			return countErr
		}
		missingItems, missingIndexes = materialImportMissingMediaItems(items, counts)
		if len(missingItems) == 0 {
			if err = s.refreshMaterialImportProfileMetadata(ctx, task, group, profileId); err != nil {
				return err
			}
			if err = s.ensureMaterialImportTelegramIndex(ctx, task, group, profileId); err != nil {
				return err
			}
			_ = s.appendMaterialImportPublishLog(ctx, task, profileId, "reused", fmt.Sprintf("资料媒体已完整，跳过重复导入：%s", strings.TrimSpace(group.Title)))
			return s.materialImportMarkGroupDone(ctx, group.Id, profileId, 0)
		}
	}
	_ = s.materialImportMarkGroupRunning(ctx, group.Id)
	_, _, err = s.downloadMaterialImportItems(ctx, task, group.Id, missingItems, client)
	if err != nil {
		return err
	}
	for index, itemIndex := range missingIndexes {
		items[itemIndex] = missingItems[index]
	}
	if err = s.replaceMaterialImportGroupMedia(ctx, group.Id, task.Id, task.TenantId, task.AccountId, items); err != nil {
		return err
	}
	if profileId > 0 {
		if err = s.saveMaterialImportProfileMissingMedia(ctx, task, group, profileId, missingItems); err != nil {
			return err
		}
		if err = s.refreshMaterialImportProfileMetadata(ctx, task, group, profileId); err != nil {
			return err
		}
		if err = s.ensureMaterialImportTelegramIndex(ctx, task, group, profileId); err != nil {
			return err
		}
		if err = s.syncProfileNoteIndex(ctx, profileId); err != nil {
			return err
		}
		return s.materialImportMarkGroupDone(ctx, group.Id, profileId, 0)
	}
	profileId, err = s.saveMaterialImportGroupProfile(ctx, task, group, items)
	if err != nil {
		return err
	}
	return s.materialImportMarkGroupDone(ctx, group.Id, profileId, 0)
}

func (s *sSysPublish) downloadMaterialImportItems(ctx context.Context, task *sysin.MaterialImportTaskModel, groupID int64, items []collectMediaItem, client *telegram.Client) (int, int, error) {
	if len(items) == 0 {
		return 0, 0, nil
	}
	concurrency := accountCollectMediaConcurrency(ctx)
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 4 {
		concurrency = 4
	}
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	done := 0
	failed := 0
	group, groupCtx := errgroup.WithContext(ctx)
	for index := range items {
		index := index
		group.Go(func() error {
			select {
			case sem <- struct{}{}:
			case <-groupCtx.Done():
				return groupCtx.Err()
			}
			defer func() { <-sem }()

			item := normalizeCollectMediaItem(items[index])
			if materialImportMediaItemReady(item) {
				mu.Lock()
				done++
				mu.Unlock()
				return nil
			}
			if strings.TrimSpace(item.StoragePath) != "" {
				item.StoragePath = ""
				items[index].StoragePath = ""
			}
			meta, ok := gotdCollectMediaMetaFromItem(item)
			if !ok {
				mu.Lock()
				failed++
				mu.Unlock()
				return nil
			}
			itemCtx, cancel := context.WithTimeout(groupCtx, materialImportMediaItemTimeout)
			down, err := s.downloadTelegramMediaWithClient(itemCtx, task.TenantId, task.TgAccountId, item, meta, client)
			cancel()
			if err != nil {
				return err
			}
			if strings.TrimSpace(down.Path) != "" {
				mu.Lock()
				items[index].StoragePath = down.Path
				mu.Unlock()
			}
			mu.Lock()
			done++
			mu.Unlock()
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return 0, 0, err
	}
	if err := s.materialImportUpdateGroupProgress(ctx, groupID, done, failed); err != nil {
		return 0, 0, err
	}
	return done, failed, nil
}

func materialImportMediaItemReady(item collectMediaItem) bool {
	if strings.TrimSpace(item.FileUrl) != "" {
		return true
	}
	path := strings.TrimSpace(item.StoragePath)
	return path != "" && fileNonEmpty(resolveTelegramLocalPath(path))
}

type materialImportProfileMediaCounts struct {
	Display int
	Verify  int
}

func (s *sSysPublish) materialImportProfileMediaCounts(ctx context.Context, profileId int64) (materialImportProfileMediaCounts, error) {
	if profileId <= 0 {
		return materialImportProfileMediaCounts{}, nil
	}
	mod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).WhereNull("deleted_at")
	display, err := mod.Where("purpose", "display").Count()
	if err != nil {
		return materialImportProfileMediaCounts{}, gerror.Wrap(err, "统计资料展示媒体失败")
	}
	verify, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).Where("purpose", "verify").WhereNull("deleted_at").Count()
	if err != nil {
		return materialImportProfileMediaCounts{}, gerror.Wrap(err, "统计资料验证媒体失败")
	}
	return materialImportProfileMediaCounts{Display: display, Verify: verify}, nil
}

func materialImportMissingMediaItems(items []collectMediaItem, counts materialImportProfileMediaCounts) ([]collectMediaItem, []int) {
	needed := map[string]int{
		"display": counts.Display,
		"verify":  counts.Verify,
	}
	incoming := map[string]int{"display": 0, "verify": 0}
	for _, item := range items {
		incoming[materialImportMediaPurpose(item)]++
	}
	missing := make(map[string]int, 2)
	for purpose, total := range incoming {
		missing[purpose] = total - needed[purpose]
		if missing[purpose] < 0 {
			missing[purpose] = 0
		}
	}
	selected := make([]collectMediaItem, 0)
	indexes := make([]int, 0)
	seen := map[string]int{"display": 0, "verify": 0}
	for index, item := range items {
		purpose := materialImportMediaPurpose(item)
		position := seen[purpose]
		seen[purpose] = position + 1
		if position < needed[purpose] || missing[purpose] <= 0 {
			continue
		}
		selected = append(selected, item)
		indexes = append(indexes, index)
		missing[purpose]--
	}
	return selected, indexes
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

func (s *sSysPublish) materialImportMarkGroupDone(ctx context.Context, id int64, profileId int64, taskProfileId int64) error {
	_, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).Where("id", id).Data(g.Map{
		"status":          sysin.MaterialImportStatusSuccess,
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
	meta := gotdCollectMediaMeta{
		Kind:          strings.TrimSpace(item.SourceKind),
		Id:            item.SourceMediaId,
		AccessHash:    item.SourceAccessHash,
		FileReference: append([]byte(nil), item.SourceFileReference...),
		ThumbSize:     strings.TrimSpace(item.SourceThumbSize),
		MimeType:      strings.TrimSpace(item.SourceMimeType),
		DCID:          item.SourceDCId,
		Size:          item.SourceSize,
	}
	if meta.Id <= 0 {
		return gotdCollectMediaMeta{}, false
	}
	return meta, true
}
