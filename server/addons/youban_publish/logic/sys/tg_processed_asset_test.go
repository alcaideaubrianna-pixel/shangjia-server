package sys

import "testing"

func TestTelegramProcessedAssetVariantHashChangesWithProtectionInputs(t *testing.T) {
	base := &telegramMediaItem{Id: 701711, MediaType: "image", AntiScanEnabled: true, AntiScanMode: "background_replace", AntiScanBackgroundURL: "https://img.example/bg.jpg"}
	hash := telegramProcessedAssetVariantHash(base)
	if len(hash) != 64 {
		t.Fatalf("unexpected hash length: %d", len(hash))
	}
	changed := *base
	changed.AntiScanBackgroundURL = "https://img.example/bg-2.jpg"
	if got := telegramProcessedAssetVariantHash(&changed); got == hash {
		t.Fatal("protection variant hash did not change when background changed")
	}
}
