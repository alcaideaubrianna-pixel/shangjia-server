package sys

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
)

func (s *sSysPublish) canonicalCollectProfileMedia(ctx context.Context, event gdb.Record, content *collectContentResult) (*collectContentResult, error) {
	if content == nil {
		content = &collectContentResult{}
	}
	resolved := *content
	contentItems := collectMediaRowsToItemsFromJSON(content.MediaJSON)
	displayItems := make([]collectMediaItem, 0, len(contentItems))
	for _, item := range contentItems {
		if strings.EqualFold(strings.TrimSpace(item.Purpose), collectMaterialRoleVerify) {
			continue
		}
		item.Purpose = collectMaterialRoleDisplay
		displayItems = append(displayItems, item)
	}

	if eventID := event["id"].Int64(); eventID > 0 {
		rows, err := s.collectEventMediaRows(ctx, eventID)
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 {
			displayItems = mergeCollectMediaEnrichment(
				collectMediaRowsToItems(rows, collectMaterialRoleDisplay),
				displayItems,
			)
		}
	}

	verifyItems := make([]collectMediaItem, 0)
	verify, err := s.pairedCollectVerifyEvent(ctx, event["id"].Int64())
	if err != nil {
		return nil, err
	}
	if !verify.IsEmpty() {
		rows, rowErr := s.collectEventMediaRows(ctx, verify["id"].Int64())
		if rowErr != nil {
			return nil, rowErr
		}
		if len(rows) > 0 {
			verifyItems = mergeCollectMediaEnrichment(
				collectMediaRowsToItems(rows, collectMaterialRoleVerify),
				collectMediaJSONWithPurposeItems(content.MediaJSON, collectMaterialRoleVerify),
			)
		} else {
			verifyItems = collectMediaJSONWithPurposeItems(verify["media_json"].String(), collectMaterialRoleVerify)
		}
	}
	if len(verifyItems) == 0 {
		verifyItems = collectMediaJSONWithPurposeItems(content.MediaJSON, collectMaterialRoleVerify)
	}

	mediaJSON, mediaCount := mergeCollectMediaJSON(
		collectMediaItemsJSON(displayItems),
		collectMediaItemsJSON(verifyItems),
	)
	resolved.MediaJSON = mediaJSON
	resolved.MediaCount = mediaCount
	resolved.DedupeKey = collectHash(resolved.NormalizedText + ":" + collectMediaSignature(mediaJSON))
	return &resolved, nil
}

func mergeCollectMediaEnrichment(baseItems []collectMediaItem, enrichedItems []collectMediaItem) []collectMediaItem {
	if len(baseItems) == 0 || len(enrichedItems) == 0 {
		return baseItems
	}
	enrichedByKey := make(map[string]collectMediaItem, len(enrichedItems)*2)
	for _, item := range enrichedItems {
		for _, key := range collectMediaMatchKeys(item) {
			enrichedByKey[key] = item
		}
	}
	for index := range baseItems {
		var enriched collectMediaItem
		for _, key := range collectMediaMatchKeys(baseItems[index]) {
			if item, ok := enrichedByKey[key]; ok {
				enriched = item
				break
			}
		}
		if strings.TrimSpace(enriched.FilePhash) != "" {
			baseItems[index].FilePhash = strings.TrimSpace(enriched.FilePhash)
		}
		if strings.TrimSpace(enriched.FileMd5) != "" {
			baseItems[index].FileMd5 = strings.TrimSpace(enriched.FileMd5)
		}
		if strings.TrimSpace(baseItems[index].PosterUrl) == "" {
			baseItems[index].PosterUrl = strings.TrimSpace(enriched.PosterUrl)
		}
	}
	return baseItems
}

func collectMediaMatchKeys(item collectMediaItem) []string {
	keys := make([]string, 0, 2)
	if fingerprint := strings.TrimSpace(collectMediaFingerprint(item)); fingerprint != "" {
		keys = append(keys, "fingerprint:"+fingerprint)
	}
	if sourceKey := strings.TrimSpace(collectMediaSourceKey(item)); sourceKey != "" {
		keys = append(keys, "source:"+sourceKey)
	}
	return keys
}

func collectMediaJSONWithPurposeItems(mediaJSON string, purpose string) []collectMediaItem {
	data := collectMediaJSONWithPurpose(mediaJSON, purpose)
	return collectMediaRowsToItemsFromJSON(data)
}

func collectMediaItemsJSON(items []collectMediaItem) string {
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func classifyCollectPublishMedia(event gdb.Record, items []collectMediaItem) ([]collectMediaItem, []collectMediaItem) {
	text := ""
	if !event.IsEmpty() {
		text = event["raw_text"].String()
	}
	displayItems := make([]collectMediaItem, 0, len(items))
	verifyItems := make([]collectMediaItem, 0)
	unknownItems := make([]collectMediaItem, 0)
	for _, item := range items {
		switch strings.ToLower(strings.TrimSpace(item.Purpose)) {
		case "verify":
			verifyItems = append(verifyItems, item)
		case "display":
			displayItems = append(displayItems, item)
		default:
			unknownItems = append(unknownItems, item)
		}
	}
	if len(verifyItems) > 0 {
		displayItems = append(displayItems, unknownItems...)
		return displayItems, verifyItems
	}
	classification := classifyProfileMessage(text, unknownItems)
	if classification.Kind == profileMessageKindVerify {
		return displayItems, append(verifyItems, unknownItems...)
	}
	return append(displayItems, unknownItems...), nil
}
