package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestApplyProfileCoverAsset(t *testing.T) {
	tests := []struct {
		name            string
		media           *sysin.MediaModel
		wantMediaType   string
		wantFileURL     string
		wantStoragePath string
	}{
		{
			name: "video uses poster",
			media: &sysin.MediaModel{
				MediaType:         "video",
				FileUrl:           "video.mp4",
				StoragePath:       "video.mp4",
				PosterUrl:         "poster.jpg",
				PosterStoragePath: "poster.jpg",
			},
			wantMediaType:   "image",
			wantFileURL:     "poster.jpg",
			wantStoragePath: "poster.jpg",
		},
		{
			name: "video keeps file without poster",
			media: &sysin.MediaModel{
				MediaType:   "video",
				FileUrl:     "video.mp4",
				StoragePath: "video.mp4",
			},
			wantMediaType:   "video",
			wantFileURL:     "video.mp4",
			wantStoragePath: "video.mp4",
		},
		{
			name: "image keeps original file",
			media: &sysin.MediaModel{
				MediaType:         "image",
				FileUrl:           "image.jpg",
				StoragePath:       "image.jpg",
				PosterUrl:         "unused.jpg",
				PosterStoragePath: "unused.jpg",
			},
			wantMediaType:   "image",
			wantFileURL:     "image.jpg",
			wantStoragePath: "image.jpg",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			applyProfileCoverAsset(test.media)
			if test.media.MediaType != test.wantMediaType {
				t.Fatalf("MediaType = %q, want %q", test.media.MediaType, test.wantMediaType)
			}
			if test.media.FileUrl != test.wantFileURL {
				t.Fatalf("FileUrl = %q, want %q", test.media.FileUrl, test.wantFileURL)
			}
			if test.media.StoragePath != test.wantStoragePath {
				t.Fatalf("StoragePath = %q, want %q", test.media.StoragePath, test.wantStoragePath)
			}
		})
	}
}

func TestProfileListCountCacheKeyIgnoresPagination(t *testing.T) {
	first := &sysin.ProfileListInp{}
	first.Page = 1
	first.PerPage = 20
	second := &sysin.ProfileListInp{}
	second.Page = 2
	second.PerPage = 50
	if profileListCountCacheKey("accounts", 25, 0, []int64{473, 472}, first) != profileListCountCacheKey("accounts", 25, 0, []int64{472, 473}, second) {
		t.Fatal("pagination or account ordering changed the count cache key")
	}
}
