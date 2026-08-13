package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type CollectBotMediaGroupRepairOptions struct {
	GroupedIDs []string
	Since      *gtime.Time
	Limit      int
	DryRun     bool
}

type CollectBotMediaGroupRepairResult struct {
	Groups     int
	Events     int
	Media      int
	Requeued   int
	GroupedIDs []string
}

// RepairCollectBotMediaGroups merges Bot media-group messages that were
// incorrectly persisted as separate business events. It is idempotent.
func RepairCollectBotMediaGroups(ctx context.Context, options CollectBotMediaGroupRepairOptions) (*CollectBotMediaGroupRepairResult, error) {
	if options.Limit <= 0 || options.Limit > 1000 {
		options.Limit = 100
	}
	groups, err := brokenCollectBotMediaGroups(ctx, options)
	if err != nil {
		return nil, err
	}
	result := &CollectBotMediaGroupRepairResult{GroupedIDs: make([]string, 0, len(groups))}
	service := NewSysPublish()
	for _, group := range groups {
		groupedID := strings.TrimSpace(group["source_grouped_id"].String())
		if groupedID == "" {
			continue
		}
		result.Groups++
		result.Events += group["event_count"].Int()
		result.Media += group["media_count"].Int()
		result.GroupedIDs = append(result.GroupedIDs, groupedID)
		if options.DryRun {
			continue
		}
		canonical, repairErr := repairCollectBotMediaGroup(ctx, service, group)
		if repairErr != nil {
			return result, repairErr
		}
		if canonical.IsEmpty() {
			continue
		}
		if err = service.enqueueCollectProcess(ctx, collectProcessQueuePayload{
			EventId: canonical["id"].Int64(), SourceId: canonical["source_id"].Int64(),
			TenantId: canonical["tenant_id"].Int64(), AccountId: canonical["account_id"].Int64(),
		}, 0); err != nil {
			return result, gerror.Wrap(err, "重新投递Bot媒体组采集事件失败")
		}
		result.Requeued++
	}
	return result, nil
}

func brokenCollectBotMediaGroups(ctx context.Context, options CollectBotMediaGroupRepairOptions) (gdb.Result, error) {
	model := pdao.YoubanPublishCollectEvent.DB().Model(pdao.YoubanPublishCollectEvent.Table()+" e").Safe().Ctx(ctx).
		Fields("e.source_id,e.tenant_id,e.account_id,e.source_chat_id,e.source_grouped_id,COUNT(DISTINCT e.id) AS event_count,COUNT(m.id) AS media_count").
		LeftJoin(pdao.YoubanPublishCollectEventMedia.Table()+" m", "m.event_id=e.id").
		Where("e.source_type", sysin.CollectSourceTypeBot).
		Where("e.source_grouped_id <> ''").
		Where("e.status", sysin.CollectEventStatusIgnored).
		Where("e.error_message", "消息不是资料组或验证组")
	if options.Since != nil {
		model = model.WhereGTE("e.created_at", options.Since)
	}
	if len(options.GroupedIDs) > 0 {
		model = model.WhereIn("e.source_grouped_id", options.GroupedIDs)
	}
	rows, err := model.Group("e.source_id,e.tenant_id,e.account_id,e.source_chat_id,e.source_grouped_id").
		OrderAsc("MIN(e.id)").Limit(options.Limit).All()
	return rows, gerror.Wrap(err, "读取异常Bot媒体组失败")
}

func repairCollectBotMediaGroup(ctx context.Context, service *sSysPublish, group gdb.Record) (gdb.Record, error) {
	var canonical gdb.Record
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		events, err := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).
			Where("source_id", group["source_id"].Int64()).
			Where("source_chat_id", group["source_chat_id"].String()).
			Where("source_grouped_id", group["source_grouped_id"].String()).
			OrderAsc("source_message_id").OrderAsc("id").All()
		if err != nil {
			return gerror.Wrap(err, "读取Bot媒体组事件失败")
		}
		if len(events) < 2 {
			return nil
		}
		canonicalIndex := 0
		for index, event := range events {
			if strings.TrimSpace(event["raw_text"].String()) != "" {
				canonicalIndex = index
				break
			}
		}
		canonical = events[canonicalIndex]
		canonicalID := canonical["id"].Int64()
		childIDs := make([]int64, 0, len(events)-1)
		for index, event := range events {
			if index == canonicalIndex {
				continue
			}
			childIDs = append(childIDs, event["id"].Int64())
		}
		media, err := tx.Model(pdao.YoubanPublishCollectEventMedia.Table()).Ctx(ctx).
			WhereIn("event_id", append([]int64{canonicalID}, childIDs...)).
			OrderAsc("source_message_id").OrderAsc("sort_index").OrderAsc("id").All()
		if err != nil {
			return gerror.Wrap(err, "读取Bot媒体组媒体失败")
		}
		seen := make(map[string]struct{}, len(media))
		sortIndex := 1
		for _, item := range media {
			key := strings.TrimSpace(item["source_media_key"].String())
			if _, exists := seen[key]; exists {
				if _, err = tx.Model(pdao.YoubanPublishCollectEventMedia.Table()).Ctx(ctx).Where("id", item["id"].Int64()).Delete(); err != nil {
					return gerror.Wrap(err, "删除Bot媒体组重复媒体失败")
				}
				continue
			}
			seen[key] = struct{}{}
			if _, err = tx.Model(pdao.YoubanPublishCollectEventMedia.Table()).Ctx(ctx).Where("id", item["id"].Int64()).Data(g.Map{
				"event_id": canonicalID, "sort_index": sortIndex, "updated_at": gtime.Now(),
			}).Update(); err != nil {
				return gerror.Wrap(err, "合并Bot媒体组媒体失败")
			}
			sortIndex++
		}
		if _, err = tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).Where("id", canonicalID).Data(g.Map{
			"status": sysin.CollectEventStatusPending, "material_role": collectMaterialRolePending,
			"material_group_status": collectMaterialGroupCollecting, "error_message": "", "processed_at": nil,
			"updated_at": gtime.Now(),
		}).Update(); err != nil {
			return gerror.Wrap(err, "重置Bot媒体组主事件失败")
		}
		if len(childIDs) > 0 {
			if _, err = tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).WhereIn("id", childIDs).Data(g.Map{
				"status": sysin.CollectEventStatusIgnored, "material_role": "superseded",
				"material_parent_event_id": canonicalID, "material_group_status": "repaired",
				"error_message": "媒体组已合并到主事件", "processed_at": gtime.Now(), "updated_at": gtime.Now(),
			}).Update(); err != nil {
				return gerror.Wrap(err, "标记Bot媒体组子事件失败")
			}
		}
		return nil
	})
	if err != nil || canonical.IsEmpty() {
		return canonical, err
	}
	if err = service.syncCollectEventMediaSnapshot(ctx, canonical["id"].Int64()); err != nil {
		return nil, err
	}
	return canonical, nil
}
