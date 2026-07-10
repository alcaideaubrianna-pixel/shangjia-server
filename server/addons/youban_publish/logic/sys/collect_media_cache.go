package sys

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/entity"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/storager"
	"hotgo/internal/model"
	"hotgo/internal/service"
	"hotgo/utility/file"
)

type collectMediaRetryError struct {
	message string
	delay   time.Duration
}

func (e *collectMediaRetryError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func newCollectMediaRetryError(message string, delay time.Duration) *collectMediaRetryError {
	if delay <= 0 {
		delay = 15 * time.Second
	}
	return &collectMediaRetryError{message: message, delay: delay}
}

func (s *sSysPublish) prepareCollectMediaAsset(ctx context.Context, event gdb.Record, item collectMediaItem) (collectMediaItem, error) {
	if !strings.HasPrefix(strings.TrimSpace(item.FileId), "gotd:") {
		return item, nil
	}
	if strings.TrimSpace(item.StoragePath) != "" || strings.TrimSpace(item.FileUrl) != "" {
		item.FileId = ""
		return item, nil
	}
	return item, gerror.New("账号采集媒体尚未缓存")
}

func (s *sSysPublish) ExecuteCollectMediaCache(ctx context.Context, payload collectMediaQueuePayload) error {
	ctx = collectMediaRuntimeContext(ctx, payload.AccountId)
	lock := s.collectMediaAccountLock(payload.TenantId, payload.TgAccountId)
	lock.Lock()
	defer lock.Unlock()
	s.waitCollectMediaAccountInterval(ctx, payload.TenantId, payload.TgAccountId)
	event, err := s.collectMediaCacheEvent(ctx, payload)
	if err != nil {
		return err
	}
	s.appendCollectEventLogForRecord(ctx, event, "media", "running", "媒体缓存任务开始执行", "")
	if delay := s.collectGroupedMediaCacheDelay(ctx, event); delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	changed, err := s.cacheCollectEventStructuredMedia(ctx, event)
	if err != nil {
		if retryErr, ok := err.(*collectMediaRetryError); ok {
			_ = s.markCollectEvent(ctx, payload.EventId, sysin.CollectEventStatusMediaPending, retryErr.message)
			s.appendCollectEventLogForRecord(ctx, event, "media", "pending", retryErr.message, fmt.Sprintf("retryAfter=%s", retryErr.delay))
			return s.enqueueCollectMediaCache(ctx, payload, retryErr.delay)
		}
		_ = s.markCollectEvent(ctx, payload.EventId, sysin.CollectEventStatusFailed, err.Error())
		s.appendCollectEventLogForRecord(ctx, event, "media", "failed", err.Error(), "")
		return err
	}
	if changed {
		if err = s.syncCollectEventMediaSnapshot(ctx, payload.EventId); err != nil {
			return err
		}
	}
	s.appendCollectEventLogForRecord(ctx, event, "media", "ready", "媒体缓存任务处理完成", "")
	return s.processCollectEvent(ctx, payload.EventId, payload.TenantId, payload.AccountId)
}

func collectMediaRuntimeContext(ctx context.Context, accountId int64) context.Context {
	current := contexts.Get(ctx)
	if current == nil {
		return context.WithValue(ctx, consts.ContextHTTPKey, &model.Context{
			Module:    consts.AppApi,
			AddonName: "youban_publish",
			User: &model.Identity{
				Id:  accountId,
				App: consts.AppApi,
			},
			Data: g.Map{},
		})
	}
	if current.Module == "" {
		current.Module = consts.AppApi
	}
	if current.AddonName == "" {
		current.AddonName = "youban_publish"
	}
	if current.User == nil || current.User.Id <= 0 {
		current.User = &model.Identity{Id: accountId, App: consts.AppApi}
	}
	if current.User.App == "" {
		current.User.App = consts.AppApi
	}
	return ctx
}

func (s *sSysPublish) collectMediaCacheEvent(ctx context.Context, payload collectMediaQueuePayload) (gdb.Record, error) {
	row, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("id", payload.EventId).
		Where("tenant_id", payload.TenantId).
		Where("account_id", payload.AccountId).
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集媒体缓存事件失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("采集媒体缓存事件不存在")
	}
	if collectEventAlreadyMatched(row["status"].String()) {
		return row, nil
	}
	return row, nil
}

