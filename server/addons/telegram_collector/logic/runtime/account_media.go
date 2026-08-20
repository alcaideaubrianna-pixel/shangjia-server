package runtime

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

var errAccountMediaSourceGone = errors.New("Telegram原消息已删除或已无可用媒体")

type accountMediaTaskHandler struct{}

func init() {
	collectorservice.RegisterAccountTaskHandler(sysin.AccountTaskTypeMediaDownload, &accountMediaTaskHandler{})
}

func (h *accountMediaTaskHandler) HandleAccountTask(ctx context.Context, client *telegram.Client, task *sysin.AccountTask) (*sysin.AccountMediaDownloadResult, error) {
	if client == nil || task == nil {
		return nil, gerror.New("Telegram账号媒体任务参数无效")
	}
	provider := collectorservice.AccountMedia()
	if provider == nil {
		return nil, gerror.New("Telegram账号媒体存储未注册")
	}
	media := task.Media
	path, refreshed, err := downloadAccountTaskMedia(ctx, client, provider, task, media)
	if err != nil {
		if errors.Is(err, errAccountMediaSourceGone) {
			return &sysin.AccountMediaDownloadResult{Media: media, ErrorCode: "source_gone", ErrorMessage: err.Error()}, nil
		}
		return nil, err
	}
	defer os.Remove(path)
	task.Media = refreshed
	result, err := provider.StoreMedia(ctx, task, path)
	if err != nil {
		return nil, gerror.Wrap(err, "保存Telegram账号媒体失败")
	}
	if result == nil {
		return nil, gerror.New("保存Telegram账号媒体未返回结果")
	}
	if result.Media.FileID == "" {
		result.Media = refreshed
	}
	return result, nil
}

func downloadAccountTaskMedia(ctx context.Context, client *telegram.Client, provider collectorservice.AccountMediaProvider, task *sysin.AccountTask, media sysin.CollectorMediaItem) (string, sysin.CollectorMediaItem, error) {
	refreshed := media
	if !accountMediaMetadataValid(refreshed) {
		var err error
		refreshed, err = refreshAccountTaskMedia(ctx, client, provider, task, media)
		if err != nil {
			return "", media, err
		}
	}
	path, err := transferAccountTaskMedia(ctx, client, task.AccountID, refreshed)
	if !accountMediaReferenceExpired(err) {
		return path, refreshed, err
	}
	refreshed, refreshErr := refreshAccountTaskMedia(ctx, client, provider, task, media)
	if refreshErr != nil {
		return "", media, refreshErr
	}
	path, err = transferAccountTaskMedia(ctx, client, task.AccountID, refreshed)
	return path, refreshed, err
}

func transferAccountTaskMedia(ctx context.Context, client *telegram.Client, accountID int64, media sysin.CollectorMediaItem) (string, error) {
	location, ok := accountMediaLocation(media)
	if !ok {
		return "", gerror.New("Telegram媒体缺少有效下载元数据")
	}
	dir := filepath.Join(os.TempDir(), "youban-telegram-collector")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", gerror.Wrap(err, "创建Telegram媒体临时目录失败")
	}
	file, err := os.CreateTemp(dir, "account-media-*"+accountMediaExtension(media))
	if err != nil {
		return "", gerror.Wrap(err, "创建Telegram媒体临时文件失败")
	}
	path := file.Name()
	if closeErr := file.Close(); closeErr != nil {
		_ = os.Remove(path)
		return "", closeErr
	}
	_ = os.Remove(path)
	timeout := accountMediaDownloadTimeout(media.SourceSize)
	downloadCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	threads := normalizeAccountMediaThreads(g.Cfg().MustGet(ctx, "telegramCollector.media.concurrency", accountMediaDefaultThreads).Int())
	startedAt := time.Now()
	if err = globalAccountMediaTransfer.download(downloadCtx, accountID, client, location, path, media.SourceSize, media.SourceDCID, threads); err != nil {
		_ = os.Remove(path)
		return "", gerror.Wrap(err, "下载Telegram媒体失败")
	}
	g.Log().Infof(ctx, "Telegram账号媒体下载完成 accountId:%d fileId:%s size:%d duration:%s", accountID, media.FileID, media.SourceSize, time.Since(startedAt).Round(time.Millisecond))
	return path, nil
}

