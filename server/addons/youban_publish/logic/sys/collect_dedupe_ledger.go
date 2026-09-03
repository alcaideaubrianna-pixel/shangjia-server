package sys

import (
	"context"
	"sort"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type collectDedupeSignature struct {
	layer string
	value string
	total int
	count int
}

func (material collectDedupeMaterial) signatures(includePHash bool) []collectDedupeSignature {
	items := make([]collectDedupeSignature, 0, 3)
	if material.textHash != "" {
		items = append(items, collectDedupeSignature{layer: "text_hash", value: material.textHash})
	}
	if material.mediaKey != "" && material.mediaTotal > 0 && material.mediaCount == material.mediaTotal {
		items = append(items, collectDedupeSignature{layer: "media_fingerprint", value: material.mediaKey, total: material.mediaTotal, count: material.mediaCount})
	}
	if includePHash && material.imagePHashKey != "" && material.imageTotal > 0 && material.imagePHashCount == material.imageTotal {
		items = append(items, collectDedupeSignature{layer: "image_phash", value: material.imagePHashKey, total: material.imageTotal, count: material.imagePHashCount})
	}
	return items
}

type collectDedupeLedgerHit struct {
	layer      string
	eventID    int64
	channelID  int64
	lastSeenAt time.Time
}

func loadCollectDedupeLedgerHits(ctx context.Context, tenantID, accountID int64, channelIDs []int64, material collectDedupeMaterial, includePHash bool) ([]collectDedupeLedgerHit, error) {
	if len(channelIDs) == 0 {
		return nil, nil
	}
	hits := make([]collectDedupeLedgerHit, 0, len(channelIDs))
	for _, signature := range material.signatures(includePHash) {
		model := g.DB().Model(publishCollectDedupeEntryTable+" e").Safe().Ctx(ctx).
			Fields("e.last_event_id,e.channel_id,e.last_seen_at").
			Where("e.tenant_id", tenantID).Where("e.account_id", accountID).
			WhereIn("e.channel_id", channelIDs).Where("e.layer", signature.layer).
			Where("e.signature", signature.value).Where("e.item_total", signature.total).
			Where("e.signature_count", signature.count).
			Where("EXISTS (SELECT 1 FROM " + publishCollectDedupeSourceTable + " ds WHERE ds.entry_id=e.id)")
		rows, err := model.All()
		if err != nil {
			return nil, gerror.Wrap(err, "查询采集去重账本失败")
		}
		for _, row := range rows {
			seenAt := row["last_seen_at"].GTime()
			if seenAt == nil {
				continue
			}
			hits = append(hits, collectDedupeLedgerHit{layer: signature.layer, eventID: row["last_event_id"].Int64(), channelID: row["channel_id"].Int64(), lastSeenAt: seenAt.Time})
		}
	}
	return hits, nil
}

func matchCollectDedupeLedgerHit(hits []collectDedupeLedgerHit, channelIDs []int64, days int, now time.Time) (string, int64, int64) {
	channels := make(map[int64]struct{}, len(channelIDs))
	for _, channelID := range channelIDs {
		channels[channelID] = struct{}{}
	}
	for _, hit := range hits {
		if _, ok := channels[hit.channelID]; !ok {
			continue
		}
		if days > 0 && hit.lastSeenAt.Before(now.AddDate(0, 0, -days)) {
			continue
		}
		return hit.layer, hit.eventID, hit.channelID
	}
	return "", 0, 0
}

func reserveCollectDedupeLedgerTx(ctx context.Context, tx gdb.TX, event gdb.Record, rule gdb.Record, dispatchID int64, channelIDs []int64, material collectDedupeMaterial) error {
	seenAt := event["received_at"].GTime()
	if seenAt == nil {
		seenAt = gtime.Now()
	}
	signatures := material.signatures(true)
	if len(signatures) == 0 {
		return markCollectDedupeDispatchHandledTx(ctx, tx, event["tenant_id"].Int64(), event["account_id"].Int64(), event["source_id"].Int64(), rule["id"].Int64(), dispatchID, event["id"].Int64(), seenAt)
	}
	for _, channelID := range uniqueIds(channelIDs) {
		for _, signature := range signatures {
			data := g.Map{
				"tenant_id": event["tenant_id"].Int64(), "account_id": event["account_id"].Int64(), "channel_id": channelID,
				"layer": signature.layer, "signature": signature.value, "item_total": signature.total, "signature_count": signature.count,
				"first_event_id": event["id"].Int64(), "last_event_id": event["id"].Int64(), "first_seen_at": seenAt,
				"last_seen_at": seenAt, "created_at": gtime.Now(), "updated_at": gtime.Now(),
			}
			if _, err := tx.Model(publishCollectDedupeEntryTable).Ctx(ctx).Data(data).InsertIgnore(); err != nil {
				return gerror.Wrap(err, "创建采集去重账本失败")
			}
			entry, err := tx.Model(publishCollectDedupeEntryTable).Ctx(ctx).Fields("id").
				Where("tenant_id", event["tenant_id"].Int64()).Where("account_id", event["account_id"].Int64()).Where("channel_id", channelID).
				Where("layer", signature.layer).Where("signature", signature.value).Where("item_total", signature.total).Where("signature_count", signature.count).One()
			if err != nil {
				return gerror.Wrap(err, "读取采集去重账本失败")
			}
			if entry.IsEmpty() {
				return gerror.New("采集去重账本写入后不存在")
			}
			if _, err = tx.Model(publishCollectDedupeEntryTable).Ctx(ctx).Where("id", entry["id"].Int64()).Data(g.Map{
				"last_event_id": event["id"].Int64(), "last_seen_at": seenAt, "updated_at": gtime.Now(),
			}).Update(); err != nil {
				return gerror.Wrap(err, "更新采集去重账本失败")
			}
			if _, err = tx.Model(publishCollectDedupeSourceTable).Ctx(ctx).Data(g.Map{
				"entry_id": entry["id"].Int64(), "tenant_id": event["tenant_id"].Int64(), "account_id": event["account_id"].Int64(),
				"source_id": event["source_id"].Int64(), "rule_id": rule["id"].Int64(), "dispatch_id": dispatchID,
				"event_id": event["id"].Int64(), "accepted_at": seenAt, "created_at": gtime.Now(),
			}).InsertIgnore(); err != nil {
				return gerror.Wrap(err, "登记采集去重来源失败")
			}
		}
	}
	return nil
}

func markCollectDedupeDispatchHandledTx(ctx context.Context, tx gdb.TX, tenantID, accountID, sourceID, ruleID, dispatchID, eventID int64, acceptedAt *gtime.Time) error {
	_, err := tx.Model(publishCollectDedupeSourceTable).Ctx(ctx).Data(g.Map{
		"entry_id": 0, "tenant_id": tenantID, "account_id": accountID, "source_id": sourceID,
		"rule_id": ruleID, "dispatch_id": dispatchID, "event_id": eventID, "accepted_at": acceptedAt, "created_at": gtime.Now(),
	}).InsertIgnore()
	return gerror.Wrap(err, "标记无签名采集分发失败")
}

func releaseCollectDedupeLedgerByDispatches(ctx context.Context, dispatchIDs []int64) error {
	dispatchIDs = uniqueIds(dispatchIDs)
	if len(dispatchIDs) == 0 {
		return nil
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		return releaseCollectDedupeLedgerByDispatchesTx(ctx, tx, dispatchIDs)
	})
}

