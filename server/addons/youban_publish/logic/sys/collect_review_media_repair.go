package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

const collectReviewMediaRepairLimit = 100

func RepairCollectReviewMedia(ctx context.Context, reviewIds []int64) error {
	return service.SysPublish().RepairCollectReviewMedia(ctx, reviewIds)
}

func (s *sSysPublish) RepairCollectReviewMedia(ctx context.Context, reviewIds []int64) error {
	reviewIds = uniqueIds(reviewIds)
	if len(reviewIds) == 0 {
		return gerror.New("必须指定待修复的审核ID")
	}
	if len(reviewIds) > collectReviewMediaRepairLimit {
		return gerror.Newf("单次最多修复%d条审核资料", collectReviewMediaRepairLimit)
	}
	reviewCols := pdao.YoubanPublishCollectReview.Columns()
	reviews, err := pdao.YoubanPublishCollectReview.Ctx(ctx).
		WhereIn(reviewCols.Id, reviewIds).
		OrderAsc(reviewCols.Id).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取待修复审核资料失败")
	}
	if len(reviews) != len(reviewIds) {
		return gerror.Newf("待修复审核资料不存在 expected:%d actual:%d", len(reviewIds), len(reviews))
	}
	repaired := 0
	requeued := 0
	for _, review := range reviews {
		eventId := review[reviewCols.EventId].Int64()
		repairCtx := collectMediaRuntimeContext(ctx, review[reviewCols.AccountId].Int64())
		event, eventErr := pdao.YoubanPublishCollectEvent.Ctx(repairCtx).Where("id", eventId).One()
		if eventErr != nil {
			return gerror.Wrapf(eventErr, "读取审核采集事件失败 reviewId:%d", review[reviewCols.Id].Int64())
		}
		if event.IsEmpty() {
			return gerror.Newf("审核采集事件不存在 reviewId:%d eventId:%d", review[reviewCols.Id].Int64(), eventId)
		}
		queued, repairErr := s.repairCollectReviewEventMedia(repairCtx, event)
		if repairErr != nil {
			return gerror.Wrapf(repairErr, "修复审核媒体失败 reviewId:%d", review[reviewCols.Id].Int64())
		}
		if queued {
			requeued++
			continue
		}
		content, contentErr := s.collectContentFromEvent(repairCtx, event)
		if contentErr != nil {
			return gerror.Wrapf(contentErr, "读取审核媒体快照失败 reviewId:%d", review[reviewCols.Id].Int64())
		}
		prepared, prepareErr := s.prepareCollectMaterial(repairCtx, event, content)
		if prepareErr != nil {
			return gerror.Wrapf(prepareErr, "持久化审核媒体失败 reviewId:%d", review[reviewCols.Id].Int64())
		}
		if prepared == nil || len(prepared.Media) == 0 {
			return gerror.Newf("审核资料没有可恢复媒体 reviewId:%d", review[reviewCols.Id].Int64())
		}
		repaired++
		g.Log().Infof(repairCtx, "审核媒体修复完成 reviewId:%d eventId:%d media:%d", review[reviewCols.Id].Int64(), eventId, len(prepared.Media))
	}
	g.Log().Infof(ctx, "审核媒体修复任务完成 requested:%d repaired:%d requeued:%d", len(reviewIds), repaired, requeued)
	return nil
}

func (s *sSysPublish) repairCollectReviewEventMedia(ctx context.Context, rootEvent gdb.Record) (bool, error) {
	events := []gdb.Record{rootEvent}
	verify, err := s.pairedCollectVerifyEvent(ctx, rootEvent["id"].Int64())
	if err != nil {
		return false, err
	}
	if !verify.IsEmpty() {
		events = append(events, verify)
	}
	queued := false
	rootQueued := false
	for _, event := range events {
		rows, rowErr := s.collectEventMediaRows(ctx, event["id"].Int64())
		if rowErr != nil {
			return false, rowErr
		}
		missing := false
		for _, row := range rows {
			if row == nil || strings.TrimSpace(row.FileUrl) != "" {
				continue
			}
			storagePath := strings.TrimSpace(row.StoragePath)
			if storagePath != "" && !isCollectMediaCachePath(storagePath) {
				continue
			}
			if storagePath != "" {
				localPath, pathErr := resolveMediaLocalPath(storagePath)
				if pathErr == nil && fileExists(localPath) {
					continue
				}
			} else if strings.TrimSpace(row.SourceFileId) == "" && strings.TrimSpace(row.SourceMessageRef) == "" {
				continue
			}
			missing = true
			_, updateErr := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where("id", row.Id).Data(g.Map{
				"storage_path":  "",
				"poster_url":    "",
				"cache_status":  collectMediaCachePending,
				"error_message": "等待重新下载审核媒体",
				"updated_at":    gtime.Now(),
			}).Update()
			if updateErr != nil {
				return false, gerror.Wrapf(updateErr, "重置审核媒体缓存状态失败 mediaId:%d", row.Id)
			}
		}
		if !missing {
			continue
		}
		if err = s.markCollectEvent(ctx, event["id"].Int64(), sysin.CollectEventStatusMediaPending, "等待重新下载审核媒体"); err != nil {
			return false, err
		}
		if _, err = s.enqueueCollectMediaCacheDeferred(ctx, collectMediaQueuePayloadFromEvent(event), 0); err != nil {
			return false, gerror.Wrap(err, "重新投递审核媒体下载任务失败")
		}
		queued = true
		rootQueued = rootQueued || event["id"].Int64() == rootEvent["id"].Int64()
		g.Log().Warningf(ctx, "审核媒体本地缓存不存在，已重新投递下载 eventId:%d", event["id"].Int64())
	}
	if queued && !rootQueued {
		if err = s.markCollectEvent(ctx, rootEvent["id"].Int64(), sysin.CollectEventStatusMediaPending, "等待验证媒体重新下载"); err != nil {
			return false, err
		}
		if _, err = s.enqueueCollectMediaCacheDeferred(ctx, collectMediaQueuePayloadFromEvent(rootEvent), 15*time.Second); err != nil {
			return false, gerror.Wrap(err, "重新投递审核正文处理任务失败")
		}
	}
	return queued, nil
}
