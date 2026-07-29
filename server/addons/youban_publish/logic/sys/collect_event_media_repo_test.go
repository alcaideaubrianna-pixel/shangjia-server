package sys

import (
	"testing"

	"hotgo/addons/youban_publish/internal/model/entity"
)

func TestCollectMediaRowsToItemsPreservesPurpose(t *testing.T) {
	items := collectMediaRowsToItems([]*entity.YoubanPublishCollectEventMedia{{
		MediaType:    "video",
		SourceFileId: "gotd:-100:42",
		MetaJson:     `{"kind":"document"}`,
	}}, collectMaterialRoleVerify)
	if len(items) != 1 {
		t.Fatalf("items length = %d, want 1", len(items))
	}
	if items[0].Purpose != collectMaterialRoleVerify {
		t.Fatalf("purpose = %q, want %q", items[0].Purpose, collectMaterialRoleVerify)
	}
}