func releaseCollectDedupeLedgerByDispatchesTx(ctx context.Context, tx gdb.TX, dispatchIDs []int64) error {
	dispatchIDs = uniqueIds(dispatchIDs)
	if len(dispatchIDs) == 0 {
		return nil
	}
	var entryIDs []int64
	if err := tx.Model(publishCollectDedupeSourceTable).Ctx(ctx).Fields("entry_id").WhereIn("dispatch_id", dispatchIDs).Scan(&entryIDs); err != nil {
		return gerror.Wrap(err, "读取待释放采集去重来源失败")
	}
	if _, err := tx.Model(publishCollectDedupeSourceTable).Ctx(ctx).WhereIn("dispatch_id", dispatchIDs).Delete(); err != nil {
		return gerror.Wrap(err, "释放采集去重来源失败")
	}
	return reconcileCollectDedupeEntriesTx(ctx, tx, entryIDs)
}

func releaseCollectSourceDedupeLedger(ctx context.Context, tenantID, accountID, sourceID int64) ([]string, error) {
	if tenantID <= 0 || accountID <= 0 || sourceID <= 0 {
		return nil, nil
	}
	cacheKeys := make([]string, 0)
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var entryIDs []int64
		if err := tx.Model(publishCollectDedupeSourceTable).Ctx(ctx).Fields("entry_id").Where("tenant_id", tenantID).Where("account_id", accountID).Where("source_id", sourceID).Scan(&entryIDs); err != nil {
			return gerror.Wrap(err, "读取采集源去重来源失败")
		}
		if len(entryIDs) > 0 {
			entries, entryErr := tx.Model(publishCollectDedupeEntryTable).Ctx(ctx).Fields("channel_id,layer,signature").WhereIn("id", uniqueIds(entryIDs)).All()
			if entryErr != nil {
				return gerror.Wrap(entryErr, "读取采集源去重缓存键失败")
			}
			for _, entry := range entries {
				cacheKeys = append(cacheKeys, collectDedupeCacheKey(tenantID, accountID, entry["channel_id"].Int64(), entry["layer"].String(), entry["signature"].String()))
			}
		}
		if _, err := tx.Model(publishCollectDedupeSourceTable).Ctx(ctx).Where("tenant_id", tenantID).Where("account_id", accountID).Where("source_id", sourceID).Delete(); err != nil {
			return gerror.Wrap(err, "清理采集源去重来源失败")
		}
		return reconcileCollectDedupeEntriesTx(ctx, tx, entryIDs)
	})
	return cacheKeys, err
}

