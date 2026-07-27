package sys

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/library/storager"
)

const (
	feiniuPublishTgMessageTable      = "hg_youban_publish_tg_message"
	feiniuPublishTgMessageCacheTable = "hg_youban_publish_tg_message_cache"
)

func (s *sSysSync) syncSourceMessageLinks(ctx context.Context, cfg gdb.Record, row gdb.Record, profileId, accountId int64) error {
	if profileId <= 0 || accountId <= 0 {
		return nil
	}
	chatId := row["source_tg_chat_id"].Int64()
	messageId := row["source_message_id"].Int64()
	if chatId <= 0 || messageId <= 0 {
		return nil
	}
	if err := s.upsertFeiNiuTgMessageCache(ctx, cfg, row); err != nil {
		return err
	}
	if err := s.upsertFeiNiuTgMessageRelation(ctx, cfg, row, profileId, accountId); err != nil {
		return err
	}
	return nil
}

func (s *sSysSync) upsertFeiNiuTgMessageCache(ctx context.Context, cfg gdb.Record, row gdb.Record) error {
	chatId := row["source_tg_chat_id"].Int64()
	messageId := row["source_message_id"].Int64()
	if chatId <= 0 || messageId <= 0 {
		return nil
	}
	messageText := strings.TrimSpace(row["plain_text"].String())
	if messageText == "" {
		messageText = strings.TrimSpace(row["title"].String())
	}
	if messageText == "" {
		messageText = strings.TrimSpace(row["note_code"].String())
	}
	mediaType := feiNiuSyncMessageMediaType(row)
	if mediaType == "" {
		return nil
	}
	now := gtime.Now()
	targetChatId := normalizeTelegramChannelChatIDLocal(strconv.FormatInt(chatId, 10))
	data := g.Map{
		"tenant_id":      cfg["target_tenant_id"].Int64(),
		"tg_account_id":  cfg["tg_account_id"].Int64(),
		"channel_id":     row["source_channel_id"].Int64(),
		"target_chat_id": targetChatId,
		"tg_message_id":  messageId,
		"message_text":   messageText,
		"media_type":     mediaType,
		"message_date":   feiNiuSyncMessageDate(row),
		"media_group_id": feiNiuSyncMediaGroupID(row),
		"updated_at":     now,
	}
	mod := g.DB().Model(feiniuPublishTgMessageCacheTable).Safe().Ctx(ctx).
		Where("tenant_id", data["tenant_id"]).
		Where("channel_id", data["channel_id"]).
		Where("tg_message_id", messageId)
	existing, err := mod.One()
	if err != nil {
		return gerror.Wrap(err, "读取 FeiNiu TG 消息缓存失败")
	}
	if existing.IsEmpty() {
		data["created_at"] = now
		if _, err = g.DB().Model(feiniuPublishTgMessageCacheTable).Safe().Ctx(ctx).Data(data).Insert(); err != nil {
			return gerror.Wrap(err, "写入 FeiNiu TG 消息缓存失败")
		}
		return nil
	}
	_, err = g.DB().Model(feiniuPublishTgMessageCacheTable).Safe().Ctx(ctx).
		Where("id", existing["id"].Int64()).
		Data(data).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新 FeiNiu TG 消息缓存失败")
	}
	return nil
}

