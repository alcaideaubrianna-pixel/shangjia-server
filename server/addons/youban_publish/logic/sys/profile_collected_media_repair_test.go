package sys

import (
	"testing"

	"hotgo/addons/youban_publish/internal/model/entity"
)

func TestCollectMediaTypesEquivalent(t *testing.T) {
	tests := []struct {
		source string
		target string
		want   bool
	}{
		{source: "photo", target: "image", want: true},
		{source: "image", target: "photo", want: true},
		{source: "video", target: "video", want: true},
		{source: "document", target: "video", want: false},
	}
	for _, test := range tests {
		if got := collectMediaTypesEquivalent(test.source, test.target); got != test.want {
			t.Fatalf("collectMediaTypesEquivalent(%q, %q) = %t, want %t", test.source, test.target, got, test.want)
		}
	}
}

func TestNextCompatibleCollectMediaPreservesMixedMediaOrder(t *testing.T) {
	rows := []*collectEventMediaRow{
		{YoubanPublishCollectEventMedia: &entity.YoubanPublishCollectEventMedia{Id: 11, MediaType: "photo"}},
		{YoubanPublishCollectEventMedia: &entity.YoubanPublishCollectEventMedia{Id: 12, MediaType: "video"}},
		{YoubanPublishCollectEventMedia: &entity.YoubanPublishCollectEventMedia{Id: 13, MediaType: "photo"}},
	}
	used := make(map[int64]struct{})
	firstImage := nextCompatibleCollectMedia(rows, "image", used)
	if firstImage == nil || firstImage.Id != 11 {
		t.Fatalf("first image = %#v, want id 11", firstImage)
	}
	used[firstImage.Id] = struct{}{}
	video := nextCompatibleCollectMedia(rows, "video", used)
	if video == nil || video.Id != 12 {
		t.Fatalf("video = %#v, want id 12", video)
	}
	used[video.Id] = struct{}{}
	secondImage := nextCompatibleCollectMedia(rows, "image", used)
	if secondImage == nil || secondImage.Id != 13 {
		t.Fatalf("second image = %#v, want id 13", secondImage)
	}
}

func TestNextCompatibleCollectMediaAdvancesPastUnavailableSource(t *testing.T) {
	rows := []*collectEventMediaRow{
		{YoubanPublishCollectEventMedia: &entity.YoubanPublishCollectEventMedia{Id: 21, MediaType: "photo"}},
		{YoubanPublishCollectEventMedia: &entity.YoubanPublishCollectEventMedia{Id: 22, MediaType: "photo", SourceMessageRef: "file-22"}},
	}
	used := make(map[int64]struct{})
	first := nextCompatibleCollectMedia(rows, "image", used)
	if first == nil || first.Id != 21 {
		t.Fatalf("first image = %#v, want id 21", first)
	}
	used[first.Id] = struct{}{}
	second := nextCompatibleCollectMedia(rows, "image", used)
	if second == nil || second.Id != 22 {
		t.Fatalf("second image = %#v, want id 22", second)
	}
}
