package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/storager"
	basesysin "hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
)

func (s *sSysPublish) AdminMediaList(ctx context.Context, in *sysin.MediaListInp) (list []*sysin.MediaModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.mediaListByTenant(ctx, in.TaskId, account.TenantId)
}

func (s *sSysPublish) AdminMediaDelete(ctx context.Context, in *sysin.MediaDeleteInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.deleteMediaByTenant(ctx, in.Id, account.TenantId, account.Id)
}

func (s *sSysPublish) ServerMediaList(ctx context.Context, in *sysin.MediaListInp) (list []*sysin.MediaModel, err error) {
	if in == nil {
		return nil, gerror.New("任务ID不能为空")
	}
	return s.mediaListByTenant(ctx, in.TaskId, 0)
}

func (s *sSysPublish) ServerMediaDelete(ctx context.Context, in *sysin.MediaDeleteInp) (err error) {
	if in == nil {
		return gerror.New("媒体ID不能为空")
	}
	return s.deleteMediaByTenant(ctx, in.Id, 0, contexts.GetUserId(ctx))
}

func (s *sSysPublish) MyMediaUpload(ctx context.Context, in *sysin.MediaUploadInp, file *ghttp.UploadFile) (res *sysin.MediaModel, err error) {
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
	attachment, err := service.CommonUpload().UploadFile(ctx, uploadType, file)
	if err != nil {
		return nil, err
	}
	return s.saveMediaAttachment(ctx, task, in, attachment)
}

func (s *sSysPublish) MyMediaList(ctx context.Context, in *sysin.MediaListInp) (list []*sysin.MediaModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.mediaList(ctx, in.TaskId, account.Id)
}

func (s *sSysPublish) MyMediaDelete(ctx context.Context, in *sysin.MediaDeleteInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	return s.deleteMedia(ctx, in.Id, account.Id)
}

func (s *sSysPublish) saveMediaAttachment(ctx context.Context, task gdb.Record, in *sysin.MediaUploadInp, attachment *basesysin.AttachmentListModel) (res *sysin.MediaModel, err error) {
	if attachment == nil || attachment.Id <= 0 {
		return nil, gerror.New("附件上传失败")
	}
	now := gtime.Now()
	data := g.Map{
		"tenant_id":     task["tenant_id"].Int64(),
		"merchant_id":   task["tenant_id"].Int64(),
		"account_id":    task["account_id"].Int64(),
		"task_id":       task["id"].Int64(),
		"profile_id":    task["profile_id"].Int64(),
		"attachment_id": attachment.Id,
		"media_type":    in.MediaType,
		"name":          attachment.Name,
		"file_url":      attachment.FileUrl,
		"storage_path":  attachment.Path,
		"mime_type":     attachment.MimeType,
		"md5":           attachment.Md5,
		"size":          attachment.Size,
		"sort_index":    in.SortIndex,
		"status":        1,
		"updated_by":    contexts.GetUserId(ctx),
		"updated_at":    now,
	}
	existingId, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("task_id", task["id"].Int64()).
		Where("attachment_id", attachment.Id).
		WhereNull("deleted_at").
		Fields("id").
		Value()
	if err != nil {
		return nil, gerror.Wrap(err, "检查任务媒体失败")
	}
	mediaId := existingId.Int64()
	if mediaId > 0 {
		_, err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("id", mediaId).Data(data).Update()
	} else {
		data["created_by"] = contexts.GetUserId(ctx)
		data["created_at"] = now
		mediaId, err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Data(data).InsertAndGetId()
	}
	if err != nil {
		return nil, gerror.Wrap(err, "保存任务媒体失败")
	}
	if err = s.refreshTaskMediaCount(ctx, task["id"].Int64()); err != nil {
		return nil, err
	}
	if task["profile_id"].Int64() > 0 {
		if err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			return s.syncTaskMediaToProfile(ctx, tx, task["id"].Int64(), task["profile_id"].Int64())
		}); err != nil {
			return nil, err
		}
	}
	var media *sysin.MediaModel
	if err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("id", mediaId).Scan(&media); err != nil {
		return nil, gerror.Wrap(err, "读取任务媒体失败")
	}
	return media, nil
}

