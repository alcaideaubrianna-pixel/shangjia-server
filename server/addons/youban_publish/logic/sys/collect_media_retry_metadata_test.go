package sys

import (
	"encoding/json"
	"testing"
)

func TestCollectMediaRetryPreservesReuseIdentity(t *testing.T) {
	stored := `{"reused_published_media_id":123,"existing":"keep","source":"old"}`
	merged := mergeCollectMediaRetryMetadata(stored, `{"reused_published_media_id":999,"source":"new"}`)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(merged), &result); err != nil {
		t.Fatal(err)
	}
	if result["reused_published_media_id"] != float64(123) || result["existing"] != "keep" || result["source"] != "new" {
		t.Fatalf("unexpected merge: %s", merged)
	}
	item := collectMediaItem{FileMd5: "digest", FileUrl: "https://example.com/image.jpg", DebugMetaJson: merged}
	if reusedPublishedCollectMetadata(item) == nil {
		t.Fatal("retry lost published-asset reuse")
	}
	for _, invalid := range []string{"broken", "null", "[]"} {
		if got := mergeCollectMediaRetryMetadata(stored, invalid); got != stored {
			t.Fatalf("invalid metadata erased stored fields: %s", got)
		}
	}
}
