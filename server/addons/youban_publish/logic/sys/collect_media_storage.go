package sys

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/internal/library/storager"
	basesysin "hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
	fileutil "hotgo/utility/file"
)

func (s *sSysPublish) uploadCollectMediaToStorage(ctx context.Context, mediaType string, storagePath string) (*basesysin.AttachmentListModel, error) {
	localPath, err := resolveMediaLocalPath(storagePath)
	if err != nil {
		return nil, err
	}
	if localPath == "" {
		return nil, gerror.New("采集媒体本地缓存文件不存在")
	}
	mediaBytes := fileSizeValue(localPath)

	header, cleanup, err := fileutil.NewMultipartFileHeaderFromPath(localPath, filepath.Base(localPath))
	if err != nil {
		return nil, err
	}
	defer cleanup()

	uploadType := storager.KindImg
	if strings.EqualFold(strings.TrimSpace(mediaType), "video") {
		uploadType = storager.KindVideo
	}
	startedAt := time.Now()
	attachment, err := service.CommonUpload().UploadFile(ctx, uploadType, &ghttp.UploadFile{FileHeader: header})
	if err != nil {
		g.Log().Info(ctx, "媒体上传网络流量", g.Map{
			"log_type": "media_network_traffic", "event": "media_upload_to_storage",
			"role": "media-worker", "direction": "internet_egress", "destination": "object_storage",
			"mediaType": mediaType, "mediaFileBytes": mediaBytes, "byteSemantics": "source_file_size",
			"durationMs": time.Since(startedAt).Milliseconds(), "result": "error",
		})
		return nil, gerror.Wrap(err, "采集媒体上传云存储失败")
	}
	if attachment == nil {
		return nil, gerror.New("采集媒体上传云存储未返回附件")
	}
	if strings.TrimSpace(attachment.FileUrl) == "" && strings.TrimSpace(attachment.Path) == "" {
		return nil, gerror.New("采集媒体上传云存储未返回地址")
	}
	g.Log().Info(ctx, "媒体上传网络流量", g.Map{
		"log_type": "media_network_traffic", "event": "media_upload_to_storage",
		"role": "media-worker", "direction": "internet_egress", "destination": "object_storage",
		"mediaType": mediaType, "mediaFileBytes": mediaBytes, "byteSemantics": "source_file_size",
		"durationMs": time.Since(startedAt).Milliseconds(), "result": "success",
	})
	return attachment, nil
}
