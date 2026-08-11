package sys

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type collectTimelineCollectorEvent struct {
	Id          int64
	SourceId    int64
	ChatId      string
	MessageId   int64
	ReceivedAt  *gtime.Time
	CreatedAt   *gtime.Time
	ProcessedAt *gtime.Time
	DeliveredAt *gtime.Time
}

type collectTimelineAggregate struct {
	ReceivedAt       *gtime.Time
	CollectorCreated *gtime.Time
	CollectorReady   *gtime.Time
	CollectorDone    *gtime.Time
}

func (s *sSysPublish) fillCollectMaterialDiagnoseTimelines(ctx context.Context, tenantId, accountId int64, items []*sysin.CollectMaterialDiagnoseItem) ([]*sysin.CollectMaterialTimelineModel, error) {
	eventIds := make([]int64, 0, len(items))
	for _, item := range items {
		if item != nil && item.EventId > 0 {
			eventIds = append(eventIds, item.EventId)
		}
	}
	eventIds = uniqueIds(eventIds)
	if len(eventIds) == 0 {
		return make([]*sysin.CollectMaterialTimelineModel, 0), nil
	}

	rows, err := g.DB().Model(publishCollectEventTable+" e").Safe().Ctx(ctx).
		LeftJoin(publishCollectDispatchTable+" d", "d.event_id=e.id").
		LeftJoin("hg_content_profile p", "p.id=d.profile_id").
		LeftJoin(publishTgJobTable+" j", "j.collect_event_id=e.id AND j.profile_id=d.profile_id").
		Fields(`e.id AS event_id,e.source_id,e.source_type,e.source_chat_id,e.source_message_id,e.source_grouped_id,e.media_count,e.received_at,e.created_at AS publish_event_created_at,
d.id AS dispatch_id,d.status AS dispatch_status,d.created_at AS dispatch_created_at,
p.id AS profile_id,p.profile_no,p.created_at AS profile_created_at,
j.id AS job_id,j.channel_id,j.status AS job_status,j.created_at AS job_created_at,j.dispatched_at,j.sent_at`).
		Where("e.tenant_id", tenantId).
		Where("e.account_id", accountId).
		WhereIn("e.id", eventIds).
		OrderAsc("e.id").
		OrderAsc("j.id").
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集端到端时间线失败")
	}

	messageIdsByEvent, sourceIds, messageIds, err := collectTimelineMessageIds(ctx, tenantId, accountId, eventIds)
	if err != nil {
		return nil, err
	}
	collectorByMessage, err := collectTimelineCollectorEvents(ctx, tenantId, sourceIds, messageIds)
	if err != nil {
		return nil, err
	}
	collectorByEvent := collectTimelineCollectorAggregates(rows, messageIdsByEvent, collectorByMessage)
	mediaReadyByEvent, err := collectTimelineMediaReadyAt(ctx, tenantId, accountId, eventIds)
	if err != nil {
		return nil, err
	}

	result := make([]*sysin.CollectMaterialTimelineModel, 0, len(rows))
	for _, row := range rows {
		eventId := row["event_id"].Int64()
		timeline := &sysin.CollectMaterialTimelineModel{
			EventId:         eventId,
			ProfileId:       row["profile_id"].Int64(),
			ProfileNo:       row["profile_no"].String(),
			SourceId:        row["source_id"].Int64(),
			SourceType:      row["source_type"].String(),
			SourceChatId:    row["source_chat_id"].String(),
			SourceMessageId: row["source_message_id"].Int64(),
			SourceGroupedId: row["source_grouped_id"].String(),
			MediaCount:      row["media_count"].Int(),
			DispatchId:      row["dispatch_id"].Int64(),
			DispatchStatus:  row["dispatch_status"].String(),
			JobId:           row["job_id"].Int64(),
			ChannelId:       row["channel_id"].Int64(),
			JobStatus:       row["job_status"].String(),
			Nodes:           make([]*sysin.CollectMaterialTimelineNodeModel, 0, 10),
		}
		aggregate := collectorByEvent[eventId]
		appendCollectTimelineNode(timeline, "tg_received", "TG消息收到", aggregate.ReceivedAt)
		appendCollectTimelineNode(timeline, "collector_persisted", "Collector原始事件入库", aggregate.CollectorCreated)
		appendCollectTimelineNode(timeline, "collector_ready", "Collector原始事件处理完成", aggregate.CollectorReady)
		appendCollectTimelineNode(timeline, "collector_delivered", "Collector交付发布插件", aggregate.CollectorDone)
		appendCollectTimelineNode(timeline, "publish_event_persisted", "发布采集事件入库", row["publish_event_created_at"].GTime())
		appendCollectTimelineNode(timeline, "media_ready", "媒体下载与缓存完成", mediaReadyByEvent[eventId])
		appendCollectTimelineNode(timeline, "dispatch_created", "采集分发任务创建", row["dispatch_created_at"].GTime())
		appendCollectTimelineNode(timeline, "profile_created", "资料入库", row["profile_created_at"].GTime())
		appendCollectTimelineNode(timeline, "job_created", "TG推送Job创建", row["job_created_at"].GTime())
		appendCollectTimelineNode(timeline, "job_dispatched", "TG推送Job进入发送", row["dispatched_at"].GTime())
		appendCollectTimelineNode(timeline, "job_sent", "TG推送成功", row["sent_at"].GTime())
		finalizeCollectTimeline(timeline)
		result = append(result, timeline)
	}
	return result, nil
}