func reconcileCollectDedupeEntriesTx(ctx context.Context, tx gdb.TX, entryIDs []int64) error {
	entryIDs = uniqueIds(entryIDs)
	if len(entryIDs) == 0 {
		return nil
	}
	rows, err := tx.Model(publishCollectDedupeSourceTable).Ctx(ctx).
		Fields("entry_id,event_id,accepted_at").WhereIn("entry_id", entryIDs).
		OrderAsc("entry_id").OrderDesc("accepted_at").OrderDesc("id").All()
	if err != nil {
		return gerror.Wrap(err, "读取剩余采集去重来源失败")
	}
	latest := make(map[int64]gdb.Record, len(entryIDs))
	earliest := make(map[int64]gdb.Record, len(entryIDs))
	for _, row := range rows {
		entryID := row["entry_id"].Int64()
		if _, exists := latest[entryID]; !exists {
			latest[entryID] = row
		}
		earliest[entryID] = row
	}
	for _, entryID := range entryIDs {
		row, exists := latest[entryID]
		if !exists {
			if _, err = tx.Model(publishCollectDedupeEntryTable).Ctx(ctx).Where("id", entryID).Delete(); err != nil {
				return gerror.Wrap(err, "清理采集孤立去重账本失败")
			}
			continue
		}
		if _, err = tx.Model(publishCollectDedupeEntryTable).Ctx(ctx).Where("id", entryID).Data(g.Map{
			"first_event_id": earliest[entryID]["event_id"].Int64(), "first_seen_at": earliest[entryID]["accepted_at"].GTime(),
			"last_event_id": row["event_id"].Int64(), "last_seen_at": row["accepted_at"].GTime(), "updated_at": gtime.Now(),
		}).Update(); err != nil {
			return gerror.Wrap(err, "回退采集去重账本状态失败")
		}
	}
	return nil
}

func collectDedupeSignatureLockKeys(material collectDedupeMaterial) []string {
	keys := make([]string, 0, 3)
	for _, item := range material.signatures(true) {
		keys = append(keys, item.layer+":"+item.value)
	}
	if len(keys) == 0 {
		keys = append(keys, "empty")
	}
	sort.Strings(keys)
	return keys
}

func (s *sSysPublish) backfillCollectDedupeLedger(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 200
	}
	dispatches, err := g.DB().Model(publishCollectDispatchTable+" d").Safe().Ctx(ctx).
		Fields("d.id,d.tenant_id,d.account_id,d.event_id,d.source_id,d.rule_id").
		WhereIn("d.status", []string{"pending", "reviewing", "sent"}).
		Where("NOT EXISTS (SELECT 1 FROM " + publishCollectDedupeSourceTable + " ds WHERE ds.dispatch_id=d.id)").
		Where("EXISTS (SELECT 1 FROM " + publishCollectSourceTable + " s WHERE s.id=d.source_id AND s.deleted_at IS NULL)").
		OrderAsc("d.id").Limit(limit).All()
	if err != nil || len(dispatches) == 0 {
		return gerror.Wrap(err, "读取待回填采集分发失败")
	}
	dispatchIDs := make([]int64, 0, len(dispatches))
	eventIDs := make([]int64, 0, len(dispatches))
	for _, dispatch := range dispatches {
		dispatchIDs = append(dispatchIDs, dispatch["id"].Int64())
		eventIDs = append(eventIDs, dispatch["event_id"].Int64())
	}
	channels, err := collectDispatchChannelMap(ctx, dispatchIDs)
	if err != nil {
		return err
	}
	events, err := g.DB().Model(publishCollectEventTable).Ctx(ctx).WhereIn("id", uniqueIds(eventIDs)).All()
	if err != nil {
		return gerror.Wrap(err, "读取待回填采集事件失败")
	}
	eventsByID := make(map[int64]gdb.Record, len(events))
	for _, event := range events {
		eventsByID[event["id"].Int64()] = event
	}
	mediaByEvent, err := s.collectEventMediaItemsByEvent(ctx, uniqueIds(eventIDs))
	if err != nil {
		return err
	}
	if err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, dispatch := range dispatches {
			event := eventsByID[dispatch["event_id"].Int64()]
			if event.IsEmpty() || len(channels[dispatch["id"].Int64()]) == 0 {
				if markerErr := markCollectDedupeDispatchHandledTx(ctx, tx, dispatch["tenant_id"].Int64(), dispatch["account_id"].Int64(), dispatch["source_id"].Int64(), dispatch["rule_id"].Int64(), dispatch["id"].Int64(), dispatch["event_id"].Int64(), gtime.Now()); markerErr != nil {
					return markerErr
				}
				continue
			}
			rule := gdb.Record{"id": dispatch["rule_id"]}
			if reserveErr := reserveCollectDedupeLedgerTx(ctx, tx, event, rule, dispatch["id"].Int64(), channels[dispatch["id"].Int64()], collectDedupeMaterialFromEventRecord(event, mediaByEvent[event["id"].Int64()])); reserveErr != nil {
				return reserveErr
			}
		}
		return nil
	}); err != nil {
		return err
	}
	g.Log().Infof(ctx, "采集永久去重账本回填完成 dispatches:%d", len(dispatches))
	return nil
}
