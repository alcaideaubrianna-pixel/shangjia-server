package sys

import (
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
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

func TestCollectMediaJSONWithPurpose(t *testing.T) {
	mediaJSON := `[{"type":"video","fileId":"video-1"}]`
	got := collectMediaJSONWithPurpose(mediaJSON, "verify")

	if got == mediaJSON {
		t.Fatal("expected media purpose to be added")
	}
	if !materialImportHasVerifyMedia(got) {
		t.Fatalf("media json=%s, expected verify media", got)
	}
}