func (s *sSysPublish) collectGroupedMediaCacheDelay(ctx context.Context, event gdb.Record) time.Duration {
	if event.IsEmpty() || strings.TrimSpace(event["source_grouped_id"].String()) == "" {
		return 0
	}
	updatedAt := s.collectGroupedMediaLastIngestAt(ctx, event)
	if updatedAt == nil {
		return 0
	}
	elapsed := collectLocalElapsedSince(updatedAt)
	if elapsed < 0 {
		return 0
	}
	if elapsed >= collectGroupedEventDelay {
		return 0
	}
	return collectGroupedEventDelay - elapsed + 500*time.Millisecond
}

func (s *sSysPublish) collectGroupedMediaLastIngestAt(ctx context.Context, event gdb.Record) *gtime.Time {
	mediaCols := pdao.YoubanPublishCollectEventMedia.Columns()
	row, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Fields("MAX("+mediaCols.UpdatedAt+") AS updated_at").
		Where(mediaCols.EventId, event["id"].Int64()).
		One()
	if err == nil && !row.IsEmpty() {
		if value := row["updated_at"].GTime(); value != nil {
			return value
		}
	}
	if value := event["created_at"].GTime(); value != nil {
		return value
	}
	return event["updated_at"].GTime()
}

func collectLocalElapsedSince(value *gtime.Time) time.Duration {
	if value == nil {
		return 0
	}
	wallClock := value.Time
	localTime := time.Date(
		wallClock.Year(),
		wallClock.Month(),
		wallClock.Day(),
		wallClock.Hour(),
		wallClock.Minute(),
		wallClock.Second(),
		wallClock.Nanosecond(),
		time.Local,
	)
	return time.Since(localTime)
}

