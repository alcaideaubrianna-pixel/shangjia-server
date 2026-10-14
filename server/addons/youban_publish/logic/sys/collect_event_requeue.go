package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	pdao "hotgo/addons/youban_publish/internal/dao"
)

// RequeueCollectEvents replays explicitly paired display events. It does not
// run the realtime classification window or infer missing verification groups.
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
func RequeueCollectEventsBestEffort(ctx context.Context, eventIDs []int64) (int, []int64, error) {
	service := NewSysPublish()
	processed := 0
	failed := make([]int64, 0)
	for _, eventID := range uniqueIds(eventIDs) {
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
		if event["status"].String() == CollectRepairSuspendedStatus || event["material_role"].String() != collectMaterialRoleDisplay {
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

func (s *sSysPublish) completeTargetCollectEvent(ctx context.Context, payload collectProcessQueuePayload) error {
	display, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(payload.EventId).One()
	if err != nil {
		return gerror.Wrap(err, "重新读取展示采集事件失败")
	}
	verify, err := s.pairedCollectVerifyEvent(ctx, payload.EventId)
	if err != nil {
		return err
	}
	if !verify.IsEmpty() {
		ready, readyErr := s.collectEventMediaReady(ctx, verify["id"].Int64())
		if readyErr != nil {
			return gerror.Wrap(readyErr, "验证视频完整性检查失败")
		}
		if !ready {
			return gerror.Newf("验证视频媒体尚未完整缓存 eventId:%d verifyEventId:%d", payload.EventId, verify["id"].Int64())
		}
		for _, field := range []string{"tenant_id", "account_id", "source_id", "source_chat_id"} {
			if display[field].String() != verify[field].String() {
				return gerror.Newf("展示组与验证组来源不一致 eventId:%d", payload.EventId)
			}
		}
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
	events := []gdb.Record{display}
	if !verify.IsEmpty() {
		events = append(events, verify)
	}
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
