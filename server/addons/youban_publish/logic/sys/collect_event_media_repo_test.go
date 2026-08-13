package sys

import (
	"testing"

	"hotgo/addons/youban_publish/internal/model/entity"
)

func TestCollectMediaRowsToItemsPreservesPurpose(t *testing.T) {
	items := collectMediaRowsToItems([]*collectEventMediaRow{{YoubanPublishCollectEventMedia: &entity.YoubanPublishCollectEventMedia{
		Id: 73, MediaType: "video", SourceFileId: "gotd:-100:42", MetaJson: `{"kind":"document"}`,
	}}}, collectMaterialRoleVerify)
	if len(items) != 1 {
		t.Fatalf("items length = %d, want 1", len(items))
	}
	if items[0].Purpose != collectMaterialRoleVerify {
		t.Fatalf("purpose = %q, want %q", items[0].Purpose, collectMaterialRoleVerify)
	}
	if items[0].EventMediaId != 73 {
		t.Fatalf("event media id = %d, want 73", items[0].EventMediaId)
	}
}

func TestCollectMediaRowsToItemsPreservesCacheAndSimilarityMetadata(t *testing.T) {
	row := &collectEventMediaRow{
		YoubanPublishCollectEventMedia: &entity.YoubanPublishCollectEventMedia{
			Id: 74, MediaType: "photo", SourceFileId: "photo-id", FileUrl: "https://cdn.test/photo.jpg",
			StoragePath: "attachment/photo.jpg", PosterUrl: "https://cdn.test/poster.jpg", MetaJson: `{"width":1080}`,
		},
		SourceKind: "photo", SourceMediaId: 101, SourceAccessHash: 102, SourceFileReference: []byte{1, 2, 3},
		SourceThumbSize: "x", SourceMimeType: "image/jpeg", SourceDCId: 4, SourceSize: 2048,
		FileMd5: "media-md5", FilePhash: "abcdef0123456789",
	}
	items := collectMediaRowsToItems([]*collectEventMediaRow{row}, collectMaterialRoleDisplay)
	if len(items) != 1 {
		t.Fatalf("items=%d", len(items))
	}
	item := items[0]
	if item.FileMd5 != row.FileMd5 || item.FilePhash != row.FilePhash || item.StoragePath != row.StoragePath || item.FileUrl != row.FileUrl {
		t.Fatalf("cache/similarity metadata lost: %#v", item)
	}
	if item.SourceMediaId != row.SourceMediaId || item.SourceAccessHash != row.SourceAccessHash || item.SourceMimeType != row.SourceMimeType || item.SourceSize != row.SourceSize {
		t.Fatalf("telegram metadata lost: %#v", item)
	}
}
