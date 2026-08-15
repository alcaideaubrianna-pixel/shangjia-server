package sys

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/storager"
	"hotgo/internal/model"
	isc "hotgo/internal/service"
	fileutil "hotgo/utility/file"
)

func (s *sSysBot) persistQuickPushMedia(ctx context.Context, operatorAccountId int64, media []*publishsysin.MessageTemplateMediaInp) ([]*publishsysin.MessageTemplateMediaInp, error) {
	if len(media) == 0 {
		return media, nil
	}
	uploadCtx := quickPushMediaUploadContext(ctx, operatorAccountId)
	result := make([]*publishsysin.MessageTemplateMediaInp, 0, len(media))
	for _, item := range media {
		if item == nil {
			continue
		}
		stored := *item
		fileURL, storagePath, err := persistQuickPushMediaFile(uploadCtx, item.MediaType, item.Name, item.FileUrl)
		if err != nil {
			return nil, gerror.Wrapf(err, "转存Telegram媒体失败 mediaType:%s name:%s", item.MediaType, item.Name)
		}
		stored.FileUrl = fileURL
		stored.StoragePath = storagePath
		if strings.TrimSpace(item.PosterUrl) != "" {
			posterURL, posterPath, posterErr := persistQuickPushMediaFile(uploadCtx, "image", item.Name+"-poster", item.PosterUrl)
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

func persistQuickPushMediaFile(ctx context.Context, mediaType string, name string, sourceURL string) (string, string, error) {
	data, err := downloadTelegramAsset(ctx, sourceURL)
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
