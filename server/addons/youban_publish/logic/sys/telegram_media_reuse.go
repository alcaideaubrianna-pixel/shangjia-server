package sys

import (
	"context"
	"strings"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
)

// telegramMediaReuseInput contains the stable Telegram identity and the
// downloaded-file fingerprint available before a download starts.
type telegramMediaReuseInput struct {
	MediaType     string
	FileUniqueID  string
	SourceMediaID int64
	SourceDCID    int
	Variant       string
	FileMD5       string
	SourceSize    int64
	SourceMime    string
}

// resolveTelegramMediaReuse applies the same identity-first lookup for
// account collection, bot collection and TG material import. A miss is not an
// error: callers may continue with the normal Telegram download path.
func resolveTelegramMediaReuse(ctx context.Context, in telegramMediaReuseInput) (*collectorin.MediaCacheEntry, bool, error) {
	identityAvailable := strings.TrimSpace(in.FileUniqueID) != "" || in.SourceMediaID > 0
	if identityAvailable {
		cached, ok, err := lookupTelegramCollectorMediaIdentityCache(ctx, in.MediaType, in.FileUniqueID, in.SourceMediaID, in.SourceDCID, in.Variant)
		if err != nil || ok {
			return cached, ok, err
		}
	}
	if strings.TrimSpace(in.FileMD5) == "" || in.SourceSize <= 0 {
		return nil, false, nil
	}
	return lookupTelegramCollectorMediaCache(ctx, in.MediaType, in.FileMD5, in.SourceSize, in.SourceMime)
}
