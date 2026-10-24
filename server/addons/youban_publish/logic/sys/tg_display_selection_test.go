package sys

import (
	"encoding/json"
	"hotgo/addons/youban_publish/model/input/sysin"
	"reflect"
	"testing"
)

func TestDisplaySelectionPolicy(t *testing.T) {
	on, off := true, false
	for _, tc := range []struct {
		name                   string
		images, required, want int
		policy                 *sysin.ProfileRandomMediaPolicy
	}{
		{"ten default", 10, 0, 10, nil},
		{"eleven automatic", 11, 0, 10, nil},
		{"manual default five", 8, 0, 5, &sysin.ProfileRandomMediaPolicy{Enabled: &on}},
		{"manual three", 8, 1, 3, &sysin.ProfileRandomMediaPolicy{Enabled: &on, Count: 3}},
		{"required exceed target", 12, 7, 7, &sysin.ProfileRandomMediaPolicy{Enabled: &on, Count: 5}},
		{"all required exceed album", 12, 12, 12, nil},
		{"explicit all", 12, 0, 12, &sysin.ProfileRandomMediaPolicy{Enabled: &off, Count: 5}},
		{"fewer than requested", 3, 0, 3, &sysin.ProfileRandomMediaPolicy{Enabled: &on, Count: 5}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			media := make([]*telegramMediaItem, tc.images)
			for i := range media {
				media[i] = &telegramMediaItem{Id: int64(i + 1), MediaType: "image", MustSend: i < tc.required}
			}
			got := selectTelegramDisplayMediaWithPolicy(telegramJobRecord{Id: 91}, media, tc.policy)
			if len(got) != tc.want {
				t.Fatalf("got %d want %d", len(got), tc.want)
			}
			for i := 0; i < tc.required; i++ {
				if got[len(got)-tc.required+i].Id != int64(i+1) {
					t.Fatal("required media must all be last")
				}
			}
			if !reflect.DeepEqual(got, selectTelegramDisplayMediaWithPolicy(telegramJobRecord{Id: 91}, media, tc.policy)) {
				t.Fatal("retry order changed")
			}
			for i, item := range media {
				if item.Id != int64(i+1) {
					t.Fatal("input mutated")
				}
			}
		})
	}
}

func TestDisplayVideoSelectionAndSnapshot(t *testing.T) {
	media := []*telegramMediaItem{{Id: 1, MediaType: "video"}, {Id: 2, MediaType: "video"}, {Id: 3, MediaType: "image"}}
	job := telegramJobRecord{Id: 5}
	got := selectTelegramDisplayMediaWithPolicy(job, media, nil)
	if len(got) != 3 {
		t.Fatal("media within the limit must all be selected")
	}
	ids := make([]int64, 0, len(got))
	for _, item := range got {
		ids = append(ids, item.Id)
	}
	payload, _ := json.Marshal(ids)
	media[0], media[2] = media[2], media[0]
	restored, err := restoreTelegramDisplaySelection(string(payload), media)
	if err != nil || !reflect.DeepEqual(restored, got) {
		t.Fatalf("restore changed selection: %v", err)
	}
	if _, err = restoreTelegramDisplaySelection("[999]", media); err == nil {
		t.Fatal("must not reselect missing snapshot media")
	}
	if _, err = restoreTelegramDisplaySelection("bad", media); err == nil {
		t.Fatal("must reject corrupt snapshot")
	}
	media[1].MustSend = true
	got = selectTelegramDisplayMediaWithPolicy(job, media, nil)
	if len(got) != 3 {
		t.Fatal("required media must not reduce media within the limit")
	}
	media[2].MustSend = true // second required video after input reorder
	got = selectTelegramDisplayMediaWithPolicy(job, media, nil)
	if len(got) != 3 {
		t.Fatal("all required videos retained")
	}
}

func TestDisplaySelectionLimitsTotalImagesAndVideos(t *testing.T) {
	on := true
	media := []*telegramMediaItem{
		{Id: 1, MediaType: "video"}, {Id: 2, MediaType: "video"},
		{Id: 3, MediaType: "video"}, {Id: 4, MediaType: "image"},
	}
	if got := selectTelegramDisplayMediaWithPolicy(telegramJobRecord{Id: 7}, media, &sysin.ProfileRandomMediaPolicy{Enabled: &on, Count: 5}); len(got) != 4 {
		t.Fatalf("media below configured limit were reduced: %d", len(got))
	}
	got := selectTelegramDisplayMediaWithPolicy(telegramJobRecord{Id: 7}, media, &sysin.ProfileRandomMediaPolicy{Enabled: &on, Count: 2})
	if len(got) != 2 {
		t.Fatalf("selected=%d, want total media limit 2", len(got))
	}
}

func TestDisplaySelectionChangesBetweenJobs(t *testing.T) {
	media := make([]*telegramMediaItem, 20)
	for i := range media {
		media[i] = &telegramMediaItem{Id: int64(i + 1), MediaType: "image"}
	}
	a := selectTelegramDisplayMediaWithPolicy(telegramJobRecord{Id: 1}, media, nil)
	b := selectTelegramDisplayMediaWithPolicy(telegramJobRecord{Id: 2}, media, nil)
	if reflect.DeepEqual(a, b) {
		t.Fatal("new jobs must vary selection/order")
	}
}

func TestDisplaySelectionDoesNotConsumeVerificationVideos(t *testing.T) {
	media := []*telegramMediaItem{{Id: 1, MediaType: "image", Purpose: "display"}, {Id: 2, MediaType: "video", Purpose: "verify", MustSend: true}}
	got := selectTelegramDisplayMediaWithPolicy(telegramJobRecord{Id: 1}, media, nil)
	if len(got) != 1 || got[0].Id != 1 || media[1].Purpose != "verify" {
		t.Fatal("verification media must stay separate")
	}
}

func TestRequiredImagesFollowRequiredDisplayVideos(t *testing.T) {
	media := []*telegramMediaItem{{Id: 1, MediaType: "image", MustSend: true}, {Id: 2, MediaType: "video", MustSend: true}, {Id: 3, MediaType: "image"}}
	got := selectTelegramDisplayMediaWithPolicy(telegramJobRecord{Id: 1}, media, nil)
	if len(got) != 3 || got[0].Id != 1 || got[1].Id != 2 || got[2].Id != 3 {
		t.Fatal("media within the limit must preserve profile order")
	}
}
