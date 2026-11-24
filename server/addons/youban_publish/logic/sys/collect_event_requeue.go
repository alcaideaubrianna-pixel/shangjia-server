package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	pdao "hotgo/addons/youban_publish/internal/dao"
)

// RequeueCollectEvents accepts explicitly paired display events for repair.
// Legacy authorization recovery is handed to the existing process queue;
// other maintenance completion keeps its existing synchronous behavior.
func RequeueCollectEvents(ctx context.Context, eventIDs []int64) (int, error) {
	processed, failed, err := RequeueCollectEventsBestEffort(ctx, eventIDs)
	if err != nil {
		return processed, err
	}
	if len(failed) > 0 {
		return processed, gerror.Newf("采集事件重放失败 %d 条，首条事件:%d", len(failed), failed[0])
	}
	return processed, nil
}

// RequeueCollectEventsBestEffort processes each event independently. A stale
// or unmatched event must not stop a repair batch from processing later events.
// Its count is accepted repair calls, including deduplicated legacy wakeups;
// it is not a count of newly persisted profiles or completed publications.
func RequeueCollectEventsBestEffort(ctx context.Context, eventIDs []int64) (int, []int64, error) {
	eventIDs = uniqueIds(eventIDs)
	if len(eventIDs) > 1000 {
		return 0, nil, gerror.New("单次采集事件重放不能超过1000条")
	}
	service := NewSysPublish()
	processed := 0
	failed := make([]int64, 0)
	for _, eventID := range eventIDs {
		if eventID <= 0 {
			continue
		}
		event, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(eventID).One()
		if err != nil {
			failed = append(failed, eventID)
			g.Log().Warningf(ctx, "读取采集事件失败 eventId:%d err:%+v", eventID, err)
			continue
		}
		if event.IsEmpty() {
			failed = append(failed, eventID)
			g.Log().Warningf(ctx, "采集事件不存在 eventId:%d", eventID)
			continue
		}
		payload := collectProcessQueuePayload{
			TenantId: event["tenant_id"].Int64(), AccountId: event["account_id"].Int64(),
			EventId: eventID, SourceId: event["source_id"].Int64(), SourceChatId: event["source_chat_id"].String(),
		}
		// Maintenance replay must never enter the realtime window: it may
		// ingest a display group without verification after the timeout.
		if isCollectRepairSuspended(event["status"].String()) || event["material_role"].String() != collectMaterialRoleDisplay {
			failed = append(failed, eventID)
			g.Log().Warningf(ctx, "采集事件未确认展示组，拒绝重放 eventId:%d", eventID)
			continue
		}
		if err = service.completeTargetCollectEvent(ctx, payload); err != nil {
			failed = append(failed, eventID)
			g.Log().Warningf(ctx, "完成采集事件媒体和入库失败 eventId:%d err:%+v", eventID, err)
			continue
		}
		processed++
	}
	g.Log().Info(ctx, "指定采集事件重新处理完成", g.Map{"eventIds": eventIDs, "processed": processed, "failed": len(failed)})
	return processed, failed, nil
}

// RepairCollectMediaEvents rebuilds media without changing publish or review
// state. It is used for historical cache corruption where normal event replay
// must remain protected by its publication-state guards.
func RepairCollectMediaEvents(ctx context.Context, eventIDs []int64) (int, []int64, error) {
	eventIDs = uniqueIds(eventIDs)
	processed := 0
	failed := make([]int64, 0)
	service := NewSysPublish()
	for _, eventID := range eventIDs {
		event, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(eventID).One()
		if err != nil || event.IsEmpty() || event["material_role"].String() != collectMaterialRoleDisplay {
			failed = append(failed, eventID)
			continue
		}
		if _, err = service.cacheCollectEventStructuredMedia(ctx, event); err != nil {
			failed = append(failed, eventID)
			continue
		}
		if err = service.syncCollectEventMediaSnapshot(ctx, eventID); err != nil {
			failed = append(failed, eventID)
			continue
		}
		processed++
	}
	g.Log().Info(ctx, "历史采集媒体修复完成", g.Map{"processed": processed, "failed": len(failed)})
	return processed, failed, nil
}

func (s *sSysPublish) completeTargetCollectEvent(ctx context.Context, payload collectProcessQueuePayload) error {
	// Authorization and bounded parent/child restoration precede every media
	// read/download. The returned records retain their persisted account IDs.
	display, verifies, restored, err := s.prepareCollectEventRequeue(ctx, payload)
	if err != nil {
		return err
	}
	// The scoped maintenance entry is authorized above. It may explicitly
	// recheck repaired configuration without waiting for an automatic cooldown.
	ctx = context.WithValue(ctx, collectManualReplayContextKey{}, payload.EventId)
	if restored {
		// prepare has committed both the ignored events and canceled media.
		// Do not spend the HTTP/MCP deadline downloading media: the ordinary
		// parent+verify worker resumes the preserved pair. Queue dedupe is an
		// accepted existing wakeup, never a profile-persistence receipt.
		// The optional request scope may use the resolved business owner.
		// Queue identity must keep the event's original storage account/chat.
		payload.AccountId = collectEventStorageAccountID(display)
		payload.SourceChatId = display["source_chat_id"].String()
		if err := s.enqueueCollectProcess(ctx, payload, 0); err != nil {
			return gerror.Wrap(err, "误拦截状态已恢复，但提交父展示处理任务失败，请重试")
		}
		g.Log().Info(ctx, "旧频道授权误拦截恢复已接受", g.Map{
			"event": "collect_requeue_accepted", "eventId": payload.EventId,
			"tenantId": payload.TenantId, "accountId": payload.AccountId,
			"sourceId": payload.SourceId, "sourceChatId": payload.SourceChatId,
			"pairedVerifyCount": len(verifies), "outcome": "accepted",
		})
		return nil
	}
	for _, verify := range verifies {
		verifyRows, rowsErr := s.collectEventMediaRows(ctx, verify["id"].Int64())
		if rowsErr != nil {
			return rowsErr
		}
		hasVideo := false
		for _, row := range verifyRows {
			if row != nil && row.MediaType == "video" {
				hasVideo = true
			}
		}
		if !hasVideo {
			return gerror.Newf("验证组没有视频 eventId:%d", payload.EventId)
		}
	}
	events := append([]gdb.Record{display}, verifies...)
	for _, event := range events {
		changed, cacheErr := s.cacheCollectEventStructuredMedia(ctx, event)
		if cacheErr != nil {
			return cacheErr
		}
		if changed {
			if cacheErr = s.syncCollectEventMediaSnapshot(ctx, event["id"].Int64()); cacheErr != nil {
				return cacheErr
			}
		}
		ready, readyErr := s.collectEventMediaReady(ctx, event["id"].Int64())
		if readyErr != nil {
			return readyErr
		}
		if !ready {
			return gerror.Newf("采集事件媒体尚未完整缓存 eventId:%d", event["id"].Int64())
		}
	}
	if err = s.processCollectEvent(ctx, payload.EventId, payload.TenantId, payload.AccountId); err != nil {
		return err
	}
	return nil
}
