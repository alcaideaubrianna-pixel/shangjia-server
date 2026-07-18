package sys

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
	"time"

	"hotgo/addons/lazysheep_tggo/model"
)

func TestNoteFingerprintUsesMediaURLsOnly(t *testing.T) {
	first, _ := noteFingerprint(noteContent{Items: []noteItem{
		{Type: noteTypeImage, Content: "https://img.example/a.jpg", Title: "深圳测试 v1"},
		{Type: noteTypeVideo, Content: "https://img.example/b.mp4"},
		{Type: noteTypeText, Content: "第一段文案"},
	}})
	second, _ := noteFingerprint(noteContent{Items: []noteItem{
		{Type: noteTypeText, Content: "第二段文案"},
		{Type: noteTypeVideo, Content: "https://img.example/b.mp4", Title: "深圳测试 v2"},
		{Type: noteTypeImage, Content: "https://img.example/a.jpg"},
	}})
	if first == "" {
		t.Fatal("fingerprint should not be empty")
	}
	if first != second {
		t.Fatalf("same media urls should produce same fingerprint, got %s and %s", first, second)
	}
}

func TestNoteFingerprintKeepsMediaURLCount(t *testing.T) {
	first, _ := noteFingerprint(noteContent{Items: []noteItem{
		{Type: noteTypeImage, Content: "https://img.example/a.jpg"},
	}})
	second, _ := noteFingerprint(noteContent{Items: []noteItem{
		{Type: noteTypeImage, Content: "https://img.example/a.jpg"},
		{Type: noteTypeVideo, Content: "https://img.example/a.jpg"},
	}})
	if first == second {
		t.Fatal("different media url counts should not produce same fingerprint")
	}
}

func TestNoteFingerprintWithoutMediaIsEmpty(t *testing.T) {
	fingerprint, _ := noteFingerprint(noteContent{Items: []noteItem{
		{Type: noteTypeTitle, Content: "标题"},
		{Type: noteTypeText, Content: "纯文本"},
	}})
	if fingerprint != "" {
		t.Fatalf("expected empty fingerprint for note without media, got %s", fingerprint)
	}
}

func TestPullDedupRecordFreshUsesSevenDayWindow(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	fresh := pullDedupRecord{SeenAt: now.Add(-7*24*time.Hour + time.Second).Format(time.RFC3339)}
	if !pullDedupRecordFresh(fresh, now) {
		t.Fatal("record inside 7 days should be treated as duplicate")
	}
	expired := pullDedupRecord{SeenAt: now.Add(-7 * 24 * time.Hour).Format(time.RFC3339)}
	if pullDedupRecordFresh(expired, now) {
		t.Fatal("record at or beyond 7 days should not be treated as duplicate")
	}
}

func TestPullDedupRecordFreshKeepsUnknownLegacyRecords(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	if !pullDedupRecordFresh(pullDedupRecord{}, now) {
		t.Fatal("legacy record without seenAt should stay duplicate")
	}
	if !pullDedupRecordFresh(pullDedupRecord{SeenAt: "bad-time"}, now) {
		t.Fatal("legacy record with invalid seenAt should stay duplicate")
	}
}

func TestSelectQuickMediaItemsForPushMergesVerifyVideo(t *testing.T) {
	items := []noteItem{
		{Type: noteTypeImage, Content: "https://img.example/1.jpg"},
		{Type: noteTypeImage, Content: "https://img.example/2.jpg"},
		{Type: noteTypeVideo, Content: "https://img.example/verify.mp4", VerifyVideo: true},
	}
	selected, merged := selectQuickMediaItemsForPush(items, map[string]any{"mergeVerifyInGroup": true})
	if !merged {
		t.Fatal("expected verify video to be merged into media group")
	}
	if len(selected) != len(items) {
		t.Fatalf("expected all media to be kept, got %d", len(selected))
	}
	if selected[2].Content != "https://img.example/verify.mp4" {
		t.Fatalf("expected verify video to stay in group, got %s", selected[2].Content)
	}
}

