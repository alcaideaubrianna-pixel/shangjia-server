package sys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"golang.org/x/sync/errgroup"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const materialImportMediaItemTimeout = 10 * time.Minute
const materialImportGroupTimeout = 12 * time.Minute
const materialImportStaleGroupAfter = 15 * time.Minute
const materialImportDispatchTimeout = 30 * time.Second
const materialImportGroupConcurrency = 4

func (s *sSysPublish) executeMaterialImportMedia(ctx context.Context, taskId int64) error {
	task, err := s.materialImportTaskById(ctx, taskId, 0)
	if err != nil {
		return err
	}
	if err = s.materialImportEnsureNotCanceled(ctx, task.Id); err != nil {
		return err
	}
	return s.materialImportDownloadGroups(ctx, task)
}

func (s *sSysPublish) materialImportDownloadGroups(ctx context.Context, task *sysin.MaterialImportTaskModel) error {
	// Task heartbeat can remain fresh while a worker is blocked in Telegram I/O.
	// Requeue stale groups independently so one group cannot hold the import forever.
	if err := s.materialImportResetStaleGroups(ctx, task.Id); err != nil {
		return err
	}
	groups, err := s.materialImportPendingMediaGroups(ctx, task.Id)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		return s.materialImportMarkSuccess(ctx, task.Id, task.UpdatedBy, g.Map{"message": "资料导入完成"})
	}
	concurrency := materialImportGroupConcurrency
	g.Log().Infof(ctx, "资料导入媒体分组开始调度 taskId:%d groups:%d concurrency:%d", task.Id, len(groups), concurrency)
	jobs := make(chan *sysin.MaterialImportGroupModel)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for group := range jobs {
				if err := s.materialImportEnsureNotCanceled(ctx, task.Id); err != nil {
					materialImportSendErr(errCh, err)
					return
				}
				groupCtx, cancel := context.WithTimeout(ctx, materialImportGroupTimeout)
				groupErr := s.materialImportDownloadGroup(groupCtx, task, group)
				cancel()
				if errors.Is(groupErr, context.DeadlineExceeded) {
					groupErr = gerror.New("TG媒体分组下载超时，已自动重试")
				}
				if groupErr != nil {
					if collectMediaSourceGone(groupErr) {
						_ = s.materialImportMarkGroupDiscarded(ctx, group.Id, group.MediaTotal, groupErr.Error())
						_ = s.refreshMaterialImportTaskStats(ctx, task.Id)
						continue
					}
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
		dispatchErr := dispatchMaterialImportGroup(ctx, jobs, errCh, group, materialImportDispatchTimeout)
		if dispatchErr != nil {
			close(jobs)
			wg.Wait()
			return s.materialImportHandleMediaRetry(ctx, task, dispatchErr)
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

func dispatchMaterialImportGroup(ctx context.Context, jobs chan<- *sysin.MaterialImportGroupModel, errCh <-chan error, group *sysin.MaterialImportGroupModel, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = materialImportDispatchTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case jobs <- group:
		return nil
	case err := <-errCh:
		return err
	case <-timer.C:
		return newCollectMediaFairnessRetryError("资料导入媒体工作线程无响应", 15*time.Second)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *sSysPublish) materialImportResetStaleGroups(ctx context.Context, taskID int64) error {
	if taskID <= 0 {
		return nil
	}
	_, err := pdao.YoubanPublishMaterialImportGroup.Ctx(ctx).
		Where("task_id", taskID).
		Where("status", sysin.MaterialImportStatusRunning).
		WhereLTE("updated_at", gtime.Now().Add(-materialImportStaleGroupAfter)).
		Data(g.Map{
			"status":        sysin.MaterialImportStatusPending,
			"media_done":    0,
			"media_failed":  0,
			"error_message": "媒体下载长时间无进度，已自动重试",
			"updated_at":    gtime.Now(),
		}).Update()
	return err
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

func (s *sSysPublish) materialImportDownloadGroup(ctx context.Context, task *sysin.MaterialImportTaskModel, group *sysin.MaterialImportGroupModel) error {
	if err := s.materialImportMarkGroupRunning(ctx, group.Id); err != nil {
		return gerror.Wrap(err, "标记TG导入分组处理中失败")
	}
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
	_, failed, err := s.downloadMaterialImportItems(ctx, task, group.Id, missingItems)
	if err != nil {
		return err
	}
	if failed > 0 {
		return gerror.Newf("TG媒体下载失败：%d个媒体", failed)
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

func (s *sSysPublish) downloadMaterialImportItems(ctx context.Context, task *sysin.MaterialImportTaskModel, groupID int64, items []collectMediaItem) (int, int, error) {
	if len(items) == 0 {
		return 0, 0, nil
	}
	concurrency := materialImportGroupConcurrency
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	progressMu := new(sync.Mutex)
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
				currentDone, currentFailed := done, failed
				mu.Unlock()
				if err := s.materialImportPersistItemProgress(groupCtx, progressMu, groupID, currentDone, currentFailed); err != nil {
					return err
				}
				return nil
			}
			if strings.TrimSpace(item.StoragePath) != "" {
				item.StoragePath = ""
				items[index].StoragePath = ""
			}
			itemCtx, cancel := context.WithTimeout(groupCtx, materialImportMediaItemTimeout)
			down, err := s.downloadTelegramMedia(itemCtx, task.TenantId, task.AccountId, task.TgAccountId, 0, item)
			cancel()
			if err != nil {
				if collectMediaSourceGone(err) {
					return err
				}
				if collectMediaRetryErrorFrom(err) != nil {
					return err
				}
				mu.Lock()
				failed++
				currentDone, currentFailed := done, failed
				mu.Unlock()
				if progressErr := s.materialImportPersistItemProgress(groupCtx, progressMu, groupID, currentDone, currentFailed); progressErr != nil {
					return progressErr
				}
				return nil
			}
			if strings.TrimSpace(down.Path) != "" {
				mu.Lock()
				items[index].StoragePath = down.Path
				items[index].FileUrl = down.FileUrl
				if strings.TrimSpace(down.Item.FileId) != "" {
					items[index] = down.Item
					items[index].StoragePath = down.Path
					items[index].FileUrl = down.FileUrl
				}
				mu.Unlock()
			}
			mu.Lock()
			done++
			currentDone, currentFailed := done, failed
			mu.Unlock()
			return s.materialImportPersistItemProgress(groupCtx, progressMu, groupID, currentDone, currentFailed)
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

func (s *sSysPublish) materialImportPersistItemProgress(ctx context.Context, mu *sync.Mutex, groupID int64, done, failed int) error {
	mu.Lock()
	defer mu.Unlock()
	return s.materialImportUpdateGroupProgress(ctx, groupID, done, failed)
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
