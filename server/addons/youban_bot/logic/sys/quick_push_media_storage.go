package sys

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/storager"
	isc "hotgo/internal/service"
	fileutil "hotgo/utility/file"
)

func (s *sSysBot) persistQuickPushMedia(ctx context.Context, media []*publishsysin.MessageTemplateMediaInp) []*publishsysin.MessageTemplateMediaInp {
	if len(media) == 0 {
		return media
	}
	result := make([]*publishsysin.MessageTemplateMediaInp, 0, len(media))
	for _, item := range media {
		if item == nil {
			continue
		}
		stored := *item
		fileURL, storagePath, err := persistQuickPushMediaFile(ctx, item.MediaType, item.Name, item.FileUrl)
		if err != nil {
			g.Log().Warningf(ctx, "快速推送媒体转存失败，保留Telegram复制发送能力 mediaType:%s name:%s err:%+v", item.MediaType, item.Name, err)
			stored.FileUrl = ""
			stored.StoragePath = ""
		} else {
			stored.FileUrl = fileURL
			stored.StoragePath = storagePath
		}
		if strings.TrimSpace(item.PosterUrl) != "" {
			posterURL, posterPath, posterErr := persistQuickPushMediaFile(ctx, "image", item.Name+"-poster", item.PosterUrl)
			if posterErr != nil {
				g.Log().Warningf(ctx, "快速推送媒体封面转存失败 name:%s err:%+v", item.Name, posterErr)
				stored.PosterUrl = ""
				stored.PosterStoragePath = ""
			} else {
				stored.PosterUrl = posterURL
				stored.PosterStoragePath = posterPath
			}
		}
		result = append(result, &stored)
	}
	return result
}

func persistQuickPushMediaFile(ctx context.Context, mediaType string, name string, sourceURL string) (string, string, error) {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		return "", "", nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", "", err
	}
	client, err := telegramHTTPClient(telegramProxyUrl(ctx))
	if err != nil {
		return "", "", err
	}
	client.Timeout = 2 * time.Minute
	response, err := client.Do(request)
	if err != nil {
		return "", "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", "", gerror.Newf("下载Telegram媒体失败，状态码:%d", response.StatusCode)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return "", "", err
	}
	if len(data) == 0 {
		return "", "", gerror.New("Telegram媒体内容为空")
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
