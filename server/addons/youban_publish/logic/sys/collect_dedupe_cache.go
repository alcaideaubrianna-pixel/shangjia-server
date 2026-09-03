package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"

	pdao "hotgo/addons/youban_publish/internal/dao"
)

const collectDedupeCacheKeyPrefix = "youban_publish:collect:dedupe:v3"

type collectDedupeCacheEntry struct {
	EventID    int64
	ReceivedAt int64
}

func collectDedupeCacheKey(tenantID, accountID, channelID int64, layer, signature string) string {
	return fmt.Sprintf("%s:%d:%d:%d:%s:%s", collectDedupeCacheKeyPrefix, tenantID, accountID, channelID, strings.TrimSpace(layer), strings.TrimSpace(signature))
}

func collectDedupeSourceCacheIndexKey(tenantID, accountID, sourceID int64) string {
	return fmt.Sprintf("%s:source:%d:%d:%d", collectDedupeCacheKeyPrefix, tenantID, accountID, sourceID)
}

func collectDedupeCacheValue(eventID int64, receivedAt time.Time) string {
	return fmt.Sprintf("%d:%d", eventID, receivedAt.Unix())
}

func parseCollectDedupeCacheValue(value string) (collectDedupeCacheEntry, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return collectDedupeCacheEntry{}, false
	}
	eventID, eventErr := strconv.ParseInt(parts[0], 10, 64)
	receivedAt, timeErr := strconv.ParseInt(parts[1], 10, 64)
	if eventErr != nil || timeErr != nil || eventID <= 0 || receivedAt <= 0 {
		return collectDedupeCacheEntry{}, false
	}
	return collectDedupeCacheEntry{EventID: eventID, ReceivedAt: receivedAt}, true
}

func readCollectDedupeCache(ctx context.Context, keys []string) (map[string]collectDedupeCacheEntry, error) {
	result := make(map[string]collectDedupeCacheEntry, len(keys))
	if len(keys) == 0 {
		return result, nil
	}
	args := make([]interface{}, 0, len(keys))
	for _, key := range keys {
		args = append(args, key)
	}
	value, err := g.Redis().Do(ctx, "MGET", args...)
	if err != nil {
		return nil, err
	}
	values := value.Array()
	for index, item := range values {
		if index >= len(keys) || item == nil {
			continue
		}
		entry, ok := parseCollectDedupeCacheValue(gvar.New(item).String())
		if ok {
			result[keys[index]] = entry
		}
	}
	return result, nil
}

func writeCollectDedupeCache(ctx context.Context, values map[string]collectDedupeCacheEntry) error {
	if len(values) == 0 {
		return nil
	}
	args := make([]interface{}, 0, len(values)*2)
	for key, entry := range values {
		if strings.TrimSpace(key) == "" || entry.EventID <= 0 || entry.ReceivedAt <= 0 {
			continue
		}
		args = append(args, key, fmt.Sprintf("%d:%d", entry.EventID, entry.ReceivedAt))
	}
	if len(args) == 0 {
		return nil
	}
	_, err := g.Redis().Do(ctx, "MSET", args...)
	return err
}

func clearCollectDedupeCacheKeys(ctx context.Context, tenantID, accountID, sourceID int64, keys []string) error {
	indexKey := collectDedupeSourceCacheIndexKey(tenantID, accountID, sourceID)
	indexed, err := g.Redis().Do(ctx, "SMEMBERS", indexKey)
	if err != nil {
		return err
	}
	keys = append(keys, indexed.Strings()...)
	keys = uniqueStrings(keys)
	args := make([]interface{}, 0, len(keys)+1)
	for _, key := range keys {
		args = append(args, key)
	}
	args = append(args, indexKey)
	_, err = g.Redis().Do(ctx, "DEL", args...)
	return err
}

func collectDedupeCacheEntryValid(entry collectDedupeCacheEntry, days int, now time.Time) bool {
	if entry.EventID <= 0 || entry.ReceivedAt <= 0 {
		return false
	}
	return days <= 0 || !time.Unix(entry.ReceivedAt, 0).Before(now.AddDate(0, 0, -days))
}

func (s *sSysPublish) warmCollectDedupeCacheForSentDispatches(ctx context.Context, dispatchRows gdb.Result) error {
	if len(dispatchRows) == 0 {
		return nil
	}
	channelsByEvent := make(map[int64][]int64, len(dispatchRows))
	eventIDs := make([]int64, 0, len(dispatchRows))
	dispatchIds := make([]int64, 0, len(dispatchRows))
	for _, row := range dispatchRows {
		eventID := row["event_id"].Int64()
		if eventID <= 0 {
			continue
		}
		if _, ok := channelsByEvent[eventID]; !ok {
			eventIDs = append(eventIDs, eventID)
		}
		dispatchIds = append(dispatchIds, row["id"].Int64())
	}
	dispatchChannels, err := collectDispatchChannelMap(ctx, dispatchIds)
	if err != nil {
		return err
	}
	for _, row := range dispatchRows {
		eventID := row["event_id"].Int64()
		channelsByEvent[eventID] = append(channelsByEvent[eventID], dispatchChannels[row["id"].Int64()]...)
	}
	if len(eventIDs) == 0 {
		return nil
	}
	events, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields("id,tenant_id,account_id,source_id,text_hash,dedupe_key,received_at").
		WhereIn("id", eventIDs).
		Where("EXISTS (SELECT 1 FROM hg_youban_publish_collect_source s WHERE s.id = hg_youban_publish_collect_event.source_id AND s.tenant_id = hg_youban_publish_collect_event.tenant_id AND s.account_id = hg_youban_publish_collect_event.account_id AND s.deleted_at IS NULL)").
		All()
	if err != nil {
		return err
	}
	mediaByEvent, err := s.collectEventMediaItemsByEvent(ctx, eventIDs)
	if err != nil {
		return err
	}
	values := make(map[string]collectDedupeCacheEntry)
	keysBySource := make(map[string][]interface{})
	for _, event := range events {
		eventID := event["id"].Int64()
		receivedAt := event["received_at"].GTime()
		if eventID <= 0 || receivedAt == nil {
			continue
		}
		material := collectDedupeMaterialFromEventRecord(event, mediaByEvent[eventID])
		entry := collectDedupeCacheEntry{EventID: eventID, ReceivedAt: receivedAt.Timestamp()}
		for _, channelID := range uniqueIds(channelsByEvent[eventID]) {
			for _, item := range []struct{ layer, signature string }{
				{"media_fingerprint", material.mediaKey},
				{"text_hash", material.textHash},
				{"image_phash", material.imagePHashKey},
			} {
				if item.signature != "" {
					key := collectDedupeCacheKey(event["tenant_id"].Int64(), event["account_id"].Int64(), channelID, item.layer, item.signature)
					values[key] = entry
					indexKey := collectDedupeSourceCacheIndexKey(event["tenant_id"].Int64(), event["account_id"].Int64(), event["source_id"].Int64())
					keysBySource[indexKey] = append(keysBySource[indexKey], key)
				}
			}
		}
	}
	if err = writeCollectDedupeCache(ctx, values); err != nil {
		return err
	}
	for indexKey, keys := range keysBySource {
		if len(keys) == 0 {
			continue
		}
		if _, err = g.Redis().Do(ctx, "SADD", append([]interface{}{indexKey}, keys...)...); err != nil {
			return err
		}
	}
	return nil
}
