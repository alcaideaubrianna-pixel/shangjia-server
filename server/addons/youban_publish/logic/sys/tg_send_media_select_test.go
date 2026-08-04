package sys

import "testing"

func TestSelectTelegramDisplayMedia(t *testing.T) {
	job := telegramJobRecord{Id: 21, ProfileId: 7, ChannelId: 3}
	media := make([]*telegramMediaItem, 0, 20)
	for id := int64(1); id <= 20; id++ {
		media = append(media, &telegramMediaItem{Id: id, MediaType: "image", MustSend: id <= 3, SortIndex: int(id)})
	}
	selected := selectTelegramDisplayMedia(job, media, 10)
	if len(selected) != 10 {
		t.Fatalf("selected=%d, want 10", len(selected))
	}
	for id := int64(1); id <= 3; id++ {
		found := false
		for _, item := range selected {
			if item.Id == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("required media %d missing", id)
		}
	}
	again := selectTelegramDisplayMedia(job, media, 10)
	for i := range selected {
		if selected[i].Id != again[i].Id {
			t.Fatal("selection must be stable for retries")
		}
	}
}

func TestSelectTelegramDisplayMediaKeepsAllRequired(t *testing.T) {
	media := make([]*telegramMediaItem, 12)
	for i := range media {
		media[i] = &telegramMediaItem{Id: int64(i + 1), MediaType: "image", MustSend: true, SortIndex: i + 1}
	}
	selected := selectTelegramDisplayMedia(telegramJobRecord{Id: 1}, media, 10)
	if len(selected) != 12 {
		t.Fatalf("selected=%d, want 12", len(selected))
	}
}

func TestSelectTelegramDisplayMediaTreatsVideoAsRequired(t *testing.T) {
	media := make([]*telegramMediaItem, 0, 12)
	media = append(media, &telegramMediaItem{Id: 1, MediaType: "video", MustSend: false, SortIndex: 1})
	for id := int64(2); id <= 12; id++ {
		media = append(media, &telegramMediaItem{Id: id, MediaType: "image", MustSend: false, SortIndex: int(id)})
	}
	selected := selectTelegramDisplayMedia(telegramJobRecord{Id: 8}, media, 10)
	for _, item := range selected {
		if item.Id == 1 {
			return
		}
	}
	t.Fatal("video media must always be selected")
}