func collectTimelineMessageIds(ctx context.Context, tenantId, accountId int64, eventIds []int64) (map[int64][]int64, []int64, []int64, error) {
	rows, err := g.DB().Model("hg_youban_publish_collect_event_media").Safe().Ctx(ctx).
		Fields("event_id,source_id,source_message_id").
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("event_id", eventIds).
		WhereGT("source_message_id", 0).
		All()
	if err != nil {
		return nil, nil, nil, gerror.Wrap(err, "读取采集时间线来源消息失败")
	}
	byEvent := make(map[int64][]int64, len(eventIds))
	sourceIds := make([]int64, 0)
	messageIds := make([]int64, 0)
	for _, row := range rows {
		eventId := row["event_id"].Int64()
		messageId := row["source_message_id"].Int64()
		if eventId <= 0 || messageId <= 0 {
			continue
		}
		byEvent[eventId] = append(byEvent[eventId], messageId)
		sourceIds = append(sourceIds, row["source_id"].Int64())
		messageIds = append(messageIds, messageId)
	}
	return byEvent, uniqueIds(sourceIds), uniqueIds(messageIds), nil
}

func collectTimelineCollectorEvents(ctx context.Context, tenantId int64, sourceIds, messageIds []int64) (map[string]*collectTimelineCollectorEvent, error) {
	result := make(map[string]*collectTimelineCollectorEvent)
	if len(sourceIds) == 0 || len(messageIds) == 0 {
		return result, nil
	}
	rows, err := g.DB().Model("hg_tg_collector_event e").Safe().Ctx(ctx).
		LeftJoin("hg_tg_collector_delivery d", "d.event_id=e.id").
		Fields("e.id,e.source_id,e.chat_id,e.message_id,e.received_at,e.created_at,e.processed_at,MAX(d.updated_at) AS delivered_at").
		Where("e.tenant_id", tenantId).
		Where("e.source_type", "account").
		WhereIn("e.source_id", sourceIds).
		WhereIn("e.message_id", messageIds).
		Group("e.id,e.source_id,e.chat_id,e.message_id,e.received_at,e.created_at,e.processed_at").
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取Collector原始事件时间线失败")
	}
	for _, row := range rows {
		item := &collectTimelineCollectorEvent{
			Id:          row["id"].Int64(),
			SourceId:    row["source_id"].Int64(),
			ChatId:      row["chat_id"].String(),
			MessageId:   row["message_id"].Int64(),
			ReceivedAt:  row["received_at"].GTime(),
			CreatedAt:   row["created_at"].GTime(),
			ProcessedAt: row["processed_at"].GTime(),
			DeliveredAt: row["delivered_at"].GTime(),
		}
		result[collectTimelineMessageKey(item.SourceId, item.ChatId, item.MessageId)] = item
	}
	return result, nil
}

