package sys

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
)

// selectTelegramDisplayMedia is retained only for pre-upgrade jobs with delivery evidence.
func selectTelegramDisplayMedia(job telegramJobRecord, media []*telegramMediaItem, maxItems int, randomEnabled bool) []*telegramMediaItem {
	if maxItems <= 0 || len(media) <= maxItems {
		return media
	}
	if !randomEnabled {
		return selectFreeTelegramDisplayMedia(media, maxItems)
	}
	required := make([]*telegramMediaItem, 0, len(media))
	optional := make([]*telegramMediaItem, 0, len(media))
	for _, item := range media {
		if item == nil {
			continue
		}
		if item.MustSend || !strings.EqualFold(strings.TrimSpace(item.MediaType), "image") {
			required = append(required, item)
		} else {
			optional = append(optional, item)
		}
	}
	if len(required) >= maxItems || len(optional) == 0 {
		return required
	}
	slots := maxItems - len(required)
	sort.SliceStable(optional, func(i, j int) bool {
		return telegramMediaRandomScore(job, optional[i]) < telegramMediaRandomScore(job, optional[j])
	})
	if slots < len(optional) {
		optional = optional[:slots]
	}
	selected := append(required, optional...)
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].SortIndex != selected[j].SortIndex {
			return selected[i].SortIndex < selected[j].SortIndex
		}
		return selected[i].Id < selected[j].Id
	})
	return selected
}

func selectFreeTelegramDisplayMedia(media []*telegramMediaItem, maxItems int) []*telegramMediaItem {
	selected := make([]*telegramMediaItem, 0, maxItems)
	for _, item := range media {
		if item == nil {
			continue
		}
		if len(selected) >= maxItems {
			break
		}
		selected = append(selected, item)
	}
	return selected
}

func telegramMediaRandomScore(job telegramJobRecord, item *telegramMediaItem) uint64 {
	hasher := fnv.New64a()
	_, _ = fmt.Fprintf(hasher, "%d:%d:%d:%d", job.Id, job.ProfileId, job.ChannelId, item.Id)
	return hasher.Sum64()
}
