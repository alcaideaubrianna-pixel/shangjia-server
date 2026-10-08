package sys

import "testing"

func TestHasUsableCollectVerificationMedia(t *testing.T) {
	content := &collectContentResult{Media: []collectMediaItem{
		{Purpose: "display", Type: "image", StoragePath: "display.jpg"},
		{Purpose: "verify", Type: "video", StoragePath: "verify.mp4"},
		{Purpose: "verify", Type: "video"},
	}}
	if !hasUsableCollectVerificationMedia(content) {
		t.Fatal("expected usable verification media")
	}
	if hasUsableCollectVerificationMedia(&collectContentResult{Media: []collectMediaItem{{Purpose: "verify"}}}) {
		t.Fatal("verification media without a cached location must not be usable")
	}
}

func TestDuplicateProfileIDFromMessage(t *testing.T) {
	if got := duplicateProfileIDFromMessage("资料库已存在相同资料 profileId:509473 channelId:103 layer:text_hash"); got != 509473 {
		t.Fatalf("profile id=%d", got)
	}
	if got := duplicateProfileIDFromMessage("资料库已存在相同资料"); got != 0 {
		t.Fatalf("invalid message profile id=%d", got)
	}
}
