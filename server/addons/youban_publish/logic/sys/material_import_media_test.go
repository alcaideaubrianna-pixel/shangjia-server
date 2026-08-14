package sys

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestMaterialImportFileReferenceExpired(t *testing.T) {
	if !materialImportFileReferenceExpired("下载失败: rpc error FILE_REFERENCE_EXPIRED") {
		t.Fatal("expected FILE_REFERENCE_EXPIRED to be detected")
	}
	if materialImportFileReferenceExpired("下载失败: DC is closed") {
		t.Fatal("did not expect unrelated download error to be detected")
	}
}

func TestMaterialImportMediaItemReadyRejectsMissingLocalFile(t *testing.T) {
	item := collectMediaItem{StoragePath: "storage/cache/youban_publish/media/missing-file.jpg"}
	if materialImportMediaItemReady(item) {
		t.Fatal("expected missing local cache file to require redownload")
	}
}

func TestMaterialImportMediaItemReadyAcceptsCloudURL(t *testing.T) {
	item := collectMediaItem{FileUrl: "https://img.example.com/a.jpg", StoragePath: "missing.jpg"}
	if !materialImportMediaItemReady(item) {
		t.Fatal("expected cloud URL to be reusable")
	}
}

func TestMaterialImportUploadFileFallsBackToCloudURL(t *testing.T) {
	const expected = "cloud-media-content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, expected)
	}))
	defer server.Close()

	upload, err := materialImportUploadFileFromPath(context.Background(), "hotgo/file/missing.jpg", server.URL+"/media.jpg", "media.jpg")
	if err != nil {
		t.Fatalf("fallback upload file: %v", err)
	}
	reader, err := upload.Open()
	if err != nil {
		t.Fatalf("open upload file: %v", err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read upload file: %v", err)
	}
	if string(content) != expected {
		t.Fatalf("upload content = %q, want %q", content, expected)
	}
}
