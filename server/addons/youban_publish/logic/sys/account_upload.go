package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/internal/library/storager"
	basesysin "hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
)

func (s *sSysPublish) MyAccountImageUpload(ctx context.Context, file *ghttp.UploadFile) (*basesysin.AttachmentListModel, error) {
	if _, err := s.currentAccount(ctx); err != nil {
		return nil, err
	}
	if file == nil {
		return nil, gerror.New("没有找到上传的文件")
	}
	return service.CommonUpload().UploadFile(ctx, storager.KindImg, file)
}