func TestSelectQuickMediaItemsForPushMergesUnmarkedVideo(t *testing.T) {
	items := []noteItem{
		{Type: noteTypeImage, Content: "https://img.example/1.jpg"},
		{Type: noteTypeImage, Content: "https://img.example/2.jpg"},
		{Type: noteTypeVideo, Content: "https://img.example/video.mp4"},
	}
	selected, merged := selectQuickMediaItemsForPush(items, map[string]any{"mergeVerifyInGroup": true})
	if !merged {
		t.Fatal("expected first video to be merged into media group")
	}
	if len(selected) != len(items) {
		t.Fatalf("expected all media to be kept, got %d", len(selected))
	}
	if selected[2].Type != noteTypeVideo || selected[2].VerifyVideo {
		t.Fatalf("expected unmarked video to stay as normal video, got %#v", selected[2])
	}
}

func TestSelectQuickMediaItemsForPushKeepsTenMediaIncludingNormalAndVerifyVideo(t *testing.T) {
	items := make([]noteItem, 0, 10)
	for i := 0; i < 8; i++ {
		items = append(items, noteItem{Type: noteTypeImage, Content: fmt.Sprintf("https://img.example/%02d.jpg", i)})
	}
	items = append(items,
		noteItem{Type: noteTypeVideo, Content: "https://img.example/normal-1.mp4"},
		noteItem{Type: noteTypeVideo, Content: "https://img.example/verify-1.mp4", VerifyVideo: true},
	)
	selected, merged := selectQuickMediaItemsForPush(items, map[string]any{"mergeVerifyInGroup": true})
	if !merged {
		t.Fatal("expected verify video to be merged into media group")
	}
	if len(selected) != quickMediaGroupLimit {
		t.Fatalf("expected %d media items, got %d", quickMediaGroupLimit, len(selected))
	}
	if selected[8].Content != "https://img.example/normal-1.mp4" || selected[9].Content != "https://img.example/verify-1.mp4" {
		t.Fatalf("expected normal and verify videos to be kept, got %#v", selected[8:])
	}
}

func TestSelectQuickMediaItemsForPushPrefersThreeVerifyVideosAndOneNormalVideo(t *testing.T) {
	items := make([]noteItem, 0, 12)
	for i := 0; i < 6; i++ {
		items = append(items, noteItem{Type: noteTypeImage, Content: fmt.Sprintf("https://img.example/%02d.jpg", i)})
	}
	for i := 0; i < 3; i++ {
		items = append(items, noteItem{Type: noteTypeVideo, Content: fmt.Sprintf("https://img.example/normal-%d.mp4", i)})
	}
	for i := 0; i < 3; i++ {
		items = append(items, noteItem{Type: noteTypeVideo, Content: fmt.Sprintf("https://img.example/verify-%d.mp4", i), VerifyVideo: true})
	}
	selected, merged := selectQuickMediaItemsForPush(items, map[string]any{"mergeVerifyInGroup": true})
	if !merged {
		t.Fatal("expected verify video to be merged into media group")
	}
	if len(selected) != quickMediaGroupLimit {
		t.Fatalf("expected %d media items, got %d", quickMediaGroupLimit, len(selected))
	}
	imageCount, normalVideoCount, verifyVideoCount := 0, 0, 0
	for _, item := range selected {
		switch {
		case item.Type == noteTypeImage:
			imageCount++
		case item.Type == noteTypeVideo && item.VerifyVideo:
			verifyVideoCount++
		case item.Type == noteTypeVideo:
			normalVideoCount++
		}
	}
	if imageCount != 6 || normalVideoCount != 1 || verifyVideoCount != 3 {
		t.Fatalf("expected 6 images, 1 normal video, 3 verify videos; got images:%d normal:%d verify:%d selected:%#v", imageCount, normalVideoCount, verifyVideoCount, selected)
	}
}

