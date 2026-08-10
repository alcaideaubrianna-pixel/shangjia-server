package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
	"hotgo/internal/library/storager"
	basesysin "hotgo/internal/model/input/sysin"
)

func (s *sSysPublish) MediaMultipartCheck(ctx context.Context, in *storager.CheckMultipartParams) (*basesysin.CheckMultipartModel, error) {
	if in == nil {
		return nil, gerror.New("分片上传参数不能为空")
	}
	data, err := storager.CheckMultipart(ctx, in)
	if err != nil {
		return nil, err
	}
	return &basesysin.CheckMultipartModel{CheckMultipartModel: data}, nil
}

func (s *sSysPublish) MediaMultipartPart(ctx context.Context, in *storager.UploadPartParams) (*basesysin.UploadPartModel, error) {
	if in == nil {
		return nil, gerror.New("分片上传参数不能为空")
	}
	data, err := storager.UploadPart(ctx, in)
	if err != nil {
		return nil, err
	}
	return &basesysin.UploadPartModel{UploadPartModel: data}, nil
}

func (s *sSysPublish) AdminMediaMultipartAttach(ctx context.Context, in *sysin.MediaMultipartAttachInp) (*sysin.MediaModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.attachMultipartMedia(ctx, in, account.TenantId, 0)
}

func (s *sSysPublish) MyMediaMultipartAttach(ctx context.Context, in *sysin.MediaMultipartAttachInp) (*sysin.MediaModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.attachMultipartMedia(ctx, in, account.TenantId, account.Id)
}

func (s *sSysPublish) attachMultipartMedia(ctx context.Context, in *sysin.MediaMultipartAttachInp, tenantId, accountId int64) (*sysin.MediaModel, error) {
	if in == nil || in.AttachmentId <= 0 {
		return nil, gerror.New("附件ID不能为空")
	}
	if err := in.MediaUploadInp.Filter(ctx); err != nil {
		return nil, err
	}
	task, err := s.resolveMediaEditTask(ctx, &in.MediaUploadInp, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	attachment := new(basesysin.AttachmentListModel)
	mod := dao.SysAttachment.Ctx(ctx).
		Where(dao.SysAttachment.Columns().Id, in.AttachmentId).
		Where(dao.SysAttachment.Columns().Status, 1)
	if accountId > 0 {
		mod = mod.Where(dao.SysAttachment.Columns().MemberId, accountId)
	}
	if err = mod.Scan(&attachment.SysAttachment); err != nil {
		return nil, gerror.Wrap(err, "读取分片附件失败")
	}
	if attachment.Id <= 0 {
		return nil, gerror.New("分片附件不存在或无权使用")
	}
	res, err := s.saveMediaAttachment(ctx, task, &in.MediaUploadInp, attachment, nil, nil, "")
	if err != nil {
		return nil, err
	}
	if res != nil && res.Id > 0 {
		if err = s.enqueueMediaProcess(ctx, res.Id, 0); err != nil {
			g.Log().Warningf(ctx, "分片媒体处理任务入队失败 media_id:%d err:%+v", res.Id, err)
		}
	}
	return res, nil
}
