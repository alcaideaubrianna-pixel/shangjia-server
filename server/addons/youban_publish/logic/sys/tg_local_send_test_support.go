package sys

import "context"

// PrepareLocalBotAPITestMedia exposes the production media normalization path
// to the isolated Local Bot API diagnostic command.
func PrepareLocalBotAPITestMedia(ctx context.Context, path, mediaType string) (string, func(), error) {
	return NewSysPublish().prepareTelegramMediaUploadFile(ctx, &telegramMediaItem{MediaType: mediaType}, path, nil)
}
