package sys

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"path/filepath"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"

	botconsts "hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/hgrds/lock"
	"hotgo/internal/library/storager"
	isc "hotgo/internal/service"
	fileutil "hotgo/utility/file"
)

const (
	customEmojiCacheTTL     = 30 * 24 * time.Hour
	customEmojiMissCacheTTL = 10 * time.Minute
)

var customEmojiFetchLocker = lock.NewConfig(2*time.Minute, 100*time.Millisecond)

type customEmojiRow struct {
	CustomEmojiId string `orm:"custom_emoji_id"`
	FileUniqueId  string `orm:"file_unique_id"`
	AttachmentId  int64  `orm:"attachment_id"`
	StoragePath   string `orm:"storage_path"`
	FileUrl       string `orm:"file_url"`
	FileFormat    string `orm:"file_format"`
	RenderType    string `orm:"render_type"`
	FallbackEmoji string `orm:"fallback_emoji"`
	Width         int    `orm:"width"`
	Height        int    `orm:"height"`
	Status        int    `orm:"status"`
}

func (s *sSysBot) ResolveCustomEmojis(ctx context.Context, in *sysin.CustomEmojiResolveInp) ([]*sysin.CustomEmojiModel, error) {
	if err := in.Filter(ctx); err != nil {
		return nil, err
	}
	resolved, missing, err := s.cachedCustomEmojis(ctx, in.EmojiIds)
	if err != nil {
		return nil, err
	}
	if len(missing) > 0 {
		if err = s.resolveMissingCustomEmojis(ctx, missing); err != nil {
			return nil, err
		}
		refreshed, _, refreshErr := s.cachedCustomEmojis(ctx, missing)
		if refreshErr != nil {
			return nil, refreshErr
		}
		for emojiId, item := range refreshed {
			resolved[emojiId] = item
		}
	}
	list := make([]*sysin.CustomEmojiModel, 0, len(resolved))
	for _, emojiId := range in.EmojiIds {
		if item := resolved[emojiId]; item != nil {
			list = append(list, item)
		}
	}
	return list, nil
}

func (s *sSysBot) cachedCustomEmojis(ctx context.Context, emojiIds []string) (map[string]*sysin.CustomEmojiModel, []string, error) {
	resolved := make(map[string]*sysin.CustomEmojiModel, len(emojiIds))
	dbIds := make([]string, 0, len(emojiIds))
	for _, emojiId := range emojiIds {
		value, cacheErr := cache.Instance().Get(ctx, customEmojiCacheKey(emojiId))
		if cacheErr == nil && !value.IsNil() {
			var item sysin.CustomEmojiModel
			if scanErr := value.Scan(&item); scanErr == nil && item.EmojiId != "" && item.FileUrl != "" {
				resolved[emojiId] = &item
				continue
			}
		}
		missValue, missErr := cache.Instance().Get(ctx, customEmojiMissCacheKey(emojiId))
		if missErr == nil && !missValue.IsNil() {
			continue
		}
		dbIds = append(dbIds, emojiId)
	}
	if len(dbIds) == 0 {
		return resolved, nil, nil
	}
	var rows []*customEmojiRow
	if err := g.DB().Model(customEmojiTable).Safe().Ctx(ctx).
		WhereIn("custom_emoji_id", dbIds).Where("status", 1).Scan(&rows); err != nil {
		return nil, nil, gerror.Wrap(err, "读取Telegram自定义Emoji缓存失败")
	}
	for _, row := range rows {
		if row == nil || row.CustomEmojiId == "" || row.FileUrl == "" {
			continue
		}
		item := customEmojiModel(row)
		resolved[row.CustomEmojiId] = item
		_ = cache.Instance().Set(ctx, customEmojiCacheKey(row.CustomEmojiId), item, customEmojiCacheTTL)
	}
	missing := make([]string, 0, len(dbIds))
	for _, emojiId := range dbIds {
		if resolved[emojiId] == nil {
			missing = append(missing, emojiId)
		}
	}
	return resolved, missing, nil
}

func (s *sSysBot) resolveMissingCustomEmojis(ctx context.Context, emojiIds []string) error {
	mutex := customEmojiFetchLocker.Mutex(botconsts.CustomEmojiFetchLockKey)
	if err := mutex.Lock(ctx); err != nil {
		return gerror.Wrap(err, "等待Telegram自定义Emoji缓存锁失败")
	}
	defer func() { _ = mutex.Unlock(context.Background()) }()

	_, missing, err := s.cachedCustomEmojis(ctx, emojiIds)
	if err != nil || len(missing) == 0 {
		return err
	}
	token, err := s.OfficialBotToken(ctx)
	if err != nil {
		return err
	}
	bot, err := s.telegramBot(ctx, token)
	if err != nil {
		return err
	}
	callCtx, cancel := telegramAPICtx()
	stickers, err := bot.GetCustomEmojiStickers(callCtx, &tgbot.GetCustomEmojiStickersParams{CustomEmojiIDs: missing})
	cancel()
	if err != nil {
		return gerror.Wrap(err, "读取Telegram自定义Emoji失败")
	}
	returned := make(map[string]struct{}, len(stickers))
	for _, sticker := range stickers {
		if sticker == nil || strings.TrimSpace(sticker.CustomEmojiID) == "" {
			continue
		}
		emojiId := strings.TrimSpace(sticker.CustomEmojiID)
		returned[emojiId] = struct{}{}
		item, persistErr := s.persistCustomEmoji(ctx, bot, token, sticker)
		if persistErr != nil {
			g.Log().Warning(ctx, "缓存Telegram自定义Emoji失败", g.Map{"emojiId": emojiId, "err": persistErr})
			_ = cache.Instance().Set(ctx, customEmojiMissCacheKey(emojiId), 1, customEmojiMissCacheTTL)
			continue
		}
		_ = cache.Instance().Set(ctx, customEmojiCacheKey(emojiId), item, customEmojiCacheTTL)
		_, _ = cache.Instance().Remove(ctx, customEmojiMissCacheKey(emojiId))
	}
	for _, emojiId := range missing {
		if _, ok := returned[emojiId]; !ok {
			_ = cache.Instance().Set(ctx, customEmojiMissCacheKey(emojiId), 1, customEmojiMissCacheTTL)
		}
	}
	return nil
}

