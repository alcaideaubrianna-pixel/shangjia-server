package sys

import "testing"

func TestMaterialImportMissingMediaItems(t *testing.T) {
	items := []collectMediaItem{
		{Type: "photo", Purpose: "display", FileId: "display-1"},
		{Type: "photo", Purpose: "display", FileId: "display-2"},
		{Type: "video", Purpose: "display", FileId: "display-3"},
		{Type: "video", Purpose: "verify", FileId: "verify-1"},
	}

	selected, indexes := materialImportMissingMediaItems(items, materialImportProfileMediaCounts{Display: 3})
	if len(selected) != 1 || len(indexes) != 1 {
		t.Fatalf("selected=%d indexes=%d, want one missing verify media", len(selected), len(indexes))
	}
	if selected[0].Purpose != "verify" || indexes[0] != 3 {
		t.Fatalf("selected=%+v indexes=%v, want verify item at index 3", selected, indexes)
	}
}

func TestMaterialImportMissingMediaItemsDoesNotDuplicateCompleteProfile(t *testing.T) {
	items := []collectMediaItem{
		{Type: "photo", Purpose: "display", FileId: "display-1"},
		{Type: "video", Purpose: "verify", FileId: "verify-1"},
	}

	selected, indexes := materialImportMissingMediaItems(items, materialImportProfileMediaCounts{Display: 1, Verify: 1})
	if len(selected) != 0 || len(indexes) != 0 {
		t.Fatalf("selected=%+v indexes=%v, want no missing media", selected, indexes)
	}
}

func TestMaterialImportMissingMediaItemsDefaultsPurposeToDisplay(t *testing.T) {
	items := []collectMediaItem{
		{Type: "photo", FileId: "display-1"},
		{Type: "photo", FileId: "display-2"},
	}

	selected, indexes := materialImportMissingMediaItems(items, materialImportProfileMediaCounts{Display: 1})
	if len(selected) != 1 || indexes[0] != 1 {
		t.Fatalf("selected=%+v indexes=%v, want second display media", selected, indexes)
	}
}
