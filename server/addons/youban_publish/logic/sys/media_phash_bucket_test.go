package sys

import "testing"

func TestMediaPHashBucketMediaTypeConditionUsesVisualMedia(t *testing.T) {
	condition, args := mediaPHashBucketMediaTypeCondition("b.media_type", "video")
	if condition != "b.media_type IN ('image', 'video')" {
		t.Fatalf("unexpected condition: %s", condition)
	}
	if len(args) != 0 {
		t.Fatalf("visual media condition should not have args: %#v", args)
	}
}

func TestMediaPHashBucketMediaTypeConditionKeepsUnknownTypesExact(t *testing.T) {
	condition, args := mediaPHashBucketMediaTypeCondition("b.media_type", "audio")
	if condition != "b.media_type = ?" {
		t.Fatalf("unexpected condition: %s", condition)
	}
	if len(args) != 1 || args[0] != "audio" {
		t.Fatalf("unexpected args: %#v", args)
	}
}