func (s *sSysPublish) mediaList(ctx context.Context, taskId int64, accountId int64) (list []*sysin.MediaModel, err error) {
	if taskId <= 0 {
		return nil, gerror.New("任务ID不能为空")
	}
	if _, err = s.getTask(ctx, taskId, accountId); err != nil {
		return nil, err
	}
	err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		WhereNull("deleted_at").
		OrderAsc("sort_index").
		OrderAsc("id").
		Scan(&list)
	if err != nil {
		return nil, gerror.Wrap(err, "获取任务媒体失败")
	}
	if list == nil {
		list = []*sysin.MediaModel{}
	}
	return list, nil
}

func (s *sSysPublish) mediaListByTenant(ctx context.Context, taskId int64, tenantId int64) (list []*sysin.MediaModel, err error) {
	if taskId <= 0 {
		return nil, gerror.New("任务ID不能为空")
	}
	if _, err = s.getTaskByTenant(ctx, taskId, tenantId); err != nil {
		return nil, err
	}
	mod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	err = mod.
		OrderAsc("sort_index").
		OrderAsc("id").
		Scan(&list)
	if err != nil {
		return nil, gerror.Wrap(err, "获取任务媒体失败")
	}
	if list == nil {
		list = []*sysin.MediaModel{}
	}
	return list, nil
}

func (s *sSysPublish) deleteMedia(ctx context.Context, id int64, accountId int64) (err error) {
	if id <= 0 {
		return gerror.New("媒体ID不能为空")
	}
	mod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("id", id).WhereNull("deleted_at")
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	row, err := mod.One()
	if err != nil {
		return gerror.Wrap(err, "读取任务媒体失败")
	}
	if row.IsEmpty() {
		return gerror.New("任务媒体不存在")
	}
	if _, err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("id", id).Data(g.Map{
		"deleted_by": contexts.GetUserId(ctx),
		"deleted_at": gtime.Now(),
	}).Update(); err != nil {
		return gerror.Wrap(err, "删除任务媒体失败")
	}
	if err = s.refreshTaskMediaCount(ctx, row["task_id"].Int64()); err != nil {
		return err
	}
	if row["profile_id"].Int64() > 0 {
		mediaColumns := dao.ContentMedia.Columns()
		_, _ = dao.ContentMedia.Ctx(ctx).
			Where(mediaColumns.ProfileId, row["profile_id"].Int64()).
			Where(mediaColumns.SourceAssetId, row["attachment_id"].Int64()).
			Data(g.Map{mediaColumns.DeletedAt: gtime.Now()}).
			Update()
	}
	return nil
}

func (s *sSysPublish) deleteMediaByTenant(ctx context.Context, id int64, tenantId int64, operatorId int64) (err error) {
	if id <= 0 {
		return gerror.New("媒体ID不能为空")
	}
	mod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("id", id).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	row, err := mod.One()
	if err != nil {
		return gerror.Wrap(err, "读取任务媒体失败")
	}
	if row.IsEmpty() {
		return gerror.New("任务媒体不存在")
	}
	updateMod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("id", id)
	if tenantId > 0 {
		updateMod = updateMod.Where("tenant_id", tenantId)
	}
	if _, err = updateMod.Data(g.Map{
		"deleted_by": operatorId,
		"deleted_at": gtime.Now(),
	}).Update(); err != nil {
		return gerror.Wrap(err, "删除任务媒体失败")
	}
	if err = s.refreshTaskMediaCount(ctx, row["task_id"].Int64()); err != nil {
		return err
	}
	if row["profile_id"].Int64() > 0 {
		mediaColumns := dao.ContentMedia.Columns()
		_, _ = dao.ContentMedia.Ctx(ctx).
			Where(mediaColumns.ProfileId, row["profile_id"].Int64()).
			Where(mediaColumns.SourceAssetId, row["attachment_id"].Int64()).
			Data(g.Map{mediaColumns.DeletedAt: gtime.Now()}).
			Update()
	}
	return nil
}