func (s *sSysBot) persistCustomEmoji(ctx context.Context, bot *tgbot.Bot, token string, sticker *models.Sticker) (*sysin.CustomEmojiModel, error) {
	callCtx, cancel := telegramAPICtx()
	file, err := bot.GetFile(callCtx, &tgbot.GetFileParams{FileID: sticker.FileID})
	cancel()
	if err != nil {
		return nil, err
	}
	if file == nil || strings.TrimSpace(file.FilePath) == "" {
		return nil, gerror.New("Telegram自定义Emoji文件地址为空")
	}
	data, err := downloadTelegramAsset(ctx, telegramFileURL(token, file.FilePath))
	if err != nil {
		return nil, err
	}
	fileFormat, renderType, uploadType := customEmojiFormat(sticker, file.FilePath)
	storedFormat := fileFormat
	if renderType == "lottie" {
		data, err = decompressTelegramTGS(data)
		if err != nil {
			return nil, err
		}
		storedFormat = "json"
	}
	filename := "telegram-custom-emoji-" + strings.TrimSpace(sticker.CustomEmojiID) + "." + storedFormat
	header, err := fileutil.NewMultipartFileHeader(filename, data)
	if err != nil {
		return nil, err
	}
	attachment, err := isc.CommonUpload().UploadFile(ctx, uploadType, &ghttp.UploadFile{FileHeader: header})
	if err != nil {
		return nil, err
	}
	if attachment == nil || strings.TrimSpace(attachment.FileUrl) == "" {
		return nil, gerror.New("系统存储未返回Emoji文件地址")
	}
	now := gtime.Now()
	row := &customEmojiRow{
		CustomEmojiId: strings.TrimSpace(sticker.CustomEmojiID),
		FileUniqueId:  strings.TrimSpace(sticker.FileUniqueID),
		AttachmentId:  attachment.Id,
		StoragePath:   strings.TrimSpace(attachment.Path),
		FileUrl:       strings.TrimSpace(attachment.FileUrl),
		FileFormat:    fileFormat,
		RenderType:    renderType,
		FallbackEmoji: strings.TrimSpace(sticker.Emoji),
		Width:         sticker.Width,
		Height:        sticker.Height,
		Status:        1,
	}
	_, err = g.DB().Model(customEmojiTable).Safe().Ctx(ctx).Data(g.Map{
		"custom_emoji_id": row.CustomEmojiId,
		"file_unique_id":  row.FileUniqueId,
		"attachment_id":   row.AttachmentId,
		"storage_path":    row.StoragePath,
		"file_url":        row.FileUrl,
		"file_format":     row.FileFormat,
		"render_type":     row.RenderType,
		"fallback_emoji":  row.FallbackEmoji,
		"width":           row.Width,
		"height":          row.Height,
		"status":          row.Status,
		"created_at":      now,
		"updated_at":      now,
	}).OnConflict("custom_emoji_id").OnDuplicateEx("id", "custom_emoji_id", "created_at").Save()
	if err != nil {
		return nil, gerror.Wrap(err, "保存Telegram自定义Emoji缓存失败")
	}
	return customEmojiModel(row), nil
}

func customEmojiFormat(sticker *models.Sticker, filePath string) (string, string, string) {
	if sticker.IsAnimated {
		return "tgs", "lottie", storager.KindOther
	}
	if sticker.IsVideo {
		return "webm", "video", storager.KindVideo
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".")
	if ext == "" {
		ext = "webp"
	}
	return ext, "image", storager.KindImg
}

func decompressTelegramTGS(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, gerror.Wrap(err, "解压Telegram动画Emoji失败")
	}
	defer reader.Close()
	result, err := io.ReadAll(reader)
	if err != nil {
		return nil, gerror.Wrap(err, "读取Telegram动画Emoji失败")
	}
	if len(result) == 0 {
		return nil, gerror.New("Telegram动画Emoji内容为空")
	}
	return result, nil
}

func customEmojiModel(row *customEmojiRow) *sysin.CustomEmojiModel {
	return &sysin.CustomEmojiModel{
		EmojiId:       row.CustomEmojiId,
		FileUrl:       row.FileUrl,
		RenderType:    row.RenderType,
		FileFormat:    row.FileFormat,
		FallbackEmoji: row.FallbackEmoji,
		Width:         row.Width,
		Height:        row.Height,
	}
}

func customEmojiCacheKey(emojiId string) string {
	return botconsts.CustomEmojiCachePrefix + emojiId
}

func customEmojiMissCacheKey(emojiId string) string {
	return botconsts.CustomEmojiMissCachePrefix + emojiId
}