func TestSelectQuickMediaItemsForPushKeepsVerifyVideosWhenOverLimit(t *testing.T) {
	items := make([]noteItem, 0, 15)
	for i := 0; i < 12; i++ {
		items = append(items, noteItem{Type: noteTypeImage, Content: fmt.Sprintf("https://img.example/%02d.jpg", i)})
	}
	items = append(items,
		noteItem{Type: noteTypeVideo, Content: "https://img.example/normal-1.mp4"},
		noteItem{Type: noteTypeVideo, Content: "https://img.example/verify-1.mp4", VerifyVideo: true},
		noteItem{Type: noteTypeVideo, Content: "https://img.example/verify-2.mp4", VerifyVideo: true},
	)
	selected, merged := selectQuickMediaItemsForPush(items, map[string]any{"mergeVerifyInGroup": true})
	if !merged {
		t.Fatal("expected verify video to be merged into media group")
	}
	if len(selected) != quickMediaGroupLimit {
		t.Fatalf("expected %d media items, got %d", quickMediaGroupLimit, len(selected))
	}
	foundVerify := map[string]bool{}
	for _, item := range selected {
		if item.Type == noteTypeVideo && item.VerifyVideo {
			foundVerify[item.Content] = true
		}
	}
	if !foundVerify["https://img.example/verify-1.mp4"] || !foundVerify["https://img.example/verify-2.mp4"] {
		t.Fatalf("expected verify videos to be preferred, got %#v", selected)
	}
}

func TestSelectQuickMediaItemsForPushCanBeDisabled(t *testing.T) {
	items := []noteItem{
		{Type: noteTypeImage, Content: "https://img.example/1.jpg"},
		{Type: noteTypeVideo, Content: "https://img.example/verify.mp4", VerifyVideo: true},
	}
	selected, merged := selectQuickMediaItemsForPush(items, map[string]any{"mergeVerifyInGroup": false})
	if merged {
		t.Fatal("expected merge mode to be disabled")
	}
	if len(selected) != len(items) {
		t.Fatalf("expected original items, got %d", len(selected))
	}
}

func TestCollectorMergeVerifyGroupEnabledDefaultsToEnabled(t *testing.T) {
	if !collectorMergeVerifyGroupEnabled(nil, nil) {
		t.Fatal("expected merge verify group to be enabled by default")
	}
	if !collectorMergeVerifyGroupEnabled(nil, map[string]any{}) {
		t.Fatal("expected old binding without merge setting to be enabled")
	}
	if collectorMergeVerifyGroupEnabled(nil, map[string]any{collectorMergeVerifyGroupStateKey: false}) {
		t.Fatal("expected explicit binding false to disable merge")
	}
	plugins := map[string]*model.PluginConfig{
		"collector": {
			Settings: map[string]any{"mergeVerifyInGroup": false},
		},
	}
	if !collectorMergeVerifyGroupEnabled(plugins, map[string]any{}) {
		t.Fatal("expected old global false to keep default merge enabled")
	}
}

func TestSplitQuickMediaAssetsWithMergeModeKeepsSmallMediaTogether(t *testing.T) {
	assets := make([]quickMediaAsset, 0, quickMediaGroupLimit)
	for i := 0; i < quickMediaGroupLimit-1; i++ {
		assets = append(assets, quickMediaAsset{
			Type:      noteTypeImage,
			SourceURL: fmt.Sprintf("https://img.example/%02d.jpg", i),
			Data:      make([]byte, 8),
		})
	}
	assets = append(assets, quickMediaAsset{
		Type:        noteTypeVideo,
		SourceURL:   "https://img.example/verify.mp4",
		Data:        make([]byte, 8),
		VerifyVideo: true,
	})
	parts := splitQuickMediaAssetsWithMode(assets, true)
	if len(parts) != 1 {
		t.Fatalf("expected merged media to stay in one group, got %d", len(parts))
	}
	if len(parts[0]) != quickMediaGroupLimit {
		t.Fatalf("expected %d items in merged group, got %d", quickMediaGroupLimit, len(parts[0]))
	}
}