func collectTimelineCollectorAggregates(rows gdb.Result, messageIdsByEvent map[int64][]int64, collectorByMessage map[string]*collectTimelineCollectorEvent) map[int64]collectTimelineAggregate {
	result := make(map[int64]collectTimelineAggregate)
	for _, row := range rows {
		eventId := row["event_id"].Int64()
		if _, exists := result[eventId]; exists {
			continue
		}
		aggregate := collectTimelineAggregate{}
		sourceId := row["source_id"].Int64()
		chatId := row["source_chat_id"].String()
		for _, messageId := range messageIdsByEvent[eventId] {
			item := collectorByMessage[collectTimelineMessageKey(sourceId, chatId, messageId)]
			if item == nil {
				continue
			}
			aggregate.ReceivedAt = earlierCollectTimelineTime(aggregate.ReceivedAt, item.ReceivedAt)
			aggregate.CollectorCreated = earlierCollectTimelineTime(aggregate.CollectorCreated, item.CreatedAt)
			aggregate.CollectorReady = laterCollectTimelineTime(aggregate.CollectorReady, item.ProcessedAt)
			aggregate.CollectorDone = laterCollectTimelineTime(aggregate.CollectorDone, item.DeliveredAt)
		}
		result[eventId] = aggregate
	}
	return result
}

func collectTimelineMediaReadyAt(ctx context.Context, tenantId, accountId int64, eventIds []int64) (map[int64]*gtime.Time, error) {
	rows, err := g.DB().Model("hg_youban_publish_collect_event_media").Safe().Ctx(ctx).
		Fields("event_id,MAX(updated_at) AS ready_at").
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("event_id", eventIds).
		Where("cache_status", collectMediaCacheReady).
		Group("event_id").
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集媒体完成时间失败")
	}
	result := make(map[int64]*gtime.Time, len(rows))
	for _, row := range rows {
		result[row["event_id"].Int64()] = row["ready_at"].GTime()
	}
	return result, nil
}

func appendCollectTimelineNode(timeline *sysin.CollectMaterialTimelineModel, stage, label string, at *gtime.Time) {
	if timeline == nil || at == nil || at.IsZero() {
		return
	}
	timeline.Nodes = append(timeline.Nodes, &sysin.CollectMaterialTimelineNodeModel{Stage: stage, Label: label, At: at})
}

func finalizeCollectTimeline(timeline *sysin.CollectMaterialTimelineModel) {
	if timeline == nil || len(timeline.Nodes) == 0 {
		return
	}
	sort.SliceStable(timeline.Nodes, func(i, j int) bool {
		if timeline.Nodes[i] == nil || timeline.Nodes[i].At == nil {
			return false
		}
		if timeline.Nodes[j] == nil || timeline.Nodes[j].At == nil {
			return true
		}
		return timeline.Nodes[i].At.Before(timeline.Nodes[j].At)
	})
	start := timeline.Nodes[0].At
	previous := start
	previousStage := timeline.Nodes[0].Stage
	previousLabel := timeline.Nodes[0].Label
	for _, node := range timeline.Nodes {
		if node == nil || node.At == nil {
			continue
		}
		node.DurationFromStartMs = collectTimelineDurationMs(start, node.At)
		node.DurationFromPreviousMs = collectTimelineDurationMs(previous, node.At)
		if node.DurationFromPreviousMs > timeline.BottleneckDurationMs {
			timeline.BottleneckDurationMs = node.DurationFromPreviousMs
			timeline.BottleneckStage = previousStage + "->" + node.Stage
			timeline.BottleneckLabel = previousLabel + " → " + node.Label
		}
		previous = node.At
		previousStage = node.Stage
		previousLabel = node.Label
	}
	timeline.TotalDurationMs = collectTimelineDurationMs(start, timeline.Nodes[len(timeline.Nodes)-1].At)
}

func collectTimelineDurationMs(from, to *gtime.Time) int64 {
	if from == nil || to == nil || to.Before(from) {
		return 0
	}
	return to.Sub(from).Milliseconds()
}

func earlierCollectTimelineTime(current, candidate *gtime.Time) *gtime.Time {
	if candidate == nil {
		return current
	}
	if current == nil || candidate.Before(current) {
		return candidate
	}
	return current
}

func laterCollectTimelineTime(current, candidate *gtime.Time) *gtime.Time {
	if candidate == nil {
		return current
	}
	if current == nil || candidate.After(current) {
		return candidate
	}
	return current
}

func collectTimelineMessageKey(sourceId int64, chatId string, messageId int64) string {
	return fmt.Sprintf("%d:%s:%d", sourceId, strings.TrimSpace(chatId), messageId)
}

func sortCollectMaterialTimelines(items []*sysin.CollectMaterialTimelineModel) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].EventId != items[j].EventId {
			return items[i].EventId > items[j].EventId
		}
		return items[i].JobId < items[j].JobId
	})
}