func refreshAccountTaskMedia(ctx context.Context, client *telegram.Client, provider collectorservice.AccountMediaProvider, task *sysin.AccountTask, media sysin.CollectorMediaItem) (sysin.CollectorMediaItem, error) {
	chatID, messageID, ok := parseAccountMediaFileID(media.FileID)
	if !ok {
		return media, gerror.New("Telegram媒体缺少可刷新的原消息引用")
	}
	peer, err := provider.ResolvePeer(ctx, task.TenantID, task.AccountID, chatID, client)
	if err != nil {
		return media, err
	}
	var result tg.MessagesMessagesClass
	if channel, channelOK := peer.(*tg.InputPeerChannel); channelOK {
		result, err = client.API().ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: channel.ChannelID, AccessHash: channel.AccessHash},
			ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: messageID}},
		})
	} else {
		result, err = client.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer: peer, OffsetID: messageID + 1, AddOffset: -1, Limit: 1, MaxID: messageID, MinID: messageID,
		})
	}
	if err != nil {
		return media, gerror.Wrap(err, "重新读取Telegram原消息失败")
	}
	for _, message := range accountHistoryMessages(result) {
		if message == nil || message.ID != messageID {
			continue
		}
		items := accountMessageMedia(message, chatID)
		if len(items) == 0 {
			return media, gerror.Wrap(errAccountMediaSourceGone, "Telegram原消息已不存在媒体")
		}
		item := items[0]
		item.FileID = media.FileID
		item.Purpose = media.Purpose
		return item, nil
	}
	return media, gerror.Wrap(errAccountMediaSourceGone, "Telegram原消息不存在或无权读取")
}

func accountHistoryMessages(result tg.MessagesMessagesClass) []*tg.Message {
	items := make([]*tg.Message, 0)
	appendItems := func(messages []tg.MessageClass) {
		for _, item := range messages {
			if message, ok := item.(*tg.Message); ok {
				items = append(items, message)
			}
		}
	}
	switch value := result.(type) {
	case *tg.MessagesMessages:
		appendItems(value.Messages)
	case *tg.MessagesMessagesSlice:
		appendItems(value.Messages)
	case *tg.MessagesChannelMessages:
		appendItems(value.Messages)
	}
	return items
}

func accountMediaMetadataValid(media sysin.CollectorMediaItem) bool {
	return media.SourceMediaID > 0 && strings.TrimSpace(media.SourceKind) != ""
}

func accountMediaLocation(media sysin.CollectorMediaItem) (tg.InputFileLocationClass, bool) {
	if !accountMediaMetadataValid(media) {
		return nil, false
	}
	if strings.EqualFold(strings.TrimSpace(media.SourceKind), sysin.MediaKindPhoto) {
		thumb := strings.TrimSpace(media.SourceThumbSize)
		if thumb == "" {
			thumb = "y"
		}
		return &tg.InputPhotoFileLocation{ID: media.SourceMediaID, AccessHash: media.SourceAccessHash, FileReference: media.SourceFileReference, ThumbSize: thumb}, true
	}
	return &tg.InputDocumentFileLocation{ID: media.SourceMediaID, AccessHash: media.SourceAccessHash, FileReference: media.SourceFileReference}, true
}

func parseAccountMediaFileID(fileID string) (string, int, bool) {
	fileID = strings.TrimSpace(fileID)
	if !strings.HasPrefix(fileID, "gotd:") {
		return "", 0, false
	}
	raw := strings.TrimPrefix(fileID, "gotd:")
	index := strings.LastIndex(raw, ":")
	if index <= 0 || index >= len(raw)-1 {
		return "", 0, false
	}
	messageID, err := strconv.Atoi(raw[index+1:])
	if err != nil || messageID <= 0 {
		return "", 0, false
	}
	return raw[:index], messageID, true
}

func accountMediaReferenceExpired(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToUpper(err.Error())
	return strings.Contains(message, "FILE_REFERENCE_EXPIRED") || strings.Contains(message, "FILE_REFERENCE_INVALID")
}

func accountMediaDownloadTimeout(size int64) time.Duration {
	switch {
	case size > 50<<20:
		return 3 * time.Minute
	case size > 10<<20:
		return 2 * time.Minute
	default:
		return time.Minute
	}
}

func accountMediaExtension(media sysin.CollectorMediaItem) string {
	if strings.EqualFold(strings.TrimSpace(media.Type), sysin.MediaKindVideo) {
		return ".mp4"
	}
	if extensions, _ := mime.ExtensionsByType(strings.TrimSpace(media.SourceMimeType)); len(extensions) > 0 {
		return extensions[0]
	}
	if strings.EqualFold(strings.TrimSpace(media.Type), sysin.MediaKindPhoto) {
		return ".jpg"
	}
	return fmt.Sprintf(".%s", sysin.MediaKindFile)
}