func (s *sSysPublish) cacheCollectEventStructuredMedia(ctx context.Context, event gdb.Record) (bool, error) {
	rows, err := s.collectEventMediaRows(ctx, event["id"].Int64())
	if err != nil {
		return false, err
	}
	items := collectMediaRowsToItems(rows)
	s.appendCollectEventLogForRecord(ctx, event, "media", "checking", "开始检查媒体缓存方式", fmt.Sprintf("media=%d", len(items)))
	changed := false
	if event["source_type"].String() == sysin.CollectSourceTypeAccount {
		s.appendCollectEventLogForRecord(ctx, event, "media", "downloading", "账号采集媒体使用下载缓存，保证带文案媒体组可原格式发送", "")
	} else {
		forwarded, forwardChanged, err := s.forwardCollectMediaToBackup(ctx, event, items)
		if err != nil {
			if retryErr, ok := err.(*collectMediaRetryError); ok {
				s.appendCollectEventLogForRecord(ctx, event, "media", "downloading", "备份频道转存暂不可用，改用下载兜底", retryErr.Error())
			} else {
				return false, err
			}
		}
		if forwardChanged {
			s.appendCollectEventLogForRecord(ctx, event, "media", "forwarded", "媒体已转存到备份频道", "")
			return s.updateCollectEventMediaItems(ctx, rows, forwarded)
		}
	}
	downloadRows := make([]*entity.YoubanPublishCollectEventMedia, 0, len(rows))
	downloadedItems := make([]collectMediaItem, 0, len(rows))
	for index, row := range rows {
		if row == nil || !collectMediaRowNeedsCache(row.SourceFileId, row.StoragePath, row.FileUrl, row.BackupChatId, row.BackupMessageId) {
			continue
		}
		_, _ = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where("id", row.Id).Data(g.Map{
			"cache_status":  collectMediaCacheDownloading,
			"error_message": "",
			"updated_at":    gtime.Now(),
		}).Update()
		s.appendCollectEventLogForRecord(ctx, event, "media", "downloading", "开始下载账号采集媒体", row.SourceFileId)
		cached, err := s.downloadGotdCollectMedia(ctx, event["tenant_id"].Int64(), event["account_id"].Int64(), event["tg_account_id"].Int64(), items[index])
		if err != nil {
			_, _ = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where("id", row.Id).Data(g.Map{
				"cache_status":  collectMediaCacheFailed,
				"error_message": err.Error(),
				"updated_at":    gtime.Now(),
			}).Update()
			return changed, err
		}
		downloadRows = append(downloadRows, row)
		downloadedItems = append(downloadedItems, collectMediaItem{
			Type:        items[index].Type,
			FileUrl:     cached.FileUrl,
			StoragePath: cached.Path,
			PosterUrl:   items[index].PosterUrl,
			MetaJson:    items[index].MetaJson,
		})
	}
	if len(downloadRows) == 0 {
		return changed, nil
	}
	backupItems, uploadedToBackup, backupErr := s.cacheDownloadedCollectMediaGroupToBackup(ctx, event, downloadedItems)
	if backupErr != nil {
		s.appendCollectEventLogForRecord(ctx, event, "media", "retry", "下载媒体组推送备份频道失败，改用目标频道直接上传", backupErr.Error())
	}
	if uploadedToBackup {
		downloadedItems = backupItems
	}
	for index, row := range downloadRows {
		cachedItem := downloadedItems[index]
		_, err = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where("id", row.Id).Data(g.Map{
			"source_file_id":    cachedItem.FileId,
			"source_ref_type":   collectMediaRefType(cachedItem),
			"file_url":          cachedItem.FileUrl,
			"storage_path":      cachedItem.StoragePath,
			"backup_chat_id":    collectMediaBackupChatId(cachedItem),
			"backup_message_id": collectMediaBackupMessageId(cachedItem),
			"cache_status":      collectMediaCacheReady,
			"error_message":     "",
			"updated_at":        gtime.Now(),
		}).Update()
		if err != nil {
			return changed, gerror.Wrap(err, "更新采集媒体下载结果失败")
		}
		changed = true
	}
	return changed, nil
}

