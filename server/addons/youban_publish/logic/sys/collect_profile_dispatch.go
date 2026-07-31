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

func (s *sSysPublish) submitCollectProfileDispatch(ctx context.Context, dispatchId, profileId int64, event gdb.Record) error {
	if dispatchId <= 0 || profileId <= 0 || event.IsEmpty() {
		return gerror.New("采集分发参数不完整")
	}
	channelMap, err := collectDispatchChannelMap(ctx, []int64{dispatchId})
	if err != nil {
		return err
	}
	channelIds := channelMap[dispatchId]
	if len(channelIds) == 0 {
		return gerror.New("采集规则未配置目标频道")
	}
	operationNo := fmt.Sprintf("collect:%d", dispatchId)
	if _, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where("id", dispatchId).Data(g.Map{
		"profile_id": profileId,
		"status":     sysin.CollectDispatchStatusPending, "error_message": "", "updated_at": gtime.Now(),
	}).Update(); err != nil {
		return gerror.Wrap(err, "更新采集分发资料失败")
	}
	meta := telegramProfilePublishMeta{
		CollectEventId:         event["id"].Int64(),
		CollectSourceId:        event["source_id"].Int64(),
		CollectSourceChatId:    strings.TrimSpace(event["source_chat_id"].String()),
		CollectSourceMessageId: event["source_message_id"].Int64(),
	}
	if err := s.submitProfilePublishWithMeta(ctx, profileId, event["tenant_id"].Int64(), event["account_id"].Int64(), event["account_id"].Int64(), operationNo, channelIds, false, meta); err != nil {
		_ = s.markCollectDispatchFailed(ctx, dispatchId, err.Error())
		return gerror.Wrap(err, "创建采集TG发送任务失败")
	}
	return nil
}

func (s *sSysPublish) markCollectDispatchSentByProfile(ctx context.Context, profileId, eventId int64) error {
	if profileId <= 0 || eventId <= 0 {
		return nil
	}
	rows, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields("id,event_id").Where("profile_id", profileId).Where("event_id", eventId).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing}).All()
	if err != nil {
		return gerror.Wrap(err, "读取采集分发事件失败")
	}
	_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("profile_id", profileId).Where("event_id", eventId).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing}).
		Data(g.Map{"status": sysin.CollectDispatchStatusSent, "error_message": "", "finished_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新采集分发发送状态失败")
	}
	for _, row := range rows {
		_, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", row["event_id"].Int64()).
			Where("status", sysin.CollectEventStatusDispatched).
			Data(g.Map{"status": sysin.CollectEventStatusProcessed, "error_message": "", "processed_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
		if err != nil {
			return gerror.Wrap(err, "更新采集事件完成状态失败")
		}
	}
	if err = s.warmCollectDedupeCacheForSentDispatches(ctx, rows); err != nil {
		g.Log().Warningf(ctx, "采集分发成功后写入去重缓存失败 profileId:%d eventId:%d err:%+v", profileId, eventId, err)
	}
	return nil
}

func (s *sSysPublish) markCollectDispatchFailedByProfile(ctx context.Context, profileId, eventId int64, message string) error {
	if profileId <= 0 || eventId <= 0 {
		return nil
	}
	rows, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields("event_id").Where("profile_id", profileId).Where("event_id", eventId).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing}).All()
	if err != nil {
		return gerror.Wrap(err, "读取采集失败分发事件失败")
	}
	_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("profile_id", profileId).Where("event_id", eventId).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing}).
		Data(g.Map{"status": sysin.CollectDispatchStatusFailed, "error_message": message, "finished_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新采集分发失败状态失败")
	}
	return s.markCollectEventsFailedByDispatchRows(ctx, rows, message)
}
