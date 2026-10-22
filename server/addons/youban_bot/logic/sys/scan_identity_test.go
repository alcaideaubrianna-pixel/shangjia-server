package sys

import (
	"context"
	"testing"

	"github.com/go-telegram/bot/models"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
)

type scanKnownIdentityService struct {
	publishService.ISysPublish
	t     *testing.T
	calls int
}

func (s *scanKnownIdentityService) BotMediaFingerprintCached(context.Context, string) bool {
	return false
}
func (s *scanKnownIdentityService) BotMediaSearchKnownItems(_ context.Context, in *publishsysin.BotMediaSearchInp) ([]bool, error) {
	s.calls++
	if in.TenantId != 5 || in.AccountId != 6 || len(in.Items) != 2 {
		s.t.Fatalf("scope missing: %+v", in)
	}
	return []bool{true, true}, nil
}

func TestScanLedgerHitBeforeGetFileAndVideoThumbnail(t *testing.T) {
	var previous publishService.ISysPublish
	func() { defer func() { _ = recover() }(); previous = publishService.SysPublish() }()
	stub := &scanKnownIdentityService{ISysPublish: previous, t: t}
	publishService.RegisterSysPublish(stub)
	t.Cleanup(func() { publishService.RegisterSysPublish(previous) })
	batch := make([]*publishsysin.BotMediaSearchItem, 0, 2)
	for _, msg := range []*models.Message{
		{Photo: []models.PhotoSize{{FileID: "small", FileUniqueID: "small-id", FileSize: 10}, {FileID: "large", FileUniqueID: "large-id", FileSize: 100}}},
		{Video: &models.Video{FileID: "video", FileUniqueID: "video-id"}},
	} {
		// No Bot configuration or token exists. Reaching getFile fails the test.
		items, err := NewSysBot().resolveScanSearchItems(context.Background(), 12, &botProfileAccount{TenantId: 5, AccountId: 6}, msg)
		if err != nil || len(items) != 1 || items[0].FileUrl != "" || items[0].FileId == "" {
			t.Fatalf("identity shortcut failed: %+v %v", items, err)
		}
		if msg.Video == nil && len(items[0].FileUniqueIds) != 2 {
			t.Fatal("photo aliases lost")
		}
		batch = append(batch, items...)
	}
	if stub.calls != 0 {
		t.Fatal("intake performed per-image queries")
	}
	resolved, err := NewSysBot().resolveScanBatch(context.Background(), 12, &botProfileAccount{TenantId: 5, AccountId: 6}, batch)
	if err != nil || len(resolved) != 2 {
		t.Fatalf("batch=%v err=%v", resolved, err)
	}
	if stub.calls != 1 {
		t.Fatalf("preflight calls=%d", stub.calls)
	}
}