func (s *sSysPublish) refreshTaskMediaCount(ctx context.Context, taskId int64) error {
	count, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "统计任务媒体失败")
	}
	_, err = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", taskId).Data(g.Map{
		"media_count": count,
		"updated_at":  gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新任务媒体数量失败")
	}
	return nil
}

func (s *sSysPublish) syncTaskMediaToProfile(ctx context.Context, tx gdb.TX, taskId int64, profileId int64) error {
	var list []*sysin.MediaModel
	if err := tx.Model(publishMediaTable).Ctx(ctx).
		Where("task_id", taskId).
		WhereNull("deleted_at").
		OrderAsc("sort_index").
		OrderAsc("id").
		Scan(&list); err != nil {
		return gerror.Wrap(err, "读取任务媒体失败")
	}
	if len(list) == 0 {
		return nil
	}
	mediaColumns := dao.ContentMedia.Columns()
	now := gtime.Now()
	imageCount := 0
	videoCount := 0
	var coverMediaId int64
	for _, item := range list {
		mediaType := item.MediaType
		if mediaType == "" {
			mediaType = "image"
		}
		if mediaType == "video" {
			videoCount++
		} else {
			imageCount++
		}
		data := g.Map{
			mediaColumns.ProfileId:           profileId,
			mediaColumns.SourceAssetId:       item.AttachmentId,
			mediaColumns.MediaType:           mediaType,
			mediaColumns.SortIndex:           item.SortIndex,
			mediaColumns.OriginalStoragePath: item.StoragePath,
			mediaColumns.DisplayStoragePath:  item.StoragePath,
			mediaColumns.PreviewStoragePath:  item.StoragePath,
			mediaColumns.BinaryMd5:           item.Md5,
			mediaColumns.ProcessStatus:       "raw",
			mediaColumns.EncryptStatus:       "none",
			mediaColumns.Status:              1,
			mediaColumns.DeletedAt:           nil,
			mediaColumns.UpdatedAt:           now,
		}
		existing, err := tx.Model(dao.ContentMedia.Table()).Ctx(ctx).
			Where(mediaColumns.ProfileId, profileId).
			Where(mediaColumns.SourceAssetId, item.AttachmentId).
			Fields(mediaColumns.Id).
			Value()
		if err != nil {
			return gerror.Wrap(err, "检查资料媒体失败")
		}
		if existing.Int64() > 0 {
			if _, err = tx.Model(dao.ContentMedia.Table()).Ctx(ctx).Where(mediaColumns.Id, existing.Int64()).Data(data).Update(); err != nil {
				return gerror.Wrap(err, "更新资料媒体失败")
			}
			if coverMediaId == 0 && mediaType == "image" {
				coverMediaId = existing.Int64()
			}
			continue
		}
		data[mediaColumns.CreatedAt] = now
		id, err := tx.Model(dao.ContentMedia.Table()).Ctx(ctx).Data(data).InsertAndGetId()
		if err != nil {
			return gerror.Wrap(err, "创建资料媒体失败")
		}
		if coverMediaId == 0 && mediaType == "image" {
			coverMediaId = id
		}
	}
	if _, err := tx.Model(publishMediaTable).Ctx(ctx).
		Where("task_id", taskId).
		WhereNull("deleted_at").
		Data(g.Map{"profile_id": profileId, "updated_at": now}).
		Update(); err != nil {
		return gerror.Wrap(err, "回写任务媒体资料ID失败")
	}
	profileColumns := dao.ContentProfile.Columns()
	data := g.Map{
		profileColumns.ImageCount: imageCount,
		profileColumns.VideoCount: videoCount,
		profileColumns.UpdatedAt:  now,
	}
	if coverMediaId > 0 {
		data[profileColumns.CoverMediaId] = coverMediaId
	}
	if _, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(profileColumns.Id, profileId).Data(data).Update(); err != nil {
		return gerror.Wrap(err, "更新资料媒体数量失败")
	}
	return nil
}
