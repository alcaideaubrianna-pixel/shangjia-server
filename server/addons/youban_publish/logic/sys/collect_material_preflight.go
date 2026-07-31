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
)

type collectMaterialPreflightResult struct {
	Rules  []gdb.Record
	Stage  string
	Reason string
}

func (s *sSysPublish) preflightCollectMaterialGroup(ctx context.Context, event gdb.Record, rules []gdb.Record) (*collectMaterialPreflightResult, error) {
	result := &collectMaterialPreflightResult{Stage: "precheck"}
	if len(rules) == 0 {
		result.Reason = "未命中可用规则"
		return result, nil
	}
	candidateRules, reasons := s.precheckCollectEventRules(event, rules)
	if len(candidateRules) == 0 {
		result.Reason = "未命中规则或被屏蔽"
		if len(reasons) > 0 {
			result.Reason = strings.Join(uniqueStrings(reasons), "；")
		}
		return result, nil
	}
	if !collectEventNeedsMaterialPreflight(event["status"].String()) {
		result.Rules = candidateRules
		return result, nil
	}
	content, err := s.collectContentFromEvent(ctx, event)
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料组媒体失败")
	}
	filtered, err := s.filterCollectRulesByEarlyDedupeBatch(ctx, event, content, candidateRules)
	if err != nil {
		return nil, gerror.Wrap(err, "资料组前置去重失败")
	}
	if len(filtered) == 0 {
		result.Stage = "dedupe"
		result.Reason = "图文重复"
		return result, nil
	}
	result.Rules = filtered
	return result, nil
}

type collectEarlyDedupeCandidate struct {
	eventID    int64
	receivedAt *gtime.Time
	channels   []int64
	sent       map[int64]bool
	material   collectDedupeMaterial
}

func (s *sSysPublish) filterCollectRulesByEarlyDedupeBatch(ctx context.Context, event gdb.Record, content *collectContentResult, rules []gdb.Record) ([]gdb.Record, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	current := collectDedupeMaterialFromEvent(event, content)
	if current.mediaKey == "" && current.textHash == "" {
		return rules, nil
	}
	remainingRules, err := filterCollectRulesByEarlyDedupeCache(ctx, event, current, rules, time.Now())
	if err != nil {
		g.Log().Warningf(ctx, "读取采集前置去重缓存失败，回退数据库 eventId:%d err:%+v", event["id"].Int64(), err)
		remainingRules = rules
	}
	if len(remainingRules) == 0 {
		return nil, nil
	}
	queryDays := collectDedupeBroadQueryDays(remainingRules)
	rows, err := s.collectDedupeCandidateEvents(ctx, event, current, queryDays)
	if err != nil {
		return nil, err
	}
	candidates, err := s.buildCollectDedupeCandidates(ctx, event, rows, false)
	if err != nil {
		return nil, err
	}
	filtered := make([]gdb.Record, 0, len(remainingRules))
	cacheWrites := make(map[string]collectDedupeCacheEntry)
	for _, rule := range remainingRules {
		if rule["dedupe_enabled"].Int() != 1 {
			filtered = append(filtered, rule)
			continue
		}
		targetIDs := collectRuleTargetChannelIds(rule)
		layer, previousEventID, channelID, cacheable, receivedAt := matchCollectEarlyDedupeCandidate(current, candidates, targetIDs, rule["dedupe_days"].Int(), time.Now())
		if layer == "" {
			filtered = append(filtered, rule)
			continue
		}
		g.Log().Infof(ctx, "采集资料组前置去重跳过 eventId:%d previousEventId:%d ruleId:%d channelId:%d layer:%s", event["id"].Int64(), previousEventID, rule["id"].Int64(), channelID, layer)
		if cacheable && receivedAt > 0 {
			signature := current.textHash
			if layer == "media_fingerprint" {
				signature = current.mediaKey
			}
			cacheWrites[collectDedupeCacheKey(event["tenant_id"].Int64(), event["account_id"].Int64(), channelID, layer, signature)] = collectDedupeCacheEntry{EventID: previousEventID, ReceivedAt: receivedAt}
		}
	}
	if err = writeCollectDedupeCache(ctx, cacheWrites); err != nil {
		g.Log().Warningf(ctx, "回填采集前置去重缓存失败 eventId:%d err:%+v", event["id"].Int64(), err)
	}
	return filtered, nil
}

