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

func TestMediaPHashDeduplicateProfilesKeepsBestMediaMatch(t *testing.T) {
	items := mediaPHashDeduplicateProfiles([]publishProfilePHashDistance{
		{ProfileId: 9, MediaId: 100, Distance: 8},
		{ProfileId: 9, MediaId: 101, Distance: 4},
		{ProfileId: 10, MediaId: 102, Distance: 6},
	})
	if len(items) != 2 {
		t.Fatalf("deduplicated matches = %d, want 2", len(items))
	}
	for _, item := range items {
		if item.ProfileId == 9 && (item.MediaId != 101 || item.Distance != 4) {
			t.Fatalf("profile 9 best match = %#v", item)
		}
	}
}
