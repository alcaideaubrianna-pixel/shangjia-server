package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/net/gtrace"

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
	uploadType := storager.KindImg
	if in.MediaType == "video" {
		uploadType = storager.KindVideo
	}
	assets, err := requireMediaUploadAssets(ctx, in.MediaType, file, poster)
	if err != nil {
		return nil, err
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
		PosterUrl:         normalizeMediaFileURL(mediaPosterURL(assets.Poster), mediaPosterStoragePathValue(assets.Poster)),
		PosterStoragePath: mediaPosterStoragePathValue(assets.Poster),
		AssetHash:         attachment.Md5,
		SortIndex:         in.SortIndex,
	}, nil
}

func (s *sSysPublish) saveUploadedTaskMedia(ctx context.Context, task gdb.Record, in *sysin.MediaUploadInp, file *ghttp.UploadFile, poster *ghttp.UploadFile, originalFile *ghttp.UploadFile) (res *sysin.MediaModel, err error) {
	totalStartedAt := time.Now()
	defer func() {
		logMediaUploadStage(ctx, "total", totalStartedAt, in, file, err)
	}()
	if task["status"].String() != sysin.PublishTaskStatusDraft {
		return nil, gerror.New("仅草稿任务可以上传媒体")
	}
	if file == nil {
		return nil, gerror.New("没有找到上传的文件")
	}
	if err = validatePublishMediaSize(in.MediaType, file); err != nil {
		return nil, err
	}
	uploadType := storager.KindImg
	if in.MediaType == "video" {
		uploadType = storager.KindVideo
	}
	assetsStartedAt := time.Now()
	assets, err := requireMediaUploadAssets(ctx, in.MediaType, file, poster)
	logMediaUploadStage(ctx, "prepare_assets", assetsStartedAt, in, file, err)
	if err != nil {
		return nil, err
	}
	attachmentStartedAt := time.Now()
	attachment, err := service.CommonUpload().UploadFile(ctx, uploadType, file)
	logMediaUploadStage(ctx, "upload_main", attachmentStartedAt, in, file, err)
	if err != nil {
		return nil, err
	}
	var originalAttachment *basesysin.AttachmentListModel
	if originalFile != nil && in.MediaType == "image" {
		originalStartedAt := time.Now()
		originalAttachment, err = service.CommonUpload().UploadFile(ctx, storager.KindImg, originalFile)
		logMediaUploadStage(ctx, "upload_original", originalStartedAt, in, originalFile, err)
		if err != nil {
			return nil, err
		}
	}
	saveStartedAt := time.Now()
	res, err = s.saveMediaAttachment(ctx, task, in, attachment, mediaPosterAttachment(assets.Poster), originalAttachment, assets.PerceptualHash)
	logMediaUploadStage(ctx, "save_media", saveStartedAt, in, file, err)
	return res, err
}

func validatePublishMediaSize(mediaType string, file *ghttp.UploadFile) error {
	if file == nil || mediaType != "video" {
		return nil
	}
	uploadConfig := storager.GetConfig()
	if uploadConfig != nil && uploadConfig.FileSize > 0 && file.Size > uploadConfig.FileSize*1024*1024 {
		return gerror.Newf("视频大小不能超过%vMB", uploadConfig.FileSize)
	}
	return nil
}

func logMediaUploadStage(ctx context.Context, stage string, startedAt time.Time, in *sysin.MediaUploadInp, file *ghttp.UploadFile, err error) {
	duration := time.Since(startedAt)
	mediaType := ""
	taskId := int64(0)
	if in != nil {
		mediaType = strings.TrimSpace(in.MediaType)
		taskId = in.TaskId
	}
	if err == nil && mediaType != "video" && duration < 500*time.Millisecond {
		return
	}
	fileName := ""
	fileSize := int64(0)
	if file != nil {
		fileName = file.Filename
		fileSize = file.Size
	}
	uploadTraceId := ""
	uploadUid := ""
	if in != nil {
		uploadTraceId = strings.TrimSpace(in.UploadTraceId)
		uploadUid = strings.TrimSpace(in.UploadUid)
	}
	traceId := gtrace.GetTraceID(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "上架媒体上传阶段失败 stage:%s duration_ms:%d task_id:%d uid:%s upload_trace_id:%s trace_id:%s media_type:%s file_size:%d file_name:%s err:%+v", stage, duration.Milliseconds(), taskId, uploadUid, uploadTraceId, traceId, mediaType, fileSize, fileName, err)
		return
	}
	g.Log().Infof(ctx, "上架媒体上传阶段 stage:%s duration_ms:%d task_id:%d uid:%s upload_trace_id:%s trace_id:%s media_type:%s file_size:%d file_name:%s", stage, duration.Milliseconds(), taskId, uploadUid, uploadTraceId, traceId, mediaType, fileSize, fileName)
}
