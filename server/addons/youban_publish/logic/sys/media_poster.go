package sys

import (
	"context"

	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/internal/library/storager"
	basesysin "hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
)

func uploadMediaPoster(ctx context.Context, file *ghttp.UploadFile) (*basesysin.AttachmentListModel, error) {
	if file == nil {
		return nil, nil
	}
	return service.CommonUpload().UploadFile(ctx, storager.KindImg, file)
}

func posterFileUrl(attachment *basesysin.AttachmentListModel) string {
	if attachment == nil {
		return ""
	}
	return attachment.FileUrl
}

func posterStoragePath(attachment *basesysin.AttachmentListModel) string {
	if attachment == nil {
		return ""
	}
	return attachment.Path
}