func (s *sSysPublish) cacheDownloadedCollectMediaGroupToBackup(ctx context.Context, event gdb.Record, items []collectMediaItem) ([]collectMediaItem, bool, error) {
	if len(items) == 0 {
		return items, false, nil
	}
	channels, err := s.collectEventBackupChannels(ctx, event)
	if err != nil {
		return items, false, err
	}
	if len(channels) == 0 {
		return items, false, nil
	}
	for _, backup := range rotateCollectBackupChannels(channels, event["id"].Int64()) {
		if backup == nil || strings.TrimSpace(backup.ChannelId) == "" {
			continue
		}
		botIds, err := s.collectBackupCopyValidationBotIds(ctx, event, backup.ChannelId)
		if err != nil {
			return items, false, err
		}
		if len(botIds) == 0 {
			continue
		}
		botToken, err := s.telegramJobBotToken(ctx, botIds[0], event["tenant_id"].Int64())
		if err != nil {
			return items, false, err
		}
		bot, err := s.telegramBot(ctx, botToken)
		if err != nil {
			return items, false, err
		}
		chatId := normalizeTelegramChannelChatID(backup.ChannelId)
		media := make([]*telegramMediaItem, 0, len(items))
		for index, item := range items {
			media = append(media, &telegramMediaItem{
				Id:          int64(index + 1),
				MediaType:   collectPublishMediaType(item.Type),
				FileUrl:     item.FileUrl,
				StoragePath: item.StoragePath,
				PosterUrl:   item.PosterUrl,
			})
		}
		messages, err := s.sendTelegramMediaSet(ctx, bot, chatId, "collect_backup", "", media)
		if err != nil {
			s.appendCollectEventLogForRecord(ctx, event, "media", "retry", "下载媒体组推送备份频道失败，尝试下一个备份频道", backup.ChannelId+": "+err.Error())
			continue
		}
		if len(messages) != len(items) {
			s.cleanupTelegramSentMessages(ctx, bot, chatId, messages, "下载媒体组推送备份频道返回数量不完整")
			s.appendCollectEventLogForRecord(ctx, event, "media", "retry", "下载媒体组推送备份频道返回数量不完整，尝试下一个备份频道", backup.ChannelId)
			continue
		}
		messageIds := make([]int, 0, len(messages))
		for _, message := range messages {
			if message == nil || message.MessageId <= 0 {
				continue
			}
			messageIds = append(messageIds, int(message.MessageId))
		}
		if len(messageIds) != len(items) {
			s.cleanupTelegramSentMessages(ctx, bot, chatId, messages, "下载媒体组推送备份频道缺少消息ID")
			s.appendCollectEventLogForRecord(ctx, event, "media", "retry", "下载媒体组推送备份频道缺少消息ID，尝试下一个备份频道", backup.ChannelId)
			continue
		}
		if err = s.validateCollectBackupCopyRefs(ctx, event, backup.ChannelId, messageIds); err != nil {
			s.appendCollectEventLogForRecord(ctx, event, "media", "retry", "下载媒体组备份消息不可复制，尝试下一个备份频道", backup.ChannelId+": "+err.Error())
			continue
		}
		cached := make([]collectMediaItem, len(items))
		for index, item := range items {
			cached[index] = item
			cached[index].FileId = telegramCopyMediaFileId(backup.ChannelId, messageIds[index])
		}
		s.appendCollectEventLogForRecord(ctx, event, "media", "forwarded", "下载媒体组已推送到备份频道", fmt.Sprintf("channel=%s messages=%v", backup.ChannelId, messageIds))
		return cached, true, nil
	}
	return items, false, gerror.New("下载媒体组推送所有备份频道失败")
}

func collectMediaBackupChatId(item collectMediaItem) string {
	ref, ok := telegramCopyMediaRefFromFileId(item.FileId)
	if !ok {
		return ""
	}
	return ref.ChatId
}

func collectMediaBackupMessageId(item collectMediaItem) int {
	ref, ok := telegramCopyMediaRefFromFileId(item.FileId)
	if !ok {
		return 0
	}
	return ref.MessageId
}

func (s *sSysPublish) updateCollectEventMediaItems(ctx context.Context, rows []*entity.YoubanPublishCollectEventMedia, items []collectMediaItem) (bool, error) {
	changed := false
	cols := pdao.YoubanPublishCollectEventMedia.Columns()
	for index, row := range rows {
		if row == nil || index >= len(items) {
			continue
		}
		item := normalizeCollectMediaItem(items[index])
		data := g.Map{
			cols.SourceFileId:  item.FileId,
			cols.SourceRefType: collectMediaRefType(item),
			cols.FileUrl:       item.FileUrl,
			cols.StoragePath:   item.StoragePath,
			cols.PosterUrl:     item.PosterUrl,
			cols.CacheStatus:   collectMediaCacheReady,
			cols.ErrorMessage:  "",
			cols.UpdatedAt:     gtime.Now(),
		}
		if ref, ok := telegramCopyMediaRefFromFileId(item.FileId); ok {
			data[cols.BackupChatId] = ref.ChatId
			data[cols.BackupMessageId] = ref.MessageId
		}
		if _, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where(cols.Id, row.Id).Data(data).Update(); err != nil {
			return changed, gerror.Wrap(err, "更新采集媒体备份结果失败")
		}
		changed = true
	}
	return changed, nil
}

func (s *sSysPublish) collectEventNeedsMediaCache(ctx context.Context, event gdb.Record) bool {
	rows, err := s.collectEventMediaRows(ctx, event["id"].Int64())
	if err != nil {
		return false
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		if collectMediaRowNeedsCache(row.SourceFileId, row.StoragePath, row.FileUrl, row.BackupChatId, row.BackupMessageId) {
			return true
		}
	}
	return false
}