func TestSplitQuickMediaAssetsSplitsByCount(t *testing.T) {
	assets := make([]quickMediaAsset, 0, quickMediaGroupLimit+1)
	for i := 0; i < quickMediaGroupLimit+1; i++ {
		assets = append(assets, quickMediaAsset{
			Type:      noteTypeImage,
			SourceURL: fmt.Sprintf("https://img.example/%02d.jpg", i),
			Data:      make([]byte, 8),
		})
	}
	parts := splitQuickMediaAssets(assets)
	if len(parts) != 2 {
		t.Fatalf("expected media to split by count only, got %d groups", len(parts))
	}
	if len(parts[0]) != quickMediaGroupLimit || len(parts[1]) != 1 {
		t.Fatalf("unexpected group sizes: %d and %d", len(parts[0]), len(parts[1]))
	}
}

func TestSplitQuickMediaAssetsSplitsByUploadSize(t *testing.T) {
	assets := []quickMediaAsset{
		{Type: noteTypeImage, SourceURL: "https://img.example/1.jpg", Data: make([]byte, quickMediaGroupMaxUploadBytes/2+1)},
		{Type: noteTypeImage, SourceURL: "https://img.example/2.jpg", Data: make([]byte, quickMediaGroupMaxUploadBytes/2+1)},
	}
	parts := splitQuickMediaAssetsWithMode(assets, false)
	if len(parts) != 2 {
		t.Fatalf("expected media to split by upload size, got %d groups", len(parts))
	}
	if len(parts[0]) != 1 || len(parts[1]) != 1 {
		t.Fatalf("unexpected group sizes: %d and %d", len(parts[0]), len(parts[1]))
	}
}

func TestSplitQuickMediaAssetsSendsDocumentAlone(t *testing.T) {
	assets := []quickMediaAsset{
		{Type: noteTypeImage, SourceURL: "https://img.example/1.jpg", Data: make([]byte, 8)},
		{Type: quickMediaTypeDocument, SourceURL: "https://img.example/big.jpg", Data: make([]byte, quickPhotoMaxBytes+1)},
		{Type: noteTypeImage, SourceURL: "https://img.example/2.jpg", Data: make([]byte, 8)},
	}
	parts := splitQuickMediaAssetsWithMode(assets, false)
	if len(parts) != 3 {
		t.Fatalf("expected document to be isolated, got %d groups", len(parts))
	}
	if parts[1][0].Type != quickMediaTypeDocument {
		t.Fatalf("expected middle group to be document, got %#v", parts[1][0])
	}
}

func TestSplitQuickMediaAssetsMergeModeForcesSingleGroup(t *testing.T) {
	assets := []quickMediaAsset{
		{Type: noteTypeImage, SourceURL: "https://img.example/1.jpg", Data: make([]byte, quickMediaGroupMaxUploadBytes)},
		{Type: noteTypeVideo, SourceURL: "https://img.example/verify.mp4", Data: make([]byte, quickMediaGroupMaxUploadBytes), VerifyVideo: true},
	}
	parts := splitQuickMediaAssetsWithMode(assets, true)
	if len(parts) != 1 {
		t.Fatalf("expected merge mode to force a single group, got %d", len(parts))
	}
	if len(parts[0]) != 2 {
		t.Fatalf("expected both media in forced group, got %d", len(parts[0]))
	}
}

func TestCompressQuickPhotoForTelegram(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2200, 2200))
	for y := 0; y < 2200; y++ {
		for x := 0; x < 2200; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	name, data, err := compressQuickPhotoForTelegram(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if name == "" || len(data) == 0 {
		t.Fatal("expected compressed photo")
	}
	if len(data) > quickPhotoMaxBytes {
		t.Fatalf("compressed photo is too large: %d", len(data))
	}
}

func TestMediaPHashFromBytesIsStable(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 4), B: 128, A: 255})
		}
	}
	var first bytes.Buffer
	if err := jpeg.Encode(&first, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	var second bytes.Buffer
	if err := jpeg.Encode(&second, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	firstHash := mediaPHashFromBytes(first.Bytes())
	secondHash := mediaPHashFromBytes(second.Bytes())
	if firstHash == "" || secondHash == "" {
		t.Fatalf("expected non-empty hashes: %q %q", firstHash, secondHash)
	}
	if firstHash != secondHash {
		t.Fatalf("expected stable perceptual hash, got %s and %s", firstHash, secondHash)
	}
}
