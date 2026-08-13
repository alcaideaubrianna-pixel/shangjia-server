package sys

import "testing"

func TestMaterialImportLatestSourceMessageID(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int
	}{
		{name: "sorted ids", value: "151,152,153,154,155", want: 155},
		{name: "unsorted ids", value: "155,151,154", want: 155},
		{name: "invalid values", value: "abc,0,-1,156", want: 156},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := materialImportLatestSourceMessageID(test.value); got != test.want {
				t.Fatalf("latest source message id = %d, want %d", got, test.want)
			}
		})
	}
}

func TestMaterialImportVerifyMessageMustBeContinuous(t *testing.T) {
	if got := materialImportLatestSourceMessageID("151,152,153,154,155"); got+1 != 156 {
		t.Fatal("message 156 should be continuous with the profile group")
	}
	if got := materialImportLatestSourceMessageID("151,152,153,154,155"); got+1 == 125 {
		t.Fatal("message 125 must not be attached to the profile group")
	}
}

func TestMaterialImportHasVerifyMedia(t *testing.T) {
	if !materialImportHasVerifyMedia([]collectMediaItem{{Type: "video", Purpose: "verify"}}) {
		t.Fatal("expected verify media to be detected")
	}
	if materialImportHasVerifyMedia([]collectMediaItem{{Type: "video", Purpose: "display"}}) {
		t.Fatal("display media must not be treated as verify media")
	}
}

func TestCollectMaterialUnitsPairDisplayAndVerify(t *testing.T) {
	units := []*collectMaterialUnit{
		{RawText: "昵称：A100", MessageId: 151, Messages: []int{151}, Media: []collectMediaItem{{Type: "photo", FileId: "photo-1"}}},
		{MessageId: 152, Messages: []int{152}, Media: []collectMediaItem{{Type: "video", FileId: "video-1"}}},
		{RawText: "昵称：A101", MessageId: 153, Messages: []int{153}, Media: []collectMediaItem{{Type: "photo", FileId: "photo-2"}}},
	}
	paired := pairCollectMaterialUnits(units)
	if len(paired) != 2 {
		t.Fatalf("paired units=%d, want 2", len(paired))
	}
	if len(paired[0].Media) != 2 || paired[0].Media[1].Purpose != "verify" {
		t.Fatalf("first display unit did not receive one verify group: %#v", paired[0].Media)
	}
}

func TestCollectMaterialUnitsPairAcrossIgnoredMessages(t *testing.T) {
	units := []*collectMaterialUnit{
		{RawText: "昵称：A200\n城市：北京", MessageId: 200, Media: []collectMediaItem{{Type: "image", FileId: "photo-1"}}},
		{RawText: "随手发的一条干扰消息", MessageId: 201},
		{MessageId: 202, Media: []collectMediaItem{{Type: "video", FileId: "verify-1"}}},
		{RawText: "昵称：A201\n城市：上海", MessageId: 203, Media: []collectMediaItem{{Type: "image", FileId: "photo-2"}}},
		{MessageId: 204, Media: []collectMediaItem{{Type: "video", FileId: "verify-2"}}},
	}
	paired := pairCollectMaterialUnits(units)
	if len(paired) != 2 {
		t.Fatalf("paired units=%d, want 2", len(paired))
	}
	for index, unit := range paired {
		verifyCount := 0
		for _, media := range unit.Media {
			if media.Purpose == collectMaterialRoleVerify {
				verifyCount++
			}
		}
		if verifyCount != 1 {
			t.Fatalf("paired unit %d verify count=%d, want 1", index, verifyCount)
		}
	}
}

func TestCollectMaterialUnitsNeverMixDisplayAndVerifyGroups(t *testing.T) {
	units := []*collectMaterialUnit{
		{GroupedId: "display-1", RawText: "昵称：A300\n城市：广州", MessageId: 300, Messages: []int{300, 301}, Media: []collectMediaItem{
			{Type: "photo", FileId: "photo-1"}, {Type: "video", FileId: "display-video"},
		}},
		{GroupedId: "verify-1", MessageId: 302, Messages: []int{302}, Media: []collectMediaItem{{Type: "video", FileId: "verify-video"}}},
	}
	paired := pairCollectMaterialUnits(units)
	if len(paired) != 1 || len(paired[0].Media) != 3 {
		t.Fatalf("paired=%#v", paired)
	}
	for _, media := range paired[0].Media {
		if media.FileId == "display-video" && media.Purpose == collectMaterialRoleVerify {
			t.Fatal("display video must never be relabeled as verification media")
		}
		if media.FileId == "verify-video" && media.Purpose != collectMaterialRoleVerify {
			t.Fatal("verification video must remain separated by purpose")
		}
	}
}