func collectMediaRowNeedsCache(sourceFileId string, storagePath string, fileUrl string, backupChatId string, backupMessageId int64) bool {
	return strings.HasPrefix(strings.TrimSpace(sourceFileId), "gotd:") &&
		strings.TrimSpace(storagePath) == "" &&
		strings.TrimSpace(fileUrl) == "" &&
		!(strings.TrimSpace(backupChatId) != "" && backupMessageId > 0)
}

func collectEventMediaCacheView(mediaJSON string, mediaCount int, status string, errorMessage string) (string, string) {
	if mediaCount <= 0 || strings.TrimSpace(mediaJSON) == "" || strings.TrimSpace(mediaJSON) == "[]" {
		return "none", "无媒体"
	}
	var items []collectMediaItem
	if err := json.Unmarshal([]byte(mediaJSON), &items); err != nil {
		return "failed", "媒体数据异常"
	}
	pending := 0
	cached := 0
	for _, item := range items {
		if strings.HasPrefix(strings.TrimSpace(item.FileId), "gotd:") &&
			strings.TrimSpace(item.StoragePath) == "" &&
			strings.TrimSpace(item.FileUrl) == "" {
			pending++
			continue
		}
		if strings.TrimSpace(item.StoragePath) != "" || strings.TrimSpace(item.FileUrl) != "" || strings.TrimSpace(item.FileId) != "" {
			cached++
		}
	}
	if pending > 0 {
		if strings.TrimSpace(status) == sysin.CollectEventStatusFailed && strings.TrimSpace(errorMessage) != "" {
			return "failed", errorMessage
		}
		return "caching", fmt.Sprintf("%d 个媒体等待缓存", pending)
	}
	if cached > 0 {
		return "cached", fmt.Sprintf("%d 个媒体已缓存", cached)
	}
	return "none", "无可缓存媒体"
}

