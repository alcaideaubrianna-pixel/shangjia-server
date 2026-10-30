package sys

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// telegramProcessedAssetVariantHash identifies the exact protection variant
// that produced a processed upload. It intentionally includes every input
// that can change anti-scan or background replacement output.
func telegramProcessedAssetVariantHash(media *telegramMediaItem) string {
	if media == nil {
		return ""
	}
	// v2 invalidates hashes written by the old reuse path, which could stamp
	// an existing Telegram file_id as if it had been regenerated with the
	// current background replacement settings.
	// v4 invalidates historical processed file IDs written before background
	// replacement failures were isolated from reusable assets.
	payload := fmt.Sprintf("v4|%d|%s|%t|%d|%s|%s|%s|%s|%s", media.Id, media.MediaType, media.AntiScanEnabled, media.AntiScanSeed, media.AntiScanMode, media.AntiScanBackgroundURL, media.AntiScanProvider, media.AssetHash, media.StoragePath)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