func (s *sSysSync) upsertFeiNiuTgMessageRelation(ctx context.Context, cfg gdb.Record, row gdb.Record, profileId, accountId int64) error {
	chatId := row["source_tg_chat_id"].Int64()
	messageId := row["source_message_id"].Int64()
	if chatId <= 0 || messageId <= 0 {
		return nil
	}
	now := gtime.Now()
	targetChatId := normalizeTelegramChannelChatIDLocal(strconv.FormatInt(chatId, 10))
	data := g.Map{
		"job_id":         0,
		"tenant_id":      cfg["target_tenant_id"].Int64(),
		"account_id":     accountId,
		"profile_id":     profileId,
		"bot_id":         0,
		"target_chat_id": targetChatId,
		"tg_message_id":  messageId,
		"media_group_id": feiNiuSyncMediaGroupID(row),
		"media_id":       0,
		"purpose":        "import",
		"tg_file_id":     "",
		"status":         "sent",
		"sent_at":        feiNiuSyncMessageDate(row),
		"updated_at":     now,
	}
	mod := g.DB().Model(feiniuPublishTgMessageTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).
		Where("target_chat_id", targetChatId).
		Where("tg_message_id", messageId).
		Where("purpose", "import")
	existing, err := mod.One()
	if err != nil {
		return gerror.Wrap(err, "读取 FeiNiu 资料消息关联失败")
	}
	if existing.IsEmpty() {
		data["created_at"] = now
		if _, err = g.DB().Model(feiniuPublishTgMessageTable).Safe().Ctx(ctx).Data(data).Insert(); err != nil {
			return gerror.Wrap(err, "写入 FeiNiu 资料消息关联失败")
		}
		return nil
	}
	_, err = g.DB().Model(feiniuPublishTgMessageTable).Safe().Ctx(ctx).
		Where("id", existing["id"].Int64()).
		Data(data).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新 FeiNiu 资料消息关联失败")
	}
	return nil
}

func feiNiuSyncMessageMediaType(row gdb.Record) string {
	if row["video_count"].Int() > 0 {
		return "video"
	}
	if row["image_count"].Int() > 0 {
		return "photo"
	}
	if strings.TrimSpace(row["plain_text"].String()) != "" || strings.TrimSpace(row["title"].String()) != "" {
		return "text"
	}
	return ""
}

func feiNiuSyncMessageDate(row gdb.Record) *gtime.Time {
	for _, field := range []string{"create_time", "edited_at", "update_time"} {
		if t := row[field].GTime(); t != nil {
			return t
		}
	}
	return nil
}

func feiNiuSyncMediaGroupID(row gdb.Record) string {
	for _, field := range []string{"source_grouped_id", "source_key"} {
		if v := strings.TrimSpace(row[field].String()); v != "" {
			return v
		}
	}
	return ""
}

func feiNiuImportedMediaStoragePath(row gdb.Record) string {
	for _, field := range []string{"cos_path", "origin_uri", "preview_uri", "local_file_path"} {
		if path := normalizeTelegramContentStoragePathLocal(row[field].String()); path != "" {
			return strings.TrimLeft(path, "/")
		}
	}
	if path := strings.TrimSpace(row["cos_path"].String()); path != "" {
		return strings.TrimLeft(path, "/")
	}
	return ""
}

func feiNiuImportedMediaURL(row gdb.Record) string {
	storagePath := feiNiuImportedMediaStoragePath(row)
	if storagePath == "" {
		return ""
	}
	cdnBase := feiniuMediaContentCDNBaseURL()
	if cdnBase != "" {
		return cdnBase + "/" + strings.TrimLeft(storagePath, "/")
	}
	return "/" + strings.TrimLeft(storagePath, "/")
}

func normalizeTelegramChannelChatIDLocal(chatID string) string {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" || strings.HasPrefix(chatID, "-") || strings.HasPrefix(chatID, "@") {
		return chatID
	}
	if len(chatID) < 10 {
		return chatID
	}
	for _, char := range chatID {
		if !unicode.IsDigit(char) {
			return chatID
		}
	}
	return "-100" + chatID
}

func normalizeTelegramContentStoragePathLocal(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "telegram/content/") || strings.HasPrefix(raw, "/telegram/content/") {
		return strings.TrimLeft(raw, "/")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.Path == "" {
		return ""
	}
	path := strings.TrimLeft(parsed.Path, "/")
	if idx := strings.Index(path, "telegram/content/"); idx >= 0 {
		return path[idx:]
	}
	return ""
}

func feiniuMediaContentCDNBaseURL() string {
	cdnBase := strings.TrimRight(g.Cfg().MustGet(context.Background(), "content.cdnBaseUrl", "").String(), "/")
	if cdnBase != "" {
		return cdnBase
	}
	uploadConfig := storager.GetConfig()
	if uploadConfig == nil {
		return ""
	}
	return strings.TrimRight(uploadConfig.CosPublicURL, "/")
}