func (s *sSysPublish) downloadGotdCollectMedia(ctx context.Context, tenantId int64, accountId int64, tgAccountId int64, item collectMediaItem) (*collectDownloadedMedia, error) {
	if tgAccountId <= 0 {
		return nil, gerror.New("账号采集媒体缺少TG账号")
	}
	var meta gotdCollectMediaMeta
	if err := json.Unmarshal([]byte(item.MetaJson), &meta); err != nil || meta.Id <= 0 {
		return nil, gerror.New("账号采集媒体缺少下载元数据")
	}
	var result *collectDownloadedMedia
	usedRuntime, err := s.executeAccountCollectOperation(ctx, tgAccountId, 10*time.Minute, func(runCtx context.Context, client *telegram.Client) error {
		downloaded, err := s.downloadGotdCollectMediaWithClient(runCtx, tenantId, accountId, tgAccountId, item, meta, client)
		if err != nil {
			return err
		}
		result = downloaded
		return nil
	})
	if err != nil {
		return nil, err
	}
	if usedRuntime {
		return result, nil
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return nil, err
	}
	account, err := s.accountCollectTgAccount(ctx, tgAccountId)
	if err != nil {
		return nil, err
	}
	client, err := s.newAccountCollectClient(ctx, conf, account, tg.NewUpdateDispatcher())
	if err != nil {
		return nil, err
	}
	err = client.Run(ctx, func(runCtx context.Context) error {
		if _, err := client.Self(runCtx); err != nil {
			return err
		}
		result, err = s.downloadGotdCollectMediaWithClient(runCtx, tenantId, accountId, tgAccountId, item, meta, client)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *sSysPublish) downloadGotdCollectMediaWithClient(ctx context.Context, tenantId int64, accountId int64, tgAccountId int64, item collectMediaItem, meta gotdCollectMediaMeta, client *telegram.Client) (*collectDownloadedMedia, error) {
	if client == nil {
		return nil, gerror.New("账号采集媒体下载客户端为空")
	}
	buf := bytes.NewBuffer(nil)
	if _, err := downloader.NewDownloader().Download(client.API(), gotdInputFileLocation(meta)).Stream(ctx, buf); err != nil {
		if !collectMediaFileReferenceExpired(err) {
			return nil, gerror.Wrap(err, "下载账号采集媒体失败")
		}
		refreshed, refreshErr := s.refreshGotdCollectMediaMeta(ctx, tenantId, tgAccountId, item, client)
		if refreshErr != nil {
			return nil, gerror.Wrap(err, "下载账号采集媒体失败")
		}
		buf.Reset()
		if _, retryErr := downloader.NewDownloader().Download(client.API(), gotdInputFileLocation(refreshed)).Stream(ctx, buf); retryErr != nil {
			return nil, gerror.Wrap(retryErr, "刷新引用后下载账号采集媒体失败")
		}
		meta = refreshed
	}
	return uploadCollectDownloadedMedia(ctx, accountId, item.Type, meta.MimeType, buf.Bytes())
}

func collectMediaFileReferenceExpired(err error) bool {
	return err != nil && strings.Contains(err.Error(), "FILE_REFERENCE_EXPIRED")
}

func (s *sSysPublish) refreshGotdCollectMediaMeta(ctx context.Context, tenantId int64, tgAccountId int64, item collectMediaItem, client *telegram.Client) (gotdCollectMediaMeta, error) {
	chatId, messageId, ok := parseGotdCollectFileId(item.FileId)
	if !ok {
		return gotdCollectMediaMeta{}, gerror.New("账号采集媒体缺少原消息引用，无法刷新文件引用")
	}
	peer, err := s.collectMediaInputPeer(ctx, tenantId, tgAccountId, client, chatId)
	if err != nil {
		return gotdCollectMediaMeta{}, err
	}
	messages, err := client.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:     peer,
		OffsetID: messageId + 1,
		Limit:    1,
	})
	if err != nil {
		return gotdCollectMediaMeta{}, gerror.Wrap(err, "刷新账号采集媒体引用失败")
	}
	modified, ok := messages.AsModified()
	if !ok {
		return gotdCollectMediaMeta{}, gerror.New("刷新账号采集媒体引用未返回消息列表")
	}
	for _, message := range modified.GetMessages() {
		msg, ok := message.(*tg.Message)
		if !ok || msg.ID != messageId {
			continue
		}
		items := gotdCollectMedia(msg, chatId)
		if len(items) == 0 {
			break
		}
		var meta gotdCollectMediaMeta
		if err = json.Unmarshal([]byte(items[0].MetaJson), &meta); err != nil || meta.Id <= 0 {
			return gotdCollectMediaMeta{}, gerror.New("刷新账号采集媒体引用返回无效元数据")
		}
		return meta, nil
	}
	return gotdCollectMediaMeta{}, gerror.New("刷新账号采集媒体引用未找到原消息")
}

func (s *sSysPublish) collectMediaInputPeer(ctx context.Context, tenantId int64, tgAccountId int64, client *telegram.Client, chatId string) (tg.InputPeerClass, error) {
	if client == nil {
		return nil, gerror.New("账号采集媒体下载客户端为空")
	}
	cache, err := s.tgChannelCacheByChannelId(ctx, tenantId, tgAccountId, chatId)
	if err == nil && cache != nil {
		if peer, peerErr := collectInputPeerChannel(cache); peerErr == nil {
			return peer, nil
		}
	}
	id, parseErr := strconv.ParseInt(strings.TrimSpace(chatId), 10, 64)
	if parseErr == nil && id > 0 {
		return &tg.InputPeerUser{UserID: id}, nil
	}
	return nil, gerror.New("账号采集媒体无法解析原消息会话")
}

func gotdInputFileLocation(meta gotdCollectMediaMeta) tg.InputFileLocationClass {
	if meta.Kind == "photo" {
		thumbSize := strings.TrimSpace(meta.ThumbSize)
		if thumbSize == "" {
			thumbSize = "y"
		}
		return &tg.InputPhotoFileLocation{
			ID:            meta.Id,
			AccessHash:    meta.AccessHash,
			FileReference: meta.FileReference,
			ThumbSize:     thumbSize,
		}
	}
	return &tg.InputDocumentFileLocation{
		ID:            meta.Id,
		AccessHash:    meta.AccessHash,
		FileReference: meta.FileReference,
	}
}

