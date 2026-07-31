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
	for _, row := range dispatchRows {
		eventID := row["event_id"].Int64()
		if eventID <= 0 {
			continue
		}
		if _, ok := channelsByEvent[eventID]; !ok {
			eventIDs = append(eventIDs, eventID)
		}
		channelsByEvent[eventID] = append(channelsByEvent[eventID], decodeInt64JSON(row["target_channel_id_json"].String())...)
	}
	if len(eventIDs) == 0 {
		return nil
	}
	events, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields("id,tenant_id,account_id,text_hash,dedupe_key,media_json,received_at").
		WhereIn("id", eventIDs).
		All()
	if err != nil {
		return err
	}
	contentByEvent := make(map[int64]gdb.Record, len(eventIDs))
	contents, err := pdao.YoubanPublishCollectContent.Ctx(ctx).
		Fields("first_event_id,last_event_id,text_hash,media_json").
		WhereIn("first_event_id", eventIDs).
		WhereOrIn("last_event_id", eventIDs).
		All()
	if err != nil {
		return err
	}
	for _, row := range contents {
		if eventID := row["last_event_id"].Int64(); eventID > 0 {
			contentByEvent[eventID] = row
		}
		if eventID := row["first_event_id"].Int64(); eventID > 0 {
			if _, ok := contentByEvent[eventID]; !ok {
				contentByEvent[eventID] = row
			}
		}
	}
	values := make(map[string]collectDedupeCacheEntry)
	for _, event := range events {
		eventID := event["id"].Int64()
		receivedAt := event["received_at"].GTime()
		if eventID <= 0 || receivedAt == nil {
			continue
		}
		material := collectDedupeMaterialFromEventRecord(event)
		if content := contentByEvent[eventID]; content != nil {
			contentMaterial := collectDedupeMaterialFromContentRecord(content)
			material.imagePHashKey = contentMaterial.imagePHashKey
		}
		entry := collectDedupeCacheEntry{EventID: eventID, ReceivedAt: receivedAt.Timestamp()}
		for _, channelID := range uniqueIds(channelsByEvent[eventID]) {
			for _, item := range []struct{ layer, signature string }{
				{"media_fingerprint", material.mediaKey},
				{"text_hash", material.textHash},
				{"image_phash", material.imagePHashKey},
			} {
				if item.signature != "" {
					values[collectDedupeCacheKey(event["tenant_id"].Int64(), event["account_id"].Int64(), channelID, item.layer, item.signature)] = entry
				}
			}
		}
	}
	return writeCollectDedupeCache(ctx, values)
}
