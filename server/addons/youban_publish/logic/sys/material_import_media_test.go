package sys

import (
	"context"
	"errors"
	"testing"
	"time"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestDispatchMaterialImportGroupTimesOutWithoutWorker(t *testing.T) {
	jobs := make(chan *sysin.MaterialImportGroupModel)
	errCh := make(chan error)
	started := time.Now()
	err := dispatchMaterialImportGroup(context.Background(), jobs, errCh, &sysin.MaterialImportGroupModel{}, 10*time.Millisecond)
	if collectMediaRetryErrorFrom(err) == nil {
		t.Fatalf("expected retry error, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("dispatch timeout took too long: %s", elapsed)
	}
}

func TestDispatchMaterialImportGroupDeliversToWorker(t *testing.T) {
	jobs := make(chan *sysin.MaterialImportGroupModel)
	errCh := make(chan error)
	want := &sysin.MaterialImportGroupModel{}
	received := make(chan *sysin.MaterialImportGroupModel, 1)
	go func() { received <- <-jobs }()

	if err := dispatchMaterialImportGroup(context.Background(), jobs, errCh, want, time.Second); err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if got := <-received; got != want {
		t.Fatal("worker received a different group")
	}
}

func TestDispatchMaterialImportGroupReturnsWorkerError(t *testing.T) {
	jobs := make(chan *sysin.MaterialImportGroupModel)
	want := errors.New("worker stopped")
	errCh := make(chan error, 1)
	errCh <- want
	if err := dispatchMaterialImportGroup(context.Background(), jobs, errCh, &sysin.MaterialImportGroupModel{}, time.Second); !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func TestMaterialImportConcurrencyIsBounded(t *testing.T) {
	if materialImportGroupConcurrency < 1 || materialImportGroupConcurrency > 4 {
		t.Fatalf("unsafe material import concurrency: %d", materialImportGroupConcurrency)
	}
}

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
