package sys

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	pdao "hotgo/addons/youban_publish/internal/dao"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/storager"
	"hotgo/internal/model"
	"hotgo/internal/service"
	"hotgo/utility/file"
)

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
	items, changed, err := s.cacheCollectEventMedia(ctx, event)
	if err != nil {
		_ = s.markCollectEvent(ctx, payload.EventId, sysin.CollectEventStatusFailed, err.Error())
		return err
	}
	if changed {
		mediaJSON, mediaCount := collectMessageMediaJSON(items)
		_, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", payload.EventId).Data(g.Map{
			"media_json":    mediaJSON,
			"media_count":   mediaCount,
			"dedupe_key":    collectHash(fmt.Sprintf("%s:%s:%d", event["raw_text"].String(), mediaJSON, mediaCount)),
			"error_message": "",
			"updated_at":    gtime.Now(),
		}).Update()
		if err != nil {
			return gerror.Wrap(err, "更新采集媒体缓存结果失败")
		}
	}
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
	if row["status"].String() == sysin.CollectEventStatusProcessed {
		return row, nil
	}
	return row, nil
}

func (s *sSysPublish) cacheCollectEventMedia(ctx context.Context, event gdb.Record) ([]collectMediaItem, bool, error) {
	var items []collectMediaItem
	if err := json.Unmarshal([]byte(event["media_json"].String()), &items); err != nil {
		return nil, false, gerror.Wrap(err, "解析采集媒体失败")
	}
	changed := false
	for idx, item := range items {
		if !strings.HasPrefix(strings.TrimSpace(item.FileId), "gotd:") {
			continue
		}
		if strings.TrimSpace(item.StoragePath) != "" || strings.TrimSpace(item.FileUrl) != "" {
			items[idx].FileId = ""
			changed = true
			continue
		}
		cached, err := s.downloadGotdCollectMedia(ctx, event["account_id"].Int64(), event["tg_account_id"].Int64(), item)
		if err != nil {
			return nil, false, err
		}
		items[idx].FileId = ""
		items[idx].FileUrl = cached.FileUrl
		items[idx].StoragePath = cached.Path
		changed = true
	}
	return items, changed, nil
}

func collectEventNeedsMediaCache(event gdb.Record) bool {
	var items []collectMediaItem
	if err := json.Unmarshal([]byte(event["media_json"].String()), &items); err != nil {
		return false
	}
	for _, item := range items {
		if strings.HasPrefix(strings.TrimSpace(item.FileId), "gotd:") &&
			strings.TrimSpace(item.StoragePath) == "" &&
			strings.TrimSpace(item.FileUrl) == "" {
			return true
		}
	}
	return false
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

func (s *sSysPublish) downloadGotdCollectMedia(ctx context.Context, accountId int64, tgAccountId int64, item collectMediaItem) (*collectDownloadedMedia, error) {
	if tgAccountId <= 0 {
		return nil, gerror.New("账号采集媒体缺少TG账号")
	}
	var meta gotdCollectMediaMeta
	if err := json.Unmarshal([]byte(item.MetaJson), &meta); err != nil || meta.Id <= 0 {
		return nil, gerror.New("账号采集媒体缺少下载元数据")
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
	var result *collectDownloadedMedia
	err = client.Run(ctx, func(runCtx context.Context) error {
		if _, err := client.Self(runCtx); err != nil {
			return err
		}
		buf := bytes.NewBuffer(nil)
		if _, err := downloader.NewDownloader().Download(client.API(), gotdInputFileLocation(meta)).Stream(runCtx, buf); err != nil {
			return gerror.Wrap(err, "下载账号采集媒体失败")
		}
		result, err = uploadCollectDownloadedMedia(runCtx, accountId, item.Type, meta.MimeType, buf.Bytes())
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
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
	if ext, _ := mime.ExtensionsByType(strings.TrimSpace(mimeType)); len(ext) > 0 {
		return ext[0]
	}
	if strings.TrimSpace(mediaType) == "video" {
		return ".mp4"
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