func uploadCollectDownloadedMedia(ctx context.Context, accountId int64, mediaType string, mimeType string, data []byte) (*collectDownloadedMedia, error) {
	if len(data) == 0 {
		return nil, gerror.New("账号采集媒体下载为空")
	}
	uploadCtx := collectMediaUploadContext(ctx, accountId)
	uploadType := storager.KindImg
	if strings.TrimSpace(mediaType) == "video" {
		uploadType = storager.KindVideo
	}
	name := "collect-media" + collectMediaExt(mediaType, mimeType)
	header, err := file.NewMultipartFileHeader(name, data)
	if err != nil {
		return nil, gerror.Wrap(err, "创建采集媒体上传文件失败")
	}
	attachment, err := service.CommonUpload().UploadFile(uploadCtx, uploadType, &ghttp.UploadFile{FileHeader: header})
	if err != nil {
		return nil, err
	}
	return &collectDownloadedMedia{FileUrl: attachment.FileUrl, Path: attachment.Path}, nil
}

func collectMediaUploadContext(ctx context.Context, accountId int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	current := contexts.Get(ctx)
	if current == nil {
		return context.WithValue(ctx, consts.ContextHTTPKey, &model.Context{
			Module:    consts.AppApi,
			AddonName: "youban_publish",
			User:      &model.Identity{Id: accountId, App: consts.AppApi},
			Data:      g.Map{},
		})
	}
	if current.Module == "" {
		current.Module = consts.AppApi
	}
	if current.AddonName == "" {
		current.AddonName = "youban_publish"
	}
	if current.User == nil || current.User.Id <= 0 {
		current.User = &model.Identity{Id: accountId, App: consts.AppApi}
	}
	if current.User.App == "" {
		current.User.App = consts.AppApi
	}
	return ctx
}

func collectMediaExt(mediaType string, mimeType string) string {
	if strings.TrimSpace(mediaType) == "video" {
		return ".mp4"
	}
	if ext, _ := mime.ExtensionsByType(strings.TrimSpace(mimeType)); len(ext) > 0 {
		return ext[0]
	}
	return ".jpg"
}

type collectDownloadedMedia struct {
	FileUrl string
	Path    string
}

func (s *sSysPublish) collectMediaAccountLock(tenantId int64, tgAccountId int64) *publishRuntimeMutex {
	key := collectMediaAccountKey(tenantId, tgAccountId)
	s.collectMediaMu.Lock()
	defer s.collectMediaMu.Unlock()
	if s.collectMediaLocks == nil {
		s.collectMediaLocks = make(map[string]*publishRuntimeMutex)
	}
	if lock := s.collectMediaLocks[key]; lock != nil {
		return lock
	}
	lock := &publishRuntimeMutex{}
	s.collectMediaLocks[key] = lock
	return lock
}

func (s *sSysPublish) waitCollectMediaAccountInterval(ctx context.Context, tenantId int64, tgAccountId int64) {
	interval := time.Duration(g.Cfg().MustGet(ctx, "youbanPublish.collect.mediaAccountIntervalMs", 1200).Int()) * time.Millisecond
	if interval <= 0 {
		return
	}
	key := collectMediaAccountKey(tenantId, tgAccountId)
	s.collectMediaMu.Lock()
	last := s.collectMediaLastTouch[key]
	wait := interval - time.Since(last)
	s.collectMediaLastTouch[key] = time.Now().Add(maxDuration(wait, 0))
	s.collectMediaMu.Unlock()
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
	}
}

func collectMediaAccountKey(tenantId int64, tgAccountId int64) string {
	return fmt.Sprintf("%d:%d", tenantId, tgAccountId)
}

func maxDuration(a time.Duration, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
