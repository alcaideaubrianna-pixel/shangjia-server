package sys

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/storager"
	"hotgo/internal/library/telegrammedia"
	"hotgo/internal/model"
	isc "hotgo/internal/service"
	fileutil "hotgo/utility/file"
)

func (s *sSysBot) persistQuickPushMedia(ctx context.Context, operatorAccountId int64, media []*publishsysin.MessageTemplateMediaInp) ([]*publishsysin.MessageTemplateMediaInp, error) {
	return s.persistTelegramMedia(ctx, operatorAccountId, media)
}

func (s *sSysBot) persistTelegramMedia(ctx context.Context, operatorAccountId int64, media []*publishsysin.MessageTemplateMediaInp) ([]*publishsysin.MessageTemplateMediaInp, error) {
	if len(media) == 0 {
		return media, nil
	}
	client, err := telegramHTTPClient(telegramProxyUrl(ctx))
	if err != nil {
		return nil, gerror.New("创建Telegram媒体下载网络客户端失败")
	}
	client.Timeout = 2 * time.Minute
	defer client.CloseIdleConnections()
	uploadCtx, uploadCancel := context.WithTimeout(quickPushMediaUploadContext(ctx, operatorAccountId), 60*time.Second)
	defer uploadCancel()
	result := make([]*publishsysin.MessageTemplateMediaInp, len(media))
	workerLimit := 4
	if len(media) < workerLimit {
		workerLimit = len(media)
	}
	semaphore := make(chan struct{}, workerLimit)
	var wait sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex
	for index, item := range media {
		if item == nil {
			continue
		}
		wait.Add(1)
		go func(index int, item *publishsysin.MessageTemplateMediaInp) {
			defer wait.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			stored := *item
			mediaCtx, cancel := context.WithTimeout(uploadCtx, 45*time.Second)
			defer cancel()
			startedAt := time.Now()
			g.Log().Infof(ctx, "Telegram媒体持久化开始 media_index:%d media_type:%s name:%s", index, item.MediaType, item.Name)
			fileURL, storagePath, reused := lookupBotTelegramMediaReuse(mediaCtx, operatorAccountId, item.MediaType, item.TgFileId)
			var err error
			if reused {
				g.Log().Infof(ctx, "Telegram媒体存储复用 media_index:%d tg_file_id:%s", index, item.TgFileId)
			} else {
				fileURL, storagePath, err = persistQuickPushMediaFile(mediaCtx, client, item.MediaType, item.Name, item.FileUrl)
			}
			if err != nil {
				g.Log().Warningf(ctx, "Telegram媒体持久化失败 media_index:%d elapsed_ms:%d err:%+v", index, time.Since(startedAt).Milliseconds(), err)
				errMu.Lock()
				if firstErr == nil {
					firstErr = gerror.Wrapf(err, "转存Telegram媒体失败 mediaType:%s name:%s", item.MediaType, item.Name)
				}
				errMu.Unlock()
				return
			}
			stored.FileUrl = fileURL
			stored.StoragePath = storagePath
			if strings.TrimSpace(item.PosterUrl) != "" {
				posterURL, posterPath, posterErr := persistQuickPushMediaFile(mediaCtx, client, "image", item.Name+"-poster", item.PosterUrl)
				if posterErr != nil {
					g.Log().Warningf(ctx, "Telegram媒体封面转存失败 name:%s err:%+v", item.Name, posterErr)
					stored.PosterUrl = ""
					stored.PosterStoragePath = ""
				} else {
					stored.PosterUrl = posterURL
					stored.PosterStoragePath = posterPath
				}
			}
			result[index] = &stored
			g.Log().Infof(ctx, "Telegram媒体持久化完成 media_index:%d elapsed_ms:%d", index, time.Since(startedAt).Milliseconds())
		}(index, item)
	}
	done := make(chan struct{})
	go func() {
		wait.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-uploadCtx.Done():
		return nil, gerror.Wrap(uploadCtx.Err(), "Telegram媒体持久化超时")
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return result, nil
}

func quickPushMediaUploadContext(ctx context.Context, operatorAccountId int64) context.Context {
	return context.WithValue(ctx, consts.ContextHTTPKey, &model.Context{
		Module:    consts.AppApi,
		AddonName: "youban_bot",
		User: &model.Identity{
			Id:  operatorAccountId,
			App: consts.AppApi,
		},
	})
}

func persistQuickPushMediaFile(ctx context.Context, client *http.Client, mediaType string, name string, sourceURL string) (string, string, error) {
	data, err := telegrammedia.DownloadURL(ctx, client, sourceURL, profileCreateMaxMediaBytes)
	if err != nil {
		return "", "", err
	}
	filename := quickPushMediaFilename(name, mediaType)
	header, err := fileutil.NewMultipartFileHeader(filename, data)
	if err != nil {
		return "", "", err
	}
	uploadType := storager.KindImg
	if strings.EqualFold(strings.TrimSpace(mediaType), "video") {
		uploadType = storager.KindVideo
	}
	attachment, err := isc.CommonUpload().UploadFile(ctx, uploadType, &ghttp.UploadFile{FileHeader: header})
	if err != nil {
		return "", "", err
	}
	if attachment == nil {
		return "", "", gerror.New("系统存储未返回媒体地址")
	}
	return strings.TrimSpace(attachment.FileUrl), strings.TrimSpace(attachment.Path), nil
}

// lookupBotTelegramMediaReuse returns an already persisted Telegram asset.
// Telegram file IDs are stable, so reusing the storage object avoids another
// download/upload when the same media is submitted again.
func lookupBotTelegramMediaReuse(ctx context.Context, accountID int64, mediaType, tgFileID string) (string, string, bool) {
	tgFileID = strings.TrimSpace(tgFileID)
	// BOT profile media currently uses copy:<chat>:<message> as a provenance
	// reference. It is not Telegram's reusable file_id and must never be used
	// as a content identity. A real Telegram file_id is safe to reuse here;
	// stable file_unique_id support is added separately once persisted on the
	// publish media row.
	if accountID <= 0 || tgFileID == "" || strings.HasPrefix(tgFileID, "copy:") {
		return "", "", false
	}
	row, err := g.DB().Model("hg_youban_publish_media").Safe().Ctx(ctx).
		Fields("file_url,storage_path,poster_url,poster_storage_path").
		Where("account_id", accountID).
		Where("tg_file_id", tgFileID).
		Where("media_type", strings.TrimSpace(mediaType)).
		WhereNull("deleted_at").
		Where("storage_path IS NOT NULL AND storage_path <> ''").
		OrderDesc("id").One()
	if err != nil || row.IsEmpty() {
		return "", "", false
	}
	return strings.TrimSpace(row["file_url"].String()), strings.TrimSpace(row["storage_path"].String()), true
}

func quickPushMediaFilename(name string, mediaType string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "quick-push-media"
	}
	if filepath.Ext(name) != "" {
		return name
	}
	if strings.EqualFold(strings.TrimSpace(mediaType), "video") {
		return name + ".mp4"
	}
	return name + ".jpg"
}