func (s *sSysPublish) buildCollectDedupeCandidates(ctx context.Context, event gdb.Record, rows gdb.Result, includePHash bool) ([]collectEarlyDedupeCandidate, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	eventIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := row["id"].Int64(); id > 0 {
			eventIDs = append(eventIDs, id)
		}
	}
	dispatchRows, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields("id,event_id,status").
		WhereIn("event_id", eventIDs).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing, sysin.CollectDispatchStatusSent}).
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "批量读取采集去重分发记录失败")
	}
	dispatchIds := make([]int64, 0, len(dispatchRows))
	for _, row := range dispatchRows {
		dispatchIds = append(dispatchIds, row["id"].Int64())
	}
	dispatchChannels, err := collectDispatchChannelMap(ctx, dispatchIds)
	if err != nil {
		return nil, err
	}
	channelsByEvent := make(map[int64][]int64, len(dispatchRows))
	sentByEvent := make(map[int64]map[int64]bool, len(dispatchRows))
	for _, row := range dispatchRows {
		eventID := row["event_id"].Int64()
		channelIDs := dispatchChannels[row["id"].Int64()]
		channelsByEvent[eventID] = append(channelsByEvent[eventID], channelIDs...)
		if row["status"].String() == sysin.CollectDispatchStatusSent {
			if sentByEvent[eventID] == nil {
				sentByEvent[eventID] = make(map[int64]bool, len(channelIDs))
			}
			for _, channelID := range channelIDs {
				sentByEvent[eventID][channelID] = true
			}
		}
	}
	mediaByEvent, err := s.collectEventMediaItemsByEvent(ctx, eventIDs)
	if err != nil {
		return nil, err
	}
	candidates := make([]collectEarlyDedupeCandidate, 0, len(rows))
	for _, row := range rows {
		eventID := row["id"].Int64()
		if eventID <= 0 || len(channelsByEvent[eventID]) == 0 {
			continue
		}
		material := collectDedupeMaterialFromEventRecord(row, mediaByEvent[eventID])
		if !includePHash {
			material.imagePHashKey = ""
		}
		candidates = append(candidates, collectEarlyDedupeCandidate{
			eventID:    eventID,
			receivedAt: row["received_at"].GTime(),
			channels:   uniqueIds(channelsByEvent[eventID]),
			sent:       sentByEvent[eventID],
			material:   material,
		})
	}
	return candidates, nil
}

func (s *sSysPublish) filterCollectRulesByFinalDedupeBatch(ctx context.Context, event gdb.Record, content *collectContentResult, rules []gdb.Record) ([]gdb.Record, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	current := collectDedupeMaterialFromEvent(event, content)
	queryDays := collectDedupeBroadQueryDays(rules)
	rows, err := s.collectDedupeCandidateEvents(ctx, event, current, queryDays)
	if err != nil {
		return nil, err
	}
	candidates, err := s.buildCollectDedupeCandidates(ctx, event, rows, true)
	if err != nil {
		return nil, err
	}
	filtered := make([]gdb.Record, 0, len(rules))
	now := time.Now()
	for _, rule := range rules {
		if rule["dedupe_enabled"].Int() != 1 {
			filtered = append(filtered, rule)
			continue
		}
		targetIDs := collectRuleTargetChannelIds(rule)
		layer, previousEventID, channelID := matchCollectFinalDedupeCandidate(current, candidates, targetIDs, rule["dedupe_days"].Int(), now)
		if layer == "" {
			filtered = append(filtered, rule)
			continue
		}
		g.Log().Infof(ctx, "采集资料组最终去重跳过 eventId:%d previousEventId:%d ruleId:%d channelId:%d layer:%s", event["id"].Int64(), previousEventID, rule["id"].Int64(), channelID, layer)
	}
	return filtered, nil
}

func collectDedupeBroadQueryDays(rules []gdb.Record) int {
	maximum := 0
	for _, rule := range rules {
		if rule["dedupe_enabled"].Int() != 1 {
			continue
		}
		days := rule["dedupe_days"].Int()
		if days <= 0 {
			return 0
		}
		if days > maximum {
			maximum = days
		}
	}
	return maximum
}

