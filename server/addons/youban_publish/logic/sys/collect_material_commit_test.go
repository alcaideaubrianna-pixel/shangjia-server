package sys

import "testing"

func TestValidateCollectMaterialMediaRequiresEveryMediaAsset(t *testing.T) {
	tests := []struct {
		name    string
		content *collectContentResult
		wantErr bool
	}{
		{
			name: "all assets prepared",
			content: &collectContentResult{
				MediaCount: 2,
				Media: []collectMediaItem{
					{Type: "photo", StoragePath: "cache/a.jpg"},
					{Type: "video", FileUrl: "https://cdn.test/a.mp4"},
				},
			},
		},
		{
			name: "missing asset path",
			content: &collectContentResult{
				MediaCount: 2,
				Media: []collectMediaItem{
					{Type: "photo", StoragePath: "cache/a.jpg"},
					{Type: "video"},
				},
			},
			wantErr: true,
		},
		{
			name: "media count mismatch",
			content: &collectContentResult{
				MediaCount: 2,
				Media:      []collectMediaItem{{Type: "photo", StoragePath: "cache/a.jpg"}},
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCollectMaterialMedia(test.content)
			if (err != nil) != test.wantErr {
				t.Fatalf("validate error=%v, want error=%t", err, test.wantErr)
			}
		})
	}
}

func TestValidateCollectMaterialMediaKeepsVerifyAssetInGate(t *testing.T) {
	content := &collectContentResult{
		MediaCount: 2,
		Media: []collectMediaItem{
			{Type: "photo", StoragePath: "cache/a.jpg", Purpose: "display"},
			{Type: "video", Purpose: "verify"},
		},
	}

	if err := validateCollectMaterialMedia(content); err == nil {
		t.Fatal("expected verify media without a prepared path to block commit")
	}
}

func TestCollectPreparedMediaCounts(t *testing.T) {
	imageCount, videoCount, hasVerificationVideo := collectPreparedMediaCounts([]collectPreparedMedia{
		{MediaType: "image", Purpose: collectMaterialRoleDisplay},
		{MediaType: "video", Purpose: collectMaterialRoleDisplay},
		{MediaType: "video", Purpose: collectMaterialRoleVerify},
	})

	if imageCount != 1 || videoCount != 2 || hasVerificationVideo != 1 {
		t.Fatalf("counts=(%d, %d, %d), want (1, 2, 1)", imageCount, videoCount, hasVerificationVideo)
	}
}

func TestValidatePreparedCollectMediaRequiresVideoPoster(t *testing.T) {
	if err := validatePreparedCollectMedia(collectPreparedMedia{MediaType: "image"}); err != nil {
		t.Fatalf("image should not require poster: %v", err)
	}
	if err := validatePreparedCollectMedia(collectPreparedMedia{MediaType: "video", PosterURL: "/attachment/poster.jpg"}); err != nil {
		t.Fatalf("video poster URL should pass: %v", err)
	}
	if err := validatePreparedCollectMedia(collectPreparedMedia{MediaType: "video", PosterStoragePath: "attachment/poster.jpg"}); err != nil {
		t.Fatalf("video poster storage path should pass: %v", err)
	}
	if err := validatePreparedCollectMedia(collectPreparedMedia{MediaType: "video"}); err == nil {
		t.Fatal("video without poster should block material commit")
	}
}

func TestIsCollectMediaCachePath(t *testing.T) {
	for _, path := range []string{
		"storage/cache/youban_publish/media/a.jpg",
		"resource/public/storage/cache/youban_publish/media/a.jpg",
		"/resource/public/storage/cache/youban_publish/media/a.jpg",
	} {
		if !isCollectMediaCachePath(path) {
			t.Fatalf("path %q should be recognized as local collection cache", path)
		}
	}
	for _, path := range []string{"hotgo/file/2026/a.jpg", "https://cdn.test/a.jpg", ""} {
		if isCollectMediaCachePath(path) {
			t.Fatalf("path %q should not be recognized as local collection cache", path)
		}
	}
}

func TestCollectPreparedContentSnapshotAlignsReviewAndProfilePayload(t *testing.T) {
	prepared := &collectPreparedMaterial{
		Content: &collectContentResult{RawText: "北京 老师", NormalizedText: "北京 老师", TextHash: "text-hash"},
		Media: []collectPreparedMedia{
			{
				EventMediaId: 11,
				Purpose:      collectMaterialRoleDisplay, MediaType: "image", FileId: "photo-1",
				FileURL: "https://cdn.test/photo.jpg", StoragePath: "storage/photo.jpg",
				PerceptualHash: "abcdef0123456789", MD5: "photo-md5", MetaJSON: `{"id":1}`,
			},
			{
				EventMediaId: 12,
				Purpose:      collectMaterialRoleVerify, MediaType: "video", FileId: "video-1",
				FileURL: "https://cdn.test/verify.mp4", StoragePath: "storage/verify.mp4",
			},
		},
	}

	snapshot := collectPreparedContentSnapshot(prepared)
	if snapshot == nil || snapshot.MediaCount != 2 {
		t.Fatalf("snapshot=%#v, want two media items", snapshot)
	}
	if snapshot.Media[0].FilePhash != "abcdef0123456789" || snapshot.Media[0].FileUrl != "https://cdn.test/photo.jpg" {
		t.Fatalf("display media snapshot mismatch: %#v", snapshot.Media[0])
	}
	if snapshot.Media[0].EventMediaId != 11 || snapshot.Media[1].EventMediaId != 12 {
		t.Fatalf("event media ids were not preserved: %#v", snapshot.Media)
	}
	if snapshot.Media[1].Purpose != collectMaterialRoleVerify {
		t.Fatalf("verify media purpose mismatch: %#v", snapshot.Media[1])
	}
	if snapshot.DedupeKey == "" {
		t.Fatal("prepared snapshot must contain dedupe key")
	}
}
