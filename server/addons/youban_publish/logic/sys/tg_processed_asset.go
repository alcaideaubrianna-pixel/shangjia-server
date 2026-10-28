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
	payload := fmt.Sprintf("v1|%d|%s|%t|%d|%s|%s", media.Id, media.MediaType, media.AntiScanEnabled, media.AntiScanSeed, media.AntiScanMode, media.AntiScanBackgroundURL)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
