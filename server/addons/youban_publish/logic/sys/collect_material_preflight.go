package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
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
	channelIDs := make([]int64, 0)
	for _, rule := range remainingRules {
		channelIDs = append(channelIDs, collectRuleTargetChannelIds(rule)...)
	}
	hits, err := loadCollectDedupeLedgerHits(ctx, event["tenant_id"].Int64(), event["account_id"].Int64(), uniqueIds(channelIDs), current, false)
	if err != nil {
		return nil, err
	}
	filtered := make([]gdb.Record, 0, len(remainingRules))
	now := time.Now()
	for _, rule := range remainingRules {
		if rule["dedupe_enabled"].Int() != 1 {
			filtered = append(filtered, rule)
			continue
		}
		targetIDs := collectRuleTargetChannelIds(rule)
		layer, previousEventID, channelID := matchCollectDedupeLedgerHit(hits, targetIDs, rule["dedupe_days"].Int(), now)
		if layer == "" {
			filtered = append(filtered, rule)
			continue
		}
		g.Log().Infof(ctx, "采集资料组前置去重跳过 eventId:%d previousEventId:%d ruleId:%d channelId:%d layer:%s", event["id"].Int64(), previousEventID, rule["id"].Int64(), channelID, layer)
	}
	return filtered, nil
}

func (s *sSysPublish) filterCollectRulesByFinalDedupeBatch(ctx context.Context, event gdb.Record, content *collectContentResult, rules []gdb.Record) ([]gdb.Record, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	current := collectDedupeMaterialFromEvent(event, content)
	channelIDs := make([]int64, 0)
	for _, rule := range rules {
		channelIDs = append(channelIDs, collectRuleTargetChannelIds(rule)...)
	}
	hits, err := loadCollectDedupeLedgerHits(ctx, event["tenant_id"].Int64(), event["account_id"].Int64(), uniqueIds(channelIDs), current, true)
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
		layer, previousEventID, channelID := matchCollectDedupeLedgerHit(hits, targetIDs, rule["dedupe_days"].Int(), now)
		if layer == "" {
			filtered = append(filtered, rule)
			continue
		}
		g.Log().Infof(ctx, "采集资料组最终去重跳过 eventId:%d previousEventId:%d ruleId:%d channelId:%d layer:%s", event["id"].Int64(), previousEventID, rule["id"].Int64(), channelID, layer)
	}
	return filtered, nil
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
			for _, item := range []struct{ layer, signature string }{{"text_hash", current.textHash}, {"media_fingerprint", current.mediaKey}} {
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
