package sys

import (
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestClassifyCollectPublishMediaKeepsVerifyMediaSeparate(t *testing.T) {
	display, verify := classifyCollectPublishMedia(gdb.Record{}, []collectMediaItem{
		{Type: "photo", FileId: "photo-1"},
		{Type: "video", Purpose: "verify", FileId: "video-1"},
		{Type: "photo", FileId: "photo-2"},
	})

	if len(display) != 2 || len(verify) != 1 {
		t.Fatalf("display=%d verify=%d, want display=2 verify=1", len(display), len(verify))
	}
	if verify[0].Purpose != "verify" {
		t.Fatalf("verify purpose=%q, want verify", verify[0].Purpose)
	}
}

func TestCollectMediaJSONHasPurposeVideo(t *testing.T) {
	mediaJSON := `[{"type":"video","purpose":"display"},{"type":"video","purpose":"verify"}]`

	if !collectMediaJSONHasVideo(mediaJSON) {
		t.Fatal("expected any video to be detected")
	}
	if !collectMediaJSONHasPurposeVideo(mediaJSON, "verify") {
		t.Fatal("expected verify video to be detected")
	}
	if collectMediaJSONHasPurposeVideo(`[{"type":"video","purpose":"display"}]`, "verify") {
		t.Fatal("display video must not be treated as verify video")
	}
}

func TestShouldRecoverCollectEventWithUnmatchedVerifyVideo(t *testing.T) {
	row := gdb.Record{
		"status":        g.NewVar(sysin.CollectEventStatusFailed),
		"error_message": g.NewVar("验证视频暂未匹配到前序资料，等待前序资料完成处理"),
	}

	if !shouldRecoverCollectEvent(row) {
		t.Fatal("expected unmatched verify video event to be recoverable")
	}
}

func TestMergeCollectMediaEnrichmentKeepsCanonicalPathAndPHash(t *testing.T) {
	base := []collectMediaItem{{
		Type:        "photo",
		Purpose:     "display",
		FileId:      "copy:-1001:10",
		StoragePath: "cache/current.jpg",
		SourceKind:  "photo", SourceMediaId: 100, SourceAccessHash: 200, SourceThumbSize: "w",
	}}
	enriched := []collectMediaItem{{
		Type:        "photo",
		Purpose:     "display",
		FileId:      "copy:-1001:10",
		StoragePath: "cache/old.jpg",
		FilePhash:   "abcdef0123456789",
		FileMd5:     "media-md5",
		SourceKind:  "photo", SourceMediaId: 100, SourceAccessHash: 200, SourceThumbSize: "w",
	}}

	merged := mergeCollectMediaEnrichment(base, enriched)
	if len(merged) != 1 {
		t.Fatalf("merged media=%d, want 1", len(merged))
	}
	if merged[0].StoragePath != "cache/current.jpg" {
		t.Fatalf("storage path=%q, want canonical current path", merged[0].StoragePath)
	}
	if merged[0].FilePhash != "abcdef0123456789" || merged[0].FileMd5 != "media-md5" {
		t.Fatalf("metadata not preserved: %#v", merged[0])
	}
}