func matchCollectEarlyDedupeCandidate(current collectDedupeMaterial, candidates []collectEarlyDedupeCandidate, targetIDs []int64, days int, now time.Time) (string, int64, int64, bool, int64) {
	if len(targetIDs) == 0 {
		return "", 0, 0, false, 0
	}
	var cutoff time.Time
	if days > 0 {
		cutoff = now.AddDate(0, 0, -days)
	}
	for _, candidate := range candidates {
		if !cutoff.IsZero() && candidate.receivedAt != nil && candidate.receivedAt.Time.Before(cutoff) {
			continue
		}
		channelID := firstOverlappingInt64(targetIDs, candidate.channels)
		if channelID <= 0 {
			continue
		}
		if layer := current.matchLayer(candidate.material, collectDedupePhaseEarly); layer != "" {
			receivedAt := int64(0)
			if candidate.receivedAt != nil {
				receivedAt = candidate.receivedAt.Timestamp()
			}
			return layer, candidate.eventID, channelID, candidate.sent[channelID], receivedAt
		}
	}
	return "", 0, 0, false, 0
}

func matchCollectFinalDedupeCandidate(current collectDedupeMaterial, candidates []collectEarlyDedupeCandidate, targetIDs []int64, days int, now time.Time) (string, int64, int64) {
	if len(targetIDs) == 0 {
		return "", 0, 0
	}
	var cutoff time.Time
	if days > 0 {
		cutoff = now.AddDate(0, 0, -days)
	}
	for _, candidate := range candidates {
		if !cutoff.IsZero() && candidate.receivedAt != nil && candidate.receivedAt.Time.Before(cutoff) {
			continue
		}
		channelID := firstOverlappingInt64(targetIDs, candidate.channels)
		if channelID <= 0 {
			continue
		}
		layer := current.matchLayer(candidate.material, collectDedupePhaseEarly)
		if layer == "" {
			layer = current.matchLayer(candidate.material, collectDedupePhasePHash)
		}
		if layer != "" {
			return layer, candidate.eventID, channelID
		}
	}
	return "", 0, 0
}

func filterCollectRulesByEarlyDedupeCache(ctx context.Context, event gdb.Record, current collectDedupeMaterial, rules []gdb.Record, now time.Time) ([]gdb.Record, error) {
	type cacheCheck struct {
		ruleID    int64
		channelID int64
		layer     string
		key       string
		days      int
	}
	checks := make([]cacheCheck, 0, len(rules)*2)
	keys := make([]string, 0, len(rules)*2)
	seenKeys := make(map[string]struct{})
	for _, rule := range rules {
		if rule["dedupe_enabled"].Int() != 1 {
			continue
		}
		for _, channelID := range collectRuleTargetChannelIds(rule) {
			for _, item := range []struct{ layer, signature string }{{"media_fingerprint", current.mediaKey}, {"text_hash", current.textHash}} {
				if item.signature == "" {
					continue
				}
				key := collectDedupeCacheKey(event["tenant_id"].Int64(), event["account_id"].Int64(), channelID, item.layer, item.signature)
				checks = append(checks, cacheCheck{ruleID: rule["id"].Int64(), channelID: channelID, layer: item.layer, key: key, days: rule["dedupe_days"].Int()})
				if _, ok := seenKeys[key]; !ok {
					seenKeys[key] = struct{}{}
					keys = append(keys, key)
				}
			}
		}
	}
	values, err := readCollectDedupeCache(ctx, keys)
	if err != nil {
		return nil, err
	}
	duplicatedRules := make(map[int64]cacheCheck)
	for _, check := range checks {
		entry, ok := values[check.key]
		if ok && collectDedupeCacheEntryValid(entry, check.days, now) {
			duplicatedRules[check.ruleID] = check
		}
	}
	filtered := make([]gdb.Record, 0, len(rules))
	for _, rule := range rules {
		if hit, ok := duplicatedRules[rule["id"].Int64()]; ok {
			g.Log().Infof(ctx, "采集资料组前置去重缓存命中 eventId:%d ruleId:%d channelId:%d layer:%s", event["id"].Int64(), hit.ruleID, hit.channelID, hit.layer)
			continue
		}
		filtered = append(filtered, rule)
	}
	return filtered, nil
}

func collectEventNeedsMaterialPreflight(status string) bool {
	switch strings.TrimSpace(status) {
	case sysin.CollectEventStatusPrechecked,
		sysin.CollectEventStatusMediaPending,
		sysin.CollectEventStatusMediaReady,
		sysin.CollectEventStatusProcessed,
		sysin.CollectEventStatusDispatched:
		return false
	default:
		return true
	}
}
