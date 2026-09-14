package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestSanitizeProfileMediaOriginals(t *testing.T) {
	newMedia := func() []*sysin.MediaModel {
		return []*sysin.MediaModel{{
			OriginalAttachmentId: 12,
			OriginalFileUrl:      "https://example.com/original.jpg",
			OriginalStoragePath:  "original.jpg",
		}}
	}

	editable := newMedia()
	sanitizeProfileMediaOriginals(editable, true)
	if editable[0].OriginalAttachmentId == 0 || editable[0].OriginalFileUrl == "" || editable[0].OriginalStoragePath == "" {
		t.Fatal("editable profile should retain original media fields")
	}

	readonly := newMedia()
	sanitizeProfileMediaOriginals(readonly, false)
	if readonly[0].OriginalAttachmentId != 0 || readonly[0].OriginalFileUrl != "" || readonly[0].OriginalStoragePath != "" {
		t.Fatal("readonly profile should hide original media fields")
	}
}

func TestNormalizeMediaListFileURLPreservesExplicitRawStatus(t *testing.T) {
	media := []*sysin.MediaModel{{
		Name:       "legacy-edited.jpg",
		EditStatus: "raw",
	}}

	normalizeMediaListFileURL(media)

	if media[0].EditStatus != "raw" {
		t.Fatalf("expected explicit raw status, got %q", media[0].EditStatus)
	}
}
