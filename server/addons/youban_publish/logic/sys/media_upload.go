package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/storager"
	basesysin "hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
)

func (s *sSysPublish) AdminMediaUpload(ctx context.Context, in *sysin.MediaUploadInp, file *ghttp.UploadFile, poster *ghttp.UploadFile, originalFile *ghttp.UploadFile) (res *sysin.MediaModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	task, err := s.getTaskByTenant(ctx, in.TaskId, account.TenantId)
	if err != nil {
		return nil, err
	}
	return s.saveUploadedTaskMedia(ctx, task, in, file, poster, originalFile)
}

func (s *sSysPublish) MyMediaUpload(ctx context.Context, in *sysin.MediaUploadInp, file *ghttp.UploadFile, poster *ghttp.UploadFile, originalFile *ghttp.UploadFile) (res *sysin.MediaModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	task, err := s.getTask(ctx, in.TaskId, account.Id)
	if err != nil {
		return nil, err
	}
	return s.saveUploadedTaskMedia(ctx, task, in, file, poster, originalFile)
}

func (s *sSysPublish) AdminMessageTemplateMediaUpload(ctx context.Context, in *sysin.MessageTemplateMediaUploadInp, file *ghttp.UploadFile, poster *ghttp.UploadFile) (*sysin.MessageTemplateMediaModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("上传参数不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if file == nil {
		return nil, gerror.New("没有找到上传的文件")
	}
	uploadType := storager.KindImg
	if in.MediaType == "video" {
		uploadType = storager.KindVideo
	}
	posterResult, err := uploadMediaPosterForType(ctx, in.MediaType, poster)
	if err != nil {
		return nil, err
	}
	if in.MediaType == "video" && posterResult == nil {
		posterResult, err = uploadVideoPosterWithPHash(ctx, file)
		if err != nil {
			g.Log().Warningf(ctx, "自动生成消息模板视频封面失败 mediaType:%s name:%s err:%+v", in.MediaType, file.Filename, err)
		}
	}
	attachment, err := service.CommonUpload().UploadFile(ctx, uploadType, file)
	if err != nil {
		return nil, err
	}
	return &sysin.MessageTemplateMediaModel{
		TenantId:          account.TenantId,
		MediaType:         in.MediaType,
		Name:              attachment.Name,
		FileUrl:           normalizeMediaFileURL(attachment.FileUrl, attachment.Path),
		StoragePath:       attachment.Path,
		PosterUrl:         normalizeMediaFileURL(posterFileUrl(posterAttachment(posterResult)), posterStoragePath(posterAttachment(posterResult))),
		PosterStoragePath: posterStoragePath(posterAttachment(posterResult)),
		AssetHash:         attachment.Md5,
		SortIndex:         in.SortIndex,
	}, nil
}

func (s *sSysPublish) saveUploadedTaskMedia(ctx context.Context, task gdb.Record, in *sysin.MediaUploadInp, file *ghttp.UploadFile, poster *ghttp.UploadFile, originalFile *ghttp.UploadFile) (*sysin.MediaModel, error) {
	if task["status"].String() != sysin.PublishTaskStatusDraft {
		return nil, gerror.New("仅草稿任务可以上传媒体")
	}
	if file == nil {
		return nil, gerror.New("没有找到上传的文件")
	}
	uploadType := storager.KindImg
	if in.MediaType == "video" {
		uploadType = storager.KindVideo
	}
	perceptualHash := ""
	var err error
	if in.MediaType == "image" {
		perceptualHash, err = uploadImagePHash(file)
		if err != nil {
			return nil, err
		}
	}
	posterResult, err := uploadMediaPosterForType(ctx, in.MediaType, poster)
	if err != nil {
		return nil, err
	}
	if in.MediaType == "video" && posterResult == nil {
		posterResult, err = uploadVideoPosterWithPHash(ctx, file)
		if err != nil {
			g.Log().Warningf(ctx, "自动生成视频封面失败 taskId:%d mediaType:%s name:%s err:%+v", in.TaskId, in.MediaType, file.Filename, err)
		}
	}
	if in.MediaType == "video" && posterResult != nil {
		perceptualHash = posterResult.PerceptualHash
	}
	attachment, err := service.CommonUpload().UploadFile(ctx, uploadType, file)
	if err != nil {
		return nil, err
	}
	var originalAttachment *basesysin.AttachmentListModel
	if originalFile != nil && in.MediaType == "image" {
		originalAttachment, err = service.CommonUpload().UploadFile(ctx, storager.KindImg, originalFile)
		if err != nil {
			return nil, err
		}
	}
	return s.saveMediaAttachment(ctx, task, in, attachment, posterAttachment(posterResult), originalAttachment, perceptualHash)
}

func uploadMediaPosterForType(ctx context.Context, mediaType string, poster *ghttp.UploadFile) (*videoPosterResult, error) {
	if mediaType != "video" {
		return nil, nil
	}
	if poster == nil {
		return nil, nil
	}
	perceptualHash, err := uploadImagePHash(poster)
	if err != nil {
		return nil, err
	}
	attachment, err := uploadMediaPoster(ctx, poster)
	if err != nil {
		return nil, err
	}
	return &videoPosterResult{Attachment: attachment, PerceptualHash: perceptualHash}, nil
}

func posterAttachment(res *videoPosterResult) *basesysin.AttachmentListModel {
	if res == nil {
		return nil
	}
	return res.Attachment
}
